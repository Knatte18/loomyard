// seam_enforcement_test.go enforces PATTERN-shed-driver's import seam and this package's Told-Geometry Invariant membership:
// production code in internal/sheddriver takes every path from its caller and reaches recipe behaviour only through the seams its arming module fills.

package sheddriver

import (
	"go/parser"
	"path"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// sheddriverAllowedImports are the only non-stdlib import paths production code in this package may use.
var sheddriverAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/buildvcs",
	"github.com/Knatte18/loomyard/internal/configengine",
	"github.com/Knatte18/loomyard/internal/editdirective",
	"github.com/Knatte18/loomyard/internal/lock",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/modelspec",
	"github.com/Knatte18/loomyard/internal/output",
	"github.com/Knatte18/loomyard/internal/parentdirective",
	"github.com/Knatte18/loomyard/internal/pauserecord",
	"github.com/Knatte18/loomyard/internal/segmentcolor",
	"github.com/Knatte18/loomyard/internal/shedengine",
	// shedrun is admitted for the driver-kind vocabulary and its validator only:
	// every shedrun path constructor takes a *lyxcwd.Location this package can never hold.
	"github.com/Knatte18/loomyard/internal/shedrun",
	"github.com/Knatte18/loomyard/internal/shedverbs",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	// yaml.v3 decodes the verdict and the config documents.
	"gopkg.in/yaml.v3",
}

// sheddriverDeniedImport reports whether importPath is one of the four imports the package never makes:
// the cwd resolver, the loom engine, the loom shed and any internal CLI package.
func sheddriverDeniedImport(importPath string) bool {
	switch importPath {
	case "github.com/Knatte18/loomyard/internal/lyxcwd",
		"github.com/Knatte18/loomyard/internal/loomengine",
		"github.com/Knatte18/loomyard/internal/loomshed":
		return true
	}
	return strings.HasPrefix(importPath, "github.com/Knatte18/loomyard/internal/") && strings.HasSuffix(path.Base(importPath), "cli")
}

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist, and separately names any production import of a denied package.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/sheddriver", sheddriverAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/sheddriver"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if importPath := strings.Trim(imp.Path.Value, `"`); sheddriverDeniedImport(importPath) {
				deniedFound = append(deniedFound, f.Rel+" imports "+importPath)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "sheddriver denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; sheddriver must not import lyxcwd, loomengine, loomshed or an internal CLI package: %v", deniedFound)
	}
}
