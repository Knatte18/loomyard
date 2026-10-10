// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/loomrecipe takes every absolute path it operates on from its caller and has no
// direct production import of internal/lyxcwd.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd
// denylist, mirroring internal/shedrecipe's and internal/loomshed's own reasoning: it catches the
// excluded import and anything else that would drag geometry resolution in, with no list
// maintenance beyond a genuine new dependency.
//
// github.com/Knatte18/loomyard/contracts/recipes sits outside internal/ and so must be allowlisted explicitly, since the stdlib test is "the first path segment contains no dot", which a full module path never satisfies.

package loomrecipe

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// loomrecipeAllowedImports are the only non-stdlib import paths production code in this package may
// use.
var loomrecipeAllowedImports = []string{
	"github.com/Knatte18/loomyard/contracts/recipes",
	"github.com/Knatte18/loomyard/internal/loomshed",
	"github.com/Knatte18/loomyard/internal/shedbuild",
	"github.com/Knatte18/loomyard/internal/shedrecipe",
	"github.com/Knatte18/loomyard/internal/shedengine",
}

// loomrecipeDeniedLyxcwdImport is the exact import path the Told-Geometry Invariant excludes from
// this package's production files, named here so a violation of that specific rule is reported by
// name rather than only implied by its absence from the allowlist above.
const loomrecipeDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist, and separately asserts that no production import path is loomrecipeDeniedLyxcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/loomrecipe", loomrecipeAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/loomrecipe"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == loomrecipeDeniedLyxcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "loomrecipe denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", loomrecipeDeniedLyxcwdImport, deniedFound)
	}
}
