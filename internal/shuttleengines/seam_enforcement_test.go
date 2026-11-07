// seam_enforcement_test.go enforces this package's told-geometry membership.
// Production code here imports only the engine packages it registers, reed's name seam and the agent-name parser.
// It never imports internal/lymcwd.

package shuttleengines

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/agentname",
	"github.com/Knatte18/loomyard/internal/configengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine/codexengine",
	"github.com/Knatte18/loomyard/internal/reedengine",
}

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist.
// The allowlist excludes internal/lymcwd, and the test fails on any allowed path that goes unused.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/shuttleengines", allowedImports...)
}
