// gogitcaller_test.go machine-checks PATTERN-gogit-read-helper: in internal/gitrepo's own non-test source, readGoGit is the only function that calls goGit, so no go-git read can bypass its whole-read retry.
// It is untagged and spawns nothing, a pure go/ast scan.

package gitrepo

import (
	"go/ast"
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// goGitCallerMinScannedFiles is the vacuous-scan floor: fewer non-test .go files than this means the directory resolution is misconfigured rather than the package having shrunk.
const goGitCallerMinScannedFiles = 5

// TestGoGitCallers_OnlyReadGoGitCallsGoGit fails when any function of internal/gitrepo's non-test source other than readGoGit calls goGit, and when the scan finds no call from readGoGit at all.
// See PATTERN-gogit-read-helper.
func TestGoGitCallers_OnlyReadGoGitCallsGoGit(t *testing.T) {
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
		t.Errorf("PATTERN-gogit-read-helper violated:\n%s", strings.Join(violations, "\n"))
	}
}
