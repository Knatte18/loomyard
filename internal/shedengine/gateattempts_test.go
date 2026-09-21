// gateattempts_test.go covers HistoryEntry.GateAttempts end to end: a producer's OutputPointer
// reaches Step's persisted HistoryEntry, and the status file on disk actually shows the count, per
// the producer-gate contract's "recorded in the row's envelope/history so the status file
// shows it" requirement. It reuses newTestShed, seedStatus, readStatus, and commonSeed from
// testsupport_test.go rather than redeclaring any of them.

package shedengine

import (
	"context"
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

func TestStep_GateAttempts_StuckExhaustion_PersistsCount(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	attempts := 3
	producer := gatedOutcomeProducer(Stuck, "", &attempts)
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if len(res.History) != 1 {
		t.Fatalf("len(History) = %d; want 1", len(res.History))
	}
	if got := res.History[0].GateAttempts; got == nil || *got != 3 {
		t.Errorf("History[0].GateAttempts = %v; want pointer to 3", got)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if len(got.History) != 1 {
		t.Fatalf("persisted len(History) = %d; want 1", len(got.History))
	}
	if gp := got.History[0].GateAttempts; gp == nil || *gp != 3 {
		t.Errorf("persisted History[0].GateAttempts = %v; want pointer to 3 -- the status file must show the exhausted gate's attempt count", gp)
	}
}

func TestStep_GateAttempts_PassAfterRetry_PersistsCount(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	attempts := 2
	producer := gatedOutcomeProducer(Done, "out.md", &attempts)
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if got := res.History[0].GateAttempts; got == nil || *got != 2 {
		t.Errorf("History[0].GateAttempts = %v; want pointer to 2 (passed after two re-prompts)", got)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if gp := got.History[0].GateAttempts; gp == nil || *gp != 2 {
		t.Errorf("persisted History[0].GateAttempts = %v; want pointer to 2 -- the status file must show a pass-after-retry count, not just an exhaustion count", gp)
	}
}

func TestStep_GateAttempts_PassedFirstTry_ZeroIsShownNotOmitted(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	attempts := 0
	producer := gatedOutcomeProducer(Done, "out.md", &attempts)
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}

	got := readStatus(t, statusPath, statusLockPath)
	gp := got.History[0].GateAttempts
	if gp == nil {
		t.Fatalf("persisted History[0].GateAttempts = nil; want a non-nil pointer to 0 -- a gate that passed on the first try is a distinct, meaningful verdict from an ungated call and must not collapse to the same absent field")
	}
	if *gp != 0 {
		t.Errorf("persisted History[0].GateAttempts = %d; want 0", *gp)
	}
}

func TestStep_GateAttempts_UngatedProducer_OmitsField(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	producer := fixedOutcomeProducer(Done, "out.md")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if got := res.History[0].GateAttempts; got != nil {
		t.Errorf("History[0].GateAttempts = %v; want nil for an ungated producer", *got)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if gp := got.History[0].GateAttempts; gp != nil {
		t.Errorf("persisted History[0].GateAttempts = %v; want nil -- an ungated row must not gain a spurious gate_attempts field", *gp)
	}
}
