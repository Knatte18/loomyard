// leaf_enforcement_test.go enforces internal/toolfloor's import cap from PATTERN-leaf-packages:
// production code in internal/toolfloor imports the standard library alone.

package toolfloor

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLeafInvariant_StandardLibraryOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/toolfloor")
}
