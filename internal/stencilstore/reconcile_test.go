// reconcile_test.go covers Read, Reconcile, and ForceRefresh against a bare t.TempDir(), with no git and no hub fixture -- see fakeRegistry below for the Registry stand-in every test here uses.
//
// The tests that capture the logger's output swap process-global state, so they stay serial and run before the parallel ones resume.

package stencilstore

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// fakeRegistry implements Registry over a plain map, so no test in this package depends on the real
// stencils package.
type fakeRegistry struct {
	names    []string
	defaults map[string][]byte
}

// newFakeRegistry builds a fakeRegistry whose Names() returns defaults' keys in sorted order.
func newFakeRegistry(defaults map[string][]byte) *fakeRegistry {
	names := make([]string, 0, len(defaults))
	for name := range defaults {
		names = append(names, name)
	}
	sort.Strings(names)
	return &fakeRegistry{names: names, defaults: defaults}
}

// Names implements Registry.
func (r *fakeRegistry) Names() []string {
	return r.names
}

// Default implements Registry.
func (r *fakeRegistry) Default(name string) ([]byte, bool) {
	content, ok := r.defaults[name]
	return content, ok
}

// readAll returns the raw bytes on disk at baseDir for name, or nil if absent.
func readAll(t *testing.T, baseDir, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(Path(baseDir, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("os.ReadFile(%s) failed: %v", Path(baseDir, name), err)
	}
	return content
}

// seedBoard runs a production Reconcile of registry into a fresh board directory and returns it.
func seedBoard(t *testing.T, registry *fakeRegistry) string {
	t.Helper()
	baseDir := t.TempDir()
	if _, err := Reconcile(baseDir, registry, ModeProduction, Source{}); err != nil {
		t.Fatalf("seed Reconcile(...) returned error: %v", err)
	}
	return baseDir
}

// writeBoardCopy writes content as the board copy of name, creating its family directory.
func writeBoardCopy(t *testing.T, baseDir, name string, content []byte) {
	t.Helper()
	path := Path(baseDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}
}

// seedGitattributesForTest pre-creates baseDir/.gitattributes so a test's own Reconcile call can
// assert on written without that call also seeding .gitattributes for the first time.
func seedGitattributesForTest(t *testing.T, baseDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(baseDir, gitattributesName), []byte(gitattributesContent), 0o644); err != nil {
		t.Fatalf("seedGitattributesForTest: os.WriteFile failed: %v", err)
	}
}

// TestReconcile_SeedsAbsentFileAndGitattributesOnce covers the seeding run: an absent board copy is written stamped with its own hash and reported in written, and .gitattributes is seeded then too, but never rewritten once it exists and never listed by the registry.
func TestReconcile_SeedsAbsentFileAndGitattributesOnce(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	registry := newFakeRegistry(map[string][]byte{"family-one": []byte("shipped body\n")})

	written, err := Reconcile(baseDir, registry, ModeProduction, Source{})
	if err != nil {
		t.Fatalf("Reconcile(...) returned error: %v", err)
	}

	onDisk := readAll(t, baseDir, "family-one")
	stamp, ok := ParseStamp(onDisk)
	if !ok || stamp != BodyHash(onDisk) {
		t.Fatalf("seeded file's stamp = (%q, %v); want it to equal its own BodyHash", stamp, ok)
	}
	if !slices.Contains(written, RelPath("family-one")) {
		t.Errorf("Reconcile(...) written = %v; want it to include %q", written, RelPath("family-one"))
	}
	if !slices.Contains(written, gitattributesName) {
		t.Errorf("Reconcile(...) written = %v; want it to include %q on the seeding run", written, gitattributesName)
	}

	content, err := os.ReadFile(filepath.Join(baseDir, gitattributesName))
	if err != nil {
		t.Fatalf("os.ReadFile(.gitattributes) failed: %v", err)
	}
	if string(content) != gitattributesContent {
		t.Errorf(".gitattributes content = %q; want %q", content, gitattributesContent)
	}

	if err := os.WriteFile(filepath.Join(baseDir, gitattributesName), []byte("custom content\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}

	written, err = Reconcile(baseDir, registry, ModeProduction, Source{})
	if err != nil {
		t.Fatalf("Reconcile(...) returned error: %v", err)
	}
	if slices.Contains(written, gitattributesName) {
		t.Errorf("Reconcile(...) rewrote an existing .gitattributes")
	}
	content, err = os.ReadFile(filepath.Join(baseDir, gitattributesName))
	if err != nil {
		t.Fatalf("os.ReadFile(.gitattributes) failed: %v", err)
	}
	if string(content) != "custom content\n" {
		t.Errorf(".gitattributes content = %q; want it left untouched", content)
	}

	if slices.Contains(registry.Names(), gitattributesName) {
		t.Errorf("registry.Names() = %v; want it to never include %q", registry.Names(), gitattributesName)
	}
}

// TestReconcile_UntouchedBoardCopy covers a board copy still matching its own stamp across a second reconcile: an unchanged default leaves every file alone, and a changed default refreshes it in production mode but never in dev mode.
func TestReconcile_UntouchedBoardCopy(t *testing.T) {
	t.Parallel()

	recordedWriter := Writer{Revision: "recorded-rev", Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	runningWriter := Writer{Revision: "running-rev", Time: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)}
	orderedAs := func(ordering Ordering) func(Writer) Ordering {
		return func(Writer) Ordering { return ordering }
	}
	tests := []struct {
		name          string
		mode          Mode
		updated       string
		recorded      Writer
		older         func(Writer) Ordering
		wantRefreshed bool
	}{
		{name: "UnchangedDefaultLeftAlone", mode: ModeProduction},
		{name: "ChangedDefaultRefreshedInProductionMode", mode: ModeProduction, updated: "updated shipped body\n", wantRefreshed: true},
		{name: "ChangedDefaultNotRefreshedInDevMode", mode: ModeDev, updated: "updated shipped body\n"},
		{name: "ChangedDefaultNotRefreshedInUnstampedMode", mode: ModeUnstamped, updated: "updated shipped body\n"},
		{name: "RecordedOlderRefreshed", mode: ModeProduction, updated: "updated shipped body\n", recorded: recordedWriter, older: orderedAs(RecordedOlder), wantRefreshed: true},
		{name: "RecordedNotOlderLeftAlone", mode: ModeProduction, updated: "updated shipped body\n", recorded: recordedWriter, older: orderedAs(RecordedNotOlder)},
		{name: "UnknownOrderingLeftAlone", mode: ModeProduction, updated: "updated shipped body\n", recorded: recordedWriter, older: orderedAs(OrderingUnknown)},
		{name: "NilOlderLeftAlone", mode: ModeProduction, updated: "updated shipped body\n", recorded: recordedWriter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registry := newFakeRegistry(map[string][]byte{
				"family-one": []byte("original shipped body\n"),
				"family-two": []byte("body two\n"),
			})
			baseDir := t.TempDir()
			if _, err := Reconcile(baseDir, registry, ModeProduction, Source{Writer: tt.recorded}); err != nil {
				t.Fatalf("seed Reconcile(...) returned error: %v", err)
			}
			beforeOne := readAll(t, baseDir, "family-one")
			beforeTwo := readAll(t, baseDir, "family-two")
			if tt.updated != "" {
				registry.defaults["family-one"] = []byte(tt.updated)
			}

			written, err := Reconcile(baseDir, registry, tt.mode, Source{Writer: runningWriter, Older: tt.older})
			if err != nil {
				t.Fatalf("Reconcile(...) returned error: %v", err)
			}

			wantWritten := 0
			if tt.wantRefreshed {
				wantWritten = 1
			}
			if len(written) != wantWritten {
				t.Errorf("Reconcile(...) written = %v; want %d entries", written, wantWritten)
			}
			afterOne := readAll(t, baseDir, "family-one")
			if tt.wantRefreshed {
				if BodyHash(afterOne) != BodyHash([]byte(tt.updated)) {
					t.Errorf("Reconcile(...) did not refresh: BodyHash(onDisk) = %q; want %q", BodyHash(afterOne), BodyHash([]byte(tt.updated)))
				}
				if got := ParseWriter(afterOne); got.Revision != runningWriter.Revision || !got.Time.Equal(runningWriter.Time) {
					t.Errorf("refreshed file's writer = %+v; want the running binary %+v", got, runningWriter)
				}
			} else if string(afterOne) != string(beforeOne) {
				t.Errorf("Reconcile(...) modified an untouched file: before=%q after=%q", beforeOne, afterOne)
			}
			if afterTwo := readAll(t, baseDir, "family-two"); string(afterTwo) != string(beforeTwo) {
				t.Errorf("Reconcile(...) modified family-two: before=%q after=%q", beforeTwo, afterTwo)
			}
		})
	}
}

// TestReconcile_RestampsRowWhoseBodyMatchesDefault covers a board copy whose body equals the shipped default but whose stamp names an older hash (an edit later reverted): both modes restamp it, and it classifies as untouched afterwards.
func TestReconcile_RestampsRowWhoseBodyMatchesDefault(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		mode Mode
	}{
		{"ProductionMode", ModeProduction},
		{"DevMode", ModeDev},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			shipped := []byte("shipped body\n")
			registry := newFakeRegistry(map[string][]byte{"family-one": shipped})
			staleHash := BodyHash([]byte("some other old default\n"))
			writeBoardCopy(t, baseDir, "family-one", ApplyStamp(shipped, staleHash))
			seedGitattributesForTest(t, baseDir)

			written, err := Reconcile(baseDir, registry, tt.mode, Source{})
			if err != nil {
				t.Fatalf("Reconcile(...) returned error: %v", err)
			}
			if len(written) != 1 {
				t.Errorf("Reconcile(...) written = %v; want one restamp entry", written)
			}

			onDisk := readAll(t, baseDir, "family-one")
			stamp, ok := ParseStamp(onDisk)
			if !ok || stamp != BodyHash(onDisk) {
				t.Fatalf("restamped file's stamp = (%q, %v); want it to equal its own BodyHash", stamp, ok)
			}
			if got := Classify(onDisk, true, shipped); got != StateUntouched {
				t.Errorf("Classify(restamped file) = %v; want %v", got, StateUntouched)
			}
		})
	}
}

// TestForceRefresh_PerformsRefreshRowFromDevSkippedFile covers a forced refresh of a file a dev pass skipped, under each mode, and the running binary being recorded as its writer only under ModeProduction.
func TestForceRefresh_PerformsRefreshRowFromDevSkippedFile(t *testing.T) {
	t.Parallel()

	running := Writer{Revision: "running-rev", Time: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)}
	for _, tt := range []struct {
		name       string
		mode       Mode
		wantWriter string
	}{
		{"ProductionMode", ModeProduction, running.Revision},
		{"DevMode", ModeDev, ""},
		{"UnstampedMode", ModeUnstamped, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := []byte("original shipped body\n")
			registry := newFakeRegistry(map[string][]byte{"family-one": original})
			baseDir := seedBoard(t, registry)

			updated := []byte("updated shipped body\n")
			registry.defaults["family-one"] = updated
			if _, err := Reconcile(baseDir, registry, ModeDev, Source{}); err != nil {
				t.Fatalf("Reconcile(ModeDev) returned error: %v", err)
			}
			skipped := readAll(t, baseDir, "family-one")
			if BodyHash(skipped) != BodyHash(original) {
				t.Fatalf("precondition failed: ModeDev already refreshed the file")
			}

			written, err := ForceRefresh(baseDir, registry, tt.mode, Source{Writer: running})
			if err != nil {
				t.Fatalf("ForceRefresh(...) returned error: %v", err)
			}
			if len(written) != 1 {
				t.Errorf("ForceRefresh(...) written = %v; want one entry", written)
			}

			refreshed := readAll(t, baseDir, "family-one")
			if BodyHash(refreshed) != BodyHash(updated) {
				t.Errorf("ForceRefresh(...) did not refresh: BodyHash(refreshed) = %q; want %q", BodyHash(refreshed), BodyHash(updated))
			}
			if got := ParseWriter(refreshed).Revision; got != tt.wantWriter {
				t.Errorf("recorded writer revision = %q; want %q", got, tt.wantWriter)
			}
		})
	}
}

// TestWritesDue covers the dry-run classification: nothing due on a settled board, and a write due for an absent file, a missing .gitattributes, or an untouched copy the mode would refresh, but not for one it would leave or for an edited copy.
// The pass logs nothing, even over a copy a mode leaves behind.
// It captures the logger's output, which is process-global state, so it stays serial.
func TestWritesDue(t *testing.T) {
	tests := []struct {
		name  string
		mode  Mode
		setup func(t *testing.T, baseDir string, registry *fakeRegistry)
		want  bool
	}{
		{"SettledBoard", ModeProduction, func(t *testing.T, baseDir string, registry *fakeRegistry) {}, false},
		{"AbsentFile", ModeProduction, func(t *testing.T, baseDir string, registry *fakeRegistry) {
			if err := os.Remove(Path(baseDir, "family-one")); err != nil {
				t.Fatalf("os.Remove failed: %v", err)
			}
		}, true},
		{"MissingGitattributes", ModeDev, func(t *testing.T, baseDir string, registry *fakeRegistry) {
			if err := os.Remove(filepath.Join(baseDir, gitattributesName)); err != nil {
				t.Fatalf("os.Remove failed: %v", err)
			}
		}, true},
		{"UntouchedCopyUnderProduction", ModeProduction, func(t *testing.T, baseDir string, registry *fakeRegistry) {
			registry.defaults["family-one"] = []byte("updated shipped body\n")
		}, true},
		{"UntouchedCopyUnderDev", ModeDev, func(t *testing.T, baseDir string, registry *fakeRegistry) {
			registry.defaults["family-one"] = []byte("updated shipped body\n")
		}, false},
		{"UntouchedCopyUnderUnstamped", ModeUnstamped, func(t *testing.T, baseDir string, registry *fakeRegistry) {
			registry.defaults["family-one"] = []byte("updated shipped body\n")
		}, false},
		{"EditedCopy", ModeProduction, func(t *testing.T, baseDir string, registry *fakeRegistry) {
			writeBoardCopy(t, baseDir, "family-one", []byte("hand-edited body\n"))
			registry.defaults["family-one"] = []byte("updated shipped body\n")
		}, false},
	}
	buf := logcapture.CaptureVerbose(t)
	// The parallel subtests finish before the parent's cleanups run, so the log holds every row's output by then.
	t.Cleanup(func() {
		if buf.String() != "" {
			t.Errorf("WritesDue logged %q; want nothing", buf.String())
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registry := newFakeRegistry(map[string][]byte{"family-one": []byte("original shipped body\n")})
			baseDir := seedBoard(t, registry)
			tt.setup(t, baseDir, registry)
			before := readAll(t, baseDir, "family-one")

			got, err := WritesDue(baseDir, registry, tt.mode, Source{})
			if err != nil {
				t.Fatalf("WritesDue(...) returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("WritesDue(...) = %v; want %v", got, tt.want)
			}
			if after := readAll(t, baseDir, "family-one"); string(after) != string(before) {
				t.Errorf("WritesDue(...) modified the board copy: before=%q after=%q", before, after)
			}
		})
	}
}

// TestReconcile_EditedBoardCopyIsNeverModified covers a board copy reconcile must leave alone even under a newer default: a hand-edited body under its old stamp, and a body with no stamp at all.
// Read returns the content as it is on disk.
func TestReconcile_EditedBoardCopyIsNeverModified(t *testing.T) {
	t.Parallel()

	shipped := []byte("shipped body\n")
	tests := []struct {
		name   string
		onDisk []byte
	}{
		{"HandEditedBody", ApplyStamp([]byte("hand-edited body\n"), BodyHash(shipped))},
		{"MissingStamp", []byte("body with no stamp at all\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			registry := newFakeRegistry(map[string][]byte{"family-one": []byte("newer shipped body\n")})
			writeBoardCopy(t, baseDir, "family-one", tt.onDisk)
			seedGitattributesForTest(t, baseDir)

			written, err := Reconcile(baseDir, registry, ModeProduction, Source{})
			if err != nil {
				t.Fatalf("Reconcile(...) returned error: %v", err)
			}
			if len(written) != 0 {
				t.Errorf("Reconcile(...) written = %v; want empty for an edited file", written)
			}
			if onDisk := readAll(t, baseDir, "family-one"); string(onDisk) != string(tt.onDisk) {
				t.Errorf("Reconcile(...) modified an edited file: onDisk=%q want=%q", onDisk, tt.onDisk)
			}

			read, err := Read(baseDir, "family-one")
			if err != nil {
				t.Fatalf("Read(...) returned error: %v", err)
			}
			if string(read) != string(tt.onDisk) {
				t.Errorf("Read(...) = %q; want the on-disk content %q", read, tt.onDisk)
			}
		})
	}
}

// TestReconcile_SourceDirNeverFailsReconcile covers the port-back drift comparison never failing a reconcile: an empty sourceDir skips it, and a worktree source that differs from the board copy only warns.
func TestReconcile_SourceDirNeverFailsReconcile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		driftSource bool
	}{
		{"EmptySourceDirSkipsDriftComparison", false},
		{"DriftingSourceDirStillSucceeds", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registry := newFakeRegistry(map[string][]byte{"family-one": []byte("board body\n")})
			baseDir := seedBoard(t, registry)

			sourceDir := ""
			if tt.driftSource {
				sourceDir = t.TempDir()
				sourcePath := filepath.Join(sourceDir, filepath.FromSlash(RelPath("family-one")))
				if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
					t.Fatalf("os.MkdirAll failed: %v", err)
				}
				if err := os.WriteFile(sourcePath, []byte("different worktree body\n"), 0o644); err != nil {
					t.Fatalf("os.WriteFile failed: %v", err)
				}
			}

			written, err := Reconcile(baseDir, registry, ModeProduction, Source{Dir: sourceDir})
			if err != nil {
				t.Fatalf("Reconcile(...) with sourceDir %q returned error: %v", sourceDir, err)
			}
			if len(written) != 0 {
				t.Errorf("Reconcile(...) written = %v; want empty on a run where only .gitattributes was already seeded", written)
			}
		})
	}
}

// TestReconcile_ModeDevRefusalNamesItsRemedy pins the dev-mode refusal's message, not just its
// behaviour. The refusal is one-way -- an older installed binary running in production mode DOES
// refresh, so it can downgrade a board's stencils, after which a newer dev build can only warn,
// on every invocation, forever. A warning that does not name "lyx stencil sync" reads as benign
// housekeeping while it is reporting that every producer will run on the older on-disk prompt.
func TestReconcile_ModeDevRefusalNamesItsRemedy(t *testing.T) {
	recorded := Writer{Revision: "recorded-rev"}
	tests := []struct {
		name  string
		mode  Mode
		older func(Writer) Ordering
		want  []string
	}{
		{"DevMode", ModeDev, nil, []string{"lyx stencil sync", "OLDER", "family-one", "family-two"}},
		{"UnstampedMode", ModeUnstamped, nil, []string{"lyx stencil sync", "update-plugins.sh", "OLDER", "family-one", "family-two"}},
		{"ProductionRecordedNotOlder", ModeProduction, func(Writer) Ordering { return RecordedNotOlder }, []string{"lyx stencil sync", "family-one", "family-two", "recorded-rev", "running-rev"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := newFakeRegistry(map[string][]byte{"family-one": []byte("original shipped body\n"), "family-two": []byte("original body two\n")})
			baseDir := t.TempDir()
			if _, err := Reconcile(baseDir, registry, ModeProduction, Source{Writer: recorded}); err != nil {
				t.Fatalf("seed Reconcile(...) returned error: %v", err)
			}
			registry.defaults["family-one"] = []byte("updated shipped body\n")
			registry.defaults["family-two"] = []byte("updated body two\n")

			buf := logcapture.CaptureVerbose(t)

			if _, err := Reconcile(baseDir, registry, tt.mode, Source{Writer: Writer{Revision: "running-rev"}, Older: tt.older}); err != nil {
				t.Fatalf("Reconcile(%v) returned error: %v", tt.mode, err)
			}

			got := buf.String()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("refusal log = %q; want it to contain %q", got, want)
				}
			}
			if lines := strings.Split(strings.TrimSpace(got), "\n"); len(lines) != 1 {
				t.Errorf("refusal log = %q; want one line listing both stencils, got %d lines", got, len(lines))
			}
		})
	}
}
