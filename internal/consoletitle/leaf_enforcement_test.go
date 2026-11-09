// leaf_enforcement_test.go enforces internal/consoletitle's import cap from PATTERN-leaf-packages:
// production code in internal/consoletitle imports the standard library alone.

package consoletitle_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLeafInvariant_StandardLibraryOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/consoletitle")
}
