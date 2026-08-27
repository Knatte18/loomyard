// run_commitstatus_test.go covers Shed.CommitStatus, the injected persist-time seam: that it is
// called on every write path a run performs, that a nil seam is a silent no-op, that a seam error
// propagates out of Run after the status-file write has already landed, that the closure receives
// the transition's own producer and state, and that it runs outside internal/state's write lock.
// Every test here is in-process, spawns no git and no process, and reuses testsupport_test.go's
// fake producers and helpers.

package shedengine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// commitStatusCall records one CommitStatus invocation's arguments, in call order.
type commitStatusCall struct {
	producer string
	state    string
}

// recordingCommitStatus returns a CommitStatus closure that appends every call it receives to
// calls, and a func reporting the total call count so far.
func recordingCommitStatus(calls *[]commitStatusCall) func(producer, state string) error {
	return func(producer, state string) error {
		*calls = append(*calls, commitStatusCall{producer: producer, state: state})
		return nil
	}
}

// TestRun_CommitStatusCalledOnEveryWritePath drives three sequential Run calls against one status
// file -- resuming across each halt exactly as TestRun_CrashRecovery and TestRun_ResumeAfterHalt
// do -- so that, between them, all six of persist's write-path kinds fire at least once: the
// resume write, a running-to-running transition, a stuck bounce, a blocked terminal, a failed
// terminal, and a done terminal. CommitStatus's call count is checked against the exact number of
// persists each run is known to perform, not merely asserted non-zero, so a call dropped on any
// one path would still be caught even though the other five still fire.
func TestRun_CommitStatusCalledOnEveryWritePath(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	var calls []commitStatusCall
	shed.CommitStatus = recordingCommitStatus(&calls)

	// Run 1: seeded StateBlocked (not running) on "A", so the first iteration performs the
	// resume write. A's Done routes forward to B (running-to-running). B's Stuck bounces to C
	// (stuck bounce). C's Stuck has no OnStuck target, so the run halts blocked.
	// Persists: resume, running-to-running, stuck-bounce, blocked-terminal -- four.
	a := fixedOutcomeProducer(Done, "")
	b := fixedOutcomeProducer(Stuck, "")
	c := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnDone: "B"},
		{Name: "B", Producer: b, OnStuck: "C"},
		{Name: "C", Producer: c},
	}
	seed := commonSeed("A")
	seed.State = StateBlocked
	seed.Error = "seeded halt for resume test"
	seedStatus(t, statusPath, statusLockPath, seed)

	result1, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run 1(...) = _, %v; want nil error", err)
	}
	if result1.Outcome != RunBlocked {
		t.Fatalf("Run 1 Result.Outcome = %q; want %q", result1.Outcome, RunBlocked)
	}
	if len(calls) != 4 {
		t.Fatalf("len(calls) after Run 1 = %d; want 4 (resume, running-to-running, stuck-bounce, blocked-terminal)", len(calls))
	}

	// Run 2: resume from the blocked halt at C, whose Call now returns a hard error, so the
	// second iteration hits the failed terminal. Persists: resume, failed-terminal -- two.
	c2 := &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
		return "", OutputPointer{}, errors.New("boom")
	}}
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: fixedOutcomeProducer(Done, "")},
		{Name: "B", Producer: fixedOutcomeProducer(Done, "")},
		{Name: "C", Producer: c2},
	}
	if _, err := shed.Run(context.Background()); err == nil {
		t.Fatalf("Run 2(...) = _, nil; want a non-nil error")
	}
	if len(calls) != 6 {
		t.Fatalf("len(calls) after Run 2 = %d; want 6 (+resume, +failed-terminal)", len(calls))
	}

	// Run 3: a fresh status file seeded StateFailed on "D", whose Call returns Done with an
	// empty OnDone, so the second iteration hits the done terminal. Persists: resume,
	// done-terminal -- two.
	d := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{
		{Name: "D", Producer: d},
	}
	seed3 := commonSeed("D")
	seed3.State = StateFailed
	seed3.Error = "seeded halt for resume test"
	seedStatus(t, statusPath, statusLockPath, seed3)

	result3, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run 3(...) = _, %v; want nil error", err)
	}
	if result3.Outcome != RunDone {
		t.Fatalf("Run 3 Result.Outcome = %q; want %q", result3.Outcome, RunDone)
	}
	if len(calls) != 8 {
		t.Fatalf("len(calls) after Run 3 = %d; want 8 (+resume, +done-terminal)", len(calls))
	}

	// The exact ordered sequence pins each of the six write-path kinds to the specific call
	// that produces it, rather than merely asserting each State value showed up somewhere.
	want := []commitStatusCall{
		{producer: "A", state: string(StateRunning)}, // Run 1's resume write.
		{producer: "B", state: string(StateRunning)}, // Run 1's running-to-running transition (A's Done routes to B).
		{producer: "C", state: string(StateRunning)}, // Run 1's stuck bounce (B's Stuck routes to C).
		{producer: "C", state: string(StateBlocked)}, // Run 1's blocked terminal (C's Stuck has no OnStuck target).
		{producer: "C", state: string(StateRunning)}, // Run 2's resume write.
		{producer: "C", state: string(StateFailed)},  // Run 2's failed terminal.
		{producer: "D", state: string(StateRunning)}, // Run 3's resume write.
		{producer: "D", state: string(StateDone)},    // Run 3's done terminal.
	}
	for i, w := range want {
		if calls[i] != w {
			t.Errorf("calls[%d] = %+v; want %+v", i, calls[i], w)
		}
	}
}

// TestRun_CommitStatusNilIsSilentNoOp asserts that a nil CommitStatus -- the zero value, and the
// only value every product wired before this task's other batches leaves it at -- changes nothing
// about a run: it completes exactly as TestRun_HappyPath does.
func TestRun_CommitStatusNilIsSilentNoOp(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	// shed.CommitStatus is left at its zero value: nil.

	p1 := fixedOutcomeProducer(Done, "")
	p2 := fixedOutcomeProducer(Done, "")
	shed.Producers = linearChain(t, []string{"A", "B"}, []ShedProducer{p1, p2})
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateDone {
		t.Errorf("persisted State = %q; want %q", got.State, StateDone)
	}
	if p1.calls != 1 || p2.calls != 1 {
		t.Errorf("p1.calls = %d, p2.calls = %d; want 1, 1", p1.calls, p2.calls)
	}
}

// TestRun_CommitStatusErrorHaltsRunWithWriteAlreadyDurable asserts that a CommitStatus error
// propagates out of Run, and that the status file on disk still carries the write persist had
// already made before the closure ran -- the write and the seam failure are two separate
// outcomes, and the first must survive the second.
func TestRun_CommitStatusErrorHaltsRunWithWriteAlreadyDurable(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	wantErr := errors.New("commitstatus: push failed")
	shed.CommitStatus = func(producer, state string) error {
		return wantErr
	}

	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	_, err := shed.Run(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run(...) error = %v; want it to wrap %v", err, wantErr)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateDone {
		t.Errorf("persisted State = %q; want %q -- the write must survive the seam's own failure", got.State, StateDone)
	}
	if got.CurrentProducer != "A" {
		t.Errorf("persisted CurrentProducer = %q; want %q", got.CurrentProducer, "A")
	}
}

// TestRun_CommitStatusReceivesTransitionProducerAndState asserts that the closure receives the
// same producer and state strings that were just written to the file, checked across the running,
// blocked, and done transitions.
func TestRun_CommitStatusReceivesTransitionProducerAndState(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	var calls []commitStatusCall
	shed.CommitStatus = recordingCommitStatus(&calls)

	a := fixedOutcomeProducer(Done, "")
	b := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnDone: "B"},
		{Name: "B", Producer: b},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	if _, err := shed.Run(context.Background()); err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}

	want := []commitStatusCall{
		{producer: "B", state: string(StateRunning)}, // A's Done routes forward to B.
		{producer: "B", state: string(StateBlocked)}, // B's Stuck has no OnStuck target.
	}
	if len(calls) != len(want) {
		t.Fatalf("len(calls) = %d; want %d: %+v", len(calls), len(want), calls)
	}
	for i, w := range want {
		if calls[i] != w {
			t.Errorf("calls[%d] = %+v; want %+v", i, calls[i], w)
		}
	}
}

// TestRun_CommitStatusRunsOutsideTheStateWriteLock is property 5, and the primary proof of this
// batch's lock-ordering requirement: a closure that itself acquires a read lock on
// StatusLockPath must complete rather than deadlock, which is the executable form of "outside the
// lock." A closure called from inside persist's mutate callback would still hold internal/state's
// own write lock on StatusLockPath, and internal/state's blocking acquire would then hang forever
// against itself -- the exact failure this test is written to catch, so it runs Run on a
// goroutine and bounds the wait rather than risking an unbounded test-suite hang.
func TestRun_CommitStatusRunsOutsideTheStateWriteLock(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.CommitStatus = func(producer, state string) error {
		l, err := lock.AcquireReadLock(statusLockPath)
		if err != nil {
			return err
		}
		return l.Release()
	}

	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	done := make(chan error, 1)
	go func() {
		_, err := shed.Run(context.Background())
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run(...) = _, %v; want nil error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run(...) did not return within 5s; CommitStatus's read-lock acquire deadlocked against persist's own write lock -- CommitStatus is being called from inside the mutate callback")
	}
}
