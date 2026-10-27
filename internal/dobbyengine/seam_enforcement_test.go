//go:build tier0

// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production code in internal/dobbyengine takes every absolute path it operates on from its caller, and imports neither internal/lymcwd nor a CLI package.
//
// The allowlist is a membership list rather than a bare denylist, so it catches the excluded imports and anything else that would drag geometry resolution in.

package dobbyengine

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// dobbyengineAllowedImports are the only non-stdlib import paths production code in this package may use.
var dobbyengineAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/boardengine",
	"github.com/Knatte18/loomyard/internal/configengine",
	"github.com/Knatte18/loomyard/internal/editdirective",
	"github.com/Knatte18/loomyard/internal/lock",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/parentdirective",
	"github.com/Knatte18/loomyard/internal/pauserecord",
	"github.com/Knatte18/loomyard/internal/proc",
	"github.com/Knatte18/loomyard/internal/segmentcolor",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/state",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"gopkg.in/yaml.v3",
}

// dobbyengineDeniedLymcwdImport is the import path the Told-Geometry Invariant excludes, named so a violation is reported by name.
const dobbyengineDeniedLymcwdImport = "github.com/Knatte18/loomyard/internal/lymcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist, and separately asserts that no production import is internal/lymcwd or an internal CLI package.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/dobbyengine", dobbyengineAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/dobbyengine"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == dobbyengineDeniedLymcwdImport || (strings.Contains(path, "/internal/") && strings.HasSuffix(path, "cli")) {
				deniedFound = append(deniedFound, f.Rel+" imports "+path)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "dobbyengine denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated: %v", deniedFound)
	}
}
