// leaf_enforcement_test.go enforces the Buildinfo Leaf Invariant:
// production code in internal/buildinfo imports nothing at all, not even the standard library.
// The check is an allowlist: any import found fails the test,
// so a future stray dependency is caught with no list to maintain.

package buildinfo

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports is empty: no non-stdlib import is permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/buildinfo", allowedImports...)
}
