// seam_enforcement_test.go pins the Told-Geometry Invariant for internal/cliwire.
// Production code in this package takes every absolute path it operates on from its caller,
// and it has no direct production import of internal/lymcwd.
// The allowlist is a membership list rather than an internal/lymcwd denylist,
// so it also catches any other import that would drag geometry resolution in.
// Adding a genuine new dependency means extending the list.

package cliwire

import (
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// cliwireAllowedImports lists the only non-stdlib import paths production code in this package may use.
var cliwireAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/standalonestate",
	"github.com/Knatte18/loomyard/internal/standalonegeom",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/stencilstore",
	"github.com/Knatte18/loomyard/internal/buildinfo",
	"github.com/Knatte18/loomyard/internal/buildvcs",
	"github.com/Knatte18/loomyard/contracts/stencils",
	"github.com/Knatte18/loomyard/contracts/specs",
}

// cliwireDeniedLymcwdImport is the import path the Told-Geometry Invariant bars from this package's production files.
// Naming it reports a violation by name, not only by its absence from the allowlist.
const cliwireDeniedLymcwdImport = "github.com/Knatte18/loomyard/internal/lymcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in cliwireAllowedImports, and separately asserts that no
// production import path is cliwireDeniedLymcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/cliwire", cliwireAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/cliwire"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if strings.Trim(imp.Path.Value, `"`) == cliwireDeniedLymcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "cliwire denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", cliwireDeniedLymcwdImport, deniedFound)
	}
}
