// bannedecl_enforcement_test.go enforces the other half of the Cliwire Sole-Wiring Invariant: neither
// internal/webstercli nor internal/burlercli may re-declare any of the helper names that used to carry
// the two CLIs' own copy of cliwire's wiring prologue. A package re-declaring one of these names is
// re-implementing part of the prologue rather than calling into it -- the partial re-implementation
// this check exists to catch even when the package still calls into cliwire for the rest, which is
// exactly what internal/cliwire/callerset_enforcement_test.go's Derive pin alone would miss.

package cliwire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// policedCliDirs are the repository-relative package directories checked for a re-declared wiring
// helper.
var policedCliDirs = []string{"internal/webstercli", "internal/burlercli"}

// bannedWiringDeclarations are the helper names a policed CLI package must never declare for
// itself: the nine names internal/webstercli and internal/burlercli used before the wiring
// prologue moved into internal/cliwire, PLUS cliwire's own current helper spellings — re-declaring
// a helper under the very name cliwire gives it is the most natural way to copy one back out, and
// the original list only banned the OLD spellings (crucible round fable-high-r7, F4). A package
// under policedCliDirs re-declaring any of them -- as a top-level function or as a method, since a
// method is exactly what caught a re-declared standaloneDefaultPlanDir -- is re-implementing part
// of the prologue rather than calling into it.
//
// The stated residual: a re-implementation under a genuinely FRESH name passes this check by
// construction — a name list cannot ban names it does not know. The Derive caller-set pin
// (callerset_enforcement_test.go) is what catches a full bottom-up copy regardless of naming; this
// check exists for the partial copy that still calls into cliwire for the rest.
var bannedWiringDeclarations = map[string]bool{
	"resolveStandaloneTarget":        true,
	"repositoryRootOf":               true,
	"refuseNestedStandaloneGeometry": true,
	"normalizeForContainment":        true,
	"pathContains":                   true,
	"resolveToldDir":                 true,
	"samePlanDir":                    true,
	"standalonePlanDirHasContent":    true,
	"standaloneDefaultPlanDir":       true,
	// cliwire's own current spellings, banned alongside the historical ones above.
	"RepositoryRootOf":            true,
	"NormalizeForContainment":     true,
	"ResolveToldDir":              true,
	"SamePlanDir":                 true,
	"ResolvePlanDir":              true,
	"ResolveStandalone":           true,
	"RefuseTargetDirInHubMode":    true,
	"RefuseUnreadableStencilsDir": true,
	"planDirHasContent":           true,
	"resolvePlanDir":              true,
	"refuseTargetDirInHubMode":    true,
	"refuseUnreadableStencilsDir": true,
}

// TestBannedDeclarations_CliPackagesCallIntoCliwire verifies that neither internal/webstercli nor
// internal/burlercli declares a function, method, package-level var, or package-level const named
// in bannedWiringDeclarations.
// It spawns no process, so it carries no build tag.
// It parses every non-_test.go .go file under each policed directory and inspects every top-level declaration via bannedDeclNamesIn, flagging any whose declared name is banned.
// The match is on the AST, never on raw text, so a doc comment naming a function cannot trip it.
// _test.go files are skipped: the invariant is about production wiring, and a test helper is not a
// second copy of it.
func TestBannedDeclarations_CliPackagesCallIntoCliwire(t *testing.T) {
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{Roots: policedCliDirs}, func(f *scankit.File) {
		for _, name := range bannedDeclNamesIn(f.AST(t, parser.ParseComments)) {
			failures = append(failures, f.Rel+": "+name)
		}
	})
	scankit.RequireFloor(t, scanned, len(policedCliDirs), "banned wiring declaration scan")

	if len(failures) > 0 {
		t.Errorf("Cliwire Sole-Wiring Invariant violated: %s re-declares a wiring helper cliwire "+
			"already owns: %v -- a <module>cli must call into internal/cliwire's shared prologue rather "+
			"than re-implementing part of it (see PATTERN-cliwire-sole-wiring)",
			strings.Join(policedCliDirs, " and "), failures)
	}
}

// bannedDeclNamesIn returns every name astFile declares at the TOP level, in declaration order,
// that appears in bannedWiringDeclarations -- as a function or method (*ast.FuncDecl), or as a
// package-level var/const (*ast.GenDecl over a *ast.ValueSpec), whichever shape the RHS happens to
// take.
//
// The var/const half exists because a *ast.FuncDecl-only walk misses a helper re-declared as
// `var resolveStandaloneTarget = func(cwd, flag string) (string, error) { ... }` (crucible round
// sonnet-xhigh-r8, CW-2): that produces an identically-named, identically-callable package-level
// symbol -- exactly the kind of partial-copy-under-a-known-name this check's own doc comment says
// a re-declared METHOD (an FuncDecl shape the original nine-name list already covered) motivated
// checking receivers alongside plain functions for. A var holding a func literal is one further
// spelling of the same hazard, not a different one, so it is banned the same way regardless of what
// the RHS expression actually is -- the declared NAME is what matters, not whether it happens to be
// initialized with a func literal, a nil, or anything else.
func bannedDeclNamesIn(astFile *ast.File) []string {
	var names []string
	for _, decl := range astFile.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name != nil && bannedWiringDeclarations[d.Name.Name] {
				names = append(names, d.Name.Name)
			}
		case *ast.GenDecl:
			if d.Tok != token.VAR && d.Tok != token.CONST {
				continue
			}
			for _, spec := range d.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, ident := range vs.Names {
					if bannedWiringDeclarations[ident.Name] {
						names = append(names, ident.Name)
					}
				}
			}
		}
	}
	return names
}

// TestBannedDeclNamesIn is a direct unit test over bannedDeclNamesIn rather than a planted
// whole-repo fixture, so each regression lives beside the function it protects.
// A banned helper re-declared as a package-level var holding a func literal is exactly as much a
// re-implementation as the same name declared with `func`, and must be caught the same way
// (crucible round sonnet-xhigh-r8, CW-2).
// The ordinary `func` form the pre-fix walk already caught must still be caught after widening the
// match to package-level var/const declarations.
// The widened match must still discriminate on name, so an ordinary unrelated package-level var is
// not a false positive.
//
//testtiming:keep a guard self-check: pins that the declaration walk catches each banned shape and spares an unrelated name, which the real-tree scan never exercises
func TestBannedDeclNamesIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "CatchesVarFuncLiteral",
			src: `package fakecli

var resolveStandaloneTarget = func(cwd, flag string) (string, error) {
	return cwd + flag, nil
}
`,
			want: []string{"resolveStandaloneTarget"},
		},
		{
			name: "FuncDeclStillCaught",
			src: `package fakecli

func resolveStandaloneTarget(cwd, flag string) (string, error) {
	return cwd + flag, nil
}
`,
			want: []string{"resolveStandaloneTarget"},
		},
		{
			name: "UnrelatedVarNotCaught",
			src: `package fakecli

var somethingElseEntirely = 42
`,
			want: nil,
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

			if got := bannedDeclNamesIn(astFile); !slices.Equal(got, tt.want) {
				t.Errorf("bannedDeclNamesIn() = %v; want %v", got, tt.want)
			}
		})
	}
}
