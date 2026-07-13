// rundir_test.go covers the run-dir lifecycle: runDirRoot's default vs.
// configured resolution, createRunDir + saveRunState/loadRunState
// round-tripping, findRunByStrand's hit/miss paths, and sweepOrphans' age
// guard (young orphan kept, old orphan removed, live-guid dir kept).

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

func TestRunDirRoot_DefaultUsesDotLyxShuttle(t *testing.T) {
	layout := &hubgeometry.Layout{Cwd: `C:\worktree`}
	got := runDirRoot(Config{}, layout)
	want := filepath.Join(layout.DotLyxDir(), "shuttle")
	if got != want {
		t.Errorf("runDirRoot() = %q, want %q", got, want)
	}
}

func TestRunDirRoot_RelativeResolvesAgainstWorktreeRoot(t *testing.T) {
	layout := &hubgeometry.Layout{Cwd: `C:\worktree`, WorktreeRoot: `C:\worktree`}
	got := runDirRoot(Config{RunDir: "custom-runs"}, layout)
	want := filepath.Join(layout.WorktreeRoot, "custom-runs")
	if got != want {
		t.Errorf("runDirRoot() = %q, want %q", got, want)
	}
}

func TestRunDirRoot_AbsoluteUsedVerbatim(t *testing.T) {
	layout := &hubgeometry.Layout{Cwd: `C:\worktree`, WorktreeRoot: `C:\worktree`}
	// An OS-absolute RunDir must be returned verbatim, never re-joined against
	// WorktreeRoot. t.TempDir() yields an absolute path on any host (a
	// drive-rooted path on Windows, a /… path on POSIX), so the test is not
	// tied to one OS's notion of "absolute".
	abs := filepath.Join(t.TempDir(), "runs")
	got := runDirRoot(Config{RunDir: abs}, layout)
	if got != abs {
		t.Errorf("runDirRoot() = %q, want %q", got, abs)
	}
}

func TestRunState_RoundTrip(t *testing.T) {
	root := t.TempDir()
	runID, runDir, err := createRunDir(root)
	if err != nil {
		t.Fatalf("createRunDir() error: %v", err)
	}
	if runID == "" {
		t.Fatal("createRunDir() returned empty runID")
	}

	want := RunState{
		RunID:        runID,
		StrandGUID:   "strand-guid-1",
		SessionID:    "session-1",
		Interactive:  true,
		OutputFiles:  []string{filepath.Join(root, "out.md")},
		PromptPath:   filepath.Join(runDir, "prompt.md"),
		SettingsPath: filepath.Join(runDir, "settings.json"),
		EventsPath:   filepath.Join(runDir, "events.jsonl"),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := saveRunState(runDir, want); err != nil {
		t.Fatalf("saveRunState() error: %v", err)
	}

	got, found, err := loadRunState(runDir)
	if err != nil {
		t.Fatalf("loadRunState() error: %v", err)
	}
	if !found {
		t.Fatal("loadRunState() found = false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loadRunState() = %+v, want %+v", got, want)
	}
}

func TestLoadRunState_AbsentReturnsNotFound(t *testing.T) {
	runDir := t.TempDir()
	_, found, err := loadRunState(runDir)
	if err != nil {
		t.Fatalf("loadRunState() error: %v", err)
	}
	if found {
		t.Error("loadRunState() found = true, want false for absent run.json")
	}
}

// seedRun creates <root>/<id>/run.json with the given strand guid and
// returns the run directory path.
func seedRun(t *testing.T, root, id, strandGUID string) string {
	t.Helper()
	runDir := filepath.Join(root, id)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := saveRunState(runDir, RunState{RunID: id, StrandGUID: strandGUID}); err != nil {
		t.Fatalf("saveRunState: %v", err)
	}
	return runDir
}

func TestFindRunByStrand_Hit(t *testing.T) {
	root := t.TempDir()
	seedRun(t, root, "run-a", "strand-a")
	wantDir := seedRun(t, root, "run-b", "strand-b")

	rs, dir, err := findRunByStrand(root, "strand-b")
	if err != nil {
		t.Fatalf("findRunByStrand() error: %v", err)
	}
	if rs.StrandGUID != "strand-b" {
		t.Errorf("StrandGUID = %q, want %q", rs.StrandGUID, "strand-b")
	}
	if dir != wantDir {
		t.Errorf("dir = %q, want %q", dir, wantDir)
	}
}

func TestFindRunByStrand_Miss(t *testing.T) {
	root := t.TempDir()
	seedRun(t, root, "run-a", "strand-a")

	if _, _, err := findRunByStrand(root, "does-not-exist"); err == nil {
		t.Fatal("findRunByStrand() = nil error, want error for unknown strand guid")
	}
}

// setDirMTime backdates the modification time of dir by age relative to
// referenceNow, simulating a run directory that was created age ago.
func setDirMTime(t *testing.T, dir string, referenceNow time.Time, age time.Duration) {
	t.Helper()
	mtime := referenceNow.Add(-age)
	if err := os.Chtimes(dir, mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func TestSweepOrphans_AgeGuardAndLiveGuid(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	minAge := 90 * time.Second

	// A young orphan (StrandGUID not live, but the dir is younger than
	// minAge) must survive the sweep — it may be mid-startup.
	youngOrphan := seedRun(t, root, "young-orphan", "strand-gone")
	setDirMTime(t, youngOrphan, now, 10*time.Second)

	// An old orphan (older than minAge, StrandGUID not live) must be
	// removed.
	oldOrphan := seedRun(t, root, "old-orphan", "strand-gone")
	setDirMTime(t, oldOrphan, now, 10*time.Minute)

	// A dir whose StrandGUID IS live must be kept regardless of age.
	liveDir := seedRun(t, root, "live-run", "strand-live")
	setDirMTime(t, liveDir, now, 10*time.Minute)

	strandGUIDs := map[string]bool{"strand-live": true}
	removed, err := sweepOrphans(root, strandGUIDs, minAge, now)
	if err != nil {
		t.Fatalf("sweepOrphans() error: %v", err)
	}

	if len(removed) != 1 || removed[0] != oldOrphan {
		t.Errorf("removed = %v, want [%s]", removed, oldOrphan)
	}
	if _, err := os.Stat(youngOrphan); err != nil {
		t.Errorf("young orphan dir was removed, want kept: %v", err)
	}
	if _, err := os.Stat(liveDir); err != nil {
		t.Errorf("live-guid dir was removed, want kept: %v", err)
	}
	if _, err := os.Stat(oldOrphan); !os.IsNotExist(err) {
		t.Errorf("old orphan dir still exists, want removed")
	}
}

func TestSweepOrphans_MissingRunJSONRemovedOnlyWhenOld(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	minAge := 90 * time.Second

	// A dir with no run.json at all (unreadable state), young: must be
	// kept.
	youngNoState := filepath.Join(root, "young-no-state")
	if err := os.MkdirAll(youngNoState, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	setDirMTime(t, youngNoState, now, 10*time.Second)

	// Same shape, but old: must be removed.
	oldNoState := filepath.Join(root, "old-no-state")
	if err := os.MkdirAll(oldNoState, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	setDirMTime(t, oldNoState, now, 10*time.Minute)

	removed, err := sweepOrphans(root, map[string]bool{}, minAge, now)
	if err != nil {
		t.Fatalf("sweepOrphans() error: %v", err)
	}
	if len(removed) != 1 || removed[0] != oldNoState {
		t.Errorf("removed = %v, want [%s]", removed, oldNoState)
	}
	if _, err := os.Stat(youngNoState); err != nil {
		t.Errorf("young no-state dir was removed, want kept: %v", err)
	}
}
