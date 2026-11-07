// export_test.go lets package gitexec_test shorten the remote deadline and the stall bound.
// It runs the stall check and the remote deadline on their own signal and starts the production stall timer and deadline.
// It injects a git version line and makes a git start fail.

package gitexec

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// defaultStallTimer keeps the production stall timer reachable while a test replaces it.
var defaultStallTimer = startStallTimer

// StartDefaultStallTimerForTest starts the production stall timer, with no packet trace, and returns its stop.
func StartDefaultStallTimerForTest(bound time.Duration, check func()) (stop func() bool) {
	return defaultStallTimer(bound, "", check)
}

// defaultArmDeadline keeps the production deadline context reachable while a test replaces it.
var defaultArmDeadline = armDeadline

// ArmDefaultDeadlineForTest arms the production deadline and returns its context and cancel.
func ArmDefaultDeadlineForTest(deadline time.Duration) (context.Context, context.CancelFunc) {
	return defaultArmDeadline(deadline)
}

// SetStallCheckForTest runs each armed stall check once ready reports true, not once the stall bound passes.
// The override lasts until the test ends.
// ready receives the attempt's number, counted from 1 across the test, and the path of the attempt's packet trace.
// It is polled until it reports true or the attempt ends.
// It returns a function that reports the bounds the checks were armed with so far, one per armed attempt.
// It changes process-global state, so the calling test must not be parallel.
func SetStallCheckForTest(t *testing.T, ready func(attempt int, tracePath string) bool) (armed func() []time.Duration) {
	t.Helper()

	var mu sync.Mutex
	var bounds []time.Duration
	previous := startStallTimer
	startStallTimer = func(bound time.Duration, tracePath string, check func()) func() bool {
		mu.Lock()
		bounds = append(bounds, bound)
		attempt := len(bounds)
		mu.Unlock()

		stopped := make(chan struct{})
		go func() {
			tick := time.NewTicker(5 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-stopped:
					return
				case <-tick.C:
					if ready(attempt, tracePath) {
						check()
						return
					}
				}
			}
		}()
		var once sync.Once
		return func() bool {
			once.Do(func() { close(stopped) })
			return true
		}
	}
	t.Cleanup(func() { startStallTimer = previous })
	return func() []time.Duration {
		mu.Lock()
		defer mu.Unlock()
		return append([]time.Duration(nil), bounds...)
	}
}

// SetDeadlineForTest ends each remote attempt once ready reports true, not once its deadline passes.
// The override lasts until the test ends.
// The error the attempt reports still carries the deadline it was given, as if that deadline had passed.
// ready is polled until it reports true or the attempt ends.
// It returns a function that reports the deadlines armed so far, one per remote attempt; a local command arms none.
// It changes process-global state, so the calling test must not be parallel.
func SetDeadlineForTest(t *testing.T, ready func() bool) (armed func() []time.Duration) {
	t.Helper()

	var mu sync.Mutex
	var deadlines []time.Duration
	previous := armDeadline
	armDeadline = func(deadline time.Duration) (context.Context, context.CancelFunc) {
		mu.Lock()
		deadlines = append(deadlines, deadline)
		mu.Unlock()

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			tick := time.NewTicker(5 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					if ready() {
						cancel()
						return
					}
				}
			}
		}()
		return ctx, cancel
	}
	t.Cleanup(func() { armDeadline = previous })
	return func() []time.Duration {
		mu.Lock()
		defer mu.Unlock()
		return append([]time.Duration(nil), deadlines...)
	}
}

// SetVersionProbeForTest replaces the git version probe and resets the floor cache until the test ends.
// It changes process-global state, so the calling test must not be parallel.
func SetVersionProbeForTest(t *testing.T, probe func() (string, error)) {
	t.Helper()

	previousProbe := versionProbe
	versionProbe = probe
	gitFloor = floorCache{}
	t.Cleanup(func() {
		versionProbe = previousProbe
		gitFloor = floorCache{}
	})
}

// SetRemoteDeadlineForTest sets remoteDeadline to d until the test ends.
// It changes process-global state, so the calling test must not be parallel.
func SetRemoteDeadlineForTest(t *testing.T, d time.Duration) {
	t.Helper()

	previous := remoteDeadline
	remoteDeadline = d
	t.Cleanup(func() { remoteDeadline = previous })
}

// SetStallBoundForTest sets stallBound to d until the test ends.
// It changes process-global state, so the calling test must not be parallel.
func SetStallBoundForTest(t *testing.T, d time.Duration) {
	t.Helper()

	previous := stallBound
	stallBound = d
	t.Cleanup(func() { stallBound = previous })
}

// SetStartFailuresForTest makes the next len(failures) git starts fail with failures in order, without starting git.
// It sets the wait before the first start retry to delay, until the test ends.
// The waits between start retries are recorded instead of slept.
// A nil entry starts git as usual.
// It returns a function that reports how many starts were made and one that reports the waits recorded so far.
// It changes process-global state, so the calling test must not be parallel.
func SetStartFailuresForTest(t *testing.T, delay time.Duration, failures ...error) (starts func() int, waits func() []time.Duration) {
	t.Helper()

	previousRun, previousDelay, previousSleep := runCommand, spawnAgainFirstDelay, sleepBeforeRetry
	startCount := 0
	var recorded []time.Duration
	runCommand = func(cmd *exec.Cmd) error {
		startCount++
		if startCount <= len(failures) && failures[startCount-1] != nil {
			return failures[startCount-1]
		}
		return previousRun(cmd)
	}
	spawnAgainFirstDelay = delay
	sleepBeforeRetry = func(wait time.Duration) { recorded = append(recorded, wait) }
	t.Cleanup(func() { runCommand, spawnAgainFirstDelay, sleepBeforeRetry = previousRun, previousDelay, previousSleep })
	return func() int { return startCount }, func() []time.Duration { return recorded }
}
