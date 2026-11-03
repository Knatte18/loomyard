// export_test.go exposes the add verb's pair-push seam, the open-PR lookup seam, the hub pull runner and the hibernate request reader to this package's external tests.

package fabriccli

import (
	"context"
	"io"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// SetOpenPRHeadsForTest replaces the open-PR lookup with lookup for the test's lifetime.
func SetOpenPRHeadsForTest(t *testing.T, lookup func(ctx context.Context, remoteURL string) (map[string]bool, error)) {
	t.Helper()
	prev := listOpenPRHeads
	listOpenPRHeads = lookup
	t.Cleanup(func() { listOpenPRHeads = prev })
}

// SetPairPushForTest makes "lym fabric add" push a new pair through push instead of a detached child, for the whole test binary.
// It returns the previous seam so a test can restore it.
func SetPairPushForTest(push func(*lymcwd.Location) error) (previous func(*lymcwd.Location) error) {
	previous = spawnPairPush
	spawnPairPush = push
	return previous
}

// HibernateDaemonLiveForTest reads the hibernate request of hub the way the anchor verb's refusal does.
func HibernateDaemonLiveForTest(hub string) (bool, error) {
	return hibernateDaemonLive(hub)
}

// HibernateEndedPhasesForTest returns a copy of the request phases the anchor verb reads as ended.
func HibernateEndedPhasesForTest() []string {
	return slices.Clone(hibernateEndedPhases)
}

// RunHubPullForTest runs the hub pull of the acting location l under ctx, so a test can cancel a wait on a parent lock.
func RunHubPullForTest(ctx context.Context, out io.Writer, l *lymcwd.Location) int {
	return runHubPull(ctx, out, l)
}
