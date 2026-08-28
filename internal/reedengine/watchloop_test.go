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
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
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
	}
}

// driverHook is a thread-safe scripted TmuxCmd.execHook for watchLoop driver tests: watchLoop runs
// in its own goroutine while the test goroutine both reads the recorded argv and rewrites the
// scripted answers mid-run (e.g. to simulate a resize or a hook install appearing).
type driverHook struct {
	mu    sync.Mutex
	calls [][]string

	live []LivePane

	boxAnswer string
	boxErr    error

	hookAnswer string
	hookErr    error

	selectLayoutErr error
}

// newDriverHook builds a driverHook over live, with box "100 21" (matching newTestEngine's default
// cfg.Width/Height) and no hook installed.
func newDriverHook(live []LivePane) *driverHook {
	return &driverHook{live: live, boxAnswer: "100 21"}
}

// exec is the TmuxCmd.execHook function itself.
func (h *driverHook) exec(capture bool, args ...string) (string, error) {
	h.mu.Lock()
	h.calls = append(h.calls, append([]string{}, args...))
	live := h.live
	boxAnswer, boxErr := h.boxAnswer, h.boxErr
	hookAnswer, hookErr := h.hookAnswer, h.hookErr
	selectLayoutErr := h.selectLayoutErr
	h.mu.Unlock()

	switch args[0] {
	case "has-session":
		return "", nil
	case "list-panes":
		return encodeLivePanes(live), nil
	case "display-message":
		// The generation probe (generation.go) and the window-size query (windowsize.go) both go
		// through display-message; disambiguate by the trailing format string exactly as
		// reapply_test.go's scriptedHook does.
		if args[len(args)-1] == paneGenerationFormat {
			return "$0|1|1000", nil
		}
		return boxAnswer, boxErr
	case "show-options":
		return hookAnswer, hookErr
	case "select-layout":
		if selectLayoutErr != nil {
			return "", selectLayoutErr
		}
		return "", nil
	default:
		return "", nil
	}
}

// setHook rewrites the show-options answer the next tick observes.
func (h *driverHook) setHook(answer string, err error) {
	h.mu.Lock()
	h.hookAnswer, h.hookErr = answer, err
	h.mu.Unlock()
}

// setBox rewrites the live-window-size answer the next tick observes.
func (h *driverHook) setBox(answer string, err error) {
	h.mu.Lock()
	h.boxAnswer, h.boxErr = answer, err
	h.mu.Unlock()
}

// setSelectLayoutErr rewrites the error select-layout reports on every future call.
func (h *driverHook) setSelectLayoutErr(err error) {
	h.mu.Lock()
	h.selectLayoutErr = err
	h.mu.Unlock()
}

// snapshot returns a defensive copy of every call recorded so far.
func (h *driverHook) snapshot() [][]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([][]string, len(h.calls))
	copy(out, h.calls)
	return out
}

// count reports how many recorded calls invoke subcommand.
func (h *driverHook) count(subcommand string) int {
	n := 0
	for _, c := range h.snapshot() {
		if len(c) > 0 && c[0] == subcommand {
			n++
		}
	}
	return n
}

// has reports whether any recorded call invokes subcommand.
func (h *driverHook) has(subcommand string) bool {
	return h.count(subcommand) > 0
}

// newWatchLoopTestEngine builds an Engine and a persisted ReedState the way reapply_test.go's
// newReapplyTestEngine does — one strand bound to "%1", live panes "%1" and "%2" — wired to a
// driverHook instead of reapply_test.go's non-thread-safe scriptedHook, and with cfg.Watchdog set
// to watchdog.
func newWatchLoopTestEngine(t *testing.T, watchdog string) (*Engine, *driverHook) {
	t.Helper()
	e := newTestEngine(t)
	e.cfg.Width, e.cfg.Height = 100, 21
	e.cfg.Watchdog = watchdog
	st := &ReedState{
		Strands: []Strand{
			{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
		},
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	hook := newDriverHook([]LivePane{{ID: "%1"}, {ID: "%2"}})
	e.tmux.execHook = hook.exec
	return e, hook
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

// TestWatchLoop_DisabledNeverReturnsWhileCtxLive pins that with Watchdog: "off", watchLoop issues
// no tmux call, does not return within a bounded wait, and returns only after ctx is cancelled.
func TestWatchLoop_DisabledNeverReturnsWhileCtxLive(t *testing.T) {
	e, hook := newWatchLoopTestEngine(t, "off")
	cancel, done := startWatchLoop(t, e, watchdogTestTiming())

	select {
	case err := <-done:
		t.Fatalf("watchLoop returned %v before cancellation, want it parked", err)
	case <-time.After(30 * time.Millisecond):
	}
	if len(hook.snapshot()) != 0 {
		t.Errorf("hook recorded calls %v, want zero tmux calls while disabled", hook.snapshot())
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
}

// TestWatchLoop_InvalidValueNeverReturnsWhileCtxLive pins the identical contract for an invalid
// Watchdog value: the header tail's contract is that a config typo parks the loop rather than
// killing the keepalive, so this must not return an error and must not return at all until
// cancellation.
func TestWatchLoop_InvalidValueNeverReturnsWhileCtxLive(t *testing.T) {
	e, hook := newWatchLoopTestEngine(t, "garbage")
	cancel, done := startWatchLoop(t, e, watchdogTestTiming())

	select {
	case err := <-done:
		t.Fatalf("watchLoop returned %v before cancellation, want it parked", err)
	case <-time.After(30 * time.Millisecond):
	}
	if len(hook.snapshot()) != 0 {
		t.Errorf("hook recorded calls %v, want zero tmux calls on an invalid value", hook.snapshot())
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
}

// TestWatchLoop_StaleSignalFileRemovedAtStart pins that a signal file present before the call is
// gone shortly after the loop starts.
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
func TestWatchLoop_PollModeByDefault(t *testing.T) {
	e, hook := newWatchLoopTestEngine(t, "on")
	hook.setHook("", nil)

	startWatchLoop(t, e, watchdogTestTiming())

	if !eventually(t, 300*time.Millisecond, func() bool { return hook.count("list-panes") >= 3 }) {
		t.Fatalf("list-panes calls = %d, want at least 3 poll cycles", hook.count("list-panes"))
	}
	if hook.count("show-options") == 0 {
		t.Errorf("show-options calls = 0, want poll mode to probe every cycle")
	}
}

// waitForPromotion runs the loop already promoted to signal mode against hook's current
// hookAnswer (which must already report reed's own command), by waiting for the list-panes call
// count to stop growing across two consecutive observation windows — the observable proxy for "no
// more per-cycle reapplyLayout calls", since promotion is otherwise an internal mode flag.
func waitForPromotion(t *testing.T, hook *driverHook) int {
	t.Helper()
	if !eventually(t, 200*time.Millisecond, func() bool { return hook.count("list-panes") >= 1 }) {
		t.Fatalf("list-panes calls = %d, want at least 1 (the promoting call)", hook.count("list-panes"))
	}
	var stableCount int
	if !eventually(t, 300*time.Millisecond, func() bool {
		before := hook.count("list-panes")
		time.Sleep(20 * time.Millisecond)
		after := hook.count("list-panes")
		stableCount = after
		return before == after
	}) {
		t.Fatalf("list-panes call count never stabilized, want promotion to stop per-cycle polling")
	}
	return stableCount
}

// TestWatchLoop_ModePromotion pins that with show-options scripted to return reed's own command
// string, the loop promotes: after promotion it stops issuing per-cycle reapplyLayout calls, and it
// applies only after a signal file appears.
func TestWatchLoop_ModePromotion(t *testing.T) {
	e, hook := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	hook.setHook(ownCommand, nil)

	startWatchLoop(t, e, watchdogTestTiming())

	stable := waitForPromotion(t, hook)
	// The promotion tick's own first-ever apply (lastApplied starts as the zero box, which never
	// equals a live box) already issued one select-layout; the baseline below is what the
	// signal-triggered apply below must exceed.
	baseline := hook.count("select-layout")

	// Change the box so the coming signal-triggered apply is a real, observable select-layout rather
	// than one the box-equality guard skips.
	hook.setBox("120 30", nil)
	if err := os.WriteFile(e.resizeSignalPath(), nil, 0o644); err != nil {
		t.Fatalf("WriteFile signal: %v", err)
	}

	if !eventually(t, 300*time.Millisecond, func() bool { return hook.count("list-panes") > stable }) {
		t.Errorf("list-panes calls = %d, want more than %d after the signal file appeared", hook.count("list-panes"), stable)
	}
	if !eventually(t, 100*time.Millisecond, func() bool { return hook.count("select-layout") > baseline }) {
		t.Errorf("select-layout calls = %d, want more than %d after the signal-triggered apply", hook.count("select-layout"), baseline)
	}
}

// TestWatchLoop_NeverDemotes pins that after a promotion, scripting show-options to return the
// empty string produces no further probe round trips at all — signal mode never re-probes.
func TestWatchLoop_NeverDemotes(t *testing.T) {
	e, hook := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	hook.setHook(ownCommand, nil)

	startWatchLoop(t, e, watchdogTestTiming())
	waitForPromotion(t, hook)

	probesAtPromotion := hook.count("show-options")
	hook.setHook("", nil)

	// Give the loop many more signal ticks than it took to promote; a demoting implementation would
	// re-probe and see the hook gone.
	time.Sleep(50 * time.Millisecond)
	if got := hook.count("show-options"); got != probesAtPromotion {
		t.Errorf("show-options calls = %d after clearing the hook, want unchanged from %d (signal mode never re-probes)", got, probesAtPromotion)
	}
}

// TestWatchLoop_UndecidedProbeDoesNotGuess pins that with reed.lock held for the first few cycles
// so every call defers, the mode stays poll and no promotion occurs; releasing the lock and then
// reporting the hook promotes as normal.
func TestWatchLoop_UndecidedProbeDoesNotGuess(t *testing.T) {
	e, hook := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	hook.setHook(ownCommand, nil)

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
	if got := len(hook.snapshot()); got != 0 {
		t.Errorf("hook recorded %d calls while reed.lock was held, want zero (every deferred tick issues no tmux call)", got)
	}

	if err := held.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if !eventually(t, 300*time.Millisecond, func() bool { return hook.has("show-options") }) {
		t.Errorf("no show-options probe observed after releasing reed.lock")
	}
	waitForPromotion(t, hook)
}

// TestWatchLoop_SignalConsumedByRemovalBeforeTheApply pins that in signal mode, creating the signal
// file causes exactly one select-layout after the quiet period, and the file is gone before that
// select-layout appears in the recorded argv.
func TestWatchLoop_SignalConsumedByRemovalBeforeTheApply(t *testing.T) {
	e, hook := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	hook.setHook(ownCommand, nil)

	startWatchLoop(t, e, watchdogTestTiming())
	waitForPromotion(t, hook)
	// The promotion tick's own first-ever apply already issued one select-layout (lastApplied starts
	// as the zero box); baseline is what this test's one signal must add exactly one to.
	baseline := hook.count("select-layout")

	// A differing box so the apply this signal triggers is a real, observable select-layout.
	hook.setBox("130 40", nil)
	signalPath := e.resizeSignalPath()
	if err := os.WriteFile(signalPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile signal: %v", err)
	}

	if !eventually(t, 300*time.Millisecond, func() bool { return hook.count("select-layout") > baseline }) {
		t.Fatalf("no select-layout observed after the signal file appeared")
	}
	if _, err := os.Stat(signalPath); !os.IsNotExist(err) {
		t.Errorf("signal file still present once select-layout was observed, want it removed before the apply")
	}
	if got := hook.count("select-layout"); got != baseline+1 {
		t.Errorf("select-layout calls = %d, want exactly %d for one signal", got, baseline+1)
	}
}

// TestWatchLoop_TakeEffectBoundary pins that rewriting e.cfg.Watchdog on disk-equivalent state
// (flipped directly in the fixture, standing in for a reed.yaml edit) changes nothing while the
// loop runs: the loop reads e.cfg.Watchdog exactly once, at start.
func TestWatchLoop_TakeEffectBoundary(t *testing.T) {
	e, hook := newWatchLoopTestEngine(t, "on")
	hook.setHook("", nil)

	_, done := startWatchLoop(t, e, watchdogTestTiming())

	if !eventually(t, 200*time.Millisecond, func() bool { return hook.count("list-panes") >= 2 }) {
		t.Fatalf("list-panes calls = %d, want at least 2 poll cycles before flipping the config", hook.count("list-panes"))
	}
	before := hook.count("list-panes")

	e.cfg.Watchdog = "off"

	select {
	case err := <-done:
		t.Fatalf("watchLoop returned %v after flipping cfg.Watchdog mid-run, want it to keep running", err)
	case <-time.After(20 * time.Millisecond):
	}
	if !eventually(t, 200*time.Millisecond, func() bool { return hook.count("list-panes") > before }) {
		t.Errorf("list-panes calls = %d, want continued poll cycles after flipping cfg.Watchdog mid-run (%d before)", hook.count("list-panes"), before)
	}
}

// TestWatchLoop_FailuresNeverKillTheLoop pins that with select-layout scripted to fail every time,
// the loop is still running and still responsive after an exhausted streak: a fresh signal file
// still produces a fresh select-layout attempt.
func TestWatchLoop_FailuresNeverKillTheLoop(t *testing.T) {
	e, hook := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	hook.setHook(ownCommand, nil)

	timing := watchdogTestTiming()
	_, done := startWatchLoop(t, e, timing)
	waitForPromotion(t, hook)
	// The promotion tick's own first-ever apply already issued one (successful) select-layout;
	// baseline is what this failing streak's timing.MaxAttempts attempts must add on top of.
	baseline := hook.count("select-layout")

	hook.setBox("140 50", nil)
	hook.setSelectLayoutErr(errors.New("select-layout boom"))
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
	if !eventually(t, wait, func() bool { return hook.count("select-layout") >= want }) {
		t.Fatalf("select-layout attempts = %d, want %d (the exhausted streak)", hook.count("select-layout"), want)
	}
	exhausted := hook.count("select-layout")

	select {
	case err := <-done:
		t.Fatalf("watchLoop returned %v after an exhausted retry streak, want it to keep running", err)
	case <-time.After(30 * time.Millisecond):
	}
	if got := hook.count("select-layout"); got != exhausted {
		t.Errorf("select-layout attempts = %d after the streak exhausted, want unchanged at %d (no attempts beyond the cap)", got, exhausted)
	}

	// A fresh signal is a fresh event: it must still produce a fresh attempt, cap or no cap.
	hook.setSelectLayoutErr(nil)
	if err := os.WriteFile(e.resizeSignalPath(), nil, 0o644); err != nil {
		t.Fatalf("WriteFile fresh signal: %v", err)
	}
	if !eventually(t, 200*time.Millisecond, func() bool { return hook.count("select-layout") > exhausted }) {
		t.Errorf("select-layout attempts = %d, want more than %d after a fresh signal", hook.count("select-layout"), exhausted)
	}
}

// TestWatchLoop_DeferralCostsNoBudget pins that with the lock held across the whole quiet period,
// the loop issues no tmux call and, once the lock is released, still applies for the same pending
// signal.
func TestWatchLoop_DeferralCostsNoBudget(t *testing.T) {
	e, hook := newWatchLoopTestEngine(t, "on")
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	hook.setHook(ownCommand, nil)

	timing := watchdogTestTiming()
	startWatchLoop(t, e, timing)
	waitForPromotion(t, hook)
	// The promotion tick's own first-ever apply already issued one select-layout; baseline is what
	// the once-unblocked deferred signal below must exceed.
	baseline := hook.count("select-layout")

	dotLyx := e.stateDir()
	held, err := lock.AcquireWriteLock(filepath.Join(dotLyx, reedLockFileName))
	if err != nil {
		t.Fatalf("AcquireWriteLock: %v", err)
	}

	hook.setBox("160 60", nil)
	beforeLock := hook.count("list-panes")
	if err := os.WriteFile(e.resizeSignalPath(), nil, 0o644); err != nil {
		t.Fatalf("WriteFile signal: %v", err)
	}

	// Hold the lock across (well beyond) the whole quiet period: every tick's try-lock fails, so no
	// tmux call of any kind can happen.
	time.Sleep(2 * timing.Quiet)
	if got := hook.count("list-panes"); got != beforeLock {
		t.Errorf("list-panes calls = %d while reed.lock was held across the quiet period, want unchanged at %d", got, beforeLock)
	}

	if err := held.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if !eventually(t, 300*time.Millisecond, func() bool { return hook.count("select-layout") > baseline }) {
		t.Errorf("no select-layout observed once reed.lock was released, want the deferred signal still owed")
	}
}
