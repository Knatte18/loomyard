// parentlock_test.go covers the seams of Run-Shed's parent lock backstop: the record lookup by run and the landing-in-flight read.

package battencli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// TestChildParentLock asserts Read finds the lock record whose owner is the child's run by slug and pair and Release frees it.
// It asserts KeepsLock holds the lock for a task pair running Finalize or verifying.
func TestChildParentLock(t *testing.T) {
	t.Parallel()

	t.Run("ReadFindsTheRecordOfTheChildsRunAndReleaseFreesIt", func(t *testing.T) {
		t.Parallel()
		boardDir := t.TempDir()
		taskLocation := &lyxcwd.Location{RepoName: "example", HubPath: t.TempDir(), WorktreeName: "a-slug", AnchorRel: "."}
		store := &parentlock.Store{Dir: parentlock.Dir(boardDir)}
		other := parentlock.Owner{Slug: "other-slug", Pair: "/hub/other-slug", RunID: "run-2", PID: 7, StartTime: "other-start"}
		sameSlugOtherPair := parentlock.Owner{Slug: "a-slug", Pair: "/elsewhere/a-slug", RunID: "run-3", PID: 8, StartTime: "elsewhere-start"}
		child := parentlock.Owner{Slug: "a-slug", Pair: taskLocation.WorktreePath(), RunID: "run-1", PID: 9, StartTime: "child-start"}
		for branch, owner := range map[string]parentlock.Owner{"main": other, "develop": sameSlugOtherPair, "release": child} {
			if attempt, err := store.TryAcquire(branch, owner); err != nil || !attempt.Granted {
				t.Fatalf("TryAcquire(%s) = %+v, %v; want granted", branch, attempt, err)
			}
		}

		lock := childParentLock("a-slug", func() (*lyxcwd.Location, error) { return taskLocation, nil }, boardDir, 0)
		record, found, err := lock.Read()
		if err != nil || !found || record.Branch != "release" || record.Owner != child {
			t.Fatalf("Read() = %+v, %v, %v; want the child's record on release", record, found, err)
		}
		if released, err := lock.Release(record); err != nil || !released {
			t.Fatalf("Release() = %v, %v; want true, nil", released, err)
		}
		if _, found, err := lock.Read(); err != nil || found {
			t.Errorf("Read() after the release = found %v, error %v; want none", found, err)
		}
	})

	t.Run("KeepsLockReadsTheTaskPairsStatusAndVerifyMarker", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			producer string
			marker   bool
			want     bool
		}{
			{name: "no status and no marker keeps nothing"},
			{name: "Finalize running keeps it", producer: loomshed.NameFinalize, want: true},
			{name: "another row running keeps nothing", producer: loomshed.NameMergeIn},
			{name: "a live verify marker keeps it", producer: loomshed.NameMergeIn, marker: true, want: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				taskLocation := &lyxcwd.Location{RepoName: "example", HubPath: t.TempDir(), WorktreeName: "a-slug", AnchorRel: "."}
				if tt.producer != "" {
					for _, path := range []string{shedrun.StatusFile(taskLocation, shedrun.SelfRunID), shedrun.StatusLock(taskLocation, shedrun.SelfRunID)} {
						if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
							t.Fatal(err)
						}
					}
					err := state.UpdateJSON(shedrun.StatusFile(taskLocation, shedrun.SelfRunID), shedrun.StatusLock(taskLocation, shedrun.SelfRunID), func(cur shedengine.Status, found bool) (shedengine.Status, error) {
						cur.CurrentProducer = tt.producer
						cur.State = shedengine.StateRunning
						return cur, nil
					})
					if err != nil {
						t.Fatalf("seed status: %v", err)
					}
				}
				if tt.marker {
					markerPath := verifytree.NewPaths(taskLocation.WorktreePath(), verifytree.Dir(taskLocation.AnchorPath())).Marker
					if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(markerPath, fmt.Appendf(nil, "site: landing\npid: %d\n", os.Getpid()), 0o644); err != nil {
						t.Fatal(err)
					}
				}

				lock := childParentLock("a-slug", func() (*lyxcwd.Location, error) { return taskLocation, nil }, t.TempDir(), 0)
				keeps, err := lock.KeepsLock()

				if err != nil || keeps != tt.want {
					t.Errorf("KeepsLock() = %v, %v; want %v, nil", keeps, err, tt.want)
				}
			})
		}
	})
}
