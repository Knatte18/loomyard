// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production code takes every absolute path from its caller and has no direct production import of internal/lyxcwd.
// The allowlist is a membership list rather than a bare denylist, modelled on internal/loomshed's own.

package parentreview

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// parentreviewAllowedImports are the only non-stdlib import paths production code in this package may use.
var parentreviewAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/state",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
}

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file imports only stdlib or an entry in parentreviewAllowedImports.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/parentreview", parentreviewAllowedImports...)
}
