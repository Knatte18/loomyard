// seam_enforcement_test.go enforces this package's told-geometry rule, mirroring
// internal/shedrecipe/seam_enforcement_test.go's shape exactly: production code in
// internal/shedbuild takes every absolute path it operates on from its caller and has no direct
// production import of internal/lyxcwd.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd
// denylist, for the same reason the sibling's is: it catches the excluded import and anything else
// that would drag geometry resolution in, with no list maintenance beyond a genuine new dependency.
// This package's allowlist is deliberately short: it reaches the registry, the producer-definition
// type, and the checker, and nothing else.

package shedbuild

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// shedbuildAllowedImports are the only non-stdlib import paths production code in this package may
// use.
var shedbuildAllowedImports = []string{
	"gopkg.in/yaml.v3",
	"github.com/Knatte18/loomyard/internal/shedrecipe",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/shedcheck",
	// shedtransient supplies the Shed.Transient classifier NewShed tells (Transient Stop Invariant).
	"github.com/Knatte18/loomyard/internal/shedtransient",
}

// shedbuildDeniedLyxcwdImport is the exact import path the Told-Geometry Invariant excludes from
// this package's production files, named here so a violation of that specific rule is reported by
// name rather than only implied by its absence from the allowlist above.
const shedbuildDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in shedbuildAllowedImports, and separately asserts that no
// production import path is shedbuildDeniedLyxcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/shedbuild", shedbuildAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/shedbuild"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == shedbuildDeniedLyxcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "shedbuild denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", shedbuildDeniedLyxcwdImport, deniedFound)
	}
}
