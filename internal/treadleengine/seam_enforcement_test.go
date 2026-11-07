// seam_enforcement_test.go enforces the Treadle Runner-Seam Invariant.
// Production code in internal/treadleengine imports only the standard library and the packages in allowedImports:
// it never imports internal/burlerengine, never imports internal/lymcwd directly, and never any internal/*cli package.
// The check is an allowlist, so any import outside the set fails the test.
// A stray dependency, such as a round runner's type leaking upward, is caught with no deny list to maintain.

package treadleengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports lists the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/agentconfig",
	"github.com/Knatte18/loomyard/internal/editdirective",
	"github.com/Knatte18/loomyard/internal/lock",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/parentdirective",
	"github.com/Knatte18/loomyard/internal/segmentcolor",
	"github.com/Knatte18/loomyard/internal/state",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"gopkg.in/yaml.v3",
}

// TestRunnerSeamInvariant_AllowlistOnly verifies the import allowlist.
// Every non-test .go file imports only the standard library or an entry in allowedImports.
func TestRunnerSeamInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/treadleengine", allowedImports...)
}
