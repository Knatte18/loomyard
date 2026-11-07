// callerset_enforcement_test.go pins the sole-wiring invariant:
// internal/cliwire is the only production package that calls internal/standalonestate's Derive.
// Standalone-capable CLIs get the derived state directory from cliwire's ResolveStandalone.
// Test files are exempt: they call Derive directly to build fixtures or to assert the derivation end-to-end.

package cliwire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedDeriveCallerDir is the only package directory (relative to the repository root) whose
// production code may call standalonestate.Derive.
const allowedDeriveCallerDir = "internal/cliwire"

// minDeriveScanFiles is the vacuous-scan floor for the caller-set walk.
const minDeriveScanFiles = 100

// TestDeriveCallerSet_CliwireOnly verifies that no production file outside internal/cliwire calls
// internal/standalonestate's Derive.
// It spawns no process, so it carries no build tag.
// It parses every non-_test.go .go file under the repository root,
// so a caller under tools/ or any other top-level directory is caught.
// Files are read under scankit's shared skip set.
// The walk excludes internal/standalonestate itself, since its definition and doc comments name Derive.
// It also skips allowedDeriveCallerDir.
// It matches a selector call on this file's standalonestate import whose selected name is Derive.
// The match runs on the AST, not raw text, so a doc comment naming the qualified call cannot trip it.
// A dot-import of standalonestate is refused outright:
// it turns every Derive call into a bare identifier the selector match cannot see.
//
//lym:guard
func TestDeriveCallerSet_CliwireOnly(t *testing.T) {
	tiergate.Require(t, 0)
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
		if strings.HasPrefix(f.Rel, "internal/standalonestate/") || strings.HasPrefix(f.Rel, allowedDeriveCallerDir+"/") {
			return
		}
		astFile := f.AST(t, parser.ParseComments)

		standalonestateAlias, imported := standalonestateImportAlias(astFile)
		if !imported {
			return
		}

		if standalonestateAlias == "." {
			failures = append(failures, f.Rel+" (dot-imports standalonestate, hiding every Derive call from this pin)")
			return
		}

		if callsDerive(astFile, standalonestateAlias) {
			failures = append(failures, f.Rel)
		}
	})
	scankit.RequireFloor(t, scanned, minDeriveScanFiles, "standalonestate.Derive caller-set scan")

	if len(failures) > 0 {
		t.Errorf("Cliwire Sole-Wiring Invariant violated: standalonestate.Derive is pinned to %s alone "+
			"in production code, but found call sites in: %v -- a <module>cli must derive its state "+
			"directory through cliwire.Module.ResolveStandalone rather than calling Derive itself "+
			"(see PATTERN-cli-cobra)",
			allowedDeriveCallerDir, failures)
	}
}

// standalonestateImportAlias returns the local identifier astFile uses to refer to the
// standalonestate package, and whether astFile imports standalonestate at all. It honors an explicit
// import alias, falling back to the default package name "standalonestate" when the import carries no
// alias.
func standalonestateImportAlias(astFile *ast.File) (alias string, imported bool) {
	const standalonestateImportPath = `"github.com/Knatte18/loomyard/internal/standalonestate"`
	for _, imp := range astFile.Imports {
		if imp.Path.Value != standalonestateImportPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name, true
		}
		return "standalonestate", true
	}
	return "", false
}

// callsDerive reports whether astFile names standalonestate's Derive, either by calling it
// directly (standalonestate.Derive(...)) or by capturing it as a function value (var deriveFn =
// standalonestate.Derive, later invoked as deriveFn(...)).
// It matches on the AST selector expression rather than raw text.
//
// It walks every *ast.SelectorExpr in the file, not only those in a CallExpr.Fun position,
// so the right-hand side of a var or const spec that captures Derive is matched too.
// A selector naming Derive counts as naming the symbol whether or not it sits inside a call.
func callsDerive(astFile *ast.File, standalonestateAlias string) bool {
	found := false
	ast.Inspect(astFile, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name != "Derive" {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != standalonestateAlias {
			return true
		}
		found = true
		return false
	})
	return found
}

// TestCallsDerive is a direct unit test over callsDerive, so each regression lives beside the function it protects.
// Capturing Derive as a function value makes a package a production caller, just as a direct call does,
// so it must be caught the same way.
// Direct calls must still be caught after the match widens to bare selector expressions.
// The match must still discriminate on both the selected name and the receiver alias,
// so an unrelated method named Derive is not a false positive.
//
//testtiming:keep a guard self-check: pins that the selector walk catches a direct call and a captured function value and spares an unrelated Derive, which the real-tree scan never exercises
func TestCallsDerive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "CatchesFunctionValueIndirection",
			src: `package fakecli

import "github.com/Knatte18/loomyard/internal/standalonestate"

var deriveFn = standalonestate.Derive

func resolve(target string) (string, string, error) {
	return deriveFn(target)
}
`,
			want: true,
		},
		{
			name: "DirectCallStillCaught",
			src: `package fakecli

import "github.com/Knatte18/loomyard/internal/standalonestate"

func resolve(target string) (string, string, error) {
	return standalonestate.Derive(target)
}
`,
			want: true,
		},
		{
			name: "UnrelatedSelectorNotCaught",
			src: `package fakecli

import "fmt"

type thing struct{}

func (thing) Derive() string { return "" }

func resolve() string {
	t := thing{}
	fmt.Sprintln(t.Derive())
	return ""
}
`,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()
			astFile, err := parser.ParseFile(fset, "fakecli.go", tt.src, 0)
			if err != nil {
				t.Fatalf("parse fixture source: %v", err)
			}

			if got := callsDerive(astFile, "standalonestate"); got != tt.want {
				t.Errorf("callsDerive() = %v; want %v", got, tt.want)
			}
		})
	}
}
