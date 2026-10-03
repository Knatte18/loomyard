// leaf_enforcement_test.go enforces the Tokenvocab Leaf Invariant: production code in
// internal/tokenvocab imports ONLY the standard library and internal/stencil —
// never reed, loom, or any other feature package.
// Like modelspec's leaf_enforcement_test.go, this check is an ALLOWLIST: any import outside the
// allowed set fails the test, so a future stray dependency is caught with no list maintenance
// required.

package tokenvocab

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/stencil",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/tokenvocab", allowedImports...)
}
