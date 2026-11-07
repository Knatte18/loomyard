// leaf_enforcement_test.go enforces the Standalonestate Leaf Invariant:
// production code in internal/standalonestate imports only the standard library.
// It is an allowlist: any import outside the allowed set fails the test, so a stray dependency is caught.

package standalonestate

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports is empty: no non-stdlib import is permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/standalonestate", allowedImports...)
}
