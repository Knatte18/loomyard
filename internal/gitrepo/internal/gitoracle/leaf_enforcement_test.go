// leaf_enforcement_test.go enforces the gitoracle independence rule:
// production code in internal/gitrepo/internal/gitoracle imports only the standard library and internal/gitexec, never internal/gitrepo or internal/gitkit.
// The oracle is a second implementation of gitrepo's reads, so an import of gitrepo would turn every parity test into a tautology.

package gitoracle

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/gitexec",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/gitrepo/internal/gitoracle", allowedImports...)
}
