// seam_enforcement_test.go enforces this package's no-resolver, no-<module>cli import seam: production
// code in internal/shedverbs takes every path it operates on from a told Spec, never derives one, and
// never imports back into a CLI module.
//
// The allowlist below is deliberately a membership list rather than a bare denylist, mirroring
// internal/battenshed's and internal/loomrecipe's own reasoning: it catches the excluded imports
// and anything else that would drag geometry resolution in, with no list maintenance beyond a genuine
// new dependency. shedverbs importing cobra while not being a <module>cli package is deliberate: the
// rule that matters is that an ENGINE never imports cli/cobra, and shedverbs is not an engine.

package shedverbs

import (
	"go/ast"
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// shedverbsAllowedImports are the only non-stdlib import paths production code in this package may
// use.
//
// logger is admitted for step-boundary logging and for logger.TraceFile/logger.TraceDir, the only
// path sources admitted into this package, each returning the logger's own sink location verbatim.
var shedverbsAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/buildvcs",
	"github.com/Knatte18/loomyard/internal/clihelp",
	"github.com/Knatte18/loomyard/internal/fswatch",
	"github.com/Knatte18/loomyard/internal/lock",
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/output",
	"github.com/Knatte18/loomyard/internal/proc",
	"github.com/Knatte18/loomyard/internal/state",
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/spf13/cobra",
}

// shedverbsDeniedLyxcwdImport is the exact import path the no-resolver clause excludes from this
// package's production files, named here so a violation of that specific rule is reported by name
// rather than only implied by its absence from the allowlist above.
const shedverbsDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"

// isModuleCLIImportPath reports whether importPath has the <module>cli shape this package must
// never import: an internal/ path whose final segment ends in "cli".
func isModuleCLIImportPath(importPath string) bool {
	const prefix = "github.com/Knatte18/loomyard/internal/"
	if !strings.HasPrefix(importPath, prefix) {
		return false
	}
	rest := strings.TrimPrefix(importPath, prefix)
	// rest may still contain a slash for a nested package; the module name is its first segment.
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		rest = rest[:idx]
	}
	return strings.HasSuffix(rest, "cli")
}

// TestNoResolverNoModuleCLIInvariant_AllowlistOnly verifies that every non-test .go file in this
// package imports only stdlib or an entry in shedverbsAllowedImports, and separately asserts that no
// production import path is shedverbsDeniedLyxcwdImport or matches the <module>cli shape.
func TestNoResolverNoModuleCLIInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/shedverbs", shedverbsAllowedImports...)

	var deniedFound []string
	var moduleCLIFound []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/shedverbs"}, Shallow: true}, func(f *scankit.File) {
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if importPath == shedverbsDeniedLyxcwdImport {
				deniedFound = append(deniedFound, f.Rel)
			}
			if isModuleCLIImportPath(importPath) {
				moduleCLIFound = append(moduleCLIFound, f.Rel+": "+importPath)
			}
		}
	})
	scankit.RequireFloor(t, scanned, 1, "shedverbs denied-import scan")

	if len(deniedFound) > 0 {
		t.Errorf("no-resolver seam violated; %s imported directly in: %v", shedverbsDeniedLyxcwdImport, deniedFound)
	}
	if len(moduleCLIFound) > 0 {
		t.Errorf("no-<module>cli seam violated; a <module>cli-shaped import was found in: %v", moduleCLIFound)
	}
}

// TestNoResolverInvariant_NoOSGetwd verifies that no production file in this package references
// os.Getwd, matching the no-resolver clause the new Shed Verb-Set Invariant carries in batch 6. A
// selector-expression walk is enough, since the package imports no shell runner through which
// `git rev-parse` could reach it.
func TestNoResolverInvariant_NoOSGetwd(t *testing.T) {
	var found []string

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/shedverbs"}, Shallow: true}, func(f *scankit.File) {
		ast.Inspect(f.AST(t, parser.ParseComments), func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pkgIdent.Name == "os" && sel.Sel.Name == "Getwd" {
				found = append(found, f.Rel)
			}
			return true
		})
	})
	scankit.RequireFloor(t, scanned, 1, "shedverbs os.Getwd scan")

	if len(found) > 0 {
		t.Errorf("no-resolver seam violated; os.Getwd referenced in: %v", found)
	}
}
