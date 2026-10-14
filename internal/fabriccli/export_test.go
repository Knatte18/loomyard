// export_test.go exposes the add verb's pair-push seam to this package's external tests.

package fabriccli

import "github.com/Knatte18/loomyard/internal/lyxcwd"

// SetPairPushForTest makes "lyx fabric add" push a new pair through push instead of a detached child, for the whole test binary.
// It returns the previous seam so a test can restore it.
func SetPairPushForTest(push func(*lyxcwd.Location) error) (previous func(*lyxcwd.Location) error) {
	previous = spawnPairPush
	spawnPairPush = push
	return previous
}
