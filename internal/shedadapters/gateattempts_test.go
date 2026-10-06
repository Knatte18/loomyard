package shedadapters

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestGateFailedReason_NamesAttemptsAndFindingsPath pins that both the attempt count and the
// findings path are interpolated, which the producers' own gate-failed fixtures (zero attempts,
// often no findings path) cannot tell apart from a fixed string.
//
//testtiming:keep pins that the attempt count and the findings path are both interpolated into the gate-failed reason
func TestGateFailedReason_NamesAttemptsAndFindingsPath(t *testing.T) {
	gate := &shuttleengine.GateOutcome{Passed: false, Attempts: 3, FindingsPath: "/run/gate-findings.md"}
	const want = "gate did not pass after 3 attempts; findings: /run/gate-findings.md"
	if got := gateFailedReason(gate); got != want {
		t.Errorf("gateFailedReason(%+v) = %q; want %q", gate, got, want)
	}
}
