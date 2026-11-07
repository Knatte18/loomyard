// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership.
// Production code in internal/battenrecipe takes every absolute path it operates on from its caller.
// It has no direct production import of internal/lymcwd.
//
// The allowlist below is a membership list rather than a denylist of internal/lymcwd:
// it catches the excluded import and any other import that would drag geometry resolution in.
// It needs no list maintenance beyond a genuine new dependency.
//
// github.com/Knatte18/loomyard/contracts/recipes sits outside internal/, so it must be allowlisted explicitly:
// the stdlib test counts an import as standard library when its first path segment contains no dot.
// A full module path never satisfies that test.

package battenrecipe

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// battenrecipeAllowedImports are the only non-stdlib import paths production code in this package may use.
var battenrecipeAllowedImports = []string{
	"github.com/Knatte18/loomyard/contracts/recipes",
	"github.com/Knatte18/loomyard/internal/shedbuild",
	"github.com/Knatte18/loomyard/internal/shedrecipe",
	"github.com/Knatte18/loomyard/internal/shedengine",
}

// battenrecipeDeniedLymcwdImport is the import path the Told-Geometry Invariant excludes from production files here.
// It is named so a violation is reported by name rather than only implied by its absence from the allowlist above.
const battenrecipeDeniedLymcwdImport = "github.com/Knatte18/loomyard/internal/lymcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist.
// It also asserts that no production import path is battenrecipeDeniedLymcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/battenrecipe", battenrecipeAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/battenrecipe"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == battenrecipeDeniedLymcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "battenrecipe denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", battenrecipeDeniedLymcwdImport, deniedFound)
	}
}
