// leaf_enforcement_test.go enforces internal/stopclass's import cap from PATTERN-leaf-packages:
// production code in internal/stopclass imports the standard library alone.

package stopclass_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLeafInvariant_StandardLibraryOnly(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	scankit.AssertImportAllowlist(t, "internal/stopclass")
}
