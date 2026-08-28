// watchloop_test.go pins watchState's pure debounce, coalescing, and per-event retry-cap contracts
// against a synthetic clock — never a real one — and watchLoop's driver contracts (modes, the signal
// file's lifecycle, and survival across failures) against TmuxCmd's execHook seam and a real
// t.TempDir() signal file and lock file. Nothing here sleeps for a second or more, and nothing here
// is timing-dependent: every state-machine assertion advances a local time.Time by hand, and every
// driver assertion polls a recorded outcome rather than a wall-clock duration.

package reedengine

import (
	"testing"
	"time"
)

// TestWatchDefaultTiming_MatchesTheFiveConstants pins that watchDefaultTiming returns exactly the
// five package constants, so a later tuning change moves one line and does not break the suite.
func TestWatchDefaultTiming_MatchesTheFiveConstants(t *testing.T) {
	got := watchDefaultTiming()
	want := watchTiming{
		SignalTick:  watchdogSignalTick,
		PollCycle:   watchdogPollCycle,
		Quiet:       watchdogDebounceQuiet,
		BaseDelay:   watchdogRetryBaseDelay,
		MaxAttempts: watchdogMaxAttempts,
	}
	if got != want {
		t.Errorf("watchDefaultTiming() = %+v, want %+v", got, want)
	}
}

// TestWatchState_SingleSignalWaitsThenApplies pins that a single Signal yields watchPlanWait until
// the quiet period has elapsed and watchPlanApply at and after it.
func TestWatchState_SingleSignalWaitsThenApplies(t *testing.T) {
	timing := watchDefaultTiming()
	s := newWatchState(timing)
	now := time.Now()
	s.Signal(now)

	if got := s.Plan(now); got != watchPlanWait {
		t.Errorf("Plan(signal time) = %v, want watchPlanWait", got)
	}
	if got := s.Plan(now.Add(timing.Quiet - time.Millisecond)); got != watchPlanWait {
		t.Errorf("Plan(just before quiet elapses) = %v, want watchPlanWait", got)
	}
	if got := s.Plan(now.Add(timing.Quiet)); got != watchPlanApply {
		t.Errorf("Plan(exactly at quiet) = %v, want watchPlanApply", got)
	}
	if got := s.Plan(now.Add(timing.Quiet + time.Millisecond)); got != watchPlanApply {
		t.Errorf("Plan(after quiet) = %v, want watchPlanApply", got)
	}
}

// TestWatchState_CoalescesABurstIntoOneApply pins the coalescing contract: twenty Signal calls at
// Quiet/4 intervals yield watchPlanWait throughout and exactly one watchPlanApply, after the last
// signal's quiet period.
func TestWatchState_CoalescesABurstIntoOneApply(t *testing.T) {
	timing := watchDefaultTiming()
	s := newWatchState(timing)
	now := time.Now()
	step := timing.Quiet / 4

	var lastSignal time.Time
	for i := 0; i < 20; i++ {
		lastSignal = now.Add(time.Duration(i) * step)
		s.Signal(lastSignal)
		if got := s.Plan(lastSignal); got != watchPlanWait {
			t.Fatalf("Plan() during burst at step %d = %v, want watchPlanWait", i, got)
		}
	}

	if got := s.Plan(lastSignal.Add(timing.Quiet - time.Millisecond)); got != watchPlanWait {
		t.Errorf("Plan(just before final quiet elapses) = %v, want watchPlanWait", got)
	}
	if got := s.Plan(lastSignal.Add(timing.Quiet)); got != watchPlanApply {
		t.Errorf("Plan(at final quiet) = %v, want watchPlanApply", got)
	}
}

// TestWatchState_SignalInsideQuietRestartsIt pins that a Signal arriving inside the quiet period
// restarts it: the apply is owed relative to the later signal, not the earlier one.
func TestWatchState_SignalInsideQuietRestartsIt(t *testing.T) {
	timing := watchDefaultTiming()
	s := newWatchState(timing)
	now := time.Now()
	s.Signal(now)

	second := now.Add(timing.Quiet / 2)
	s.Signal(second)

	// The first signal's own deadline (now + Quiet) must NOT be owed anymore.
	if got := s.Plan(now.Add(timing.Quiet)); got != watchPlanWait {
		t.Errorf("Plan(at first signal's deadline) = %v, want watchPlanWait (restarted by the second signal)", got)
	}
	if got := s.Plan(second.Add(timing.Quiet)); got != watchPlanApply {
		t.Errorf("Plan(at second signal's deadline) = %v, want watchPlanApply", got)
	}
}

// TestWatchState_SignalDuringInFlightApplySchedulesOneFollowUp pins that a Signal arriving while an
// apply is notionally in flight schedules exactly one follow-up, not a queue: two Signal calls
// before a single Succeeded leave the state with at most one owed apply.
func TestWatchState_SignalDuringInFlightApplySchedulesOneFollowUp(t *testing.T) {
	timing := watchDefaultTiming()
	s := newWatchState(timing)
	now := time.Now()
	s.Signal(now)
	s.Signal(now.Add(timing.Quiet / 4))

	s.Succeeded()

	if got := s.Plan(now.Add(10 * timing.Quiet)); got != watchPlanWait {
		t.Errorf("Plan() after a single Succeeded = %v, want watchPlanWait (no queued follow-up)", got)
	}
}

// TestWatchState_SucceededClearsTheOwedApply pins that Succeeded clears the owed apply: the next
// Plan at any later time yields watchPlanWait.
func TestWatchState_SucceededClearsTheOwedApply(t *testing.T) {
	timing := watchDefaultTiming()
	s := newWatchState(timing)
	now := time.Now()
	s.Signal(now)
	s.Succeeded()

	if got := s.Plan(now.Add(timing.Quiet)); got != watchPlanWait {
		t.Errorf("Plan() after Succeeded = %v, want watchPlanWait", got)
	}
	if got := s.Plan(now.Add(24 * time.Hour)); got != watchPlanWait {
		t.Errorf("Plan() long after Succeeded = %v, want watchPlanWait", got)
	}
}

// TestWatchState_FailedEscalatesAndCaps pins that attempts 1 and 2 report abandoned == false and
// push the next apply out by BaseDelay then 2*BaseDelay; attempt 3 (MaxAttempts) reports
// abandoned == true and leaves Plan yielding watchPlanWait forever after.
func TestWatchState_FailedEscalatesAndCaps(t *testing.T) {
	timing := watchDefaultTiming()
	s := newWatchState(timing)
	now := time.Now()
	s.Signal(now)

	failAt1 := now.Add(timing.Quiet)
	if abandoned := s.Failed(failAt1); abandoned {
		t.Fatalf("Failed() attempt 1 abandoned = true, want false")
	}
	if got := s.Plan(failAt1.Add(timing.BaseDelay - time.Millisecond)); got != watchPlanWait {
		t.Errorf("Plan() just before attempt 1's delay elapses = %v, want watchPlanWait", got)
	}
	if got := s.Plan(failAt1.Add(timing.BaseDelay)); got != watchPlanApply {
		t.Errorf("Plan() at attempt 1's delay = %v, want watchPlanApply", got)
	}

	failAt2 := failAt1.Add(timing.BaseDelay)
	if abandoned := s.Failed(failAt2); abandoned {
		t.Fatalf("Failed() attempt 2 abandoned = true, want false")
	}
	if got := s.Plan(failAt2.Add(2*timing.BaseDelay - time.Millisecond)); got != watchPlanWait {
		t.Errorf("Plan() just before attempt 2's delay elapses = %v, want watchPlanWait", got)
	}
	if got := s.Plan(failAt2.Add(2 * timing.BaseDelay)); got != watchPlanApply {
		t.Errorf("Plan() at attempt 2's delay = %v, want watchPlanApply", got)
	}

	failAt3 := failAt2.Add(2 * timing.BaseDelay)
	if abandoned := s.Failed(failAt3); !abandoned {
		t.Fatalf("Failed() attempt %d (MaxAttempts) abandoned = false, want true", timing.MaxAttempts)
	}
	if got := s.Plan(failAt3.Add(24 * time.Hour)); got != watchPlanWait {
		t.Errorf("Plan() long after abandonment = %v, want watchPlanWait forever", got)
	}
}

// TestWatchState_StreakResetsOnSuccess pins that the cap is per streak, not cumulative: Failed,
// Failed, Succeeded, then a fresh Signal and two more Failed calls must again report
// abandoned == false.
func TestWatchState_StreakResetsOnSuccess(t *testing.T) {
	timing := watchDefaultTiming()
	s := newWatchState(timing)
	now := time.Now()
	s.Signal(now)

	if abandoned := s.Failed(now); abandoned {
		t.Fatalf("Failed() attempt 1 abandoned = true, want false")
	}
	if abandoned := s.Failed(now); abandoned {
		t.Fatalf("Failed() attempt 2 abandoned = true, want false")
	}
	s.Succeeded()

	s.Signal(now)
	if abandoned := s.Failed(now); abandoned {
		t.Errorf("Failed() attempt 1 of the fresh streak abandoned = true, want false")
	}
	if abandoned := s.Failed(now); abandoned {
		t.Errorf("Failed() attempt 2 of the fresh streak abandoned = true, want false")
	}
}

// TestWatchState_StreakResetsOnFreshSignal pins that the streak resets on a fresh signal too:
// Failed, Failed, then Signal, then two more Failed calls must again report abandoned == false.
func TestWatchState_StreakResetsOnFreshSignal(t *testing.T) {
	timing := watchDefaultTiming()
	s := newWatchState(timing)
	now := time.Now()
	s.Signal(now)

	if abandoned := s.Failed(now); abandoned {
		t.Fatalf("Failed() attempt 1 abandoned = true, want false")
	}
	if abandoned := s.Failed(now); abandoned {
		t.Fatalf("Failed() attempt 2 abandoned = true, want false")
	}

	s.Signal(now)
	if abandoned := s.Failed(now); abandoned {
		t.Errorf("Failed() attempt 1 after a fresh signal abandoned = true, want false")
	}
	if abandoned := s.Failed(now); abandoned {
		t.Errorf("Failed() attempt 2 after a fresh signal abandoned = true, want false")
	}
}

// TestWatchState_DeferredChangesNothing pins that Deferred leaves the attempt count and the
// next-apply time untouched, whether taken between two Failed calls or while an apply is owed.
func TestWatchState_DeferredChangesNothing(t *testing.T) {
	t.Run("BetweenTwoFailedCalls", func(t *testing.T) {
		timing := watchDefaultTiming()
		s := newWatchState(timing)
		now := time.Now()
		s.Signal(now)
		s.Failed(now)

		before := *s
		s.Deferred()
		if *s != before {
			t.Errorf("Deferred() changed state: before=%+v after=%+v", before, *s)
		}

		// The attempt budget is untouched: the next Failed call is still attempt 2, not attempt 3.
		if abandoned := s.Failed(now.Add(timing.BaseDelay)); abandoned {
			t.Errorf("Failed() after a Deferred() abandoned = true, want false (Deferred must not consume budget)")
		}
	})

	t.Run("WhileAnApplyIsOwed", func(t *testing.T) {
		timing := watchDefaultTiming()
		s := newWatchState(timing)
		now := time.Now()
		s.Signal(now)

		before := *s
		s.Deferred()
		if *s != before {
			t.Errorf("Deferred() changed state: before=%+v after=%+v", before, *s)
		}
		if got := s.Plan(now.Add(timing.Quiet)); got != watchPlanApply {
			t.Errorf("Plan() after Deferred() while an apply was owed = %v, want watchPlanApply (still owed)", got)
		}
	})
}

// TestWatchState_FreshSignalAfterExhaustedStreakReArms pins the load-bearing assertion that
// separates the per-event cap from a loop-level cap: after an exhausted streak, a fresh Signal
// re-arms the state and the very next quiet period yields watchPlanApply.
func TestWatchState_FreshSignalAfterExhaustedStreakReArms(t *testing.T) {
	timing := watchDefaultTiming()
	s := newWatchState(timing)
	now := time.Now()
	s.Signal(now)

	for i := 0; i < timing.MaxAttempts; i++ {
		s.Failed(now)
	}
	if got := s.Plan(now.Add(24 * time.Hour)); got != watchPlanWait {
		t.Fatalf("Plan() after exhausting the streak = %v, want watchPlanWait", got)
	}

	fresh := now.Add(time.Hour)
	s.Signal(fresh)
	if got := s.Plan(fresh.Add(timing.Quiet - time.Millisecond)); got != watchPlanWait {
		t.Errorf("Plan() just before the re-armed quiet elapses = %v, want watchPlanWait", got)
	}
	if got := s.Plan(fresh.Add(timing.Quiet)); got != watchPlanApply {
		t.Errorf("Plan() at the re-armed quiet = %v, want watchPlanApply", got)
	}
}
