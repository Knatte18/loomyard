// seam_enforcement_test.go enforces this package's half of the Shed Recipe Registry Invariant's
// told-geometry rule: production code in internal/shedrecipe takes every absolute path it operates
// on from its caller and has no direct production import of internal/lyxcwd.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd
// denylist, mirroring internal/loomshed's own reasoning: it catches the excluded import and
// anything else that would drag geometry resolution in, with no list maintenance beyond a genuine
// new dependency.
//
// This is the largest allowlist in the repo, and that is expected: this package is the wiring layer
// that has to reach types from four producer-hosting packages at once (internal/shedengine,
// internal/preflightshed, internal/landingshed, internal/loomshed) plus the seam and stencil
// packages those producers' constructors take.
//
// Several allowlisted packages themselves import internal/lyxcwd, and that is legal: the
// Told-Geometry Invariant's membership predicate is about a direct production import, and
// transitive is explicitly fine.

package shedrecipe

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// shedrecipeAllowedImports are the only non-stdlib import paths production code in this package may
// use.
var shedrecipeAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/shedadapters",
	"github.com/Knatte18/loomyard/internal/loomshed",
	"github.com/Knatte18/loomyard/internal/landingshed",
	"github.com/Knatte18/loomyard/internal/parentreview",
	"github.com/Knatte18/loomyard/internal/planindex",
	"github.com/Knatte18/loomyard/internal/battenshed",
	"github.com/Knatte18/loomyard/internal/preflightshed",
	"github.com/Knatte18/loomyard/internal/websterengine",
	"github.com/Knatte18/loomyard/internal/burlerengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"github.com/Knatte18/loomyard/internal/stencil",
}

// shedrecipeDeniedLyxcwdImport is the exact import path the Shed Recipe Registry Invariant excludes
// from this package's production files, named here so a violation of that specific rule is reported
// by name rather than only implied by its absence from the allowlist above.
const shedrecipeDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in shedrecipeAllowedImports, and separately asserts that no
// production import path is shedrecipeDeniedLyxcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/shedrecipe", shedrecipeAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/shedrecipe"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == shedrecipeDeniedLyxcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "shedrecipe denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Shed Recipe Registry Invariant violated; %s imported directly in: %v", shedrecipeDeniedLyxcwdImport, deniedFound)
	}
}
