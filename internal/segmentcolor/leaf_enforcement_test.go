// leaf_enforcement_test.go enforces the Segmentcolor Leaf Invariant.
// Production code in internal/segmentcolor imports only the standard library, no other package at all.

package segmentcolor

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports is empty: no non-stdlib import is permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/segmentcolor", allowedImports...)
}
