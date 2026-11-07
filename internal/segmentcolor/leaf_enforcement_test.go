// leaf_enforcement_test.go enforces the Segmentcolor Leaf Invariant.
// Production code in internal/segmentcolor imports only the standard library.

package segmentcolor

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports is empty: no non-stdlib import is permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/segmentcolor", allowedImports...)
}
