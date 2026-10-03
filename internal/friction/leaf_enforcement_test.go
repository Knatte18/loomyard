// leaf_enforcement_test.go enforces the Friction Leaf Invariant: production code in
// internal/friction imports ONLY the standard library, internal/logger, internal/stencil, and
// internal/stencilstore — never a feature package (websterengine, burlerengine, loomengine, or any
// other).
// Modelled directly on internal/pattern/leaf_enforcement_test.go, this check is an ALLOWLIST: any
// import outside the allowed set fails the test, so a future stray dependency is caught with no list
// maintenance required.

package friction

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/friction", allowedImports...)
}
