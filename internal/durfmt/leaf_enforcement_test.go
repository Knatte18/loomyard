// leaf_enforcement_test.go enforces internal/durfmt's import cap from PATTERN-leaf-packages:
// production code in internal/durfmt imports the standard library alone.

package durfmt

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_StandardLibraryOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/durfmt")
}
