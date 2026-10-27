// export_test.go exposes the add verb's pair-push seam and the open-PR lookup seam to this package's external tests.

package fabriccli

import (
	"context"
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
