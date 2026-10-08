// seam_enforcement_test.go enforces the Treadle Runner-Seam Invariant:
// production code in internal/treadleengine imports ONLY the standard library, internal/editdirective, internal/lock, internal/logger, internal/parentdirective, internal/segmentcolor, internal/state, internal/stencil, internal/stencilstore, internal/shuttleengine, and gopkg.in/yaml.v3;
// it never imports internal/burlerengine, never internal/lyxcwd as a direct import, and never any internal/*cli package.
// Like internal/modelspec's leaf_enforcement_test.go, this check is an ALLOWLIST: any import
// outside the allowed set fails the test, so a future stray dependency (a round-runner's own type
// leaking upward, a convenience lyxcwd import) is caught with no list maintenance required.

package treadleengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in
// this package may use.
var allowedImports = []string{
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

// TestRunnerSeamInvariant_AllowlistOnly verifies that every non-test .go file imports only stdlib
// or an entry in allowedImports.
func TestRunnerSeamInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/treadleengine", allowedImports...)
}
