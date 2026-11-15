//go:build !windows

// cancelledrecord_integration_test.go pins the deletion of a cancelled run's record after its pair's removal.

package pairteardown_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRemoveCancelledRecord_DeletesOnlyACancelledRecordAndCommitsItsRemoval(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "cancelled-run"
	tests := []struct {
		name        string
		state       shedengine.State
		tracked     bool
		wantRemoved bool
	}{
		{"CancelledRecordTheRecordsHeadDoesNotTrackIsDeletedWithNoCommit", shedengine.StateCancelled, false, true},
		{"CancelledRecordTheRecordsHeadTracksIsDeletedAndTheRemovalCommitted", shedengine.StateCancelled, true, true},
		{"RunningRecordStays", shedengine.StateRunning, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			td, err := pairteardown.New(h.Location)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			writeStatus(t, h.Location, slug, shedengine.Status{State: tt.state, CurrentProducer: "Run-Shed"})
			if tt.tracked {
				if _, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), h.Location, []string{shedrun.RunDirRel(h.Location, slug)}, "seed the run record", fabricengine.EnvSyncOptions()); err != nil || !committed {
					t.Fatalf("seed commit = committed %v, error %v; want a commit", committed, err)
				}
			}

			removed, err := td.RemoveCancelledRecordForTest(slug)

			if err != nil || removed != tt.wantRemoved {
				t.Fatalf("removeCancelledRecord() = %v, %v; want %v, nil", removed, err, tt.wantRemoved)
			}
			if _, statErr := os.Stat(shedrun.RunDir(h.Location, slug)); os.IsNotExist(statErr) != tt.wantRemoved {
				t.Errorf("stat of the run directory = %v; want it gone: %v", statErr, tt.wantRemoved)
			}
			if tt.tracked {
				if _, found, err := fabricengine.CommittedAnchoredFile(h.Location, shedrun.StatusRel(h.Location, slug)); err != nil || found == tt.wantRemoved {
					t.Errorf("the records HEAD tracks the status = %v, %v; want tracked: %v", found, err, !tt.wantRemoved)
				}
			}
		})
	}
}
