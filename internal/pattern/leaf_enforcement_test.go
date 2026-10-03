// leaf_enforcement_test.go enforces the Pattern Leaf Invariant: production code in internal/pattern
// imports ONLY the standard library, internal/lyxdirs, internal/stencilstore, and
// internal/stencil — never a feature package (websterengine, burlerengine, loomengine, or any
// other).
// Like modelspec's and tokenvocab's leaf_enforcement_test.go, this check is an ALLOWLIST: any
// import outside the allowed set fails the test, so a future stray dependency is caught with no
// list maintenance required.

package pattern

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/lyxdirs",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"github.com/Knatte18/loomyard/internal/stencil",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/pattern", allowedImports...)
}
