// leaf_enforcement_test.go enforces the Discussionparser Sole-Parser Invariant's stdlib-only half:
// production code in internal/discussionparser imports the standard library and nothing else --
// never internal/lyxcwd, never any feature package.
// Like internal/pattern's leaf_enforcement_test.go, this check is an ALLOWLIST, empty here because
// no non-stdlib import is ever permitted, so a future stray dependency is caught with no list
// maintenance required.

package discussionparser

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports is empty: no non-stdlib import is permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/discussionparser", allowedImports...)
}
