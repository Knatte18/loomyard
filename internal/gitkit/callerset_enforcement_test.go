// callerset_enforcement_test.go enforces the gitkit Leaf Invariant's CopyRepo pin: internal/lyxcwd
// is the only caller of this package's CopyRepo, since every other package takes a real hub from
// internal/hubforge instead. This is the guard that catches a later migration leaving a hub-shaped
// call site on the primitive repo fixture rather than migrating it onto hubforge's real-hub factory.

package gitkit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// allowedCopyRepoCallerDir is the only package directory (relative to the repository root) allowed
// to call this package's CopyRepo.
const allowedCopyRepoCallerDir = "internal/lyxcwd"

// TestCopyRepoCallerSet_LyxcwdOnly verifies that no file outside internal/lyxcwd calls this
// package's CopyRepo function.
// It spawns no git, so it carries no build tag.
// It resolves the repository root from runtime.Caller(0) by walking up from this file's directory,
// then parses every .go file under internal/ and cmd/ (excluding internal/gitkit/ itself, whose own
// doc comment and definition mention CopyRepo by name) looking for a selector call expression whose
// receiver identifier is this file's gitkit import and whose selected name is CopyRepo.
// The match is on the AST, never on raw text, so a doc comment mentioning the qualified call cannot
// trip it.
func TestCopyRepoCallerSet_LyxcwdOnly(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine gitkit source directory location")
	}
	gitkitDir := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(gitkitDir)) // internal/gitkit -> internal -> repo root

	var failures []string

	for _, sub := range []string{"internal", "cmd"} {
		root := filepath.Join(repoRoot, sub)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != root && filepath.Join(repoRoot, "internal", "gitkit") == path {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") {
				return nil
			}

			fset := token.NewFileSet()
			astFile, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				t.Logf("warning: failed to parse %s: %v", path, err)
				return nil
			}

			gitkitAlias, imported := gitkitImportAlias(astFile)
			if !imported {
				return nil
			}

			relDir, relErr := filepath.Rel(repoRoot, filepath.Dir(path))
			if relErr != nil {
				return relErr
			}
			relDir = filepath.ToSlash(relDir)
			if relDir == allowedCopyRepoCallerDir {
				return nil
			}

			if callsCopyRepo(astFile, gitkitAlias) {
				relPath, _ := filepath.Rel(repoRoot, path)
				failures = append(failures, filepath.ToSlash(relPath))
			}

			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk %s: %v", root, err)
		}
	}

	if len(failures) > 0 {
		t.Errorf("gitkit Leaf Invariant violated: CopyRepo is pinned to %s alone, "+
			"but found call sites in: %v -- every other package takes a real hub from "+
			"hubforge's real-hub factory instead (see CONSTRAINTS.md's gitkit Leaf Invariant)",
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
