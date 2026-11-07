// gogitcaller_test.go is a go/ast scan.
// It checks that readGoGit is the only function in internal/gitrepo's non-test source that calls goGit.
// Any other goGit call would bypass readGoGit's whole-read retry.

package gitrepo

import (
	"go/ast"
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// goGitCallerMinScannedFiles is the least number of non-test .go files the scan must find.
// A smaller count means the directory resolution is wrong, not that the package has shrunk.
const goGitCallerMinScannedFiles = 5

// TestGoGitCallers_OnlyReadGoGitCallsGoGit fails when a non-test function calls goGit outside readGoGit.
// The scan covers internal/gitrepo's non-test source, and it also fails when it finds no call from readGoGit at all.
func TestGoGitCallers_OnlyReadGoGitCallsGoGit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	var violations []string
	readGoGitCalls := 0

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/gitrepo"}, Shallow: true}, func(f *scankit.File) {
		file := f.AST(t, parser.SkipObjectResolution)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "goGit" {
					return true
				}
				if fn.Name.Name == "readGoGit" {
					readGoGitCalls++
					return true
				}
				violations = append(violations, f.Rel+": "+fn.Name.Name+" calls goGit")
				return true
			})
		}
	})

	scankit.RequireFloor(t, scanned, goGitCallerMinScannedFiles, "goGit caller guard")
	if readGoGitCalls == 0 {
		t.Error("the scan found no goGit call inside readGoGit; the scan no longer sees the helper it guards")
	}
	if len(violations) > 0 {
		t.Errorf("PATTERN-gitrepo-client-boundary violated:\n%s", strings.Join(violations, "\n"))
	}
}
