// seam_enforcement_test.go enforces this package's half of the Told-Geometry Invariant: production
// code in internal/cliwire takes every absolute path it operates on from its caller and has no
// direct production import of internal/lyxcwd.
//
// cliwire/doc.go already makes this claim in prose ("cliwire's production dependency set is fixed
// ... internal/lyxcwd is barred by the Told-Geometry Invariant and is never needed here"); this test
// is the mechanical pin behind that prose, mirroring internal/shedrecipe's own
// seam_enforcement_test.go (crucible round sonnet-xhigh-r8, CW-3). Several other Told-Geometry
// "Bound packages" (reedengine, burlerengine, websterengine, planparser, planglyph, configengine)
// carry the same unenforced claim today with no dedicated test of their own -- a genuine, repo-wide
// gap this one test closes only for cliwire, the package this round's own mandate is auditing, not
// for the whole Bound-packages set.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd denylist,
// mirroring internal/shedrecipe's own reasoning: it catches the excluded import and anything else
// that would drag geometry resolution in, with no list maintenance beyond a genuine new dependency.

package cliwire

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// cliwireAllowedImports are the only non-stdlib import paths production code in this package may
// use -- exactly the six named in doc.go's own "production dependency set is fixed" claim.
var cliwireAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/standalonestate",
	"github.com/Knatte18/loomyard/internal/standalonegeom",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"github.com/Knatte18/loomyard/internal/buildinfo",
	"github.com/Knatte18/loomyard/contracts/stencils",
	"github.com/Knatte18/loomyard/contracts/specs",
	"github.com/Knatte18/loomyard/internal/reedengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine",
}

// cliwireDeniedLyxcwdImport is the exact import path the Told-Geometry Invariant excludes from this
// package's production files, named here so a violation of that specific rule is reported by name
// rather than only implied by its absence from the allowlist above.
const cliwireDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in cliwireAllowedImports, and separately asserts that no
// production import path is cliwireDeniedLyxcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/cliwire", cliwireAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/cliwire"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == cliwireDeniedLyxcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "cliwire denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", cliwireDeniedLyxcwdImport, deniedFound)
	}
}
