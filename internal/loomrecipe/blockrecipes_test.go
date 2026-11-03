// blockrecipes_test.go pins the motif and arras recipes against the tapestry recipe they are cut from.

package loomrecipe

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBlockRecipes asserts motif holds tapestry's Discussion rows, arras expands to tapestry's rows plus the Motif block row, arras keeps tapestry's top-level keys, and arras builds under the loom contract.
func TestBlockRecipes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	parse := func(t *testing.T, source []byte) shedbuild.Recipe {
		t.Helper()
		recipe, err := shedbuild.Parse(source)
		if err != nil {
			t.Fatalf("shedbuild.Parse() = _, %v; want nil", err)
		}
		return recipe
	}
	tapestry := parse(t, recipes.TapestryRecipe)

	t.Run("motif holds tapestry's Discussion rows", func(t *testing.T) {
		t.Parallel()
		motif := parse(t, recipes.MotifRecipe)

		start := slices.IndexFunc(tapestry.Producers, func(row shedbuild.Row) bool { return row.Name == loomshed.NameDiscussionWrite })
		if start < 0 {
			t.Fatalf("the tapestry recipe has no %s row", loomshed.NameDiscussionWrite)
		}
		want := slices.Clone(tapestry.Producers[start : start+len(motif.Producers)])
		for i := range want {
			// The block rewrites the bouncer's empty on_done to its exit.
			if want[i].Name == loomshed.NameDiscussionBouncer {
				want[i].OnDone = ""
			}
		}
		if !reflect.DeepEqual(motif.Producers, want) {
			t.Errorf("motif rows = %+v; want tapestry's Discussion rows %+v", motif.Producers, want)
		}
	})

	t.Run("arras expands to tapestry's rows plus the Motif block row", func(t *testing.T) {
		t.Parallel()
		expanded, err := parseEmbeddedRecipe(shedrun.RecipeArras)
		if err != nil {
			t.Fatalf("parseEmbeddedRecipe(arras) = _, %v; want nil", err)
		}

		var want []shedbuild.Row
		for _, row := range tapestry.Producers {
			switch row.Name {
			case loomshed.NamePreflight:
				row.OnDone = loomshed.NameMotif
				want = append(want, row, shedbuild.Row{
					Name:      loomshed.NameMotif,
					Engine:    shedrecipe.EngineBlock,
					OnDone:    loomshed.NameDiscussionWrite,
					SkipTo:    loomshed.NameDriftBaseline,
					Block:     shedrun.RecipeMotif,
					BlockGate: shedrecipe.BlockGateAlways,
				})
				continue
			case loomshed.NameDiscussionWrite, loomshed.NameDiscussionBouncer, loomshed.NameDiscussionBurlerReview, loomshed.NameDiscussionBurlerFix:
				row.BlockPath = loomshed.NameMotif
			}
			want = append(want, row)
		}
		if !reflect.DeepEqual(expanded.Producers, want) {
			t.Errorf("arras rows = %+v; want tapestry's rows with the Motif block row %+v", expanded.Producers, want)
		}
	})

	t.Run("arras keeps tapestry's top-level keys", func(t *testing.T) {
		t.Parallel()
		arras := parse(t, recipes.ArrasRecipe)

		if arras.ProfileRecipe != shedrun.RecipeTapestry {
			t.Errorf("arras ProfileRecipe = %q; want %q", arras.ProfileRecipe, shedrun.RecipeTapestry)
		}
		arras.ProfileRecipe, arras.Producers = "", nil
		want := tapestry
		want.Producers = nil
		if !reflect.DeepEqual(arras, want) {
			t.Errorf("arras top-level keys = %+v; want tapestry's %+v", arras, want)
		}
	})

	t.Run("arras builds under the loom contract", func(t *testing.T) {
		t.Parallel()
		env, paths := testEnv(t)
		if _, err := New(shedrun.RecipeArras, env, paths); err != nil {
			t.Errorf("New(arras) = _, %v; want nil", err)
		}
	})
}
