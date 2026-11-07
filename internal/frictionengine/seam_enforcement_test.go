// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership:
// production code in internal/frictionengine takes every absolute path it operates on from its caller,
// and it has no direct production import of internal/lymcwd.
//
// The allowlist below is a membership list:
// it catches the excluded import and any other new dependency that would drag geometry resolution in;
// it needs list maintenance only for a genuine new dependency.
// A transitive reach through an allowed entry is fine.

package frictionengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// frictionengineAllowedImports lists the only non-stdlib import paths production code in this package may use.
// internal/friction supplies ReportFileName and EnsureDir;
// the model-spec package resolves the reflection session's model;
// the shuttle engine is the reflection session's seam;
// the stencil store reads the reflection prompt off disk;
// the stencil filler renders it; the logger handles logging.
// The segment vocabulary names the reflection session's segment.
var frictionengineAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/friction",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/modelspec",
	"github.com/Knatte18/loomyard/internal/editdirective",
	"github.com/Knatte18/loomyard/internal/parentdirective",
	"github.com/Knatte18/loomyard/internal/segmentcolor",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
}

func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/frictionengine", frictionengineAllowedImports...)
}
