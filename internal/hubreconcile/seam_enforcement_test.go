// seam_enforcement_test.go enforces this package's membership in the Told-Geometry Invariant;
// production code takes every absolute path from its caller and has no direct production import of internal/lymcwd.

package hubreconcile

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// hubreconcileAllowedImports are the only non-stdlib import paths production code in this package may use.
var hubreconcileAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/boardengine",
	"github.com/Knatte18/loomyard/internal/buildvcs",
	"github.com/Knatte18/loomyard/internal/configengine",
	"github.com/Knatte18/loomyard/internal/configreg",
	"github.com/Knatte18/loomyard/internal/configsync",
	"github.com/Knatte18/loomyard/internal/fabricengine",
	"github.com/Knatte18/loomyard/internal/fsx",
	"github.com/Knatte18/loomyard/internal/lock",
	"github.com/Knatte18/loomyard/internal/logger",
}

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist of this package.
// Every non-test .go file imports only stdlib or an entry in hubreconcileAllowedImports.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/hubreconcile", hubreconcileAllowedImports...)
}
