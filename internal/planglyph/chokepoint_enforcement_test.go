// chokepoint_enforcement_test.go pins planglyph's two batched-boundary chokepoints per the
// overview's Decision: ast-enforcement-idiom: every (*quarry.Repo).Resolve call site must sit
// inside resolveTargets (repo.go), which owns the ensureResolveCoverage length guard, and every
// quarry.Name call site must sit inside CanonicalizeHandles (handle.go), which owns the
// length-plus-echo guard. A second call site of either kind, anywhere else in this package, is
// exactly the bypass these pins exist to catch — the guarded chokepoint would simply not run.

package planglyph

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

// chokepointGuardedFuncs names the one function each pinned selector must be called from.
var chokepointGuardedFuncs = map[string]string{
	"Resolve": "resolveTargets",
	"Name":    "CanonicalizeHandles",
}

// chokepointCallSitesIn returns every "<sel>: <enclosing func>" pair astFile's top-level function
// and method bodies call, for each ast.CallExpr whose selector matches a key of
// chokepointGuardedFuncs.
//
// The Resolve match is deliberately any-receiver: resolveTargets owns the ensureResolveCoverage
// length guard for this package's one call to (*quarry.Repo).Resolve, and a second Resolve call
// site of any kind — through a different variable, a different receiver expression, anything — is
// exactly the bypass this pin exists to catch, so matching only a specific receiver type would
// miss it. The Name match is package-qualified (quarry.Name) because Name is a plain package-level
// function, not a method, so its own selector already names the package unambiguously.
func chokepointCallSitesIn(astFile *ast.File) []string {
	var hits []string
	for _, decl := range astFile.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		enclosing := fn.Name.Name

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			switch sel.Sel.Name {
			case "Resolve":
				hits = append(hits, "Resolve: "+enclosing)
			case "Name":
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "quarry" {
					hits = append(hits, "Name: "+enclosing)
				}
			}
			return true
		})
	}
	return hits
}

// TestChokepointCallSites_PinnedToTheirGuardedFunctions verifies that every planglyph production
// .go file's Resolve call sits inside resolveTargets and every quarry.Name call sits inside
// CanonicalizeHandles. It spawns no process, so it carries no build tag, and resolves the repo
// root from runtime.Caller(0) exactly as the cliwire precedent
// (internal/cliwire/bannedecl_enforcement_test.go) does. _test.go files are skipped: the invariant
// is about production wiring, not test helpers.
func TestChokepointCallSites_PinnedToTheirGuardedFunctions(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine planglyph source directory location")
	}
	planglyphDir := filepath.Dir(thisFile)

	var failures []string

	err := filepath.WalkDir(planglyphDir, func(path string, d fs.DirEntry, err error) error {
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
		astFile, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}

		relPath, _ := filepath.Rel(planglyphDir, path)
		for _, hit := range chokepointCallSitesIn(astFile) {
			parts := strings.SplitN(hit, ": ", 2)
			sel, enclosing := parts[0], parts[1]
			if enclosing != chokepointGuardedFuncs[sel] {
				failures = append(failures, relPath+": "+hit)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", planglyphDir, err)
	}

	if len(failures) > 0 {
		t.Errorf("planglyph chokepoint invariant violated: %v -- route a new resolve call through "+
			"resolveTargets and a new naming call through CanonicalizeHandles, or move the coverage "+
			"guard with the call -- never add an unguarded batched quarry boundary", failures)
	}
}

// TestChokepointCallSitesIn_SeededSelfTest is the matcher's own seeded self-test, per the
// overview's Decision: ast-enforcement-idiom: a synthetic source string carrying an out-of-place
// call for each pinned selector must be caught by chokepointCallSitesIn before the real-tree
// assertion above is trusted at all.
func TestChokepointCallSitesIn_SeededSelfTest(t *testing.T) {
	const src = `package fakeplanglyph

func notResolveTargets(x *fakeRepo) {
	x.Resolve(nil)
}

func notCanonicalizeHandles() {
	quarry.Name(nil)
}
`
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "fakeplanglyph.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}

	got := chokepointCallSitesIn(astFile)
	want := map[string]bool{
		"Resolve: notResolveTargets":   true,
		"Name: notCanonicalizeHandles": true,
	}
	if len(got) != len(want) {
		t.Fatalf("chokepointCallSitesIn(seeded fixture) = %v; want %d hit(s) matching %v", got, len(want), want)
	}
	for _, hit := range got {
		if !want[hit] {
			t.Errorf("chokepointCallSitesIn(seeded fixture) produced unexpected hit %q", hit)
		}
	}
}
