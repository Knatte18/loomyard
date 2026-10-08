// leaf_enforcement_test.go enforces the Standalonestate Leaf Invariant: production code in
// internal/standalonestate imports only the standard library -- no other package at all.
// Like modelspec's leaf_enforcement_test.go, this check is an ALLOWLIST: any import outside the
// allowed set fails the test, so a future stray dependency is caught with no list maintenance
// required.

package standalonestate

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports is empty: no non-stdlib import is permitted.
var allowedImports = []string{}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/standalonestate", allowedImports...)
}
