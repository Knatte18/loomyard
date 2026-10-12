// leaf_enforcement_test.go enforces that production code in internal/fabricvocab imports the standard library and nothing else.
// The check is an ALLOWLIST, empty here because no non-stdlib import is ever permitted.

package fabricvocab

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports is empty: no non-stdlib import is permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/fabricvocab", allowedImports...)
}
