// leaf_enforcement_test.go enforces internal/sleepjournal's import cap from PATTERN-leaf-packages:
// production code in internal/sleepjournal imports the standard library alone.

package sleepjournal

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_StandardLibraryOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/sleepjournal")
}
