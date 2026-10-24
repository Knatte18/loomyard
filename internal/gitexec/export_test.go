// export_test.go lets package gitexec_test shorten the remote deadline and the stall bound and inject a git version line.

package gitexec

import (
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
