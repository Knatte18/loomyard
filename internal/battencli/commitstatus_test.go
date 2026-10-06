// commitstatus_test.go pins batten's per-transition status seam: the on-disk no-op-transition skip added on top of the shared statuscommit core's dispositions, which statuscommit's own tests pin.
// Every test here drives newCommitStatusSeam against injected commitStatusDeps stub closures and a real temp-file marker path, spawning no git and no process,
// so the file stays Tier 1 with no hub fixture.
package battencli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// runStatusRel is the status file's relative path for runID.
func runStatusRel(loc *lyxcwd.Location, runID string) string {
	return shedrun.StatusRel(loc, runID)
}

// TestNewCommitStatusSeam covers the on-disk no-op-transition skip over a real temp-file marker:
//   - a repeated (producer, state) pair commits and pushes exactly once, not twice, across two calls
//     to the same seam instance;
//   - the skip still holds when the seam is rebuilt from scratch between calls, the shape a fresh
//     "lyx batten step" process takes -- the one case that proves the on-disk marker rather than an
//     in-closure variable;
//   - a changed pair commits and pushes again rather than being absorbed by the skip;
//   - a missing or corrupt marker falls back to committing once rather than erroring, since the
//     marker is a cache and losing it costs one redundant commit, never correctness.
//
// The commit message pins batten's own prefix, which the shared core's tests cannot, since they
// render whatever prefix they are given.
func TestNewCommitStatusSeam(t *testing.T) {
	type transition struct{ producer, state string }

	tests := []struct {
		name string
		// markerRel is the marker path under the test's temp dir.
		markerRel     string
		corruptMarker bool
		// rebuildSeam builds a fresh seam instance, over the same marker, for every call.
		rebuildSeam bool
		calls       []transition
		wantCommits int
	}{
		{name: "RepeatedPairCommitsOnce", markerRel: "last-commit", calls: []transition{{"Run-Shed", "running"}, {"Run-Shed", "running"}}, wantCommits: 1},
		{name: "SkipSurvivesARebuiltSeam", markerRel: "last-commit", rebuildSeam: true, calls: []transition{{"Run-Shed", "running"}, {"Run-Shed", "running"}}, wantCommits: 1},
		{name: "ChangedPairCommitsAgain", markerRel: "last-commit", calls: []transition{{"Run-Shed", "running"}, {"Run-Shed", "paused"}}, wantCommits: 2},
		{name: "MissingMarkerCommitsOnce", markerRel: filepath.Join("does-not-exist", "last-commit"), calls: []transition{{"Run-Shed", "running"}}, wantCommits: 1},
		{name: "CorruptMarkerCommitsOnce", markerRel: "last-commit", corruptMarker: true, calls: []transition{{"Run-Shed", "running"}}, wantCommits: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			markerPath := filepath.Join(t.TempDir(), tt.markerRel)
			markerLockPath := markerPath + ".lock"
			if tt.corruptMarker {
				if err := os.WriteFile(markerPath, []byte("not valid json{{{"), 0o644); err != nil {
					t.Fatalf("seed corrupt marker: %v", err)
				}
			}

			var commits, pushes int
			var msg string
			newSeam := func() func(producer, state string) error {
				return newCommitStatusSeam(commitStatusDeps{
					MergeActive: func() (bool, error) { return false, nil },
					Commit:      func(m string) error { commits++; msg = m; return nil },
					Push:        func() error { pushes++; return nil },
				}, markerPath, markerLockPath)
			}

			seam := newSeam()
			for i, call := range tt.calls {
				if tt.rebuildSeam && i > 0 {
					seam = newSeam()
				}
				if err := seam(call.producer, call.state); err != nil {
					t.Fatalf("seam(%q, %q) call %d = %v; want nil", call.producer, call.state, i+1, err)
				}
			}

			if commits != tt.wantCommits {
				t.Errorf("commits = %d; want exactly %d", commits, tt.wantCommits)
			}
			if pushes != tt.wantCommits {
				t.Errorf("pushes = %d; want exactly %d", pushes, tt.wantCommits)
			}
			last := tt.calls[len(tt.calls)-1]
			if want := "batten: " + last.producer + " -> " + last.state; msg != want {
				t.Errorf("Commit msg = %q; want %q", msg, want)
			}
		})
	}
}

// TestBattenRunCommitPaths asserts a status transition commits the run's seed alongside its status
// once one exists, and commits the status alone while none does.
// A run directory is durable, fabric-synced state: a status committed without its seed leaves a
// resumed machine able to read how far the run came but not what it is running.
//
//testtiming:keep pins the seed-alongside-status commit paths, which the end-to-end rows never assert
func TestBattenRunCommitPaths(t *testing.T) {
	const runID = "some-slug"
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}

	got := battenRunCommitPaths(loc, runID)
	if len(got) != 1 || got[0] != runStatusRel(loc, runID) {
		t.Fatalf("battenRunCommitPaths with no seed on disk = %v; want just %q", got, runStatusRel(loc, runID))
	}

	if err := shedrun.WriteSeed(loc, runID, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	got = battenRunCommitPaths(loc, runID)
	want := []string{runStatusRel(loc, runID), shedrun.SeedRel(loc, runID)}
	if len(got) != len(want) {
		t.Fatalf("battenRunCommitPaths with a seed on disk = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("battenRunCommitPaths(...)[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}
