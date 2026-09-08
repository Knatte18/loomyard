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
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
// It resolves the repository root from runtime.Caller(0) exactly as
// TestDeriveCallerSet_CliwireOnly does, then for each policed directory parses every non-_test.go .go
// file and inspects every top-level declaration via bannedDeclNamesIn, flagging any whose declared
// name is banned.
// The match is on the AST, never on raw text, so a doc comment naming a function cannot trip it.
// _test.go files are skipped: the invariant is about production wiring, and a test helper is not a
// second copy of it.
func TestBannedDeclarations_CliPackagesCallIntoCliwire(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine cliwire source directory location")
	}
	cliwireDir := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(cliwireDir)) // internal/cliwire -> internal -> repo root

	var failures []string

	for _, policedDir := range policedCliDirs {
		dir := filepath.Join(repoRoot, filepath.FromSlash(policedDir))
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
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

			relPath, _ := filepath.Rel(repoRoot, path)
			for _, name := range bannedDeclNamesIn(astFile) {
				failures = append(failures, filepath.ToSlash(relPath)+": "+name)
			}

			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk %s: %v", dir, err)
		}
	}

	if len(failures) > 0 {
		t.Errorf("Cliwire Sole-Wiring Invariant violated: %s re-declares a wiring helper cliwire "+
			"already owns: %v -- a <module>cli must call into internal/cliwire's shared prologue rather "+
			"than re-implementing part of it (see CONSTRAINTS.md's Cliwire Sole-Wiring Invariant)",
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

// TestBannedDeclNamesIn_CatchesVarFuncLiteral is CW-2's own regression test (crucible round
// sonnet-xhigh-r8): a banned helper re-declared as a package-level var holding a func literal is
// exactly as much a re-implementation as the same name declared with `func`, and must be caught the
// same way. Direct unit test over bannedDeclNamesIn rather than a planted whole-repo fixture, so the
// regression lives beside the function it protects.
func TestBannedDeclNamesIn_CatchesVarFuncLiteral(t *testing.T) {
	const src = `package fakecli

var resolveStandaloneTarget = func(cwd, flag string) (string, error) {
	return cwd + flag, nil
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakecli.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	got := bannedDeclNamesIn(astFile)
	if len(got) != 1 || got[0] != "resolveStandaloneTarget" {
		t.Errorf("bannedDeclNamesIn() = %v; want [resolveStandaloneTarget] -- a var holding a func literal under a banned name must be caught exactly like a func declaration would be", got)
	}
}

// TestBannedDeclNamesIn_FuncDeclStillCaught is a plain-shape sanity check alongside the var
// regression above: the ordinary `func` form the pre-fix walk already caught must still be caught
// after widening the match to package-level var/const declarations.
func TestBannedDeclNamesIn_FuncDeclStillCaught(t *testing.T) {
	const src = `package fakecli

func resolveStandaloneTarget(cwd, flag string) (string, error) {
	return cwd + flag, nil
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakecli.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	got := bannedDeclNamesIn(astFile)
	if len(got) != 1 || got[0] != "resolveStandaloneTarget" {
		t.Errorf("bannedDeclNamesIn() = %v; want [resolveStandaloneTarget]", got)
	}
}

// TestBannedDeclNamesIn_UnrelatedVarNotCaught confirms the widened match still discriminates on
// name -- an ordinary, unrelated package-level var must not false-positive.
func TestBannedDeclNamesIn_UnrelatedVarNotCaught(t *testing.T) {
	const src = `package fakecli

var somethingElseEntirely = 42
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakecli.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	if got := bannedDeclNamesIn(astFile); len(got) != 0 {
		t.Errorf("bannedDeclNamesIn() = %v; want empty -- an unrelated var name is not a banned re-declaration", got)
	}
}
