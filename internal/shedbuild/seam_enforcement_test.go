// seam_enforcement_test.go enforces this package's told-geometry rule, mirroring
// internal/shedrecipe/seam_enforcement_test.go's shape exactly: production code in
// internal/shedbuild takes every absolute path it operates on from its caller and has no direct
// production import of internal/lymcwd.
//
// The allowlist below is a membership list rather than a bare internal/lymcwd denylist.
// It catches the excluded import and anything else that would drag geometry resolution in.
// It needs list maintenance only for a genuine new dependency.
// This package's allowlist is short:
// it reaches the registry, the producer-definition type, and the checker, and nothing else.

package shedbuild

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// shedbuildAllowedImports are the only non-stdlib import paths production code in this package may use.
var shedbuildAllowedImports = []string{
	"gopkg.in/yaml.v3",
	"github.com/Knatte18/loomyard/internal/shedrecipe",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/shedcheck",
	// shedtransient supplies the Shed.Transient classifier NewShed tells (Transient Stop Invariant).
	"github.com/Knatte18/loomyard/internal/shedtransient",
}

// shedbuildDeniedLymcwdImport is the exact import path the Told-Geometry Invariant excludes from production files.
// It names the path, so a violation is reported by name rather than only implied by its absence from the allowlist.
const shedbuildDeniedLymcwdImport = "github.com/Knatte18/loomyard/internal/lymcwd"

// TestToldGeometryInvariant_AllowlistOnly checks the imports of every production file.
// Each may import only stdlib or an entry in shedbuildAllowedImports.
// It also asserts that no production import path is shedbuildDeniedLymcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/shedbuild", shedbuildAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/shedbuild"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == shedbuildDeniedLymcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "shedbuild denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", shedbuildDeniedLymcwdImport, deniedFound)
	}
}
