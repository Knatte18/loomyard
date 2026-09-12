// step_equivalence_test.go is the test that protects the (*Shed).Run / (*Shed).Step extraction:
// for each fixture it drives one Shed to a terminal state with repeated Step calls, drives an
// identical second Shed once with Run, and asserts the two resulting status files agree on every
// clock-independent field.

package shedengine

import (
	"context"
	"testing"
)

// stepEquivalenceMaxIterations caps the stepping loop below. A regression that never leaves
// StateRunning must fail this test loudly rather than hang the suite.
const stepEquivalenceMaxIterations = 100

// driveWithStep repeatedly calls shed.Step until it returns a State other than StateRunning,
// failing the test if stepEquivalenceMaxIterations is reached first. It returns the final
// StepResult.
func driveWithStep(t *testing.T, shed *Shed) StepResult {
	t.Helper()
	for i := 0; i < stepEquivalenceMaxIterations; i++ {
		res, err := shed.Step(context.Background())
		if err != nil {
			t.Fatalf("Step iteration %d: %v", i, err)
		}
		if res.State != StateRunning {
			return res
		}
	}
	t.Fatalf("driveWithStep: exceeded %d iterations without leaving %q -- a regression that never terminates", stepEquivalenceMaxIterations, StateRunning)
	return StepResult{}
}

// assertStatusesAgree compares stepped and run on every clock-independent field:
// current_producer, state, error, all three activity fields, and the history entries'
// producer/outcome/output sequence and length.
//
// history[].at is deliberately excluded, and the exclusion is stated here rather than only in a
// code comment elsewhere: it is time.Now() via nowRFC3339(), shed.go pins the absence of an
// injectable clock on Shed as a deliberate design decision (a value tests assert structurally, via
// assertRFC3339UTC, rather than by literal), and a byte-identical whole-file assertion would
// therefore pass only when both drives happened to land inside the same second -- flaky by
// construction. Adding a clock seam to Shed purely to strengthen this assertion is rejected for
// the same reason: it would widen the struct shape the design pins for a field these assertions
// already cover structurally.
func assertStatusesAgree(t *testing.T, stepped, run Status) {
	t.Helper()

	if stepped.CurrentProducer != run.CurrentProducer {
		t.Errorf("CurrentProducer: stepped = %q, run = %q; want equal", stepped.CurrentProducer, run.CurrentProducer)
	}
	if stepped.State != run.State {
		t.Errorf("State: stepped = %q, run = %q; want equal", stepped.State, run.State)
	}
	if stepped.Error != run.Error {
		t.Errorf("Error: stepped = %q, run = %q; want equal", stepped.Error, run.Error)
	}
	if stepped.Activity.Now != run.Activity.Now {
		t.Errorf("Activity.Now: stepped = %q, run = %q; want equal", stepped.Activity.Now, run.Activity.Now)
	}
	if stepped.Activity.Last != run.Activity.Last {
		t.Errorf("Activity.Last: stepped = %q, run = %q; want equal", stepped.Activity.Last, run.Activity.Last)
	}
	if stepped.Activity.Wait != run.Activity.Wait {
		t.Errorf("Activity.Wait: stepped = %q, run = %q; want equal", stepped.Activity.Wait, run.Activity.Wait)
	}

	if len(stepped.History) != len(run.History) {
		t.Fatalf("len(History): stepped = %d, run = %d; want equal", len(stepped.History), len(run.History))
	}
	for i := range stepped.History {
		s, r := stepped.History[i], run.History[i]
		if s.Producer != r.Producer || s.Outcome != r.Outcome || s.Output != r.Output {
			t.Errorf("History[%d]: stepped = {Producer:%q Outcome:%q Output:%q}, run = {Producer:%q Outcome:%q Output:%q}; want equal (excluding At)",
				i, s.Producer, s.Outcome, s.Output, r.Producer, r.Outcome, r.Output)
		}
		assertRFC3339UTC(t, s.At)
		assertRFC3339UTC(t, r.At)
	}
}

func TestStepRunEquivalence_ReachesDone(t *testing.T) {
	names := []string{"Preflight", "Plan-Write", "Finalize"}

	steppedShed, steppedStatusPath, _, steppedStatusLockPath := newTestShed(t)
	steppedShed.Producers = linearChain(t, names, []ShedProducer{
		fixedOutcomeProducer(Done, ""),
		fixedOutcomeProducer(Done, ""),
		fixedOutcomeProducer(Done, ""),
	})
	seedStatus(t, steppedStatusPath, steppedStatusLockPath, commonSeed(names[0]))
	finalStep := driveWithStep(t, steppedShed)

	runShed, runStatusPath, _, runStatusLockPath := newTestShed(t)
	runShed.Producers = linearChain(t, names, []ShedProducer{
		fixedOutcomeProducer(Done, ""),
		fixedOutcomeProducer(Done, ""),
		fixedOutcomeProducer(Done, ""),
	})
	seedStatus(t, runStatusPath, runStatusLockPath, commonSeed(names[0]))
	runResult, err := runShed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}

	steppedStatus := readStatus(t, steppedStatusPath, steppedStatusLockPath)
	runStatus := readStatus(t, runStatusPath, runStatusLockPath)
	assertStatusesAgree(t, steppedStatus, runStatus)

	if string(runResult.Outcome) != string(finalStep.State) {
		t.Errorf("Run Result.Outcome = %q; want %q (final StepResult.State)", runResult.Outcome, finalStep.State)
	}
	if runResult.HaltedProducer != finalStep.Next {
		t.Errorf("Run Result.HaltedProducer = %q; want %q (final StepResult.Next)", runResult.HaltedProducer, finalStep.Next)
	}
	if runResult.Reason != finalStep.Reason {
		t.Errorf("Run Result.Reason = %q; want %q (final StepResult.Reason)", runResult.Reason, finalStep.Reason)
	}
	if len(runResult.History) != len(finalStep.History) {
		t.Errorf("len(Run Result.History) = %d; want %d (final StepResult.History)", len(runResult.History), len(finalStep.History))
	}
}

func TestStepRunEquivalence_ReachesBlocked(t *testing.T) {
	buildProducers := func() []ProducerDef {
		return []ProducerDef{
			{Name: "A", Producer: fixedOutcomeProducer(Stuck, ""), OnStuck: ""},
		}
	}

	steppedShed, steppedStatusPath, _, steppedStatusLockPath := newTestShed(t)
	steppedShed.Producers = buildProducers()
	seedStatus(t, steppedStatusPath, steppedStatusLockPath, commonSeed("A"))
	finalStep := driveWithStep(t, steppedShed)

	runShed, runStatusPath, _, runStatusLockPath := newTestShed(t)
	runShed.Producers = buildProducers()
	seedStatus(t, runStatusPath, runStatusLockPath, commonSeed("A"))
	runResult, err := runShed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}

	steppedStatus := readStatus(t, steppedStatusPath, steppedStatusLockPath)
	runStatus := readStatus(t, runStatusPath, runStatusLockPath)
	assertStatusesAgree(t, steppedStatus, runStatus)

	if string(runResult.Outcome) != string(finalStep.State) {
		t.Errorf("Run Result.Outcome = %q; want %q (final StepResult.State)", runResult.Outcome, finalStep.State)
	}
	if runResult.HaltedProducer != finalStep.Next {
		t.Errorf("Run Result.HaltedProducer = %q; want %q (final StepResult.Next)", runResult.HaltedProducer, finalStep.Next)
	}
	if runResult.Reason != finalStep.Reason {
		t.Errorf("Run Result.Reason = %q; want %q (final StepResult.Reason)", runResult.Reason, finalStep.Reason)
	}
	if len(runResult.History) != len(finalStep.History) {
		t.Errorf("len(Run Result.History) = %d; want %d (final StepResult.History)", len(runResult.History), len(finalStep.History))
	}
}

func TestStepRunEquivalence_BouncesBeforeDone(t *testing.T) {
	buildProducers := func() []ProducerDef {
		firstCall := true
		a := &funcProducer{}
		a.fn = func(ctx context.Context) (Outcome, OutputPointer, error) {
			if firstCall {
				firstCall = false
				return Stuck, OutputPointer{}, nil
			}
			return Done, OutputPointer{}, nil
		}
		return []ProducerDef{
			{Name: "A", Producer: a, OnStuck: "A", OnDone: "B"},
			{Name: "B", Producer: fixedOutcomeProducer(Done, "")},
		}
	}

	steppedShed, steppedStatusPath, _, steppedStatusLockPath := newTestShed(t)
	steppedShed.Producers = buildProducers()
	seedStatus(t, steppedStatusPath, steppedStatusLockPath, commonSeed("A"))
	finalStep := driveWithStep(t, steppedShed)

	runShed, runStatusPath, _, runStatusLockPath := newTestShed(t)
	runShed.Producers = buildProducers()
	seedStatus(t, runStatusPath, runStatusLockPath, commonSeed("A"))
	runResult, err := runShed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}

	steppedStatus := readStatus(t, steppedStatusPath, steppedStatusLockPath)
	runStatus := readStatus(t, runStatusPath, runStatusLockPath)
	assertStatusesAgree(t, steppedStatus, runStatus)

	if string(runResult.Outcome) != string(finalStep.State) {
		t.Errorf("Run Result.Outcome = %q; want %q (final StepResult.State)", runResult.Outcome, finalStep.State)
	}
	if runResult.HaltedProducer != finalStep.Next {
		t.Errorf("Run Result.HaltedProducer = %q; want %q (final StepResult.Next)", runResult.HaltedProducer, finalStep.Next)
	}
	if runResult.Reason != finalStep.Reason {
		t.Errorf("Run Result.Reason = %q; want %q (final StepResult.Reason)", runResult.Reason, finalStep.Reason)
	}
	if len(runResult.History) != len(finalStep.History) {
		t.Errorf("len(Run Result.History) = %d; want %d (final StepResult.History)", len(runResult.History), len(finalStep.History))
	}
	if len(runResult.History) < 3 {
		t.Fatalf("len(runResult.History) = %d; want at least 3 (a bounce, then two Done entries)", len(runResult.History))
	}
	if runResult.History[0].Outcome != Stuck {
		t.Errorf("runResult.History[0].Outcome = %q; want %q -- this fixture must bounce at least once", runResult.History[0].Outcome, Stuck)
	}
}
