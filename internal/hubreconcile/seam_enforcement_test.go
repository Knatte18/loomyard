// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production code takes every absolute path from its caller and has no direct production import of internal/lyxcwd.

package hubreconcile

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// hubreconcileAllowedImports are the only non-stdlib import paths production code in this package may use.
var hubreconcileAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/buildvcs",
	"github.com/Knatte18/loomyard/internal/configengine",
	"github.com/Knatte18/loomyard/internal/configreg",
	"github.com/Knatte18/loomyard/internal/configsync",
	"github.com/Knatte18/loomyard/internal/fabricengine",
	"github.com/Knatte18/loomyard/internal/fsx",
	"github.com/Knatte18/loomyard/internal/lock",
	"github.com/Knatte18/loomyard/internal/logger",
}

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file imports only stdlib or an entry in hubreconcileAllowedImports.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/hubreconcile", hubreconcileAllowedImports...)
}
