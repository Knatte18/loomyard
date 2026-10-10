// leaf_enforcement_test.go enforces internal/dotgit's import cap from PATTERN-leaf-packages:
// production code in internal/dotgit imports the standard library alone.

package dotgit

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_StandardLibraryOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/dotgit")
}
