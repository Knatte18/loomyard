// recordseed_test.go covers the Record-Seed producer over a temp directory and fake seams:
// the stored record it seeds and the record it leaves alone.
// They also cover its Stuck with nothing written when there is no record to seed.

package loomshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordSeedFixture is a Record-Seed producer's deps over a temp discussion directory.
// Its commit and seed seams count their calls.
type recordSeedFixture struct {
	deps      RecordSeedDeps
	commits   int
	seedReads int
}

// newRecordSeedFixture returns a fixture whose board entry holds stored as its stored record, none when stored is nil.
// discussionRow is the run recipe's Discussion row, empty for a recipe with none.
func newRecordSeedFixture(t *testing.T, stored []byte, discussionRow string) *recordSeedFixture {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "discussion")
	fixture := &recordSeedFixture{}
	fixture.deps = RecordSeedDeps{
		RecordPath:     filepath.Join(dir, "decision-record.md"),
		SupportLogPath: filepath.Join(dir, "support-log.md"),
		DiscussionDir:  dir,
		ReadStored:     func() ([]byte, bool, error) { return stored, stored != nil, nil },
		Commit: func() error {
			fixture.commits++
			return nil
		},
		DiscussionRow: discussionRow,
		Slug:          "my-task",
		SeedPath:      filepath.Join(root, "seed.json"),
		ReadSeed: func() (ReseedValues, error) {
			fixture.seedReads++
			return ReseedValues{Recipe: "tapestry", Driver: "llm", Parent: "main"}, nil
		},
	}
	return fixture
}

func TestRecordSeed_Call(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stored := []byte("## Goal\n\nthe Sketch's own record\n")

	t.Run("MissingRecordIsSeededFromTheStoredRecord", func(t *testing.T) {
		t.Parallel()
		f := newRecordSeedFixture(t, stored, "")

		outcome, pointer, err := NewRecordSeed(NameRecordSeed, f.deps).Call(context.Background())

		if err != nil || outcome != shedengine.Done || pointer.Path != f.deps.RecordPath {
			t.Fatalf("Call() = %q, %q, %v; want Done, %q, nil", outcome, pointer.Path, err, f.deps.RecordPath)
		}
		if mustRead(t, f.deps.RecordPath) != string(stored) || mustRead(t, f.deps.SupportLogPath) != string(StoredRecordSupportLog()) {
			t.Errorf("record, support log = %q, %q; want the stored record verbatim and the stored-record support log", mustRead(t, f.deps.RecordPath), mustRead(t, f.deps.SupportLogPath))
		}
		if f.commits != 1 {
			t.Errorf("Commit calls = %d; want 1", f.commits)
		}
	})

	t.Run("ExistingRecordIsLeftAsItIsAndStillCommitted", func(t *testing.T) {
		t.Parallel()
		f := newRecordSeedFixture(t, stored, NameDiscussionWrite)
		if err := os.MkdirAll(f.deps.DiscussionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		record, support := "## Goal\n\napproved by this run's Discussion\n", "support\n"
		for path, content := range map[string]string{f.deps.RecordPath: record, f.deps.SupportLogPath: support} {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		outcome, _, err := NewRecordSeed(NameRecordSeed, f.deps).Call(context.Background())

		if err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
		}
		if mustRead(t, f.deps.RecordPath) != record || mustRead(t, f.deps.SupportLogPath) != support {
			t.Error("Call() rewrote a file that existed; want both left byte for byte")
		}
		if f.commits != 1 {
			t.Errorf("Commit calls = %d; want 1, so a pair a crash left uncommitted is committed", f.commits)
		}
	})

	stuck := []struct {
		name          string
		discussionRow string
		wantSeedReads int
		want          []string
	}{
		{
			name:          "ARecipeWithADiscussionRowGoesBackToIt",
			discussionRow: NameDiscussionWrite,
			wantSeedReads: 0,
			want:          []string{`way forward: "lym loom goto --to Discussion-Write", then "lym loom resume"`},
		},
		{
			name:          "ARecipeWithNoDiscussionRowIsRunAsTapestry",
			discussionRow: "",
			wantSeedReads: 1,
			want: []string{
				`For a pair batten made, from the hub's prime worktree: "lym batten cancel my-task --remove", ` +
					`"lym board upsert '{"slug":"my-task","recipe":"tapestry"}'", ` +
					`"lym board set-status '{"slug":"my-task","status":null}'", then "lym batten run my-task".`,
				`For any other pair, from its task worktree: "rm `,
				`seed.json", "lym shed seed self --recipe tapestry --driver llm --param parent=main", ` +
					`"lym loom goto --to Discussion-Write", "lym loom start", then "lym loom resume"`,
			},
		},
	}
	for _, tt := range stuck {
		t.Run("NoStoredRecordIsStuck_"+tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordSeedFixture(t, nil, tt.discussionRow)

			outcome, pointer, err := NewRecordSeed(NameRecordSeed, f.deps).Call(context.Background())

			if err != nil || outcome != shedengine.Stuck {
				t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
			}
			for _, want := range append([]string{"there is no record to seed"}, tt.want...) {
				if !strings.Contains(pointer.Reason, want) {
					t.Errorf("Reason = %q; want it to name %q", pointer.Reason, want)
				}
			}
			if f.seedReads != tt.wantSeedReads {
				t.Errorf("ReadSeed calls = %d; want %d", f.seedReads, tt.wantSeedReads)
			}
			for _, path := range []string{f.deps.RecordPath, f.deps.SupportLogPath} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("stat %s error = %v; want nothing written", path, err)
				}
			}
			if f.commits != 0 {
				t.Errorf("Commit calls = %d; want 0", f.commits)
			}
		})
	}

	t.Run("ReadSeedErrorIsReturned", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("boom")
		f := newRecordSeedFixture(t, nil, "")
		f.deps.ReadSeed = func() (ReseedValues, error) { return ReseedValues{}, boom }

		_, _, err := NewRecordSeed(NameRecordSeed, f.deps).Call(context.Background())
		if !errors.Is(err, boom) {
			t.Fatalf("Call() error = %v; want it to wrap the seam's error", err)
		}
		for _, want := range []string{"read the run's seed", "way forward: the fault is transient, re-step the Record-Seed row"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Call() error = %q; want it to name %q", err, want)
			}
		}
	})

	t.Run("RecordWithoutSupportLogOrStoredRecordGetsALogNamingNoSource", func(t *testing.T) {
		t.Parallel()
		f := newRecordSeedFixture(t, nil, "")
		if err := os.MkdirAll(f.deps.DiscussionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		record := "## Goal\n\nkept\n"
		if err := os.WriteFile(f.deps.RecordPath, []byte(record), 0o644); err != nil {
			t.Fatal(err)
		}

		outcome, _, err := NewRecordSeed(NameRecordSeed, f.deps).Call(context.Background())

		if err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
		}
		wantLog := "## Interview\n\n" +
			"The support log was missing beside an existing record; the record was kept as it stood.\n\n" +
			"## Rejected alternatives\n\n" +
			"## Review rounds\n\n" +
			"_No rounds yet._\n\n" +
			"## Question ledger\n"
		if got := mustRead(t, f.deps.SupportLogPath); got != wantLog {
			t.Errorf("support log = %q; want %q", got, wantLog)
		}
		if got := mustRead(t, f.deps.RecordPath); got != record {
			t.Errorf("record = %q; want it unchanged %q", got, record)
		}
	})

	// The context is cancelled from inside the call, so the check made on entry does not answer first.
	t.Run("CancelDuringTheCallWinsOverStuck", func(t *testing.T) {
		t.Parallel()
		f := newRecordSeedFixture(t, nil, NameDiscussionWrite)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f.deps.ReadStored = func() ([]byte, bool, error) {
			cancel()
			return nil, false, nil
		}

		outcome, _, err := NewRecordSeed(NameRecordSeed, f.deps).Call(ctx)

		if !errors.Is(err, context.Canceled) || outcome == shedengine.Stuck {
			t.Errorf("Call() = %q, %v; want a context.Canceled error and no verdict", outcome, err)
		}
		for _, path := range []string{f.deps.RecordPath, f.deps.SupportLogPath} {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("stat %s error = %v; want nothing written", path, err)
			}
		}
	})
}
