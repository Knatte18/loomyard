// leaf_enforcement_test.go enforces the stdlib-only half of the Summaryparser Sole-Parser Invariant.
// Production code in internal/summaryparser imports only the standard library.
// It never imports internal/lymcwd or any feature package.
// The check is an allowlist, empty here, so a future stray dependency is caught with no list to maintain.

package summaryparser

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports is empty: no non-stdlib import is permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/summaryparser", allowedImports...)
}
