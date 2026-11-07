// registration_test.go guards that every internal package with func Command() *cobra.Command is wired into newRoot().
// The check reads the source's AST, so it catches a missed registration at test time.
// It runs no module code and performs no live cobra tree introspection.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// isCommandFunc reports whether fd is func Command() *cobra.Command.
func isCommandFunc(fd *ast.FuncDecl) bool {
	if fd.Name.Name != "Command" {
		return false
	}
	if fd.Recv != nil {
		return false
	}
	if fd.Type.Params.NumFields() != 0 {
		return false
	}
	if fd.Type.Results == nil || fd.Type.Results.NumFields() != 1 {
		return false
	}
	star, ok := fd.Type.Results.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name == "cobra" && sel.Sel.Name == "Command"
}

// TestRegistration_AllModulesRegistered asserts every Command() package is registered in newRoot().
//
//lym:guard
func TestRegistration_AllModulesRegistered(t *testing.T) {
	tiergate.Require(t, 0)
	repoRoot := scankit.Root(t)

	// Phase 1: walk internal/ and collect packages with Command().
	discovered := make(map[string]bool)

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal"}}, func(sf *scankit.File) {
		f := sf.AST(t, parser.SkipObjectResolution)
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if isCommandFunc(fd) {
				discovered[f.Name.Name] = true
				break
			}
		}
	})
	scankit.RequireFloor(t, scanned, 50, "registration guard")

	// Sanity sub-test: discovery must be non-empty.
	t.Run("discovered_non_empty", func(t *testing.T) {
		if len(discovered) == 0 {
			t.Error("registration guard: no packages with func Command() *cobra.Command found in internal/; the AST walk may be misconfigured")
		}
	})

	// Phase 2: parse main.go and collect packages passed to root.AddCommand or root.SetHelpCommand.
	mainPath := filepath.Join(repoRoot, "cmd", "lym", "main.go")
	mainSrc, readErr := os.ReadFile(mainPath)
	if readErr != nil {
		t.Fatalf("could not read cmd/lym/main.go: %v", readErr)
	}

	fset := token.NewFileSet()
	mainFile, parseErr := parser.ParseFile(fset, mainPath, mainSrc, parser.SkipObjectResolution)
	if parseErr != nil {
		t.Fatalf("could not parse cmd/lym/main.go: %v", parseErr)
	}

	registered := make(map[string]bool)
	ast.Inspect(mainFile, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "AddCommand" && sel.Sel.Name != "SetHelpCommand") {
			return true
		}
		for _, arg := range call.Args {
			argCall, ok := arg.(*ast.CallExpr)
			if !ok {
				continue
			}
			argSel, ok := argCall.Fun.(*ast.SelectorExpr)
			if !ok || argSel.Sel.Name != "Command" {
				continue
			}
			ident, ok := argSel.X.(*ast.Ident)
			if !ok {
				continue
			}
			registered[ident.Name] = true
		}
		return true
	})

	// Phase 3: assert discovered ⊆ registered.
	// allowlist holds packages intentionally not registered in newRoot().
	allowlist := scankit.NewAllowlist(nil)

	for pkg := range discovered {
		if allowlist.Allowed(pkg) {
			continue
		}
		if !registered[pkg] {
			t.Errorf(
				"package %q has func Command() *cobra.Command but is not registered in newRoot(); add %s.Command() to root.AddCommand(...) in cmd/lym/main.go",
				pkg, pkg,
			)
		}
	}
	allowlist.RequireNoStale(t)
}
