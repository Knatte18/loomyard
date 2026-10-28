//go:build tier0

// handback_test.go pins which blocked stops HandsBack hands to the orch.

package landingshed

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestHandsBack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		producer     string
		reason       string
		want         bool
		wantEscalate bool
	}{
		{"LandingStopAtParentAlign", "Parent-Align", LandingStopPrefix + "the fixer escalated; way forward: resolve it, then `lym loom resume`", true, false},
		{"LandingStopAtMergeIn", "Merge-In", LandingStopPrefix + "the fixer escalated; way forward: resolve it, then `lym loom resume`", true, false},
		{"LandingStopAtFinalize", "Finalize", LandingStopPrefix + "the fixer's change is over the caps; way forward: review it", true, false},
		{"UndecodableAttemptRecordAtParentAlign", "Parent-Align", LandingStopPrefix + "the fixer attempt record of Parent-Align does not decode: p; way forward: remove p", true, false},
		{"UndecodableAttemptRecordAtMergeIn", "Merge-In", LandingStopPrefix + "the fixer attempt record of Merge-In does not decode: p; way forward: remove p", true, false},
		{"UndecodableAttemptRecordAtFinalize", "Finalize", LandingStopPrefix + "the fixer attempt record of Finalize does not decode: p; way forward: remove p", true, false},
		{"LockRecheckAtFinalize", "Finalize", LandingStopPrefix + "the parent lock on \"main\" no longer names this run; nothing was landed", true, false},
		{"RerouteCapAtFinalize", "Finalize", shedengine.ReasonRerouteCapReached + ": Finalize rerouted to Parent-Align 3 times since its last done; way forward: x", true, false},
		{"RerouteCapAtWebster", "Webster", shedengine.ReasonRerouteCapReached + ": Webster rerouted to Plan-Write 3 times since its last done; way forward: x", false, false},
		{"RerouteCapAtParentAlign", "Parent-Align", shedengine.ReasonRerouteCapReached + ": Parent-Align rerouted to Webster 3 times since its last done; way forward: x", false, false},
		{"RerouteCapAtMergeIn", "Merge-In", shedengine.ReasonRerouteCapReached + ": Merge-In rerouted to Webster 3 times since its last done; way forward: x", false, false},
		{"LandingStopPrefixAtOtherRow", "Describe", LandingStopPrefix + "the fixer escalated", false, false},
		{"UnrelatedReasonAtFinalize", "Finalize", "dirty parent: the parent's code checkout has tracked changes", false, false},
		{"EscalateAtParentAlign", "Parent-Align", EscalateStopPrefix + "the fixer escalated; way forward: record the decision", false, true},
		{"EscalateAtMergeIn", "Merge-In", EscalateStopPrefix + "the fixer escalated; way forward: record the decision", false, true},
		{"EscalateAtFinalize", "Finalize", EscalateStopPrefix + "the fixer escalated; way forward: record the decision", false, true},
		{"EscalateAtOtherRow", "Describe", EscalateStopPrefix + "the fixer escalated", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := HandsBack(tt.producer, tt.reason); got != tt.want {
				t.Errorf("HandsBack(%q, %q) = %v; want %v", tt.producer, tt.reason, got, tt.want)
			}
			if got := EscalateHalt(tt.producer, tt.reason); got != tt.wantEscalate {
				t.Errorf("EscalateHalt(%q, %q) = %v; want %v", tt.producer, tt.reason, got, tt.wantEscalate)
			}
		})
	}
}
