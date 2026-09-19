// table_test.go covers table.go's recipes map and its two accessors. It stays untagged tier 1: it
// calls lookup and names alone, neither of which resolves cwd or spawns git.

package shedcli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// allGenericVerbs names the four generic subcommands shedverbs.Verbs returns, in the order its own
// doc comment declares them. It is hardcoded here rather than derived by building a throwaway
// *shedverbs.Spec and reading each returned command's Name(), because Verbs' own texts argument
// would have to carry non-empty Use strings for Name() to report anything at all -- the documented
// four-name contract is the simpler, equally authoritative source.
var allGenericVerbs = []string{"run", "step", "status", "pause"}

// TestRecipes_KeySetIsExactlyLoomAndLifecycle asserts recipes' key set is exactly {"loom",
// "lifecycle"} -- no more, no fewer.
func TestRecipes_KeySetIsExactlyLoomAndLifecycle(t *testing.T) {
	got := names()
	want := []string{"lifecycle", "loom"}
	if len(got) != len(want) {
		t.Fatalf("names() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("names()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// TestRecipes_VerbsIsSubsetOfGenericVerbs asserts every recipe's Verbs set is a subset of the four
// generic verbs, and that loom carries all four while lifecycle carries three.
func TestRecipes_VerbsIsSubsetOfGenericVerbs(t *testing.T) {
	generic := map[string]bool{}
	for _, v := range allGenericVerbs {
		generic[v] = true
	}

	for name, e := range recipes {
		for _, v := range e.Verbs {
			if !generic[v] {
				t.Errorf("recipe %q declares verb %q, outside the generic set %v", name, v, allGenericVerbs)
			}
		}
	}

	loom, err := lookup("loom")
	if err != nil {
		t.Fatalf("lookup(loom): %v", err)
	}
	if len(loom.Verbs) != 4 {
		t.Errorf("loom.Verbs = %v; want all four generic verbs", loom.Verbs)
	}

	lifecycle, err := lookup("lifecycle")
	if err != nil {
		t.Fatalf("lookup(lifecycle): %v", err)
	}
	if len(lifecycle.Verbs) != 3 {
		t.Errorf("lifecycle.Verbs = %v; want exactly three verbs", lifecycle.Verbs)
	}
	for _, v := range lifecycle.Verbs {
		if v == "step" {
			t.Errorf("lifecycle.Verbs = %v; must not include \"step\", which lifecycle has no analogue for", lifecycle.Verbs)
		}
	}
}

// TestLookup_UnknownNameNamesTheAvailableRecipes asserts an unknown recipe name's error names the
// available recipes.
func TestLookup_UnknownNameNamesTheAvailableRecipes(t *testing.T) {
	_, err := lookup("bogus-recipe")
	if err == nil {
		t.Fatal("lookup(bogus-recipe) = nil error; want a non-nil error")
	}
	for _, name := range names() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("lookup(bogus-recipe) error = %q; want it to name recipe %q", err.Error(), name)
		}
	}
}

// TestTable_NoInitNoRegisterSeam is the table clause of the new Shed Verb-Set Invariant: an AST scan
// over this package's production files asserting no init() function is declared and no exported
// Register-shaped function exists, mirroring internal/shedrecipe's own registry shape and
// internal/shedverbs' own seam test style.
func TestTable_NoInitNoRegisterSeam(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine shedcli source directory location")
	}
	pkgDir := filepath.Dir(file)

	var initFound []string
	var registerFound []string

	err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Logf("warning: failed to parse %s: %v", path, err)
			return nil
		}

		relPath, _ := filepath.Rel(pkgDir, path)

		for _, decl := range astFile.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			if fn.Name.Name == "init" {
				initFound = append(initFound, relPath)
			}
			if strings.HasPrefix(fn.Name.Name, "Register") && fn.Name.IsExported() {
				registerFound = append(registerFound, relPath+": "+fn.Name.Name)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk shedcli directory: %v", err)
	}

	if len(initFound) > 0 {
		sort.Strings(initFound)
		t.Errorf("table clause violated; init() declared in: %v", initFound)
	}
	if len(registerFound) > 0 {
		sort.Strings(registerFound)
		t.Errorf("table clause violated; an exported Register-shaped function was found in: %v", registerFound)
	}
}
