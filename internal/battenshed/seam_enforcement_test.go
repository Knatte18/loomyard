// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/battenshed takes every absolute path it operates on from its caller and has
// no direct production import of internal/lyxcwd.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd
// denylist, mirroring internal/loomrecipe's and internal/shedrecipe's own reasoning: it catches the
// excluded import and anything else that would drag geometry resolution in, with no list
// maintenance beyond a genuine new dependency.

package battenshed

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// battenshedAllowedImports are the only non-stdlib import paths production code in this package
// may use.
var battenshedAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/logger",
}

// battenshedDeniedLyxcwdImport is the exact import path the Told-Geometry Invariant excludes
// from this package's production files, named here so a violation of that specific rule is
// reported by name rather than only implied by its absence from the allowlist above.
const battenshedDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist, and separately asserts
// that no production import path is battenshedDeniedLyxcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/battenshed", battenshedAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/battenshed"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == battenshedDeniedLyxcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "battenshed denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", battenshedDeniedLyxcwdImport, deniedFound)
	}
}
