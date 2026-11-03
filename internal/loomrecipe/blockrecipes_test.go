// blockrecipes_test.go pins the tapestry recipe's Motif block against tapestry's rows as they stood before the block, and the motif recipe run alone.

package loomrecipe

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// tapestryBeforeMotifFile is the testdata file holding the tapestry recipe as it stood before its Discussion rows moved into the Motif block.
const tapestryBeforeMotifFile = "tapestry-rows-before-motif.yaml"

// motifRowNames are the motif recipe's rows, in file order.
var motifRowNames = []string{loomshed.NameDiscussionWrite, loomshed.NameDiscussionBouncer, loomshed.NameDiscussionBurlerReview, loomshed.NameDiscussionBurlerFix}

// TestTapestryMotifBlock asserts the expanded tapestry recipe is its recipe before the Motif block, config included:
// the same rows with the Motif block row after Preflight and the Discussion rows inside the block, and the same top-level keys.
func TestTapestryMotifBlock(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	source, err := os.ReadFile(filepath.Join("testdata", tapestryBeforeMotifFile))
	if err != nil {
		t.Fatalf("read %s: %v", tapestryBeforeMotifFile, err)
	}
	before, err := shedbuild.Parse(source)
	if err != nil {
		t.Fatalf("shedbuild.Parse(%s) = _, %v; want nil", tapestryBeforeMotifFile, err)
	}
	expanded, err := parseEmbeddedRecipe(shedrun.RecipeTapestry)
	if err != nil {
		t.Fatalf("parseEmbeddedRecipe(tapestry) = _, %v; want nil", err)
	}

	t.Run("rows", func(t *testing.T) {
		t.Parallel()
		var want []shedbuild.Row
		for _, row := range before.Producers {
			if row.Name == loomshed.NamePreflight {
				row.OnDone = loomshed.NameMotif
				want = append(want, row, shedbuild.Row{
					Name:      loomshed.NameMotif,
					Engine:    shedrecipe.EngineBlock,
					OnDone:    loomshed.NameDiscussionWrite,
					SkipTo:    loomshed.NameDriftBaseline,
					Block:     shedrun.RecipeMotif,
					BlockGate: shedrecipe.BlockGateUndiscussed,
				})
				continue
			}
			if slices.Contains(motifRowNames, row.Name) {
				row.BlockPath = loomshed.NameMotif
			}
			want = append(want, row)
		}
		if !reflect.DeepEqual(expanded.Producers, want) {
			t.Errorf("tapestry rows = %+v; want its rows before the Motif block, with the block row %+v", expanded.Producers, want)
		}
	})

	t.Run("top-level keys", func(t *testing.T) {
		t.Parallel()
		got, want := expanded, before
		got.Producers, want.Producers = nil, nil
		if !reflect.DeepEqual(got, want) {
			t.Errorf("tapestry top-level keys = %+v; want %+v", got, want)
		}
	})
}

// TestMotifRecipeRunsAlone asserts the motif recipe builds under the loom contract as a run of its own, holds the Discussion rows alone, and ends its run on Discussion-Bouncer's Done.
func TestMotifRecipeRunsAlone(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	env, paths := testEnv(t)

	shed, err := New(shedrun.RecipeMotif, env, paths)
	if err != nil {
		t.Fatalf("New(motif) = _, %v; want nil", err)
	}
	names := make([]string, 0, len(shed.Producers))
	var bouncer shedengine.ProducerDef
	for _, def := range shed.Producers {
		names = append(names, def.Name)
		if def.Name == loomshed.NameDiscussionBouncer {
			bouncer = def
		}
	}
	if !slices.Equal(names, motifRowNames) {
		t.Errorf("New(motif) rows = %v; want %v", names, motifRowNames)
	}
	if bouncer.OnDone != "" {
		t.Errorf("motif %s OnDone = %q; want empty, which ends the run", loomshed.NameDiscussionBouncer, bouncer.OnDone)
	}
}
