// recordwrite_test.go covers the Record-Write producer over a temp directory and fake seams: the files it writes from the board entry body, what it leaves alone on a resume, and its stops.

package loomshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordWriteFixture is a Record-Write producer over a temp discussion directory, with a counting commit seam and a stored-record seam that finds no record.
type recordWriteFixture struct {
	producer    shedengine.ShedProducer
	deps        RecordWriteDeps
	commits     int
	storedReads int
}

// newRecordWriteFixture returns a fixture whose board holds an entry with the given body.
func newRecordWriteFixture(t *testing.T, body string, found bool) *recordWriteFixture {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "discussion")
	fixture := &recordWriteFixture{}
	fixture.deps = RecordWriteDeps{
		RecordPath:     filepath.Join(dir, "decision-record.md"),
		SupportLogPath: filepath.Join(dir, "support-log.md"),
		DiscussionDir:  dir,
		ReadBody:       func() (string, bool, error) { return body, found, nil },
		ReadStored: func() ([]byte, bool, error) {
			fixture.storedReads++
			return nil, false, nil
		},
		Commit: func() error {
			fixture.commits++
			return nil
		},
	}
	fixture.producer = NewRecordWrite(NameRecordWrite, fixture.deps)
	return fixture
}

// mustRead returns the content of path.
func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestRecordWrite_Call(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("FreshDirectoryGetsBothFilesAndOneCommit", func(t *testing.T) {
		t.Parallel()
		body := "Fix the login redirect.\n\nKeep `session` intact.\n"
		f := newRecordWriteFixture(t, body, true)

		outcome, pointer, err := f.producer.Call(context.Background())
		if err != nil || outcome != shedengine.Done || pointer.Path != f.deps.RecordPath {
			t.Fatalf("Call() = %q, %q, %v; want Done, %q, nil", outcome, pointer.Path, err, f.deps.RecordPath)
		}
		if f.commits != 1 || f.storedReads != 1 {
			t.Errorf("Commit calls, stored-record reads = %d, %d; want 1, 1: an entry with no stored record reads as not found and its body is rendered", f.commits, f.storedReads)
		}
		if got := string(BoardRecord(body)); mustRead(t, f.deps.RecordPath) != got {
			t.Errorf("decision record = %q; want %q", mustRead(t, f.deps.RecordPath), got)
		}
		if !strings.Contains(mustRead(t, f.deps.RecordPath), "```markdown\n"+body+"```\n") {
			t.Errorf("decision record does not hold the body verbatim in a fenced markdown block:\n%s", mustRead(t, f.deps.RecordPath))
		}
		support := mustRead(t, f.deps.SupportLogPath)
		for _, heading := range []string{"## Interview", "## Rejected alternatives", "## Review rounds", "## Question ledger"} {
			if !strings.Contains(support, heading+"\n") {
				t.Errorf("support log lacks %q:\n%s", heading, support)
			}
		}
		if !strings.Contains(support, "_No rounds yet._") {
			t.Errorf("support log lacks the seeded review rounds line:\n%s", support)
		}
		findings, err := discussionparser.Validate(f.deps.RecordPath, f.deps.SupportLogPath)
		if err != nil || len(findings) != 0 {
			t.Errorf("Validate() = %v, %v; want no findings", findings, err)
		}
	})

	bodies := []struct {
		name string
		body string
	}{
		{"EmptyBody", ""},
		{"NoBackticks", "Plain task text"},
		{"InlineCodeOnly", "Rename `Foo` to `Bar`"},
		{"SectionHeadings", "## Decisions\nstay out\n\n## Open risks\nnone\n"},
		{"InnerFencedBlock", "Before\n```go\n## Decisions\nx := 1\n```\nAfter\n"},
		{"UnclosedInnerFence", "Before\n```go\n## Decisions\nnever closed\n"},
	}
	for _, tt := range bodies {
		t.Run("Body_"+tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordWriteFixture(t, tt.body, true)
			if _, _, err := f.producer.Call(context.Background()); err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			findings, err := discussionparser.Validate(f.deps.RecordPath, f.deps.SupportLogPath)
			if err != nil || len(findings) != 0 {
				t.Fatalf("Validate() = %v, %v; want no findings", findings, err)
			}

			added := discussionparser.AddedDecision{By: "orch", Title: "Keep the cache", Decision: "Leave it.", Rationale: "Cheap.", Date: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)}
			findings, err = discussionparser.AppendDecision(f.deps.RecordPath, f.deps.SupportLogPath, added)
			if err != nil || len(findings) != 0 {
				t.Fatalf("AppendDecision() = %v, %v; want no findings", findings, err)
			}
			record := mustRead(t, f.deps.RecordPath)
			scope := strings.Index(record, "\n## Scope\n")
			decisions := strings.Index(record[scope:], "\n## Decisions\n") + scope
			constraints := strings.Index(record[scope:], "\n## Constraints\n") + scope
			title := strings.Index(record, "Keep the cache")
			if scope < 0 || title < decisions || title > constraints {
				t.Errorf("added decision at %d; want it between the record's own Decisions (%d) and Constraints (%d) headings:\n%s", title, decisions, constraints, record)
			}
		})
	}

	t.Run("StoredRecordIsWrittenVerbatimWithItsSupportLog", func(t *testing.T) {
		t.Parallel()
		f := newRecordWriteFixture(t, "unused body", true)
		stored := "## Goal\n\nthe Sketch's own record\n"
		bodyReads := 0
		f.deps.ReadStored = func() ([]byte, bool, error) { return []byte(stored), true, nil }
		f.deps.ReadBody = func() (string, bool, error) {
			bodyReads++
			return "", true, nil
		}

		outcome, pointer, err := NewRecordWrite(NameRecordWrite, f.deps).Call(context.Background())

		if err != nil || outcome != shedengine.Done || pointer.Path != f.deps.RecordPath {
			t.Fatalf("Call() = %q, %q, %v; want Done, %q, nil", outcome, pointer.Path, err, f.deps.RecordPath)
		}
		if mustRead(t, f.deps.RecordPath) != stored || mustRead(t, f.deps.SupportLogPath) != string(StoredRecordSupportLog()) {
			t.Errorf("record, support log = %q, %q; want the stored record verbatim and the stored-record support log", mustRead(t, f.deps.RecordPath), mustRead(t, f.deps.SupportLogPath))
		}
		if bodyReads != 0 || f.commits != 1 {
			t.Errorf("ReadBody calls, commits = %d, %d; want 0, 1", bodyReads, f.commits)
		}
	})

	t.Run("ExistingPairIsLeftAsItIsAndStillCommitted", func(t *testing.T) {
		t.Parallel()
		f := newRecordWriteFixture(t, "body", true)
		if err := os.MkdirAll(f.deps.DiscussionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		record, support := "## Goal\n\nedited after Discussion\n", "support\n"
		for path, content := range map[string]string{f.deps.RecordPath: record, f.deps.SupportLogPath: support} {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		f.deps.ReadBody = func() (string, bool, error) {
			t.Error("ReadBody called while the record exists")
			return "", true, nil
		}
		outcome, _, err := NewRecordWrite(NameRecordWrite, f.deps).Call(context.Background())
		if err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
		}
		if f.storedReads != 0 {
			t.Errorf("ReadStored calls = %d; want 0 while the record exists", f.storedReads)
		}
		if mustRead(t, f.deps.RecordPath) != record || mustRead(t, f.deps.SupportLogPath) != support {
			t.Error("Call() rewrote a file that existed; want both left byte for byte")
		}
		if f.commits != 1 {
			t.Errorf("Commit calls = %d; want 1, so a pair a crash left uncommitted is committed", f.commits)
		}
	})

	t.Run("RecordWithoutSupportLogGetsTheLog", func(t *testing.T) {
		t.Parallel()
		f := newRecordWriteFixture(t, "body", false)
		if err := os.MkdirAll(f.deps.DiscussionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		record := "## Goal\n\nkept\n"
		if err := os.WriteFile(f.deps.RecordPath, []byte(record), 0o644); err != nil {
			t.Fatal(err)
		}

		outcome, _, err := f.producer.Call(context.Background())
		if err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %v; want Done, nil: the entry's body is not read while the record exists", outcome, err)
		}
		if mustRead(t, f.deps.RecordPath) != record {
			t.Error("Call() rewrote the record; want it unchanged")
		}
		if !strings.Contains(mustRead(t, f.deps.SupportLogPath), "## Question ledger") || f.commits != 1 {
			t.Errorf("support log written = false or commits = %d; want the log written and one commit", f.commits)
		}
	})

	t.Run("StoredRecordWithoutSupportLogGetsTheStoredRecordLog", func(t *testing.T) {
		t.Parallel()
		f := newRecordWriteFixture(t, "body", true)
		if err := os.MkdirAll(f.deps.DiscussionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		stored := "## Goal\n\nthe Sketch's own record\n"
		if err := os.WriteFile(f.deps.RecordPath, []byte(stored), 0o644); err != nil {
			t.Fatal(err)
		}
		f.deps.ReadStored = func() ([]byte, bool, error) { return []byte(stored), true, nil }

		outcome, _, err := NewRecordWrite(NameRecordWrite, f.deps).Call(context.Background())
		if err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
		}
		if mustRead(t, f.deps.RecordPath) != stored || mustRead(t, f.deps.SupportLogPath) != string(StoredRecordSupportLog()) {
			t.Errorf("record, support log = %q, %q; want the record kept and the stored-record support log", mustRead(t, f.deps.RecordPath), mustRead(t, f.deps.SupportLogPath))
		}
	})

	t.Run("TornTempFileIsRemoved", func(t *testing.T) {
		t.Parallel()
		f := newRecordWriteFixture(t, "body", true)
		if err := os.MkdirAll(f.deps.DiscussionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		torn := filepath.Join(f.deps.DiscussionDir, ".tmp-123")
		if err := os.WriteFile(torn, []byte("half"), 0o644); err != nil {
			t.Fatal(err)
		}

		if _, _, err := f.producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if _, err := os.Stat(torn); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("torn file stat error = %v; want it removed", err)
		}
		if mustRead(t, f.deps.RecordPath) != string(BoardRecord("body")) {
			t.Error("decision record is not whole after the torn file was cleared")
		}
	})

	t.Run("MissingEntryIsStuckNamingBoardUpsert", func(t *testing.T) {
		t.Parallel()
		f := newRecordWriteFixture(t, "", false)

		outcome, pointer, err := f.producer.Call(context.Background())
		if err != nil || outcome != shedengine.Stuck {
			t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
		}
		if !strings.Contains(pointer.Reason, `lym board upsert`) || !strings.Contains(pointer.Reason, `lym loom resume`) {
			t.Errorf("Reason = %q; want the way forward naming lym board upsert and lym loom resume", pointer.Reason)
		}
		if _, err := os.Stat(f.deps.RecordPath); !errors.Is(err, os.ErrNotExist) || f.commits != 0 {
			t.Errorf("record stat error = %v, commits = %d; want nothing written and nothing committed", err, f.commits)
		}
	})

	t.Run("SeamErrorsAreReturned", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("boom")
		for name, edit := range map[string]func(*RecordWriteDeps){
			"ReadBody":   func(d *RecordWriteDeps) { d.ReadBody = func() (string, bool, error) { return "", false, boom } },
			"ReadStored": func(d *RecordWriteDeps) { d.ReadStored = func() ([]byte, bool, error) { return nil, false, boom } },
			"Commit":     func(d *RecordWriteDeps) { d.Commit = func() error { return boom } },
		} {
			f := newRecordWriteFixture(t, "body", true)
			edit(&f.deps)
			_, _, err := NewRecordWrite(NameRecordWrite, f.deps).Call(context.Background())
			if !errors.Is(err, boom) {
				t.Errorf("%s: Call() error = %v; want it to wrap the seam's error", name, err)
			}
		}
	})

	t.Run("CancelledContextWins", func(t *testing.T) {
		t.Parallel()
		f := newRecordWriteFixture(t, "body", true)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		outcome, _, err := f.producer.Call(ctx)
		if !errors.Is(err, context.Canceled) || outcome == shedengine.Done {
			t.Errorf("Call() = %q, %v; want a context.Canceled error and no verdict", outcome, err)
		}
	})
}
