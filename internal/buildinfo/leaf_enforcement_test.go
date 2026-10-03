// leaf_enforcement_test.go enforces the Buildinfo Leaf Invariant: production code in
// internal/buildinfo imports nothing at all -- not even the standard library.
// Like tokenvocab's leaf_enforcement_test.go, this check is an ALLOWLIST: any import found fails
// the test, so a future stray dependency is caught with no list maintenance required.

package buildinfo

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports is empty: no non-stdlib import is permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/buildinfo", allowedImports...)
}
