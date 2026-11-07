// recipekeyed_test.go pins the loomcli wiring branches that read the run's recipe rows and keys, per recipe name:
// the verify source; the merge fixer's plan directory; the arbiter's recipe description;
// the landing failure tracer; and whether a finished run makes its entry a Sketch.

package loomcli

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestRecipeKeyedWiring asserts that each recipe-keyed branch picks the answer the recipe's rows and keys give.
// It runs for tapestry, motif, darn, pawl, sampler and raddle.
func TestRecipeKeyedWiring(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	if err := os.MkdirAll(fabricengine.BoardDir(loc.HubPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(board dir) = %v; want nil", err)
	}
	// The hub holds a gate.yaml and no darn.yaml: the plan source reads the gate call alone,
	// and the tree-scope source refuses for the darn.yaml it cannot load.
	writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())

	tests := []struct {
		name         string
		recipe       string
		treeScope    bool
		hasPlan      bool
		wantFragment string
		wantSketch   bool
	}{
		{"Tapestry", shedrun.RecipeTapestry, false, true, "a tapestry run, a task driven from discussion to landing", false},
		{"Motif", shedrun.RecipeMotif, false, false, "a motif run, a task's Discussion", true},
		{"Darn", shedrun.RecipeDarn, true, false, "its decision record through one build session", false},
		{"Pawl", shedrun.RecipePawl, false, false, "a beam's life", false},
		{"Sampler", shedrun.RecipeSampler, true, false, "made with the operator in one interactive session", false},
		{"Raddle", shedrun.RecipeRaddle, false, false, "splits a Sketch's stored decision record into board entries the operator approves", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &loomCLI{recipe: tt.recipe, location: loc, runID: shedrun.SelfRunID}
			if _, err := c.verifySource().command("finalize"); (err != nil) != tt.treeScope {
				t.Errorf("verifySource().command(finalize) for %s error = %v; want an error exactly when the recipe takes the tree-scope source (%t)", tt.recipe, err, tt.treeScope)
			}

			wantPlanDir := ""
			if tt.hasPlan {
				wantPlanDir = planparser.PlanDir(loc.AnchorPath())
			}
			if got := fixerPlanDir(loc, tt.recipe); got != wantPlanDir {
				t.Errorf("fixerPlanDir(%s) = %q; want %q", tt.recipe, got, wantPlanDir)
			}

			if got := arbiterRecipeDescription(tt.recipe); !strings.Contains(got, tt.wantFragment) {
				t.Errorf("arbiterRecipeDescription(%s) = %q; want it to contain %q", tt.recipe, got, tt.wantFragment)
			}

			if got := finishesAsSketch(tt.recipe); got != tt.wantSketch {
				t.Errorf("finishesAsSketch(%s) = %t; want %t", tt.recipe, got, tt.wantSketch)
			}
			if got := c.commitStatusDeps(loc).Finish != nil; got != tt.wantSketch {
				t.Errorf("commitStatusDeps(%s).Finish set = %t; want %t", tt.recipe, got, tt.wantSketch)
			}

			deps := landingDeps(loc, tt.recipe, websterengine.Geometry{}, "task", "main",
				true, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(loc), parentLockDeps(loc, shedrun.SelfRunID, "", ""))
			if (deps.TraceFailure != nil) != tt.hasPlan {
				t.Errorf("landingDeps(%s).TraceFailure set = %t; want %t, set exactly when the recipe has a Plan-Write row", tt.recipe, deps.TraceFailure != nil, tt.hasPlan)
			}
		})
	}
}
