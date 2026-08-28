// watchloop.go owns the resize watch loop: its pure decision state in the first half, and
// Engine.Watch plus its driver in the second half.

package reedengine

import (
	"time"
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
