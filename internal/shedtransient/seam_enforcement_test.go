// seam_enforcement_test.go enforces this package's import allowlist: production code in internal/shedtransient may import only the stdlib and the four packages whose classifications it translates,
// so shedengine stays stdlib-only and no leaf package ever imports shedengine.

package shedtransient

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// shedtransientAllowedImports are the only non-stdlib import paths production code in this package may use.
var shedtransientAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/gitexec",
	"github.com/Knatte18/loomyard/internal/githubclient",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
}

// TestImportAllowlistOnly verifies that every non-test .go file imports only stdlib or an entry in shedtransientAllowedImports.
func TestImportAllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/shedtransient", shedtransientAllowedImports...)
}
