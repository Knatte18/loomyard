// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production code takes every absolute path from its caller and has no direct production import of internal/lyxcwd.
// The allowlist is a membership list rather than a bare denylist, modelled on internal/loomshed's own.

package parentlock

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// parentlockAllowedImports are the only non-stdlib import paths production code in this package may use.
var parentlockAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/lock",
	"github.com/Knatte18/loomyard/internal/proc",
	"github.com/Knatte18/loomyard/internal/lyxdirs",
	"github.com/Knatte18/loomyard/internal/logger",
}

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file imports only stdlib or an entry in parentlockAllowedImports.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/parentlock", parentlockAllowedImports...)
}
