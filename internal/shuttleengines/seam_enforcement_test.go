// seam_enforcement_test.go enforces this package's told-geometry membership: production code in internal/shuttleengines imports only the engine packages it registers and reed's name seam, and never internal/lyxcwd.

package shuttleengines

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/configengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine",
	"github.com/Knatte18/loomyard/internal/reedengine",
}

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist, which excludes internal/lyxcwd, and that no allowed path goes unused.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/shuttleengines", allowedImports...)
}
