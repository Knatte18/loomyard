// export_test.go exposes the add verb's pair-push seam, the open-PR lookup seam and the hub pull runner to this package's external tests.

package fabriccli

import (
	"context"
	"io"
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

// RunHubPullForTest runs the hub pull of the acting location l under ctx, so a test can cancel a wait on a parent lock.
func RunHubPullForTest(ctx context.Context, out io.Writer, l *lymcwd.Location) int {
	return runHubPull(ctx, out, l)
}
