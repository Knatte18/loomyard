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
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedDeriveCallerDir is the only package directory (relative to the repository root) whose
// production code may call standalonestate.Derive.
const allowedDeriveCallerDir = "internal/cliwire"

// minDeriveScanFiles is the vacuous-scan floor for the caller-set walk.
const minDeriveScanFiles = 100

// TestDeriveCallerSet_CliwireOnly verifies that no production file outside internal/cliwire calls
// internal/standalonestate's Derive.
// It spawns no process, so it carries no build tag.
// It parses every non-_test.go .go file under the WHOLE repository — not just internal/ and cmd/,
// since a caller under tools/ or a future top-level directory is exactly as much a production caller
// (crucible round fable-high-r7, F3) — under scankit's shared skip set, excluding
// internal/standalonestate itself (whose own definition and doc comments name Derive) and excluding
// allowedDeriveCallerDir, looking for a selector call expression whose receiver identifier is this
// file's standalonestate import and whose selected name is Derive.
// A DOT-import of standalonestate is refused outright: it makes every Derive call a bare identifier
// the selector match cannot see, and no production file has a legitimate reason to dot-import a
// state-derivation package (same round, same finding).
// The match is on the AST, never on raw text, so a doc comment naming the qualified call cannot trip
// it.
//
//lyx:guard
func TestDeriveCallerSet_CliwireOnly(t *testing.T) {
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
			"(see PATTERN-cliwire-sole-wiring)",
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

// TestCallsDerive is a direct unit test over callsDerive rather than a planted whole-repo fixture, so each regression lives beside the function it protects.
// A package that captures standalonestate.Derive as a function value first, rather than calling it directly, is exactly as much a second production caller as a direct-call package is, and must be caught the same way (crucible round sonnet-xhigh-r8, CW-1).
// The ordinary direct-call form the pre-fix walk already caught must still be caught after widening the match to bare selector expressions.
// The widened match must still discriminate on both the selected name and the receiver alias, so an unrelated method named Derive is not a false positive.
//
//testtiming:keep a guard self-check: pins that the selector walk catches a direct call and a captured function value and spares an unrelated Derive, which the real-tree scan never exercises
func TestCallsDerive(t *testing.T) {
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
