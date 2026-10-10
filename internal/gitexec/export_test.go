// export_test.go lets package gitexec_test shorten the remote deadline.

package gitexec

import (
	"testing"
	"time"
)

// SetRemoteDeadlineForTest sets remoteDeadline to d until the test ends.
// It changes process-global state, so the calling test must not be parallel.
func SetRemoteDeadlineForTest(t *testing.T, d time.Duration) {
	t.Helper()

	previous := remoteDeadline
	remoteDeadline = d
	t.Cleanup(func() { remoteDeadline = previous })
}
