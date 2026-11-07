// seam_enforcement_test.go enforces this package's told-geometry membership:
// production code in internal/reportengine imports only the packages whose records it reads, and never internal/lymcwd.

package reportengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
// Add a path here when production code in this package imports it.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/gitrepo",
	"github.com/Knatte18/loomyard/internal/shedrun",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/loomshed",
	"github.com/Knatte18/loomyard/internal/battenrecipe",
	"github.com/Knatte18/loomyard/internal/fabricengine",
	"github.com/Knatte18/loomyard/internal/battenshed",
	"github.com/Knatte18/loomyard/internal/haltnote",
	"github.com/Knatte18/loomyard/internal/loomengine",
	"github.com/Knatte18/loomyard/internal/loomrecipe",
	"github.com/Knatte18/loomyard/internal/shedadapters",
	"github.com/Knatte18/loomyard/internal/discussionparser",
	"github.com/Knatte18/loomyard/internal/boardengine",
	"github.com/Knatte18/loomyard/internal/durfmt",
	"github.com/Knatte18/loomyard/internal/creelengine",
	"github.com/Knatte18/loomyard/internal/agentname",
	"github.com/Knatte18/loomyard/internal/raddleparser",
	"gopkg.in/yaml.v3",
}

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist:
// it excludes internal/lymcwd, and no allowed path goes unused.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/reportengine", allowedImports...)
}
