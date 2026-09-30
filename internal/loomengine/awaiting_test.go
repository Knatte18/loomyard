// awaiting_test.go pins loom's handling of the planned hand-off halt: a run parked in state
// awaiting stays coherent (resumable) and files no self-report anomaly. Untagged (Tier 1): pure.

package loomengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestCheckCoherence_AwaitingStateAndHistoryPass(t *testing.T) {
	shed := validFreshShed()
	shed.State = shedengine.StateAwaiting
	shed.History = []shedengine.HistoryEntry{
		{Producer: "Loom-Preflight", Outcome: shedengine.Awaiting, At: "2026-01-01T00:00:00Z"},
	}

	got := checkCoherence(shed, validFreshProduct(), "Loom-Preflight", []string{"Preflight", "Loom-Preflight"})
	if len(got) != 0 {
		t.Errorf("checkCoherence() = %+v; want empty", got)
	}
}

func TestDetectHaltAnomaly_AwaitingFilesNone(t *testing.T) {
	final := validFreshShed()
	final.State = shedengine.StateAwaiting
	final.Error = "waiting for PR review"

	if a, ok := detectHaltAnomaly(final, validFreshProduct()); ok {
		t.Errorf("detectHaltAnomaly() = %+v, true; want no anomaly for an awaiting halt", a)
	}
}
