// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership:
// production code in internal/alignment takes every absolute path it operates on from its caller and has no direct production import of internal/lyxcwd.
//
// The allowlist is a membership list rather than a bare internal/lyxcwd denylist, so it also catches anything else that would drag geometry resolution in.
// A transitive reach through an allowlisted dependency is fine, since the invariant polices direct production imports only.

package alignment

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// alignmentAllowedImports are the only non-stdlib import paths production code in this package may use.
var alignmentAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/editdirective",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/parentdirective",
	"github.com/Knatte18/loomyard/internal/pauserecord",
	"github.com/Knatte18/loomyard/internal/segmentcolor",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/state",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"gopkg.in/yaml.v3",
}

func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/alignment", alignmentAllowedImports...)
}
