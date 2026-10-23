// handback_test.go pins which blocked stops HandsBack hands to the orch.

package landingshed_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestHandsBack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		producer string
		reason   string
		want     bool
	}{
		{"LandingStopAtMergeIn", "Merge-In", landingshed.LandingStopPrefix + "the fixer escalated; way forward: resolve it, then `lyx loom resume`", true},
		{"LandingStopAtFinalize", "Finalize", landingshed.LandingStopPrefix + "the fixer's change is over the caps; way forward: review it", true},
		{"UndecodableAttemptRecordAtMergeIn", "Merge-In", landingshed.LandingStopPrefix + "the fixer attempt record of Merge-In does not decode: p; way forward: remove p", true},
		{"UndecodableAttemptRecordAtFinalize", "Finalize", landingshed.LandingStopPrefix + "the fixer attempt record of Finalize does not decode: p; way forward: remove p", true},
		{"LockRecheckAtFinalize", "Finalize", landingshed.LandingStopPrefix + "the parent lock on \"main\" no longer names this run; nothing was landed", true},
		{"RerouteCapAtFinalize", "Finalize", shedengine.ReasonRerouteCapReached + ": Finalize rerouted to Merge-In 3 times since its last done; way forward: x", true},
		{"RerouteCapAtWebster", "Webster", shedengine.ReasonRerouteCapReached + ": Webster rerouted to Plan-Write 3 times since its last done; way forward: x", false},
		{"RerouteCapAtMergeIn", "Merge-In", shedengine.ReasonRerouteCapReached + ": Merge-In rerouted to Webster 3 times since its last done; way forward: x", false},
		{"LandingStopPrefixAtOtherRow", "Describe", landingshed.LandingStopPrefix + "the fixer escalated", false},
		{"UnrelatedReasonAtFinalize", "Finalize", "dirty parent: the parent's code checkout has tracked changes", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := landingshed.HandsBack(tt.producer, tt.reason); got != tt.want {
				t.Errorf("HandsBack(%q, %q) = %v; want %v", tt.producer, tt.reason, got, tt.want)
			}
		})
	}
}
