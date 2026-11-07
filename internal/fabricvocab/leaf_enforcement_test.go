// leaf_enforcement_test.go enforces that production code in internal/fabricvocab imports only the standard library.
// The check is an allowlist; it is empty because no non-stdlib import is permitted.

package fabricvocab

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/fabricvocab", allowedImports...)
}
