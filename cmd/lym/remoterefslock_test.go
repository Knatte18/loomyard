// remoterefslock_test.go enforces `PATTERN-remote-refs-lock` for internal/fabricengine: a fetch there that writes remote-tracking refs goes through gitrepo, which takes the repository's remote-refs lock.
// The scan finds every git argument list holding "fetch" and fails on one that neither carries "--refmap=", which writes no tracking ref, nor sits in a function the allowlist names.

package main

import (
	"go/ast"
	"go/token"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// remoteRefsLockFetchAllowlist names the files, or as "<file>:<function>" the functions, of internal/fabricengine whose unlocked fetch is deliberate.
var remoteRefsLockFetchAllowlist = []scankit.Entry{
	{Key: "internal/fabricengine/recordsprobe.go", Why: "the file only fetches into a private probe clone, whose tracking refs no other lym call shares"},
	{Key: "internal/fabricengine/originreconcile.go:fetchMovedHeads", Why: "runs inside judgeAgainstOrigin, whose caller holds the repository's remote-refs lock"},
}

// remoteRefsLockMinScannedFiles is the vacuous-scan floor for the walk of internal/fabricengine.
const remoteRefsLockMinScannedFiles = 20

// TestRemoteRefsLock_FabricengineFetchesWritingTrackingRefsGoThroughGitrepo fails on an argument list in internal/fabricengine's non-test source that holds "fetch" without "--refmap=", outside the allowlist.
//
//lym:guard
func TestRemoteRefsLock_FabricengineFetchesWritingTrackingRefsGoThroughGitrepo(t *testing.T) {
	tiergate.Require(t, 0)
	allowlist := scankit.NewAllowlist(remoteRefsLockFetchAllowlist)

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/fabricengine"}, Shallow: true}, func(f *scankit.File) {
		file := f.AST(t, 0)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if !ok || !holdsFetchArgument(literal) || holdsString(literal, "--refmap=") {
					return true
				}
				if !allowlist.Allowed(f.Rel) && !allowlist.Allowed(f.Rel+":"+fn.Name.Name) {
					position := f.FileSet().Position(literal.Pos())
					t.Errorf("`PATTERN-remote-refs-lock` violated: %s:%d runs a git fetch that writes tracking refs outside gitrepo; use gitrepo's FetchWithArgs, which takes the remote-refs lock", f.Rel, position.Line)
				}
				return true
			})
		}
	})

	scankit.RequireFloor(t, scanned, remoteRefsLockMinScannedFiles, "remote-refs lock fetch guard")
	allowlist.RequireNoStale(t)
}

// holdsFetchArgument reports whether literal is a []string with the string "fetch" among its elements.
func holdsFetchArgument(literal *ast.CompositeLit) bool {
	array, ok := literal.Type.(*ast.ArrayType)
	if !ok {
		return false
	}
	if element, ok := array.Elt.(*ast.Ident); !ok || element.Name != "string" {
		return false
	}
	return holdsString(literal, "fetch")
}

// holdsString reports whether any element of literal is the string value want.
func holdsString(literal *ast.CompositeLit, want string) bool {
	for _, element := range literal.Elts {
		if stringValue(element) == want {
			return true
		}
	}
	return false
}

// stringValue returns the value of a string literal expression, or "" for any other expression.
func stringValue(expr ast.Expr) string {
	basic, ok := expr.(*ast.BasicLit)
	if !ok || basic.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(basic.Value)
	if err != nil {
		return ""
	}
	return value
}
