// callerset_enforcement_test.go enforces the gitkit Leaf Invariant's CopyRepo pin.
// Only internal/lymcwd may call CopyRepo; every other package takes a real hub from internal/hubforge.
// It catches a call site that stays on the primitive repo fixture instead of moving to hubforge's real-hub factory.

package gitkit

import (
	"go/ast"
	"go/parser"
	"path"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedCopyRepoCallerDir is the only package directory, relative to the repository root, that may call CopyRepo.
const allowedCopyRepoCallerDir = "internal/lymcwd"

// minCopyRepoScanFiles is the fewest files the caller-set walk must scan, so an empty scan cannot pass.
const minCopyRepoScanFiles = 100

// TestCopyRepoCallerSet_LymcwdOnly verifies that no file outside internal/lymcwd calls CopyRepo.
// It spawns no git, so it carries no build tag.
// It parses every .go file under internal/ and cmd/ and skips internal/gitkit:
// its own doc comment and definition name CopyRepo.
// It looks for a selector call whose receiver is this file's gitkit import and whose selected name is CopyRepo.
// The match runs on the AST, not on raw text, so a doc comment that mentions the qualified call cannot trip it.
//
//lym:guard
func TestCopyRepoCallerSet_LymcwdOnly(t *testing.T) {
	tiergate.Require(t, 0)
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal", "cmd"}, Filter: scankit.All}, func(f *scankit.File) {
		if strings.HasPrefix(f.Rel, "internal/gitkit/") {
			return
		}
		astFile := f.AST(t, parser.ParseComments)

		gitkitAlias, imported := gitkitImportAlias(astFile)
		if !imported {
			return
		}
		if path.Dir(f.Rel) == allowedCopyRepoCallerDir {
			return
		}
		if callsCopyRepo(astFile, gitkitAlias) {
			failures = append(failures, f.Rel)
		}
	})
	scankit.RequireFloor(t, scanned, minCopyRepoScanFiles, "CopyRepo caller-set scan")

	if len(failures) > 0 {
		t.Errorf("gitkit Leaf Invariant violated: CopyRepo is pinned to %s alone, "+
			"but found call sites in: %v -- every other package takes a real hub from "+
			"hubforge's real-hub factory instead (see PATTERN-leaf-packages)",
			allowedCopyRepoCallerDir, failures)
	}
}

// gitkitImportAlias returns the local identifier astFile uses for the gitkit package.
// It reports whether astFile imports gitkit at all.
// An explicit import alias wins; without one, the default name "gitkit" applies.
func gitkitImportAlias(astFile *ast.File) (alias string, imported bool) {
	const gitkitImportPath = `"github.com/Knatte18/loomyard/internal/gitkit"`
	for _, imp := range astFile.Imports {
		if imp.Path.Value != gitkitImportPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name, true
		}
		return "gitkit", true
	}
	return "", false
}

// callsCopyRepo reports whether astFile contains a call to CopyRepo qualified by gitkitAlias.
// It matches the selector on the AST, not on raw text.
func callsCopyRepo(astFile *ast.File, gitkitAlias string) bool {
	found := false
	ast.Inspect(astFile, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name != "CopyRepo" {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != gitkitAlias {
			return true
		}
		found = true
		return false
	})
	return found
}
