// callerset_enforcement_test.go enforces half of the Cliwire Sole-Wiring Invariant: internal/cliwire
// is the only PRODUCTION caller of internal/standalonestate's Derive, since every standalone-capable
// CLI is supposed to reach the derived state directory through this package's ResolveStandalone
// rather than deriving one of its own. This is the guard that catches a whole third copy of the
// wiring built from the bottom up.
//
// The pin is production-only: it skips every _test.go file, following
// internal/treadleengine/seam_enforcement_test.go's own skip rather than
// internal/gitkit/callerset_enforcement_test.go's package-directory-only exclusion. Ten test call
// sites legitimately remain and call Derive directly to build fixtures or to assert the real
// derivation end-to-end -- they are not a second copy of the wiring, and forcing them through cliwire
// would make packages with no reason to depend on it do so: four in
// internal/burlercli/wiring_test.go, two in internal/webstercli/wiring_test.go, two in
// internal/webstercli/cli_integration_test.go, and two in
// internal/standalonegeom/reedgeom_symlink_integration_test.go.

package cliwire

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

// allowedDeriveCallerDir is the only package directory (relative to the repository root) whose
// production code may call standalonestate.Derive.
const allowedDeriveCallerDir = "internal/cliwire"

// TestDeriveCallerSet_CliwireOnly verifies that no production file outside internal/cliwire calls
// internal/standalonestate's Derive.
// It spawns no process, so it carries no build tag.
// It resolves the repository root from runtime.Caller(0) by walking up from this file's directory,
// then parses every non-_test.go .go file under the WHOLE repository — not just internal/ and cmd/,
// since a caller under tools/ or a future top-level directory is exactly as much a production caller
// (crucible round fable-high-r7, F3) — skipping .git and testdata directories, excluding
// internal/standalonestate itself (whose own definition and doc comments name Derive) and excluding
// allowedDeriveCallerDir, looking for a selector call expression whose receiver identifier is this
// file's standalonestate import and whose selected name is Derive.
// A DOT-import of standalonestate is refused outright: it makes every Derive call a bare identifier
// the selector match cannot see, and no production file has a legitimate reason to dot-import a
// state-derivation package (same round, same finding).
// The match is on the AST, never on raw text, so a doc comment naming the qualified call cannot trip
// it.
func TestDeriveCallerSet_CliwireOnly(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine cliwire source directory location")
	}
	cliwireDir := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(cliwireDir)) // internal/cliwire -> internal -> repo root

	var failures []string

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == repoRoot {
				return nil
			}
			if d.Name() == ".git" || d.Name() == "testdata" {
				return filepath.SkipDir
			}
			relDir, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return relErr
			}
			relDir = filepath.ToSlash(relDir)
			if relDir == "internal/standalonestate" || relDir == allowedDeriveCallerDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Logf("warning: failed to parse %s: %v", path, err)
			return nil
		}

		standalonestateAlias, imported := standalonestateImportAlias(astFile)
		if !imported {
			return nil
		}

		if standalonestateAlias == "." {
			relPath, _ := filepath.Rel(repoRoot, path)
			failures = append(failures, filepath.ToSlash(relPath)+" (dot-imports standalonestate, hiding every Derive call from this pin)")
			return nil
		}

		if callsDerive(astFile, standalonestateAlias) {
			relPath, _ := filepath.Rel(repoRoot, path)
			failures = append(failures, filepath.ToSlash(relPath))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk %s: %v", repoRoot, err)
	}

	if len(failures) > 0 {
		t.Errorf("Cliwire Sole-Wiring Invariant violated: standalonestate.Derive is pinned to %s alone "+
			"in production code, but found call sites in: %v -- a <module>cli must derive its state "+
			"directory through cliwire.Module.ResolveStandalone rather than calling Derive itself "+
			"(see CONSTRAINTS.md's Cliwire Sole-Wiring Invariant)",
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

// callsDerive reports whether astFile NAMES standalonestate's Derive -- either by calling it
// directly (standalonestate.Derive(...)) or by capturing it as a function value (var deriveFn =
// standalonestate.Derive, later invoked as deriveFn(...)) -- matched on the AST selector
// expression rather than on raw text.
//
// It walks every *ast.SelectorExpr in the file, not only ones sitting in a CallExpr.Fun position
// (crucible round sonnet-xhigh-r8, CW-1): the pre-fix walk matched only the direct-call shape, so a
// production file that captured Derive as a value first and called the captured identifier later
// reached Derive exactly as much as a direct call does, but the assignment's own selector
// expression -- standalonestate.Derive on the right-hand side of a var/const spec, unconnected to
// any CallExpr -- was invisible to it. A selector naming Derive is exactly as much "this file
// names the symbol" whether or not it happens to sit inside a call.
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

// TestCallsDerive_CatchesFunctionValueIndirection is CW-1's own regression test (crucible round
// sonnet-xhigh-r8): a package that captures standalonestate.Derive as a function value first,
// rather than calling it directly, is exactly as much a second production caller as a direct-call
// package is, and must be caught the same way. Direct unit test over callsDerive rather than a
// planted whole-repo fixture, so the regression lives beside the function it protects.
func TestCallsDerive_CatchesFunctionValueIndirection(t *testing.T) {
	const src = `package fakecli

import "github.com/Knatte18/loomyard/internal/standalonestate"

var deriveFn = standalonestate.Derive

func resolve(target string) (string, string, error) {
	return deriveFn(target)
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakecli.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	if !callsDerive(astFile, "standalonestate") {
		t.Error("callsDerive() = false; want true -- a var capturing standalonestate.Derive as a function value names Derive exactly as much as a direct call does")
	}
}

// TestCallsDerive_DirectCallStillCaught is a plain-shape sanity check alongside the indirection
// regression above: the ordinary direct-call form the pre-fix walk already caught must still be
// caught after widening the match to bare selector expressions.
func TestCallsDerive_DirectCallStillCaught(t *testing.T) {
	const src = `package fakecli

import "github.com/Knatte18/loomyard/internal/standalonestate"

func resolve(target string) (string, string, error) {
	return standalonestate.Derive(target)
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakecli.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	if !callsDerive(astFile, "standalonestate") {
		t.Error("callsDerive() = false; want true -- a direct standalonestate.Derive(...) call must still be caught")
	}
}

// TestCallsDerive_UnrelatedSelectorNotCaught confirms the widened match still discriminates on
// both the selected name and the receiver alias -- an unrelated method named Derive, or a Derive
// selector off some other package, must not false-positive.
func TestCallsDerive_UnrelatedSelectorNotCaught(t *testing.T) {
	const src = `package fakecli

import "fmt"

type thing struct{}

func (thing) Derive() string { return "" }

func resolve() string {
	t := thing{}
	fmt.Sprintln(t.Derive())
	return ""
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakecli.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	if callsDerive(astFile, "standalonestate") {
		t.Error("callsDerive() = true; want false -- an unrelated type's own Derive method is not standalonestate.Derive")
	}
}
