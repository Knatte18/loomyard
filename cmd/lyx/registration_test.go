// registration_test.go is a repo-wide guard that verifies every internal package exposing func
// Command() *cobra.Command is wired into newRoot() in cmd/lyx/main.go.
// It uses source-level AST analysis so it catches missed registrations at test time without
// executing any module code or performing live cobra tree introspection.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// isCommandFunc reports whether fd is func Command() *cobra.Command.
func isCommandFunc(fd *ast.FuncDecl) bool {
	if fd.Name.Name != "Command" {
		return false
	}
	// Must be a package-level function, not a method on a receiver type.
	if fd.Recv != nil {
		return false
	}
	// No input parameters — Command() takes nothing.
	if fd.Type.Params.NumFields() != 0 {
		return false
	}
	// Exactly one result field.
	if fd.Type.Results == nil || fd.Type.Results.NumFields() != 1 {
		return false
	}
	// Result type must be *cobra.Command: a StarExpr whose X is a SelectorExpr
	// with pkg "cobra" and selector "Command".
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
//lyx:guard
func TestRegistration_AllModulesRegistered(t *testing.T) {
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

	// Phase 2: parse main.go and collect packages passed to root.AddCommand.
	mainPath := filepath.Join(repoRoot, "cmd", "lyx", "main.go")
	mainSrc, readErr := os.ReadFile(mainPath)
	if readErr != nil {
		t.Fatalf("could not read cmd/lyx/main.go: %v", readErr)
	}

	fset := token.NewFileSet()
	mainFile, parseErr := parser.ParseFile(fset, mainPath, mainSrc, parser.SkipObjectResolution)
	if parseErr != nil {
		t.Fatalf("could not parse cmd/lyx/main.go: %v", parseErr)
	}

	registered := make(map[string]bool)
	ast.Inspect(mainFile, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// Look for a selector call whose method is "AddCommand" (e.g. root.AddCommand).
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "AddCommand" {
			return true
		}
		// Each argument must be of the form <ident>.Command(); collect the ident name.
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
				"package %q has func Command() *cobra.Command but is not registered in newRoot(); add %s.Command() to root.AddCommand(...) in cmd/lyx/main.go",
				pkg, pkg,
			)
		}
	}
	allowlist.RequireNoStale(t)
}
