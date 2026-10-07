// callerset_enforcement_test.go enforces the gitkit Leaf Invariant's CopyRepo pin: internal/lyxcwd
// is the only caller of this package's CopyRepo, since every other package takes a real hub from
// internal/hubforge instead. This is the guard that catches a later migration leaving a hub-shaped
// call site on the primitive repo fixture rather than migrating it onto hubforge's real-hub factory.

package gitkit

import (
	"go/ast"
	"go/parser"
	"path"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedCopyRepoCallerDir is the only package directory (relative to the repository root) allowed
// to call this package's CopyRepo.
const allowedCopyRepoCallerDir = "internal/lyxcwd"

// minCopyRepoScanFiles is the vacuous-scan floor for the caller-set walk.
const minCopyRepoScanFiles = 100

// TestCopyRepoCallerSet_LyxcwdOnly verifies that no file outside internal/lyxcwd calls this
// package's CopyRepo function.
// It spawns no git, so it carries no build tag.
// It parses every .go file under internal/ and cmd/ (excluding internal/gitkit/ itself, whose own
// doc comment and definition mention CopyRepo by name) looking for a selector call expression whose
// receiver identifier is this file's gitkit import and whose selected name is CopyRepo.
// The match is on the AST, never on raw text, so a doc comment mentioning the qualified call cannot
// trip it.
//
//lyx:guard
func TestCopyRepoCallerSet_LyxcwdOnly(t *testing.T) {
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

// gitkitImportAlias returns the local identifier astFile uses to refer to the gitkit package, and
// whether astFile imports gitkit at all. It honors an explicit import alias, falling back to the
// default package name "gitkit" when the import carries no alias.
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

// callsCopyRepo reports whether astFile contains a call expression whose receiver is gitkitAlias
// and whose selected method name is CopyRepo, matched on the AST selector expression rather than on
// raw text.
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
