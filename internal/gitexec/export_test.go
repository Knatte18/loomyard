// export_test.go lets package gitexec_test shorten the remote deadline and the stall bound, inject a git version line and make a git start fail.

package gitexec

import (
	"os/exec"
	"testing"
	"time"
)

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

// SetStartFailuresForTest makes the next len(failures) git starts fail with failures in order, without starting git, and sets the wait before the first start retry to delay, until the test ends.
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
