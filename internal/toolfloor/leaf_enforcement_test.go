//go:build tier0

// leaf_enforcement_test.go enforces internal/toolfloor's import cap from PATTERN-leaf-packages:
// production code in internal/toolfloor imports the standard library alone.

package toolfloor

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_StandardLibraryOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/toolfloor")
}
