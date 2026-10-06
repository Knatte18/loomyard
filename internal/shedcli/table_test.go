// table_test.go covers table.go's recipes map and its two accessors. It stays untagged tier 1: it
// calls lookup and names alone, neither of which resolves cwd or spawns git.

package shedcli

import (
	"go/ast"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// tableScanMinFiles is the plausible floor for how many production .go files internal/shedcli holds.
const tableScanMinFiles = 3

// allGenericVerbs names the four generic subcommands shedverbs.Verbs returns, in the order its own
// doc comment declares them. It is hardcoded here rather than derived by building a throwaway
// *shedverbs.Spec and reading each returned command's Name(), because Verbs' own texts argument
// would have to carry non-empty Use strings for Name() to report anything at all -- the documented
// four-name contract is the simpler, equally authoritative source.
var allGenericVerbs = []string{"run", "step", "status", "pause", "goto"}

// TestRecipes_KeySet asserts recipes' key set is exactly {"batten", "loom"} and equals shedrun.RecipeNames(), so the vocabulary internal/shedrun declares and the arming table internal/shedcli declares cannot silently drift apart.
//
//testtiming:keep pins the table's key set against both literal and the shedrun vocabulary, which its covering test does not
func TestRecipes_KeySet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want []string
	}{
		{"exactly loom and batten", []string{"batten", "loom"}},
		{"matches the shedrun vocabulary", shedrun.RecipeNames()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := names()
			if len(got) != len(tt.want) {
				t.Fatalf("names() = %v; want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("names()[%d] = %q; want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestRecipes_Entries pins each recipe entry: its Verbs are all five generic verbs, its BootstrapVerb equals its own module's exported constant, and its seed-location rule is set only where the recipe's verbs run from prime alone.
// The table also cannot degenerate to all-empty or all-non-empty bootstrap verbs, which every per-entry equality passes against and which a bad merge or an over-eager "initialise the new field" edit produces.
//
//testtiming:keep pins each recipe entry's verbs, bootstrap verb and seed-location rule, which its covering test does not
func TestRecipes_Entries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		recipe            string
		wantBootstrapVerb string
		wantSeedRule      bool
	}{
		{"loom", loomcli.BootstrapVerb, false},
		{"batten", battencli.BootstrapVerb, true},
	}
	for _, tt := range tests {
		t.Run(tt.recipe, func(t *testing.T) {
			t.Parallel()
			e, err := lookup(tt.recipe)
			if err != nil {
				t.Fatalf("lookup(%s): %v", tt.recipe, err)
			}
			for _, v := range e.Verbs {
				if !slices.Contains(allGenericVerbs, v) {
					t.Errorf("recipe %q declares verb %q, outside the generic set %v", tt.recipe, v, allGenericVerbs)
				}
			}
			if len(e.Verbs) != len(allGenericVerbs) {
				t.Errorf("%s.Verbs = %v; want all of %v", tt.recipe, e.Verbs, allGenericVerbs)
			}
			if e.BootstrapVerb != tt.wantBootstrapVerb {
				t.Errorf("recipes[%s].BootstrapVerb = %q; want the owning module's constant %q", tt.recipe, e.BootstrapVerb, tt.wantBootstrapVerb)
			}
			if (e.RefuseSeedAt != nil) != tt.wantSeedRule {
				t.Errorf("recipes[%s].RefuseSeedAt set = %v; want %v", tt.recipe, e.RefuseSeedAt != nil, tt.wantSeedRule)
			}
		})
	}

	var sawEmpty, sawNonEmpty bool
	for _, e := range recipes {
		if e.BootstrapVerb == "" {
			sawEmpty = true
		} else {
			sawNonEmpty = true
		}
	}
	if !sawEmpty || !sawNonEmpty {
		t.Errorf("recipes: empty BootstrapVerb entry present = %v, non-empty present = %v; want both", sawEmpty, sawNonEmpty)
	}
}

// TestTable_NoInitNoRegisterSeam is the table clause of the new Shed Verb-Set Invariant: an AST scan
// over this package's production files asserting no init() function is declared and no exported
// Register-shaped function exists, mirroring internal/shedrecipe's own registry shape and
// internal/shedverbs' own seam test style.
func TestTable_NoInitNoRegisterSeam(t *testing.T) {
	var initFound []string
	var registerFound []string

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/shedcli"}}, func(f *scankit.File) {
		for _, decl := range f.AST(t, 0).Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			if fn.Name.Name == "init" {
				initFound = append(initFound, f.Rel)
			}
			if strings.HasPrefix(fn.Name.Name, "Register") && fn.Name.IsExported() {
				registerFound = append(registerFound, f.Rel+": "+fn.Name.Name)
			}
		}
	})
	scankit.RequireFloor(t, scanned, tableScanMinFiles, "shedcli table scan")

	if len(initFound) > 0 {
		sort.Strings(initFound)
		t.Errorf("table clause violated; init() declared in: %v", initFound)
	}
	if len(registerFound) > 0 {
		sort.Strings(registerFound)
		t.Errorf("table clause violated; an exported Register-shaped function was found in: %v", registerFound)
	}
}
