// gateattempts_test.go covers HistoryEntry.GateAttempts end to end: a producer's OutputPointer
// reaches Step's persisted HistoryEntry, and the status file on disk actually shows the count, per
// the producer-gate contract's "recorded in the row's envelope/history so the status file
// shows it" requirement. It reuses newTestShed, seedStatus, readStatus, and commonSeed from
// testsupport_test.go rather than redeclaring any of them.

package shedengine

import (
	"context"
	"strconv"
	"testing"
)

// gatedOutcomeProducer returns a *funcProducer that always reports outcome and an OutputPointer
// naming outputPath and gateAttempts, so a test can drive Step through a gated producer's exact
// return shape without a real shuttleengine/shedadapters dependency.
func gatedOutcomeProducer(outcome Outcome, outputPath string, gateAttempts *int) *funcProducer {
	return &funcProducer{
		fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
			return outcome, OutputPointer{Path: outputPath, GateAttempts: gateAttempts}, nil
		},
	}
}

// TestStep_GateAttempts pins what Step records of a producer's gate attempts, in the returned history and in the status file:
// an exhausted gate's count, a pass-after-retry count, and a first-try pass shown as a non-nil zero (a distinct verdict from an ungated call), while an ungated producer gains no gate_attempts field.
func TestStep_GateAttempts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		producer *funcProducer
		want     *int
	}{
		{"stuck exhaustion persists the count", gatedOutcomeProducer(Stuck, "", intPtr(3)), intPtr(3)},
		{"pass after retry persists the count", gatedOutcomeProducer(Done, "out.md", intPtr(2)), intPtr(2)},
		{"passed first try shows zero, not omitted", gatedOutcomeProducer(Done, "out.md", intPtr(0)), intPtr(0)},
		{"ungated producer omits the field", fixedOutcomeProducer(Done, "out.md"), nil},
	}
	describe := func(p *int) string {
		if p == nil {
			return "nil"
		}
		return "pointer to " + strconv.Itoa(*p)
	}
	same := func(got, want *int) bool {
		if got == nil || want == nil {
			return got == want
		}
		return *got == *want
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed, statusPath, _, statusLockPath := newTestShed(t)
			shed.Producers = []ProducerDef{{Name: "A", Producer: tt.producer}}
			seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

			res, err := shed.Step(context.Background())
			if err != nil {
				t.Fatalf("Step(...) = _, %v; want nil error", err)
			}
			if len(res.History) != 1 {
				t.Fatalf("len(History) = %d; want 1", len(res.History))
			}
			if got := res.History[0].GateAttempts; !same(got, tt.want) {
				t.Errorf("History[0].GateAttempts = %s; want %s", describe(got), describe(tt.want))
			}

			persisted := readStatus(t, statusPath, statusLockPath)
			if len(persisted.History) != 1 {
				t.Fatalf("persisted len(History) = %d; want 1", len(persisted.History))
			}
			if got := persisted.History[0].GateAttempts; !same(got, tt.want) {
				t.Errorf("persisted History[0].GateAttempts = %s; want %s -- the status file must show the attempt count", describe(got), describe(tt.want))
			}
		})
	}
}
