// seam_enforcement_test.go enforces this package's told-geometry membership: production code in internal/reportengine imports only the packages whose records it reads, and never internal/lyxcwd.

package reportengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
// Each later engine card extends it with what that card imports.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/gitrepo",
	"github.com/Knatte18/loomyard/internal/shedrun",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/loomshed",
	"github.com/Knatte18/loomyard/internal/battenrecipe",
	"github.com/Knatte18/loomyard/internal/fabricengine",
}

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist, which excludes internal/lyxcwd, and that no allowed path goes unused.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/reportengine", allowedImports...)
}
