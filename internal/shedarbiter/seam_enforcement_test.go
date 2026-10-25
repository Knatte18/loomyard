//go:build tier0

// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership:
// production code in internal/shedarbiter takes every path from its caller and imports no cwd resolver and no CLI package.

package shedarbiter

import (
	"go/parser"
	"path"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// shedarbiterAllowedImports are the only non-stdlib import paths production code in this package may use.
var shedarbiterAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/arbiterstop",
	"github.com/Knatte18/loomyard/internal/editdirective",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/loomengine",
	"github.com/Knatte18/loomyard/internal/parentdirective",
	"github.com/Knatte18/loomyard/internal/segmentcolor",
	"github.com/Knatte18/loomyard/internal/sheddriver",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	"github.com/Knatte18/loomyard/internal/stencil",
	"github.com/Knatte18/loomyard/internal/stencilstore",
}

// shedarbiterDeniedImport reports whether importPath is one of the imports the package never makes: the cwd resolver and any internal CLI package.
func shedarbiterDeniedImport(importPath string) bool {
	if importPath == "github.com/Knatte18/loomyard/internal/lyxcwd" {
		return true
	}
	return strings.HasPrefix(importPath, "github.com/Knatte18/loomyard/internal/") && strings.HasSuffix(path.Base(importPath), "cli")
}

// TestToldGeometryInvariant_AllowlistOnly verifies the import allowlist, and separately names any production import of a denied package.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/shedarbiter", shedarbiterAllowedImports...)

	var deniedFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/shedarbiter"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			if importPath := strings.Trim(imp.Path.Value, `"`); shedarbiterDeniedImport(importPath) {
				deniedFound = append(deniedFound, f.Rel+" imports "+importPath)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "shedarbiter denied-import scan")
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; shedarbiter must not import lyxcwd or an internal CLI package: %v", deniedFound)
	}
}
