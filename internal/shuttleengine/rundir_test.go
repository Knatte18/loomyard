// rundir_test.go covers the run-dir lifecycle: runDirRoot's default vs. configured resolution,
// createRunDir + saveRunState/loadRunState round-tripping, findRunByStrand's hit/miss paths, and
// sweepOrphans' age guard (young orphan kept, old orphan removed, live-guid dir kept).

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

func TestRunDirRoot(t *testing.T) {
	t.Parallel()

	// A real subpath of a worktree root, so the default branch's anchor-path anchoring (as
	// opposed to a worktree-root anchoring) is observable.
	defaultAnchor := filepath.Join(`C:\worktree`, "sub", "dir")
	// An OS-absolute RunDir must be returned verbatim, never re-joined against the anchor path.
	// t.TempDir() yields an absolute path on any host, so the row is not tied to one OS's notion
	// of "absolute".
	abs := filepath.Join(t.TempDir(), "runs")

	tests := []struct {
		name       string
		runDir     string
		anchorPath string
		want       string
	}{
		{
			name:       "default uses dot-lyx shuttle under the anchor path",
			anchorPath: defaultAnchor,
			want:       filepath.Join(defaultAnchor, lyxdirs.DotLyxDirName, "shuttle"),
		},
		{
			name:       "relative resolves against the anchor root",
			runDir:     "custom-runs",
			anchorPath: `C:\worktree`,
			want:       filepath.Join(`C:\worktree`, "custom-runs"),
		},
		{
			name:       "absolute is used verbatim",
			runDir:     abs,
			anchorPath: `C:\worktree`,
			want:       abs,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := runDirRoot(Config{RunDir: tt.runDir}, tt.anchorPath); got != tt.want {
				t.Errorf("runDirRoot() = %q, want %q", got, tt.want)
			}
		})
	}
}

//testtiming:keep pins that every RunState field survives save and load, and that an absent run.json reads as not found rather than as an error
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

	_, found, err = loadRunState(t.TempDir())
	if err != nil {
		t.Fatalf("loadRunState() error on an absent run.json: %v", err)
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

func TestFindRunByStrand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// seed creates the run dirs under root and returns the dir the lookup should hit, if any.
		seed       func(t *testing.T, root string) (wantDir string)
		strandGUID string
		// wantErrIn lists the fragments a miss error must name; empty means the lookup must hit.
		wantErrIn []string
		// wantErrNotIn is a fragment a miss error must not carry.
		wantErrNotIn string
	}{
		{
			name: "hit returns the run state and its directory",
			seed: func(t *testing.T, root string) string {
				seedRun(t, root, "run-a", "strand-a")
				return seedRun(t, root, "run-b", "strand-b")
			},
			strandGUID: "strand-b",
		},
		{
			// A clean scan must not hedge: every run.json was read, so "no run found" is the whole
			// truth and a could-not-be-read clause would make an ordinary caller mistake read like
			// possible corruption.
			name: "miss on a clean scan does not hedge",
			seed: func(t *testing.T, root string) string {
				seedRun(t, root, "run-a", "strand-a")
				return ""
			},
			strandGUID:   "does-not-exist",
			wantErrIn:    []string{"does-not-exist"},
			wantErrNotIn: "could not be read",
		},
		{
			// A truncated run.json is skipped so one damaged dir cannot abort the scan, but the
			// resulting miss must say the scan was incomplete: Runner.Interrupt and Runner.Send wrap
			// this error as "%q is not a shuttle strand", which sends an operator away from an agent
			// that is still live in its pane (proven live by truncating a running run's run.json).
			name: "miss names unreadable dirs",
			seed: func(t *testing.T, root string) string {
				seedRun(t, root, "run-a", "strand-a")
				damaged := seedRun(t, root, "run-damaged", "strand-damaged")
				if err := os.WriteFile(filepath.Join(damaged, runStateFileName), []byte(`{"strandGuid": "strand-dam`), 0o644); err != nil {
					t.Fatalf("truncate run.json: %v", err)
				}
				return ""
			},
			strandGUID: "strand-damaged",
			wantErrIn:  []string{"1 run directory", "could not be read", "still in its pane"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			wantDir := tt.seed(t, root)

			rs, dir, err := findRunByStrand(root, tt.strandGUID)
			if len(tt.wantErrIn) == 0 {
				if err != nil {
					t.Fatalf("findRunByStrand() error: %v", err)
				}
				if rs.StrandGUID != tt.strandGUID {
					t.Errorf("StrandGUID = %q, want %q", rs.StrandGUID, tt.strandGUID)
				}
				if dir != wantDir {
					t.Errorf("dir = %q, want %q", dir, wantDir)
				}
				return
			}
			if err == nil {
				t.Fatal("findRunByStrand() = nil error, want a miss")
			}
			for _, want := range tt.wantErrIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("findRunByStrand() error = %v; want it to name %q", err, want)
				}
			}
			if tt.wantErrNotIn != "" && strings.Contains(err.Error(), tt.wantErrNotIn) {
				t.Errorf("findRunByStrand() error = %v; want no %q clause", err, tt.wantErrNotIn)
			}
		})
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

//testtiming:keep pins the age guard over orphans, a live strand guid keeping its dir, and a dir with no run.json being removed only when old
func TestSweepOrphans_AgeGuardAndLiveGuid(t *testing.T) {
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

	// os.ReadDir hands the dirs back sorted by name, so the removed ones come back in that order.
	if want := []string{oldNoState, oldOrphan}; !reflect.DeepEqual(removed, want) {
		t.Errorf("removed = %v, want %v", removed, want)
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
	if _, err := os.Stat(youngNoState); err != nil {
		t.Errorf("young no-state dir was removed, want kept: %v", err)
	}
}

// TestSweepOrphans_OneUndeletableDirDoesNotAbandonTheRest is R6-14's regression test: returning on
// the first os.RemoveAll failure abandoned every later entry, so a single directory that cannot be
// removed made every subsequent Start sweep nothing and orphan run dirs accumulated without bound.
func TestSweepOrphans_OneUndeletableDirDoesNotAbandonTheRest(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: a read-only parent directory does not stop RemoveAll")
	}

	root := t.TempDir()
	// "a-stuck" sorts before "b-sweepable", so os.ReadDir hands the undeletable one back first.
	stuck := filepath.Join(root, "a-stuck")
	sweepable := filepath.Join(root, "b-sweepable")
	for _, dir := range []string{stuck, sweepable} {
		if err := os.MkdirAll(filepath.Join(dir, "child"), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}
	// A directory whose own write bit is cleared cannot have its child unlinked, so RemoveAll fails.
	if err := os.Chmod(stuck, 0o500); err != nil {
		t.Fatalf("Chmod(%q) error = %v", stuck, err)
	}
	t.Cleanup(func() { _ = os.Chmod(stuck, 0o755) })

	// Neither dir carries a run.json, so both are orphans; both are old enough for the age guard.
	removed, err := sweepOrphans(root, map[string]bool{}, time.Minute, time.Now().Add(time.Hour))
	if err == nil {
		t.Error("sweepOrphans() error = nil; want the undeletable directory reported")
	}
	if len(removed) != 1 || removed[0] != sweepable {
		t.Errorf("sweepOrphans() removed = %v; want [%q] — one stuck directory must not abandon the rest of the sweep", removed, sweepable)
	}
	if _, statErr := os.Stat(sweepable); !os.IsNotExist(statErr) {
		t.Errorf("os.Stat(%q) = %v; want the sweepable orphan gone", sweepable, statErr)
	}
}
