// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership:
// production code in internal/loomrecipe takes every absolute path it operates on from its caller,
// and it has no direct production import of internal/lymcwd.
//
// The allowlist catches the excluded import and any other import that would drag geometry resolution in;
// it needs no list upkeep beyond a genuine new dependency.
//
// github.com/Knatte18/loomyard/contracts/recipes sits outside internal/, so it must be allowlisted explicitly;
// the stdlib test accepts only paths whose first segment contains no dot, which a full module path never satisfies.

package loomrecipe

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// loomrecipeAllowedImports lists the only non-stdlib import paths that production code in this package may use.
var loomrecipeAllowedImports = []string{
	"github.com/Knatte18/loomyard/contracts/recipes",
	// internal/loomengine gives the raddle proposal's path, which the split-perch rows test matches.
	// It resolves no geometry.
	"github.com/Knatte18/loomyard/internal/loomengine",
	"github.com/Knatte18/loomyard/internal/loomshed",
	"github.com/Knatte18/loomyard/internal/shedbuild",
	"github.com/Knatte18/loomyard/internal/shedrecipe",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/shedrun",
}

// loomrecipeDeniedLymcwdImport is the exact import path that this package's production files must not use.
// The Told-Geometry Invariant excludes it;
// naming it reports a violation by name rather than only by its absence from the allowlist.
const loomrecipeDeniedLymcwdImport = "github.com/Knatte18/loomyard/internal/lymcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist,
// and it separately asserts that no production import path is loomrecipeDeniedLymcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/loomrecipe", loomrecipeAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/loomrecipe"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == loomrecipeDeniedLymcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "loomrecipe denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", loomrecipeDeniedLymcwdImport, deniedFound)
	}
}
