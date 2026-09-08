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

// bannedWiringDeclarations are the nine helper names internal/webstercli and internal/burlercli used
// to declare for themselves before the wiring prologue moved into internal/cliwire. A package under
// policedCliDirs re-declaring any of them -- as a top-level function or as a method, since a method is
// exactly what caught a re-declared standaloneDefaultPlanDir -- is re-implementing part of the
// prologue rather than calling into it.
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
}

// TestBannedDeclarations_CliPackagesCallIntoCliwire verifies that neither internal/webstercli nor
// internal/burlercli declares a function or method named in bannedWiringDeclarations.
// It spawns no process, so it carries no build tag.
// It resolves the repository root from runtime.Caller(0) exactly as
// TestDeriveCallerSet_CliwireOnly does, then for each policed directory parses every non-_test.go .go
// file and inspects each top-level *ast.FuncDecl -- receiver or none -- flagging any whose Name.Name
// is banned.
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

			for _, decl := range astFile.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name == nil {
					continue
				}
				if !bannedWiringDeclarations[fn.Name.Name] {
					continue
				}
				relPath, _ := filepath.Rel(repoRoot, path)
				failures = append(failures, filepath.ToSlash(relPath)+": "+fn.Name.Name)
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
