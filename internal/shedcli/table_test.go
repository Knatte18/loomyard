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

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// allGenericVerbs names the four generic subcommands shedverbs.Verbs returns, in the order its own
// doc comment declares them. It is hardcoded here rather than derived by building a throwaway
// *shedverbs.Spec and reading each returned command's Name(), because Verbs' own texts argument
// would have to carry non-empty Use strings for Name() to report anything at all -- the documented
// four-name contract is the simpler, equally authoritative source.
var allGenericVerbs = []string{"run", "step", "status", "pause"}

// TestRecipes_KeySetIsExactlyLoomAndBatten asserts recipes' key set is exactly {"loom",
// "batten"} -- no more, no fewer.
func TestRecipes_KeySetIsExactlyLoomAndBatten(t *testing.T) {
	got := names()
	want := []string{"batten", "loom"}
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
// generic verbs, and that both loom and batten carry all four now that batch 7 gave batten a step
// verb (batch 6's own PreStep hook and kind mapping made it armable).
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

	batten, err := lookup("batten")
	if err != nil {
		t.Fatalf("lookup(batten): %v", err)
	}
	if len(batten.Verbs) != 4 {
		t.Errorf("batten.Verbs = %v; want all four generic verbs", batten.Verbs)
	}
}

// TestRecipes_KeySetMatchesShedrunVocabulary is the sync meta-test the overview's
// shedrun-owns-the-recipe-name-vocabulary Shared Decision requires: this table's key set must equal
// shedrun.RecipeNames() exactly, so the vocabulary internal/shedrun declares and the arming table
// internal/shedcli declares cannot silently drift apart, mirroring the refKind<->allRefKinds
// meta-test pattern already used elsewhere in this repo.
func TestRecipes_KeySetMatchesShedrunVocabulary(t *testing.T) {
	got := names()
	want := shedrun.RecipeNames()
	if len(got) != len(want) {
		t.Fatalf("names() = %v; want shedrun.RecipeNames() = %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("names()[%d] = %q; want shedrun.RecipeNames()[%d] = %q", i, got[i], i, want[i])
		}
	}
}

// TestRecipes_BootstrapVerbMatchesOwningModuleConstant is the sync meta-test this batch's whole shape
// rests on: each entry's BootstrapVerb must equal its own module's exported constant, mirroring
// TestRecipes_KeySetMatchesShedrunVocabulary's shape. Without it, the constants and the table are two
// hand-maintained records of one fact, and a stale copy fails silently in the worst direction: a stale
// "" copied into the loom entry would make batch 5 refuse --driver llm for the one recipe that
// supports it.
func TestRecipes_BootstrapVerbMatchesOwningModuleConstant(t *testing.T) {
	loom, err := lookup("loom")
	if err != nil {
		t.Fatalf("lookup(loom): %v", err)
	}
	if loom.BootstrapVerb != loomcli.BootstrapVerb {
		t.Errorf("recipes[loom].BootstrapVerb = %q; want loomcli.BootstrapVerb = %q", loom.BootstrapVerb, loomcli.BootstrapVerb)
	}

	batten, err := lookup("batten")
	if err != nil {
		t.Fatalf("lookup(batten): %v", err)
	}
	if batten.BootstrapVerb != battencli.BootstrapVerb {
		t.Errorf("recipes[batten].BootstrapVerb = %q; want battencli.BootstrapVerb = %q", batten.BootstrapVerb, battencli.BootstrapVerb)
	}
}

// TestRecipes_SeedLocationRuleMatchesTheRecipesOwnVerbs pins which entries carry a seed-location
// rule: batten's verbs run from prime alone, so its entry must refuse a seed elsewhere, while loom's
// verbs run in any drivable worktree and its entry carries no rule.
func TestRecipes_SeedLocationRuleMatchesTheRecipesOwnVerbs(t *testing.T) {
	if recipes["batten"].RefuseSeedAt == nil {
		t.Error("recipes[batten].RefuseSeedAt = nil; want battencli's own prime-only guard")
	}
	if recipes["loom"].RefuseSeedAt != nil {
		t.Error("recipes[loom].RefuseSeedAt is set; want nil, loom seeds wherever its verbs drive")
	}
}

// TestRecipes_BootstrapVerbHasBothAnEmptyAndANonEmptyEntry asserts the table cannot degenerate to
// all-empty or all-non-empty: at least one entry must have a non-empty BootstrapVerb and at least one
// must have an empty one. Every per-entry equality assertion above passes against a table where both
// entries are empty, and that table is exactly what a bad merge or an over-eager "initialise the new
// field" edit produces -- it would make batch 5's validator refuse every recipe while this test's
// sibling above still passed.
func TestRecipes_BootstrapVerbHasBothAnEmptyAndANonEmptyEntry(t *testing.T) {
	var sawEmpty, sawNonEmpty bool
	for _, e := range recipes {
		if e.BootstrapVerb == "" {
			sawEmpty = true
		} else {
			sawNonEmpty = true
		}
	}
	if !sawEmpty {
		t.Error("recipes: no entry has an empty BootstrapVerb; want at least one")
	}
	if !sawNonEmpty {
		t.Error("recipes: no entry has a non-empty BootstrapVerb; want at least one")
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
