// watchloop_test.go pins watchState's pure debounce, coalescing, and per-event retry-cap contracts
// against a synthetic clock — never a real one — and watchLoop's driver contracts (modes, the signal
// file's lifecycle, and survival across failures) against TmuxCmd's execHook seam and a real
// t.TempDir() signal file and lock file. Nothing here sleeps for a second or more, and nothing here
// is timing-dependent: every state-machine assertion advances a local time.Time by hand, and every
// driver assertion polls a recorded outcome rather than a wall-clock duration.

package reedengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestWatchState pins the watcher's pure timing contracts against a synthetic clock, each a named step below:
// the default timing, the per-mode ticker cadence, and watchState's debounce, coalescing and per-event retry-cap behaviour.
//
//testtiming:keep pins the watcher's pure contracts on a synthetic clock: default timing, per-mode ticker cadence, debounce and coalescing of signals, one follow-up for signals during an apply, the escalating retry cap with per-streak reset, deferral costing no budget and a fresh signal re-arming an exhausted streak; its covering tests run this code without asserting it
func TestWatchState(t *testing.T) {
	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"DefaultTimingMatchesTheSixConstants", watchDefaultTimingMatchesTheSixConstants},
		{"TickerPeriodForAnswersPerModeCadence", tickerPeriodForAnswersPerModeCadence},
		{"SingleSignalWaitsThenApplies", watchStateSingleSignalWaitsThenApplies},
		{"CoalescesABurstIntoOneApply", watchStateCoalescesABurstIntoOneApply},
		{"SignalInsideQuietRestartsIt", watchStateSignalInsideQuietRestartsIt},
		{"SignalDuringInFlightApplySchedulesOneFollowUp", watchStateSignalDuringInFlightApplySchedulesOneFollowUp},
		{"SucceededClearsTheOwedApply", watchStateSucceededClearsTheOwedApply},
		{"FailedEscalatesAndCaps", watchStateFailedEscalatesAndCaps},
		{"StreakResetsOnSuccess", watchStateStreakResetsOnSuccess},
		{"StreakResetsOnFreshSignal", watchStateStreakResetsOnFreshSignal},
		{"DeferredChangesNothing", watchStateDeferredChangesNothing},
		{"FreshSignalAfterExhaustedStreakReArms", watchStateFreshSignalAfterExhaustedStreakReArms},
	}
	for _, step := range steps {
		t.Run(step.name, step.run)
	}
}

// watchDefaultTimingMatchesTheSixConstants pins that watchDefaultTiming returns exactly the
// six package constants, so a later tuning change moves one line and does not break the suite.
func watchDefaultTimingMatchesTheSixConstants(t *testing.T) {
	got := watchDefaultTiming()
	want := watchTiming{
		SignalTick:  watchdogSignalTick,
		PollCycle:   watchdogPollCycle,
		Quiet:       watchdogDebounceQuiet,
		BaseDelay:   watchdogRetryBaseDelay,
		MaxAttempts: watchdogMaxAttempts,
		Dormant:     watchdogDormantCycle,
	}
	if got != want {
		t.Errorf("watchDefaultTiming() = %+v, want %+v", got, want)
	}
}

// tickerPeriodForAnswersPerModeCadence pins tickerPeriodFor's cadence-per-mode contract
// directly: while dormant the loop refuses before any tmux round trip, so the recording hook
// observes nothing and cannot measure the interval, which is why this is pinned as a pure
// function test rather than through a driver-test timing measurement.
func tickerPeriodForAnswersPerModeCadence(t *testing.T) {
	timing := watchdogTestTiming()
	tests := []struct {
		name string
		mode watchMode
		want time.Duration
	}{
		{"Dormant", watchModeDormant, timing.Dormant},
		{"Signal", watchModeSignal, timing.SignalTick},
		{"Poll", watchModePoll, timing.PollCycle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tickerPeriodFor(tt.mode, timing); got != tt.want {
				t.Errorf("tickerPeriodFor(%v, timing) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

// watchStateSingleSignalWaitsThenApplies pins that a single Signal yields watchPlanWait until
// the quiet period has elapsed and watchPlanApply at and after it.
func watchStateSingleSignalWaitsThenApplies(t *testing.T) {
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

// watchStateCoalescesABurstIntoOneApply pins the coalescing contract: twenty Signal calls at
// Quiet/4 intervals yield watchPlanWait throughout and exactly one watchPlanApply, after the last
// signal's quiet period.
func watchStateCoalescesABurstIntoOneApply(t *testing.T) {
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

// watchStateSignalInsideQuietRestartsIt pins that a Signal arriving inside the quiet period
// restarts it: the apply is owed relative to the later signal, not the earlier one.
func watchStateSignalInsideQuietRestartsIt(t *testing.T) {
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

// watchStateSignalDuringInFlightApplySchedulesOneFollowUp pins that a Signal arriving while an
// apply is notionally in flight schedules exactly one follow-up, not a queue: two Signal calls
// before a single Succeeded leave the state with at most one owed apply.
func watchStateSignalDuringInFlightApplySchedulesOneFollowUp(t *testing.T) {
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

// watchStateSucceededClearsTheOwedApply pins that Succeeded clears the owed apply: the next
// Plan at any later time yields watchPlanWait.
func watchStateSucceededClearsTheOwedApply(t *testing.T) {
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

// watchStateFailedEscalatesAndCaps pins that attempts 1 and 2 report abandoned == false and
// push the next apply out by BaseDelay then 2*BaseDelay; attempt 3 (MaxAttempts) reports
// abandoned == true and leaves Plan yielding watchPlanWait forever after.
func watchStateFailedEscalatesAndCaps(t *testing.T) {
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

// watchStateStreakResetsOnSuccess pins that the cap is per streak, not cumulative: Failed,
// Failed, Succeeded, then a fresh Signal and two more Failed calls must again report
// abandoned == false.
func watchStateStreakResetsOnSuccess(t *testing.T) {
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

// watchStateStreakResetsOnFreshSignal pins that the streak resets on a fresh signal too:
// Failed, Failed, then Signal, then two more Failed calls must again report abandoned == false.
func watchStateStreakResetsOnFreshSignal(t *testing.T) {
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

// watchStateDeferredChangesNothing pins that Deferred leaves the attempt count and the
// next-apply time untouched, whether taken between two Failed calls or while an apply is owed.
func watchStateDeferredChangesNothing(t *testing.T) {
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

// watchStateFreshSignalAfterExhaustedStreakReArms pins the load-bearing assertion that
// separates the per-event cap from a loop-level cap: after an exhausted streak, a fresh Signal
// re-arms the state and the very next quiet period yields watchPlanApply.
func watchStateFreshSignalAfterExhaustedStreakReArms(t *testing.T) {
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

// --- watchLoop driver tests -------------------------------------------------
//
// These tests call the unexported watchLoop directly, always over a
// context.WithCancel context cancelled in a t.Cleanup, and always with a
// watchTiming whose durations are single-digit milliseconds. They reuse
// reapply_test.go's fixture shape (a hand-built Engine over a t.TempDir(), a
// persisted ReedState, and a recording TmuxCmd.execHook), observing the loop
// through the recorded argv, the signal file on disk, and a completion
// channel — never through wall-clock timing beyond "eventually, within a
// bounded poll".

// watchdogTestTiming returns a watchTiming whose every duration is a single-digit number of
// milliseconds, fast enough for an untagged test and small enough that no assertion here needs to
// wait anywhere near a second.
func watchdogTestTiming() watchTiming {
	return watchTiming{
		SignalTick:  2 * time.Millisecond,
		PollCycle:   5 * time.Millisecond,
		Quiet:       5 * time.Millisecond,
		BaseDelay:   2 * time.Millisecond,
		MaxAttempts: 3,
		Dormant:     15 * time.Millisecond,
	}
}

// newWatchLoopTestEngine builds an Engine and a persisted ReedState the way reapply_test.go's newReapplyTestEngine does — one strand bound to "%1", live panes "%1" and "%2" — wired to a fakeTmux the test goroutine may re-script mid-run while watchLoop runs in its own, and with cfg.Watchdog set to watchdog.
func newWatchLoopTestEngine(t *testing.T, watchdog string) (*Engine, *fakeTmux) {
	t.Helper()
	e := newTestEngine(t)
	e.cfg.Watchdog = watchdog
	st := &ReedState{
		Strands: []Strand{
			{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
		},
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	fake := installFakeTmux(t, e)
	fake.answerSession([]LivePane{{ID: "%1"}, {ID: "%2"}}, "100 21", nil)
	return e, fake
}

// startWatchLoop runs e.watchLoop(ctx, timing) in a goroutine, cancels ctx and drains the
// completion channel in a t.Cleanup (bounded, so a stuck loop cannot hang the test suite), and
// returns the cancel func and the completion channel for tests that want to assert on them
// directly.
func startWatchLoop(t *testing.T, e *Engine, timing watchTiming) (cancel context.CancelFunc, done chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done = make(chan error, 1)
	go func() {
		done <- e.watchLoop(ctx, timing)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
		}
	})
	return cancel, done
}

// eventually polls cond every millisecond until it reports true or timeout elapses, returning
// cond's final value either way.
func eventually(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return cond()
		}
		time.Sleep(time.Millisecond)
	}
}

// TestWatchLoop_ParkedWhenNotEnabled pins that with Watchdog "off" or an invalid value watchLoop issues no tmux call,
// does not return within a bounded wait, and returns context.Canceled only after ctx is cancelled:
// a config typo parks the loop rather than killing the keepalive, so it must not return an error and must not return at all until cancellation.
func TestWatchLoop_ParkedWhenNotEnabled(t *testing.T) {
	for _, watchdog := range []string{"off", "garbage"} {
		t.Run(watchdog, func(t *testing.T) {
			e, fake := newWatchLoopTestEngine(t, watchdog)
			cancel, done := startWatchLoop(t, e, watchdogTestTiming())

			select {
			case err := <-done:
				t.Fatalf("watchLoop returned %v before cancellation, want it parked", err)
			case <-time.After(30 * time.Millisecond):
			}
			if calls := fake.Calls(); len(calls) != 0 {
				t.Errorf("tmux calls = %v, want zero tmux calls while parked", calls)
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("watchLoop() error = %v, want context.Canceled", err)
				}
			case <-time.After(200 * time.Millisecond):
				t.Fatalf("watchLoop did not return after cancellation")
			}
		})
	}
}

// TestWatchLoop_StaleSignalFileRemovedAtStart pins that a signal file present before the call is
// gone shortly after the loop starts.
//
//testtiming:keep pins a signal file present before the loop starts being removed at start, so an old resize does not trigger an apply; its covering tests run this code without asserting it
func TestWatchLoop_StaleSignalFileRemovedAtStart(t *testing.T) {
	e, _ := newWatchLoopTestEngine(t, "on")
	signalPath := e.resizeSignalPath()
	if err := os.MkdirAll(e.stateDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(signalPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	startWatchLoop(t, e, watchdogTestTiming())

	if !eventually(t, 200*time.Millisecond, func() bool {
		_, err := os.Stat(signalPath)
		return os.IsNotExist(err)
	}) {
		t.Errorf("stale signal file at %s was not removed", signalPath)
	}
}

// TestWatchLoop_PollModeByDefault pins that with show-options reporting no hook, the loop issues
// repeated reapplyLayout cycles at PollCycle and never promotes into signal-mode behaviour.
//
//testtiming:keep pins the loop repeating reapplyLayout cycles at the poll cadence and probing the hook every cycle while show-options reports no hook, never promoting; its covering tests run this code without asserting it
func TestWatchLoop_PollModeByDefault(t *testing.T) {
	e, fake := newWatchLoopTestEngine(t, "on")
	fake.answer("show-options", "", nil)

	startWatchLoop(t, e, watchdogTestTiming())

	if !eventually(t, 300*time.Millisecond, func() bool { return fake.Count("list-panes") >= 3 }) {
		t.Fatalf("list-panes calls = %d, want at least 3 poll cycles", fake.Count("list-panes"))
	}
	if fake.Count("show-options") == 0 {
		t.Errorf("show-options calls = 0, want poll mode to probe every cycle")
	}
}

// waitForPromotion runs the loop already promoted to signal mode against fake's current show-options answer (which must already report reed's own command), by waiting for the list-panes call count to stop growing across two consecutive observation windows — the observable proxy for "no more per-cycle reapplyLayout calls", since promotion is otherwise an internal mode flag.
func waitForPromotion(t *testing.T, fake *fakeTmux) int {
	t.Helper()
	if !eventually(t, 200*time.Millisecond, func() bool { return fake.Count("list-panes") >= 1 }) {
		t.Fatalf("list-panes calls = %d, want at least 1 (the promoting call)", fake.Count("list-panes"))
	}
	var stableCount int
	if !eventually(t, 300*time.Millisecond, func() bool {
		before := fake.Count("list-panes")
		time.Sleep(20 * time.Millisecond)
		after := fake.Count("list-panes")
		stableCount = after
		return before == after
	}) {
		t.Fatalf("list-panes call count never stabilized, want promotion to stop per-cycle polling")
	}
	return stableCount
}

// TestWatchLoop_SignalMode pins, as ordered steps on one running loop, the promotion into signal mode and what the loop does there.
// With show-options returning reed's own command string the loop promotes and stops issuing per-cycle reapplyLayout calls.
// Each later step builds on the state the earlier ones left.
// Signal mode never re-probes, so scripting show-options to return the empty string afterwards produces no further probe round trips.
// A signal file then causes exactly one select-layout after the quiet period, and the file is gone before that select-layout appears in the recorded argv.
//
//testtiming:keep pins promotion into signal mode stopping per-cycle polling, a signal file producing exactly one select-layout after the file is removed, and signal mode never re-probing the hook; its covering tests run this code without asserting it
func TestWatchLoop_SignalMode(t *testing.T) {
	e, fake := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	fake.answer("show-options", ownCommand, nil)
	startWatchLoop(t, e, watchdogTestTiming())

	stable := waitForPromotion(t, fake)
	probesAtPromotion := fake.Count("show-options")
	// The promotion tick's own first-ever apply (lastApplied starts as the zero box, which never
	// equals a live box) already issued one select-layout; the baseline below is what the
	// signal-triggered apply must exceed by exactly one.
	baseline := fake.Count("select-layout")

	// A demoting implementation would re-probe and see the hook gone.
	fake.answer("show-options", "", nil)
	// Change the box so the coming signal-triggered apply is a real, observable select-layout rather
	// than one the box-equality guard skips.
	fake.answer("display-message", "130 40", nil)
	signalPath := e.resizeSignalPath()
	if err := os.WriteFile(signalPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile signal: %v", err)
	}
	if !eventually(t, 300*time.Millisecond, func() bool { return fake.Count("list-panes") > stable }) {
		t.Errorf("list-panes calls = %d, want more than %d after the signal file appeared", fake.Count("list-panes"), stable)
	}
	if !eventually(t, 300*time.Millisecond, func() bool { return fake.Count("select-layout") > baseline }) {
		t.Fatalf("select-layout calls = %d, want more than %d after the signal-triggered apply", fake.Count("select-layout"), baseline)
	}
	if _, err := os.Stat(signalPath); !os.IsNotExist(err) {
		t.Errorf("signal file still present once select-layout was observed, want it removed before the apply")
	}
	if got := fake.Count("select-layout"); got != baseline+1 {
		t.Errorf("select-layout calls = %d, want exactly %d for one signal", got, baseline+1)
	}
	time.Sleep(30 * time.Millisecond)
	if got := fake.Count("show-options"); got != probesAtPromotion {
		t.Errorf("show-options calls = %d after clearing the hook, want unchanged from %d (signal mode never re-probes)", got, probesAtPromotion)
	}
}

// TestWatchLoop_UndecidedProbeDoesNotGuess pins that with reed.lock held for the first few cycles
// so every call defers, the mode stays poll and no promotion occurs; releasing the lock and then
// reporting the hook promotes as normal.
//
//testtiming:keep pins the mode staying poll with no tmux call while reed.lock is held and every tick defers, then promoting as normal once the lock is released and the hook reported; its covering tests run this code without asserting it
func TestWatchLoop_UndecidedProbeDoesNotGuess(t *testing.T) {
	e, fake := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	fake.answer("show-options", ownCommand, nil)

	dotLyx := e.stateDir()
	if err := os.MkdirAll(dotLyx, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lockPath := filepath.Join(dotLyx, reedLockFileName)
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock: %v", err)
	}

	startWatchLoop(t, e, watchdogTestTiming())

	time.Sleep(30 * time.Millisecond)
	if got := len(fake.Calls()); got != 0 {
		t.Errorf("tmux recorded %d calls while reed.lock was held, want zero (every deferred tick issues no tmux call)", got)
	}

	if err := held.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if !eventually(t, 300*time.Millisecond, func() bool { return fake.Count("show-options") > 0 }) {
		t.Errorf("no show-options probe observed after releasing reed.lock")
	}
	waitForPromotion(t, fake)
}

// TestWatchLoop_TakeEffectBoundary pins that rewriting e.cfg.Watchdog on disk-equivalent state
// (flipped directly in the fixture, standing in for a reed.yaml edit) changes nothing while the
// loop runs: the loop reads e.cfg.Watchdog exactly once, at start.
//
//testtiming:keep pins the loop reading e.cfg.Watchdog once at start, so flipping it mid-run neither stops the loop nor changes its cadence; its covering tests run this code without asserting it
func TestWatchLoop_TakeEffectBoundary(t *testing.T) {
	e, fake := newWatchLoopTestEngine(t, "on")
	fake.answer("show-options", "", nil)

	_, done := startWatchLoop(t, e, watchdogTestTiming())

	if !eventually(t, 200*time.Millisecond, func() bool { return fake.Count("list-panes") >= 2 }) {
		t.Fatalf("list-panes calls = %d, want at least 2 poll cycles before flipping the config", fake.Count("list-panes"))
	}
	before := fake.Count("list-panes")

	e.cfg.Watchdog = "off"

	select {
	case err := <-done:
		t.Fatalf("watchLoop returned %v after flipping cfg.Watchdog mid-run, want it to keep running", err)
	case <-time.After(20 * time.Millisecond):
	}
	if !eventually(t, 200*time.Millisecond, func() bool { return fake.Count("list-panes") > before }) {
		t.Errorf("list-panes calls = %d, want continued poll cycles after flipping cfg.Watchdog mid-run (%d before)", fake.Count("list-panes"), before)
	}
}

// TestWatchLoop_FailuresNeverKillTheLoop pins that with select-layout scripted to fail every time,
// the loop is still running and still responsive after an exhausted streak: a fresh signal file
// still produces a fresh select-layout attempt.
func TestWatchLoop_FailuresNeverKillTheLoop(t *testing.T) {
	e, fake := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	fake.answer("show-options", ownCommand, nil)

	timing := watchdogTestTiming()
	_, done := startWatchLoop(t, e, timing)
	waitForPromotion(t, fake)
	// The promotion tick's own first-ever apply already issued one (successful) select-layout;
	// baseline is what this failing streak's timing.MaxAttempts attempts must add on top of.
	baseline := fake.Count("select-layout")

	fake.answer("display-message", "140 50", nil)
	fake.answer("select-layout", "", errors.New("select-layout boom"))
	if err := os.WriteFile(e.resizeSignalPath(), nil, 0o644); err != nil {
		t.Fatalf("WriteFile signal: %v", err)
	}

	// timing.MaxAttempts failing attempts, each escalating by timing.BaseDelay<<(n-1), plus generous
	// scheduling slack.
	wait := timing.Quiet
	for i := 0; i < timing.MaxAttempts; i++ {
		wait += timing.BaseDelay << i
	}
	wait += 50 * time.Millisecond

	want := baseline + timing.MaxAttempts
	if !eventually(t, wait, func() bool { return fake.Count("select-layout") >= want }) {
		t.Fatalf("select-layout attempts = %d, want %d (the exhausted streak)", fake.Count("select-layout"), want)
	}
	exhausted := fake.Count("select-layout")

	select {
	case err := <-done:
		t.Fatalf("watchLoop returned %v after an exhausted retry streak, want it to keep running", err)
	case <-time.After(30 * time.Millisecond):
	}
	if got := fake.Count("select-layout"); got != exhausted {
		t.Errorf("select-layout attempts = %d after the streak exhausted, want unchanged at %d (no attempts beyond the cap)", got, exhausted)
	}

	// A fresh signal is a fresh event: it must still produce a fresh attempt, cap or no cap.
	fake.answer("select-layout", "", nil)
	if err := os.WriteFile(e.resizeSignalPath(), nil, 0o644); err != nil {
		t.Fatalf("WriteFile fresh signal: %v", err)
	}
	if !eventually(t, 200*time.Millisecond, func() bool { return fake.Count("select-layout") > exhausted }) {
		t.Errorf("select-layout attempts = %d, want more than %d after a fresh signal", fake.Count("select-layout"), exhausted)
	}
}

// TestWatchLoop_DeferralCostsNoBudget pins that with the lock held across the whole quiet period,
// the loop issues no tmux call and, once the lock is released, still applies for the same pending
// signal.
func TestWatchLoop_DeferralCostsNoBudget(t *testing.T) {
	e, fake := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	fake.answer("show-options", ownCommand, nil)

	timing := watchdogTestTiming()
	startWatchLoop(t, e, timing)
	waitForPromotion(t, fake)
	// The promotion tick's own first-ever apply already issued one select-layout; baseline is what
	// the once-unblocked deferred signal below must exceed.
	baseline := fake.Count("select-layout")

	dotLyx := e.stateDir()
	held, err := lock.AcquireWriteLock(filepath.Join(dotLyx, reedLockFileName))
	if err != nil {
		t.Fatalf("AcquireWriteLock: %v", err)
	}

	fake.answer("display-message", "160 60", nil)
	beforeLock := fake.Count("list-panes")
	if err := os.WriteFile(e.resizeSignalPath(), nil, 0o644); err != nil {
		t.Fatalf("WriteFile signal: %v", err)
	}

	// Hold the lock across (well beyond) the whole quiet period: every tick's try-lock fails, so no
	// tmux call of any kind can happen. Hardcoded to double watchdogTestTiming's fixed 5ms Quiet
	// rather than the timing.Quiet field itself, so the literal duration below stays visible to
	// cmd/lyx's tiersleep_test.go static check (a non-constant field expression reads as
	// unresolvable there and fails closed).
	time.Sleep(10 * time.Millisecond)
	if got := fake.Count("list-panes"); got != beforeLock {
		t.Errorf("list-panes calls = %d while reed.lock was held across the quiet period, want unchanged at %d", got, beforeLock)
	}

	if err := held.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if !eventually(t, 300*time.Millisecond, func() bool { return fake.Count("select-layout") > baseline }) {
		t.Errorf("no select-layout observed once reed.lock was released, want the deferred signal still owed")
	}
}

// --- Dormant-mode driver tests -----------------------------------------------
//
// These reuse the driver-test fixture above, plus logcapture's mutex-guarded buffer:
// watchLoop writes log lines from its own goroutine while the test goroutine
// reads the buffer, so an unguarded bytes.Buffer would race under -race.

// TestWatchLoop_PollModeGoesDormantOnVanishedWorktreeRoot pins that when the told worktree root
// vanishes while the loop is in poll mode, the loop stops issuing tmux calls, keeps running rather
// than returning, and logs exactly one warning.
//
//testtiming:keep pins the poll-mode loop stopping its tmux round trips when the told worktree root vanishes, staying alive and logging exactly one dormancy warning; its covering tests run this code without asserting it
func TestWatchLoop_PollModeGoesDormantOnVanishedWorktreeRoot(t *testing.T) {
	buf := logcapture.CaptureVerbose(t)
	e, fake := newWatchLoopTestEngine(t, "on")
	fake.answer("show-options", "", nil)

	_, done := startWatchLoop(t, e, watchdogTestTiming())

	if !eventually(t, 200*time.Millisecond, func() bool { return fake.Count("list-panes") >= 2 }) {
		t.Fatalf("list-panes calls = %d, want at least 2 poll cycles before the worktree root vanishes", fake.Count("list-panes"))
	}

	if err := os.RemoveAll(e.geom.WorktreeRoot); err != nil {
		t.Fatalf("RemoveAll worktree root: %v", err)
	}

	if !eventually(t, 300*time.Millisecond, func() bool {
		before := fake.Count("list-panes")
		time.Sleep(20 * time.Millisecond)
		return fake.Count("list-panes") == before
	}) {
		t.Fatalf("list-panes call count never stabilized, want dormancy to stop the per-cycle tmux round trip")
	}

	select {
	case err := <-done:
		t.Fatalf("watchLoop returned %v after the worktree root vanished, want it to keep running", err)
	case <-time.After(20 * time.Millisecond):
	}

	if got := strings.Count(buf.String(), "dropping the resize watcher into dormant mode"); got != 1 {
		t.Errorf("dormancy warning lines = %d, want exactly 1; log:\n%s", got, buf.String())
	}
}

// TestWatchLoop_RecoversFromDormancyToItsPriorMode pins that once the told worktree root exists
// again, a dormant watcher logs exactly one recovery line and resumes at the mode it was in before
// dormancy — signal mode here, so the regression guard is that it does not come back demoted to
// poll.
func TestWatchLoop_RecoversFromDormancyToItsPriorMode(t *testing.T) {
	buf := logcapture.CaptureVerbose(t)
	e, fake := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	fake.answer("show-options", ownCommand, nil)

	timing := watchdogTestTiming()
	timing.Quiet = 30 * time.Millisecond

	_, done := startWatchLoop(t, e, timing)
	waitForPromotion(t, fake)

	signalPath := e.resizeSignalPath()
	if err := os.WriteFile(signalPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile signal: %v", err)
	}
	if !eventually(t, 200*time.Millisecond, func() bool {
		_, err := os.Stat(signalPath)
		return os.IsNotExist(err)
	}) {
		t.Fatalf("signal file was not consumed before the quiet period elapsed")
	}

	worktreeRoot := e.geom.WorktreeRoot
	if err := os.RemoveAll(worktreeRoot); err != nil {
		t.Fatalf("RemoveAll worktree root: %v", err)
	}
	if !eventually(t, 300*time.Millisecond, func() bool {
		return strings.Contains(buf.String(), "told worktree root is gone")
	}) {
		t.Fatalf("no dormancy warning observed after the worktree root vanished; log:\n%s", buf.String())
	}

	select {
	case err := <-done:
		t.Fatalf("watchLoop returned %v after the worktree root vanished, want it to keep running", err)
	case <-time.After(20 * time.Millisecond):
	}
	if err := os.MkdirAll(worktreeRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll (recreate) worktree root: %v", err)
	}

	if !eventually(t, 500*time.Millisecond, func() bool {
		return strings.Contains(buf.String(), "told worktree root is back")
	}) {
		t.Fatalf("no recovery line observed after recreating the worktree root; log:\n%s", buf.String())
	}

	if got := strings.Count(buf.String(), "told worktree root is back"); got != 1 {
		t.Errorf("recovery info lines = %d, want exactly 1; log:\n%s", got, buf.String())
	}
	if got := strings.Count(buf.String(), "dropping the resize watcher into dormant mode"); got != 1 {
		t.Errorf("dormancy warning lines after recovery = %d, want unchanged at exactly 1; log:\n%s", got, buf.String())
	}

	// Signal mode's signature, distinguishing it from poll mode: with no fresh signal file, the
	// list-panes count stabilizes rather than continuing to grow tick after tick.
	waitForPromotion(t, fake)
}

// TestWatchLoop_NonSentinelFailureDoesNotGoDormant pins the narrowing itself: a re-apply failure
// that is NOT errWorktreeRootGone must not drop the loop into dormancy, so the loop keeps
// re-applying at its existing (poll) cadence exactly as it does today.
//
//testtiming:keep pins a re-apply failure other than errWorktreeRootGone leaving the loop at its poll cadence with no dormancy warning; its covering tests run this code without asserting it
func TestWatchLoop_NonSentinelFailureDoesNotGoDormant(t *testing.T) {
	buf := logcapture.CaptureVerbose(t)
	e, fake := newWatchLoopTestEngine(t, "on")
	fake.answer("show-options", "", nil)
	fake.answer("select-layout", "", errors.New("select-layout boom"))

	startWatchLoop(t, e, watchdogTestTiming())

	if !eventually(t, 300*time.Millisecond, func() bool { return fake.Count("list-panes") >= 5 }) {
		t.Fatalf("list-panes calls = %d, want continued poll-cadence reapply attempts despite the non-sentinel failure", fake.Count("list-panes"))
	}
	if strings.Contains(buf.String(), "told worktree root is gone") {
		t.Errorf("dormancy warning logged for a non-sentinel failure, want only the sentinel to trigger dormancy:\n%s", buf.String())
	}
}
