// watchloop.go owns the resize watch loop: its pure decision state in the first half, and
// Engine.Watch plus its driver in the second half.

package reedengine

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// watchPlan is the decision one signal-mode tick yields.
type watchPlan int

const (
	// watchPlanWait means this tick does nothing.
	watchPlanWait watchPlan = iota
	// watchPlanApply means a coalesced re-apply is owed now.
	watchPlanApply
)

// watchTiming carries the loop's tunables as data so a test can drive the loop
// in milliseconds while production wires the fixed package constants through
// watchDefaultTiming.
type watchTiming struct {
	SignalTick  time.Duration
	PollCycle   time.Duration
	Quiet       time.Duration
	BaseDelay   time.Duration
	MaxAttempts int
	Dormant     time.Duration
}

// watchDefaultTiming returns the loop's production timings, sourced from the package's fixed
// watchdog* constants and nothing else.
func watchDefaultTiming() watchTiming {
	return watchTiming{
		SignalTick:  watchdogSignalTick,
		PollCycle:   watchdogPollCycle,
		Quiet:       watchdogDebounceQuiet,
		BaseDelay:   watchdogRetryBaseDelay,
		MaxAttempts: watchdogMaxAttempts,
		Dormant:     watchdogDormantCycle,
	}
}

// watchState is the signal-mode loop's whole mutable decision state: the
// coalescing debounce window plus the current failure streak. It is pure —
// it holds no clock and performs no I/O; every method is told the time.
type watchState struct {
	quiet       time.Duration
	baseDelay   time.Duration
	maxAttempts int

	pending  bool
	readyAt  time.Time
	attempts int
}

// newWatchState builds a fresh watchState from t, with no pending signal and no failure streak.
func newWatchState(t watchTiming) *watchState {
	return &watchState{
		quiet:       t.Quiet,
		baseDelay:   t.BaseDelay,
		maxAttempts: t.MaxAttempts,
	}
}

// Signal records a resize signal observed at now: it sets pending, sets readyAt to now plus the
// quiet window, and resets attempts to zero.
// Restarting readyAt on every signal is what makes the debounce trailing-edge and coalescing: a
// 20-step drag firing 20 signals in one second yields exactly one apply, after the drag settles.
// Resetting attempts is required by the failure contract — the streak is per-event, and a fresh
// resize is a fresh event.
func (s *watchState) Signal(now time.Time) {
	s.pending = true
	s.readyAt = now.Add(s.quiet)
	s.attempts = 0
}

// Plan reports what now's tick should do. It mutates nothing, so a tick may call it freely.
func (s *watchState) Plan(now time.Time) watchPlan {
	if s.pending && !now.Before(s.readyAt) {
		return watchPlanApply
	}
	return watchPlanWait
}

// Succeeded clears the owed apply and the failure streak after a successful re-apply.
func (s *watchState) Succeeded() {
	s.pending = false
	s.attempts = 0
	s.readyAt = time.Time{}
}

// Failed records a failed re-apply attempt observed at now, and reports whether this event's
// retries are now abandoned.
// While attempts is below maxAttempts, it keeps pending set and pushes readyAt out by the
// escalating delay, reporting false. Once attempts reaches maxAttempts it abandons the event:
// it clears pending, zeroes attempts and readyAt, and reports true.
// Abandoning ends THIS EVENT's retries and never the watcher: the next resize signal is itself the
// next retry trigger.
func (s *watchState) Failed(now time.Time) (abandoned bool) {
	s.attempts++
	if s.attempts < s.maxAttempts {
		s.pending = true
		s.readyAt = now.Add(s.baseDelay << (s.attempts - 1))
		return false
	}
	s.pending = false
	s.attempts = 0
	s.readyAt = time.Time{}
	return true
}

// Deferred is a documented no-op: a tick whose try-lock was unavailable is a deferral, not an
// attempt and not a failure. pending, readyAt, and attempts are all left exactly as they were, so
// the watcher reconsiders on the next tick with its budget untouched.
// The method exists precisely so this rule is a named, testable contract rather than a missing
// else branch.
func (s *watchState) Deferred() {}

// watchMode names how the loop learns about a resize.
type watchMode int

const (
	// watchModePoll re-applies once per cycle and re-probes hook availability
	// each cycle. It is the safe default: it works whether or not the hook exists.
	watchModePoll watchMode = iota
	// watchModeSignal waits on the hook-written signal file and performs no
	// geometry polling at all.
	watchModeSignal
	// watchModeDormant is the mode a watcher enters when reapplyLayout reports the told worktree
	// root is provably gone: it neither polls geometry nor consumes signals, it re-tries at the
	// dormant cadence purely so it can notice the directory coming back, and it is the only mode
	// that remembers where it came from.
	watchModeDormant
)

// Watch runs reed's resize self-heal loop for this worktree's session.
//
// Watch never returns while ctx is live, including the disabled cases, where it parks internally
// rather than returning early. It never writes to stdout or stderr and never returns an error out
// of a failure inside the loop — a non-nil return is only ever ctx.Err()-shaped, meant for logging,
// never for display. It is the only exported symbol this feature adds to the engine: the re-apply
// op, the state machine, and every helper stay package-internal.
func (e *Engine) Watch(ctx context.Context) error {
	return e.watchLoop(ctx, watchDefaultTiming())
}

// tickerPeriodFor returns the ticker period the loop should run at while in mode.
func tickerPeriodFor(mode watchMode, t watchTiming) time.Duration {
	switch mode {
	case watchModeSignal:
		return t.SignalTick
	case watchModeDormant:
		return t.Dormant
	default:
		return t.PollCycle
	}
}

// watchLoop is Watch's driver. It reads e.cfg.Watchdog exactly once, at the top, and never again:
// flipping the key on disk changes nothing until the process restarts.
func (e *Engine) watchLoop(ctx context.Context, t watchTiming) error {
	enabled, err := watchdogOption(e.cfg.Watchdog)
	if err != nil {
		// This consumer has no error channel a caller could survive — returning here would let the
		// header pane's RunE fall through and kill the keepalive — so an invalid value is off, never
		// fatal.
		logger.Warn("reed: invalid watchdog value, treating watchdog as off", "socket", e.Socket(), "session", e.SessionName(), "value", e.cfg.Watchdog, "err", err)
		enabled = false
	}
	if !enabled {
		// Park; do not return early. A parked-but-live loop is what keeps ctx-liveness the caller's
		// only signal for "this session's watcher is done".
		logger.Info("reed: resize watchdog disabled for this session", "socket", e.Socket(), "session", e.SessionName())
		<-ctx.Done()
		return ctx.Err()
	}

	// A stale file is either a previous watcher's leftover or a resize that happened while none was
	// running; the session boot that starts this watcher applies the layout itself, so consuming it
	// would only buy a redundant apply. Removing it makes the loop's initial state deterministic.
	if err := os.Remove(e.resizeSignalPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("reed: failed to remove stale resize signal file", "socket", e.Socket(), "session", e.SessionName(), "err", err)
	}

	mode := watchModePoll
	// dormantFrom remembers which mode a dormant watcher should resume as once its told worktree
	// root comes back — the only mode dormancy needs to remember, since poll and signal mode never
	// need to recall a prior mode of their own.
	var dormantFrom watchMode
	state := newWatchState(t)
	// The zero render.Box is a deliberate "nothing applied yet" sentinel and needs no companion flag:
	// a live box always has positive W/H, so it can never equal the zero box, and the first re-apply
	// therefore always runs.
	var lastApplied render.Box

	ticker := time.NewTicker(tickerPeriodFor(mode, t))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var res ReapplyResult
			var applyErr error
			now := time.Now()

			switch mode {
			case watchModePoll:
				// Poll mode always asks for the probe: re-probing each cycle is what lets a watcher
				// that started on a hook-less already-up session promote itself once the operator's
				// next attach installs the hook, and it costs nothing extra in a mode that is already
				// making a round trip per cycle. Poll mode uses neither the debouncer nor the retry
				// streak — the cycle interval is its own cadence, and this is the fallback platform's
				// only self-heal, so a per-event cap that could stop it permanently must not apply
				// here.
				res, applyErr = e.reapplyLayout(lastApplied, true)
			case watchModeSignal:
				if _, statErr := os.Stat(e.resizeSignalPath()); statErr == nil {
					// Removing before the apply is what makes a resize arriving mid-apply re-signal
					// rather than be swallowed.
					if removeErr := os.Remove(e.resizeSignalPath()); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
						logger.Warn("reed: failed to remove resize signal file", "socket", e.Socket(), "session", e.SessionName(), "err", removeErr)
					}
					state.Signal(now)
				} else if !errors.Is(statErr, fs.ErrNotExist) {
					logger.Warn("reed: failed to stat resize signal file, treating as no signal", "socket", e.Socket(), "session", e.SessionName(), "err", statErr)
				}
				if state.Plan(now) != watchPlanApply {
					continue
				}
				// Signal mode passes false because it never re-probes: the probeHook argument is what
				// makes that rule literally true rather than merely suppressing the mode transition
				// while still paying the show-options round trip on every resize.
				res, applyErr = e.reapplyLayout(lastApplied, false)
			case watchModeDormant:
				// A dormant tick asks for no probe, and it does not use the debouncer or the retry
				// streak — dormancy is not a resize-event failure mode, it is a wait for the told
				// worktree root to come back.
				res, applyErr = e.reapplyLayout(lastApplied, false)
			}

			newMode := e.handleWatchOutcome(mode, state, t, res, applyErr, &lastApplied, &dormantFrom)
			if newMode != mode {
				mode = newMode
				ticker.Stop()
				ticker = time.NewTicker(tickerPeriodFor(mode, t))
			}
		}
	}
}

// handleWatchOutcome applies one re-apply outcome to state and lastApplied, identically for both
// modes, and reports the mode the loop should run at from now on.
//
// dormantFrom is the loop's own memory of which mode a dormant watcher should resume as; this
// function is the sole reader and sole writer of it, across both the sentinel branch (which
// writes it) and the recovery branch (which reads it).
func (e *Engine) handleWatchOutcome(mode watchMode, state *watchState, t watchTiming, res ReapplyResult, err error, lastApplied *render.Box, dormantFrom *watchMode) watchMode {
	if errors.Is(err, errWorktreeRootGone) {
		// A watcher not already dormant is entering dormancy for the first time this outage: log
		// once and remember the mode it should resume as. A watcher already dormant has nothing new
		// to say — dormancy logs nothing while dormant, or it would be back to a warning every
		// dormant tick, exactly the noise this mode exists to remove.
		if mode != watchModeDormant {
			logger.Warn("reed: told worktree root is gone, dropping the resize watcher into dormant mode",
				"socket", e.Socket(), "session", e.SessionName(), "err", err)
			*dormantFrom = mode
		}
		return watchModeDormant
	}
	if mode == watchModeDormant {
		// Any non-sentinel outcome means the stat succeeded and the worktree root is a directory
		// again — recovery. Whether the re-apply itself then failed for an unrelated reason is the
		// existing error path's business below, not dormancy's; that is why this falls through into
		// the rest of the function under the restored mode instead of returning early.
		logger.Info("reed: told worktree root is back, resuming the resize watcher",
			"socket", e.Socket(), "session", e.SessionName())
		mode = *dormantFrom
	}

	if err != nil {
		logger.Warn("reed: resize re-apply failed", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		if mode == watchModeSignal {
			if abandoned := state.Failed(time.Now()); abandoned {
				logger.Warn("reed: abandoning this resize event after max attempts, watcher remains running and responsive to the next signal",
					"socket", e.Socket(), "session", e.SessionName(), "attempts", t.MaxAttempts)
			}
		}
		// Do not update lastApplied, and do not change the mode.
		return mode
	}

	if res.Deferred {
		logger.Debug("reed: op lock held, deferring this tick", "socket", e.Socket(), "session", e.SessionName())
		if mode == watchModeSignal {
			state.Deferred()
		}
		// HookKnown is false on a deferral, so the mode is simply not decided this tick.
		return mode
	}

	// A real, non-deferred call.
	if mode == watchModeSignal {
		state.Succeeded()
	}
	if res.BoxIsLive {
		*lastApplied = res.Box
	}
	// A degraded query is not an observation, so a fallback box must cause neither a spurious
	// permanent skip nor a spurious re-apply loop — leave lastApplied exactly as it was.

	if res.HookKnown && res.HookInstalled && mode == watchModePoll {
		logger.Info("reed: promoting resize watchdog to signal mode", "socket", e.Socket(), "session", e.SessionName())
		if err := os.Remove(e.resizeSignalPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("reed: failed to remove resize signal file before promotion", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}
		*state = *newWatchState(t)
		return watchModeSignal
	}
	// Signal mode never demotes and never re-probes: watchdog: off unsets the hook while an
	// already-running signal-mode watcher keeps going until the next header-pane rebuild, and a
	// signal-mode watcher with no hook receives no signals and therefore does nothing, which is
	// exactly what the operator asked for.
	return mode
}
