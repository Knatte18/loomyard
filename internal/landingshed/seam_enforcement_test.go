// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership.
// Production code in internal/landingshed takes every absolute path it operates on from its caller,
// and it has no direct production import of internal/lymcwd.
//
// The allowlist is a membership list, not an internal/lymcwd denylist.
// It catches the excluded import and any other new dependency that would drag geometry resolution in.
// It needs upkeep only for a genuine new dependency.
// A transitive reach through an allowlisted dependency is fine.
// Such a dependency may import internal/lymcwd itself; the predicate checks only direct production imports.

package landingshed

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// landingshedAllowedImports lists the only non-stdlib import paths that production code in this package may use.
var landingshedAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/boardengine", // names the merge-in pull_request value
	"github.com/Knatte18/loomyard/internal/fabricengine",
	"github.com/Knatte18/loomyard/internal/gateslot",
	"github.com/Knatte18/loomyard/internal/githubclient",
	"github.com/Knatte18/loomyard/internal/mergeresolve",
	"github.com/Knatte18/loomyard/internal/modelspec",
	"github.com/Knatte18/loomyard/internal/parentlock",
	"github.com/Knatte18/loomyard/internal/pullrequest",
	"github.com/Knatte18/loomyard/internal/configengine",
	"github.com/Knatte18/loomyard/internal/editdirective",
	"github.com/Knatte18/loomyard/internal/languageconfig",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/parentdirective",
	"github.com/Knatte18/loomyard/internal/seatengine",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/shedtransient", // classifies a failed push as a transient hard error
	"github.com/Knatte18/loomyard/internal/summaryparser",
	"github.com/Knatte18/loomyard/internal/segmentcolor",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"github.com/Knatte18/loomyard/internal/verifytree",
	"github.com/Knatte18/loomyard/internal/yamlengine",
	"gopkg.in/yaml.v3",
}

func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/landingshed", landingshedAllowedImports...)
}
