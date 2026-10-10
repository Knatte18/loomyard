// leaf_enforcement_test.go enforces the Fswatch Leaf Invariant.
// Production code in internal/fswatch imports only the standard library, fsnotify and internal/logger.

package fswatch

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/fsnotify/fsnotify",
	"github.com/Knatte18/loomyard/internal/logger",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/fswatch", allowedImports...)
}
