// interruptpolicy_meta_test.go checks loomshed.InterruptPolicies against the rows New assembles.
// It compares the table with the assembled rows, not with a standalone literal that could drift silently.
// It lives here rather than beside the table in internal/loomshed.
// wantProducerTable (in shape_test.go) is the only thing that sees the engine identity behind each row.
// A test in internal/loomshed cannot see it at all.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestInterruptPolicies_MatchAssembledRows checks loomshed.InterruptPolicies in both directions.
// The table must cover exactly the rows New assembles for the tapestry, darn, sampler, motif, raddle and pawl recipes.
// Every row that one of those recipes' current output has must carry an entry in the table.
// That catches a row added to a recipe with no policy assigned.
// Every key in the table must name a row one of them actually has.
// That keeps the table from accumulating dead entries for a renamed or removed row.
func TestInterruptPolicies_MatchAssembledRows(t *testing.T) {
	tiergate.Require(t, 0)
	env, paths := testEnv(t)
	rowNames := map[string]bool{}
	for _, name := range []string{shedrun.RecipeTapestry, shedrun.RecipeDarn, shedrun.RecipeSampler, shedrun.RecipeMotif, shedrun.RecipeRaddle, shedrun.RecipePawl} {
		// Pawl is last, since its build has no review segment to budget and the others need theirs.
		if name == shedrun.RecipePawl {
			delete(env.Segments, codeReviewSegment)
		}
		shed, err := New(name, env, paths)
		if err != nil {
			t.Fatalf("New(%s) error = %v, want nil", name, err)
		}
		for _, p := range shed.Producers {
			rowNames[p.Name] = true
			if _, ok := loomshed.InterruptPolicies[p.Name]; !ok {
				t.Errorf("New(%s) row %q has no entry in loomshed.InterruptPolicies", name, p.Name)
			}
		}
	}

	for rowName := range loomshed.InterruptPolicies {
		if !rowNames[rowName] {
			t.Errorf("loomshed.InterruptPolicies names row %q, which none of tapestry, darn, sampler, motif, raddle and pawl has", rowName)
		}
	}
}

// TestInterruptPolicies_MatchEngineIdentity cross-checks loomshed.InterruptPolicies against the row-to-engine mapping.
// The mapping is read off wantProducerTable:
// every row whose engine is "Implementer" must carry shedverbs.InterruptPolicyHandback,
// and every row whose engine is anything else must carry shedverbs.InterruptPolicyReinvoke.
// The check keeps a row that changes adapter from silently keeping the wrong policy.
// Deriving the implementer exception from the engine name keeps it from being hand-maintained.
// The handback policy applies to Webster, the only implementer the engine builds.
// The pawl recipe's rows are absent from wantProducerTable and spawn no agent of their own,
// and they carry the reinvoke policy as well.
//
//testtiming:keep pins the Webster-handback and reinvoke-otherwise policy per row engine, which TestApproveSeam_FailsToBuild never asserts
func TestInterruptPolicies_MatchEngineIdentity(t *testing.T) {
	tiergate.Require(t, 0)
	for rowName, engineName := range rowEngines() {
		policy, ok := loomshed.InterruptPolicies[rowName]
		if !ok {
			t.Errorf("loomshed.InterruptPolicies has no entry for row %q", rowName)
			continue
		}

		if engineName == "Implementer" {
			if policy != shedverbs.InterruptPolicyHandback {
				t.Errorf("row %q (engine %q) policy = %q; want %q", rowName, engineName, policy, shedverbs.InterruptPolicyHandback)
			}
			continue
		}

		if policy != shedverbs.InterruptPolicyReinvoke {
			t.Errorf("row %q (engine %q) policy = %q; want %q", rowName, engineName, policy, shedverbs.InterruptPolicyReinvoke)
		}
	}
	for _, row := range wantPawlRows() {
		if policy := loomshed.InterruptPolicies[row.name]; policy != shedverbs.InterruptPolicyReinvoke {
			t.Errorf("pawl row %q policy = %q; want %q", row.name, policy, shedverbs.InterruptPolicyReinvoke)
		}
	}
}
