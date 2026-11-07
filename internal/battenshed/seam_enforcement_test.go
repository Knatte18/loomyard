// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/battenshed takes every absolute path it operates on from its caller and has
// no direct production import of internal/lymcwd.
//
// The allowlist is a membership list rather than a bare internal/lymcwd denylist.
// It catches the excluded import and anything else that would drag geometry resolution in.
// No list maintenance is needed beyond a genuine new dependency.

package battenshed

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// battenshedAllowedImports are the only non-stdlib import paths production code in this package may use.
var battenshedAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/configengine",
	"gopkg.in/yaml.v3",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/parentlock",
	"github.com/Knatte18/loomyard/internal/pauserecord",
	"github.com/Knatte18/loomyard/internal/sheddriver",
	"github.com/Knatte18/loomyard/internal/state",
}

// battenshedDeniedLymcwdImport is the import path the Told-Geometry Invariant excludes.
// It is excluded from this package's production files.
// Naming it reports a violation of that rule by name, not only by the path's absence from the allowlist.
const battenshedDeniedLymcwdImport = "github.com/Knatte18/loomyard/internal/lymcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist.
// It also asserts that no production import path is battenshedDeniedLymcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/battenshed", battenshedAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/battenshed"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == battenshedDeniedLymcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "battenshed denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", battenshedDeniedLymcwdImport, deniedFound)
	}
}
