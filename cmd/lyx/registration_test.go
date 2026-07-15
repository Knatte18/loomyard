// registration_test.go is a repo-wide guard that verifies every internal package
// exposing func Command() *cobra.Command is wired into newRoot() in cmd/lyx/main.go.
// It uses source-level AST analysis so it catches missed registrations at test time
// without executing any module code or performing live cobra tree introspection.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isCommandFunc reports whether fd is a top-level func Command() *cobra.Command.
// It checks: no receiver (not a method), no parameters, exactly one result of
// type *cobra.Command represented as an *ast.StarExpr over a cobra.Command SelectorExpr.
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

// TestRegistration_AllModulesRegistered discovers every internal/ package that
// declares func Command() *cobra.Command via AST analysis, then asserts each
// such package is registered in newRoot() via root.AddCommand(<pkg>.Command()).
//
// The guard exists to make it impossible to ship a new module whose Command()
// is never wired into the cobra root — the "exists => registered" invariant.
func TestRegistration_AllModulesRegistered(t *testing.T) {
	// Resolve the repo root from this test file's on-disk path.
	// This file lives at cmd/lyx/registration_test.go, so two filepath.Dir
	// calls walk up to the repo root — the same pattern used by
	// internal/hubgeometry/enforcement_test.go.
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location via runtime.Caller")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(testFile)))

	// --- Phase 1: walk internal/ and collect packages with Command() ---

	internalDir := filepath.Join(repoRoot, "internal")
	// discovered maps Go package name → true for every package under internal/
	// that declares func Command() *cobra.Command.
	// Assumption: the selector identifier used in root.AddCommand(<ident>.Command())
	// equals the Go package name declared in the source — this holds for every
	// internal package in this repo (none use import aliases in main.go).
	discovered := make(map[string]bool)

	err := filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip directories, test files, and non-Go files — only production
		// source matters for the "exists => registered" invariant.
		if d.IsDir() || strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			// Skip files that cannot be parsed (e.g. build-tag-guarded platform files).
			return nil
		}

		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if isCommandFunc(fd) {
				// Record the package name and stop scanning this file — a package
				// only needs one file to declare Command().
				discovered[f.Name.Name] = true
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk internal/: %v", err)
	}

	// Sanity sub-test: the walk must discover at least one Command() package so
	// that a silently-broken walk (wrong directory, all files skipped) does not
	// produce a vacuous all-pass result.
	t.Run("discovered_non_empty", func(t *testing.T) {
		if len(discovered) == 0 {
			t.Error("registration guard: no packages with func Command() *cobra.Command found in internal/; the AST walk may be misconfigured")
		}
	})

	// --- Phase 2: parse main.go and collect packages passed to root.AddCommand ---

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

	// registered maps Go package name → true for every <ident>.Command() argument
	// passed to root.AddCommand(...) in main.go.
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

	// --- Phase 3: assert discovered ⊆ registered ---

	// allowlist holds packages that expose func Command() *cobra.Command but are
	// intentionally not registered in newRoot() (for documented future exceptions).
	// Empty today — muxpoccli, the only prior entry, was deleted once the mux
	// module it was a proof-of-concept for was built and shipped.
	allowlist := map[string]bool{}

	for pkg := range discovered {
		if allowlist[pkg] {
			continue
		}
		if !registered[pkg] {
			t.Errorf(
				"package %q has func Command() *cobra.Command but is not registered in newRoot(); add %s.Command() to root.AddCommand(...) in cmd/lyx/main.go",
				pkg, pkg,
			)
		}
	}
}
