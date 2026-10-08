// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/frictionengine takes every absolute path it operates on from its caller and has
// no direct production import of internal/lyxcwd. Modelled directly on
// internal/mergeresolve/seam_enforcement_test.go.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd denylist:
// it catches the excluded import and anything else that would drag geometry resolution in, with no
// list maintenance beyond a genuine new dependency, and a transitive reach through an allowed entry
// is explicitly fine.

package frictionengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// frictionengineAllowedImports are the only non-stdlib import paths production code in this package
// may use: internal/friction (ReportFileName and EnsureDir), the model-spec package (resolving the
// reflection session's model), the shuttle engine (the reflection session's seam), the stencil store
// (reading the reflection prompt off disk), the stencil filler (rendering it), and the logger.
// The segment vocabulary names the reflection session's segment.
var frictionengineAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/friction",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/modelspec",
	"github.com/Knatte18/loomyard/internal/parentdirective",
	"github.com/Knatte18/loomyard/internal/segmentcolor",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
}

func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/frictionengine", frictionengineAllowedImports...)
}
