// blockrecipes_test.go checks the tapestry recipe's Motif and Jacquard blocks against its pre-block rows in testdata;
// and checks that the motif and jacquard recipes build and run alone.

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

// tapestryBeforeMotifFile names the testdata file holding the tapestry recipe rows that precede its blocks.
const tapestryBeforeMotifFile = "tapestry-rows-before-motif.yaml"

// motifRowNames are the motif recipe's rows, in file order.
var motifRowNames = []string{loomshed.NameDiscussionWrite, loomshed.NameDiscussionBouncer, loomshed.NameDiscussionBurlerReview, loomshed.NameDiscussionBurlerFix}

// TestTapestryBlocks asserts the expanded tapestry recipe is the Motif block's rows followed by the Jacquard block's.
// Each row is as the pre-block recipe held it, config included, but for the edges the blocks add:
// Motif enters Discussion-Write and Discussion-Bouncer exits to Jacquard.
// Jacquard enters Preflight, whose Done enters Record-Seed ahead of Drift-Baseline.
// The top-level keys match the pre-block recipe's but for the entry, and jacquard's but for the entry equal tapestry's.
func TestTapestryBlocks(t *testing.T) {
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
		want := []shedbuild.Row{{
			Name:      loomshed.NameMotif,
			Engine:    shedrecipe.EngineBlock,
			OnDone:    loomshed.NameDiscussionWrite,
			SkipTo:    loomshed.NameJacquard,
			Block:     shedrun.RecipeMotif,
			BlockGate: shedrecipe.BlockGateUndiscussed,
		}}
		var preflight shedbuild.Row
		var jacquardRows []shedbuild.Row
		for _, row := range before.Producers {
			switch {
			case row.Name == loomshed.NamePreflight:
				preflight = row
			case slices.Contains(motifRowNames, row.Name):
				if row.Name == loomshed.NameDiscussionBouncer {
					row.OnDone = loomshed.NameJacquard
				}
				row.BlockPath = loomshed.NameMotif
				want = append(want, row)
			default:
				row.BlockPath = loomshed.NameJacquard
				jacquardRows = append(jacquardRows, row)
			}
		}
		preflight.OnDone = loomshed.NameRecordSeed
		preflight.BlockPath = loomshed.NameJacquard
		want = append(want,
			shedbuild.Row{
				Name:      loomshed.NameJacquard,
				Engine:    shedrecipe.EngineBlock,
				OnDone:    loomshed.NamePreflight,
				Block:     shedrun.RecipeJacquard,
				BlockGate: shedrecipe.BlockGateAlways,
			},
			preflight,
			shedbuild.Row{Name: loomshed.NameRecordSeed, Engine: shedrecipe.LoomSeedEngine, OnDone: loomshed.NameDriftBaseline, BlockPath: loomshed.NameJacquard},
		)
		want = append(want, jacquardRows...)

		if !reflect.DeepEqual(expanded.Producers, want) {
			t.Errorf("tapestry rows = %+v; want the Motif block's rows then the Jacquard block's %+v", expanded.Producers, want)
		}
	})

	t.Run("top-level keys", func(t *testing.T) {
		t.Parallel()
		got, want := expanded, before
		got.Producers, want.Producers = nil, nil
		want.Entry = loomshed.NameMotif
		if !reflect.DeepEqual(got, want) {
			t.Errorf("tapestry top-level keys = %+v; want %+v", got, want)
		}

		jacquard, err := parseEmbeddedRecipe(shedrun.RecipeJacquard)
		if err != nil {
			t.Fatalf("parseEmbeddedRecipe(jacquard) = _, %v; want nil", err)
		}
		if jacquard.Entry != loomshed.NamePreflight {
			t.Errorf("jacquard entry = %q; want %q", jacquard.Entry, loomshed.NamePreflight)
		}
		jacquard.Producers, jacquard.Entry = nil, got.Entry
		if !reflect.DeepEqual(jacquard, got) {
			t.Errorf("jacquard top-level keys but entry = %+v; want tapestry's %+v", jacquard, got)
		}
	})
}

// TestMotifRecipeRunsAlone asserts the motif recipe builds under the loom contract as a run of its own.
// It holds only the Discussion rows, and its run ends on Discussion-Bouncer's Done.
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

// TestJacquardRecipeRunsAlone asserts the jacquard recipe builds under the loom contract as a run of its own.
// It holds the tapestry rows from Preflight on and no Discussion row, and its run ends on Friction-Reflect's Done.
func TestJacquardRecipeRunsAlone(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	env, paths := testEnv(t)

	shed, err := New(shedrun.RecipeJacquard, env, paths)
	if err != nil {
		t.Fatalf("New(jacquard) = _, %v; want nil", err)
	}
	var want []string
	for _, row := range wantProducerTable[slices.IndexFunc(wantProducerTable, func(row wantProducerRow) bool { return row.name == loomshed.NamePreflight }):] {
		want = append(want, row.name)
	}
	names := make([]string, 0, len(shed.Producers))
	for _, def := range shed.Producers {
		names = append(names, def.Name)
	}
	if !slices.Equal(names, want) {
		t.Errorf("New(jacquard) rows = %v; want %v", names, want)
	}
	if last := shed.Producers[len(shed.Producers)-1]; last.Name != loomshed.NameFrictionReflect || last.OnDone != "" {
		t.Errorf("jacquard last row = %q with OnDone %q; want %q with an empty OnDone, which ends the run", last.Name, last.OnDone, loomshed.NameFrictionReflect)
	}
}
