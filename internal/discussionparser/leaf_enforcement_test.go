// leaf_enforcement_test.go enforces the Discussionparser Sole-Parser Invariant's stdlib-only half:
// production code in internal/discussionparser imports the standard library and nothing else:
// never internal/lymcwd, never any feature package.
// The check is an allowlist, empty here because no non-stdlib import is permitted;
// a stray dependency is caught without any list to maintain.

package discussionparser

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports is the allowlist of non-stdlib imports, empty because none are permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/discussionparser", allowedImports...)
}
