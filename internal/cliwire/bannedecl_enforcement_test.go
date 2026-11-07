// bannedecl_enforcement_test.go enforces the other half of the Cliwire Sole-Wiring Invariant:
// neither internal/webstercli nor internal/burlercli may declare a helper that cliwire's wiring prologue owns.
// Declaring one such helper re-implements part of the prologue rather than calling into it.
// The check catches that partial re-implementation even when the package still calls into cliwire for the rest.
// The Derive pin in internal/cliwire/callerset_enforcement_test.go alone would miss it.

package cliwire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// policedCliDirs are the repository-relative package directories checked for a re-declared wiring helper.
var policedCliDirs = []string{"internal/webstercli", "internal/burlercli"}

// bannedWiringDeclarations are the helper names a policed CLI package must never declare for itself.
// The list holds cliwire's current helper spellings and the older spellings the CLIs used.
// A copy of either is caught.
// Declaring one of these names in a policed package re-implements part of the prologue.
// The check covers a top-level function and a method alike.
//
// A re-implementation under a fresh name passes this check by construction:
// a name list cannot ban names it does not know.
// The Derive caller-set pin in callerset_enforcement_test.go catches a full bottom-up copy whatever its names;
// this check covers the partial copy that still calls into cliwire for the rest.
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
	// cliwire's current helper spellings.
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

// TestBannedDeclarations_CliPackagesCallIntoCliwire fails if a policed CLI package declares a banned name.
// A declaration counts whether it is a top-level function, a method, a package-level var, or a package-level const.
// It spawns no process, so it carries no build tag.
// It parses every non-test .go file under each policed directory.
// It inspects each top-level declaration and flags any whose name is banned.
// The match is on the AST, never on raw text, so a doc comment naming a function cannot trip it.
// _test.go files are skipped: the invariant is about production wiring, and a test helper is not a second copy of it.
//
//lym:guard
func TestBannedDeclarations_CliPackagesCallIntoCliwire(t *testing.T) {
	tiergate.Require(t, 0)
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
			"than re-implementing part of it (see PATTERN-cli-cobra)",
			strings.Join(policedCliDirs, " and "), failures)
	}
}

// bannedDeclNamesIn returns the banned names that astFile declares at the top level, in declaration order.
// It matches function and method declarations (*ast.FuncDecl).
// It matches package-level var and const specs (*ast.ValueSpec) whatever their right-hand side is.
// A walk over function declarations alone would miss a helper declared as a var holding a func literal.
// Such a var is the same re-implementation under the same name, so it is banned too.
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

// TestBannedDeclNamesIn tests the declaration walk directly on fixture sources.
// A banned name declared as a package-level var holding a func literal is a re-implementation,
// so the walk must catch it as it catches the same name declared with `func`.
// The match must still discriminate on name, so an unrelated package-level var is not a false positive.
//
//testtiming:keep a guard self-check: pins that the declaration walk catches each banned shape and spares an unrelated name, which the real-tree scan never exercises
func TestBannedDeclNamesIn(t *testing.T) {
	tiergate.Require(t, 0)
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
