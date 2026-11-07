// leaf_enforcement_test.go enforces the Friction Leaf Invariant.
// Production code in internal/friction imports only the standard library and the allowed internal packages.
// The allowed internal packages are internal/logger, internal/stencil and internal/stencilstore.
// A feature package is never allowed.
// The check is an allowlist: an import outside the allowed set fails the test, and no denylist needs maintenance.

package friction

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/friction", allowedImports...)
}
