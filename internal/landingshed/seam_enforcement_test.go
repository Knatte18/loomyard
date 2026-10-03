// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/landingshed takes every absolute path it operates on from its caller and has no
// direct production import of internal/lyxcwd. Modelled directly on internal/loomshed's own
// seam_enforcement_test.go.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd denylist:
// it catches the excluded import and anything else that would drag geometry resolution in, with no
// list maintenance beyond a genuine new dependency. A transitive reach through an allowlisted
// dependency (internal/fabricengine and internal/githubclient both import internal/lyxcwd
// themselves) is explicitly fine -- the invariant's membership predicate is about a direct
// production import, and transitive is never policed.

package landingshed

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// landingshedAllowedImports are the only non-stdlib import paths production code in this package
// may use.
var landingshedAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/fabricengine",
	"github.com/Knatte18/loomyard/internal/mergeresolve",
	"github.com/Knatte18/loomyard/internal/modelspec",
	"github.com/Knatte18/loomyard/internal/configengine",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/shedtransient", // classifies a failed remote call as a transient hard error
	"github.com/Knatte18/loomyard/internal/githubclient",
	"github.com/Knatte18/loomyard/internal/gitrepo",
	"github.com/Knatte18/loomyard/internal/summaryparser",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"github.com/Knatte18/loomyard/internal/verifytree",
	"github.com/google/go-github/v75/github",
	"gopkg.in/yaml.v3",
}

func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/landingshed", landingshedAllowedImports...)
}
