// gatequiescence_test.go pins the second half of the precondition the per-attempt done-signal
// narrowing rests on: the per-attempt done-signal is the next turn boundary and nothing more, and
// that narrowing holds only because no gated site can spawn an async in-process subagent whose work
// outlives the turn. Batch 1's engine_test.go pins the first half -- the in-process Agent tool is
// denied by the shipped shuttle template. This file pins the second: no gated row's own spec ever
// authorizes shuttleengine.Spec.ForkSubagents.
//
// It parses the real embedded recipe and reads no worktree, staying untagged and offline.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// gateQuiescenceFailureMessage is the shared failure text every case below reports on: a gated site
// gaining ForkSubagents re-opens the compound-quiescence question this task deliberately declined --
// turn-idle alone would no longer mean the agent is finished -- and the decision must be re-opened
// rather than this test updated to tolerate the new shape.
const gateQuiescenceFailureMessage = "re-opens the compound-quiescence question this task deliberately declined -- turn-idle alone would no longer mean the agent is finished; the decision must be re-opened, not this test"

// TestNoGatedRowAuthorizesForkSubagents parses the real embedded recipe and asserts every row carrying a non-empty "gates" config list either is a writer row (engine DiscussionWrite, PlanWrite or Describe) -- whose spec factory (internal/loomengine's DiscussionSpec/PlanSpec, or landingshed.DescribeSpec) never sets shuttleengine.Spec.ForkSubagents at all -- or is a burler row (engine BurlerRound) carrying no "cluster-fan" key in its profile: sub-map, since burlerengine.Engine.Run sets shuttleengine.Spec.ForkSubagents from p.ClusterFan != "" and from nothing else, so an absent fan is what keeps a gated round's spec unforked.
//
//testtiming:keep pins that no gated shipped row authorizes fork subagents, a guard that fires on the recipe file, which TestApproveSeam_FailsToBuild never reads
func TestNoGatedRowAuthorizesForkSubagents(t *testing.T) {
	r, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.LoomRecipe) error = %v; want nil", err)
	}

	gatedRowFound := false
	for _, row := range r.Producers {
		if gates, ok := row.Config["gates"].([]any); !ok || len(gates) == 0 {
			continue
		}
		gatedRowFound = true

		switch row.Engine {
		case "DiscussionWrite", "PlanWrite", "Describe", "PRRework":
			// A writer row's spec comes from internal/loomengine's DiscussionSpec/PlanSpec or
			// landingshed.DescribeSpec, none of which sets Spec.ForkSubagents anywhere in its own
			// construction -- the zero value (false) is what every writer spec carries by
			// construction, so a writer row identified by engine name alone is what this case
			// attests to.
			// PRRework is a writer row too: loomengine.ReworkSpec mirrors PlanSpec and sets no ForkSubagents either.
			continue
		case "BurlerRound":
			profile, ok := row.Config["profile"].(map[string]any)
			if !ok {
				t.Errorf("row %q (engine %q): config[\"profile\"] is not a map[string]any (got %T); want a profile sub-map to inspect for cluster-fan -- %s", row.Name, row.Engine, row.Config["profile"], gateQuiescenceFailureMessage)
				continue
			}
			if clusterFan, has := profile["cluster-fan"]; has && clusterFan != "" {
				t.Errorf("row %q (engine %q): profile[\"cluster-fan\"] = %v; want absent or empty on a gated burler row -- %s", row.Name, row.Engine, clusterFan, gateQuiescenceFailureMessage)
			}
		default:
			t.Errorf("row %q: carries a \"gates\" config list with engine %q, neither a writer engine (DiscussionWrite/PlanWrite/Describe/PRRework) nor BurlerRound -- %s", row.Name, row.Engine, gateQuiescenceFailureMessage)
		}
	}

	if !gatedRowFound {
		t.Fatalf("no row in the embedded recipe carries a \"gates\" config list; this test has nothing to assert -- the recipe or this test has drifted")
	}
}

// TestWebsterRoundCarriesNoGateKey asserts the Webster round -- the one row that could legitimately grow a fan, since Webster-Burler is the sole surviving fix-scope: source round with real production incentive to fan reviewers -- carries no "gates" key at all, so TestNoGatedRowAuthorizesForkSubagents's writer/burler dichotomy above is exhaustive over every gated row without needing a third case for it.
//
//testtiming:keep pins that the shipped Webster round has no gates key, a guard that fires on the recipe file, which TestApproveSeam_FailsToBuild never reads
func TestWebsterRoundCarriesNoGateKey(t *testing.T) {
	r, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.LoomRecipe) error = %v; want nil", err)
	}

	found := false
	for _, row := range r.Producers {
		if row.Engine != "Webster" {
			continue
		}
		found = true
		if gates, hasGates := row.Config["gates"]; hasGates {
			t.Errorf("row %q (engine %q): config[\"gates\"] = %v; want absent -- %s", row.Name, row.Engine, gates, gateQuiescenceFailureMessage)
		}
	}
	if !found {
		t.Fatalf("no row in the embedded recipe has engine %q; this test has nothing to assert", "Webster")
	}
}
