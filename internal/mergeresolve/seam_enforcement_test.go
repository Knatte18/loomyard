// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/mergeresolve takes every absolute path it operates on from its caller and has no
// direct production import of internal/lyxcwd. Modelled directly on
// internal/loomshed/seam_enforcement_test.go.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd denylist:
// it catches the excluded import and anything else that would drag geometry resolution in, with no
// list maintenance beyond a genuine new dependency, and a transitive reach through an allowed entry
// is explicitly fine.

package mergeresolve

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// mergeresolveAllowedImports are the only non-stdlib import paths production code in this package
// may use: the fabric engine (the merge seam), the shuttle engine (the conflict-session seam), the
// model-spec package (resolving the conflict session's model), the stencil store (reading the
// conflict prompt off disk), the stencil filler (rendering it), and the logger.
// The segment vocabulary names the conflict session's segment.
// internal/logger carries no geometry and opens no seam, so admitting it leaves the Told-Geometry
// Invariant's actual property intact -- the same call PATTERN-treadle-runner-seam's
// allowlist already makes.
var mergeresolveAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/fabricengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/modelspec",
	"github.com/Knatte18/loomyard/internal/parentdirective",
	"github.com/Knatte18/loomyard/internal/segmentcolor",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/logger",
}

func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/mergeresolve", mergeresolveAllowedImports...)
}
