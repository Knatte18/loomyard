// approveseam_test.go's single subject: the approval seam is wired as shipped, and a mis-wiring is
// rejected rather than silently accepted. Removing approve_seam from the shipped Plan-Bouncer row is
// not reachable through the sequence fixture -- the recipe is parsed unconditionally from the
// embedded document, and a nil Env.ApprovePlan fails at requireSeam before the run starts -- so the
// negative case is expressed two ways here instead: dynamically, by substituting env.ApprovePlan
// with a non-nil no-op closure and driving a real run, and statically, by parsing hand-authored
// recipe YAML through shedbuild.Parse the way overlay_seam_guard_test.go already does for its own
// fixtures.

package loomrecipe

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestSequence_NoOpApproveSeamBouncesAtRevalidate builds the sequence fixture and substitutes
// env.ApprovePlan with a non-nil closure that writes nothing, rather than the real
// planparser.SetApproved closure buildSequenceFixture wires by default. Construction succeeds --
// bouncerEntry's requireSeam guard only checks non-nil -- and Plan-Bouncer's approved settle calls
// the closure and commits, but nothing ever flips the plan's approved: flag. The run must therefore
// halt at Plan-Revalidate with Stuck and bounce to Plan-Write rather than reaching Batchifier: this
// pins that Plan-Revalidate's require_approved: true key is genuinely enforcing the approval it is
// now the only row in the recipe to check.
func TestSequence_NoOpApproveSeamBouncesAtRevalidate(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)
	env.ApprovePlan = func() error { return nil }

	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed.Producers[0].Producer = fakeAlwaysDoneProducer{}

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}

	revalidateIdx := -1
	for i, e := range result.History {
		if e.Producer == loomshed.NamePlanRevalidate && e.Outcome == shedengine.Stuck {
			revalidateIdx = i
			break
		}
	}
	if revalidateIdx == -1 {
		t.Fatalf("Run() History has no %s Stuck entry: %+v", loomshed.NamePlanRevalidate, result.History)
	}
	if revalidateIdx+1 >= len(result.History) {
		t.Fatalf("Run() History ends at the %s Stuck entry; want a following entry naming the bounce target %q", loomshed.NamePlanRevalidate, loomshed.NamePlanWrite)
	}
	if got := result.History[revalidateIdx+1].Producer; got != loomshed.NamePlanWrite {
		t.Errorf("History[%d].Producer (following the %s Stuck entry) = %q; want the declared bounce target %q", revalidateIdx+1, loomshed.NamePlanRevalidate, got, loomshed.NamePlanWrite)
	}

	for _, e := range result.History {
		if e.Producer == loomshed.NameBatchifier {
			t.Fatalf("Run() History reaches %s; want the run to bounce at %s instead", loomshed.NameBatchifier, loomshed.NamePlanRevalidate)
		}
	}
}

// TestShippedRecipe_ApproveSeamWiredOnPlanBouncerOnly parses the real embedded recipes.LoomRecipe
// and asserts the approval seam's shipped shape: approve_seam: plan on the Plan-Bouncer row,
// require_approved: true on the Plan-Revalidate row, and neither key present on any other row.
func TestShippedRecipe_ApproveSeamWiredOnPlanBouncerOnly(t *testing.T) {
	r, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.LoomRecipe) error = %v; want nil", err)
	}

	for _, row := range r.Producers {
		approveSeam, hasApproveSeam := row.Config["approve_seam"]
		requireApproved, hasRequireApproved := row.Config["require_approved"]

		switch row.Name {
		case loomshed.NamePlanBouncer:
			if !hasApproveSeam || approveSeam != "plan" {
				t.Errorf("row %q: config[\"approve_seam\"] = %v (present=%v); want \"plan\"", row.Name, approveSeam, hasApproveSeam)
			}
		case loomshed.NamePlanRevalidate:
			if !hasRequireApproved || requireApproved != true {
				t.Errorf("row %q: config[\"require_approved\"] = %v (present=%v); want true", row.Name, requireApproved, hasRequireApproved)
			}
		default:
			if hasApproveSeam {
				t.Errorf("row %q: carries an unexpected \"approve_seam\" key = %v; want it absent", row.Name, approveSeam)
			}
			if hasRequireApproved {
				t.Errorf("row %q: carries an unexpected \"require_approved\" key = %v; want it absent", row.Name, requireApproved)
			}
		}
	}
}

// approveSeamFixture returns a one-row recipe YAML document: a lone Bouncer row carrying
// approveSeam as its approve_seam config value (omitted entirely when approveSeam is empty).
func approveSeamFixture(approveSeam string) string {
	approveSeamLine := ""
	if approveSeam != "" {
		approveSeamLine = "      approve_seam: " + approveSeam + "\n"
	}
	return "version: 1\n" +
		"entry: Fixture-Bouncer\n" +
		"terminals: [Fixture-Bouncer]\n" +
		"producers:\n" +
		"  - name: Fixture-Bouncer\n" +
		"    engine: Bouncer\n" +
		"    segment: Fixture-Review\n" +
		"    config:\n" +
		"      run_subdir: fixture\n" +
		"      artifact_paths:\n" +
		"        - _lyx/fixture\n" +
		"      rubric_stencil: loom-rubric-plan-review\n" +
		approveSeamLine
}

// TestApproveSeam_NilEnvClosureFailsToBuild parses a one-row recipe naming approve_seam: plan on a
// Bouncer row and builds it against an Env whose ApprovePlan is nil, asserting Build fails: a
// present approve_seam key is guarded by requireSeam on env.ApprovePlan exactly as commit_seam is
// guarded on env.CommitPlan/env.CommitDiscussion, so a document naming the key against a nil seam
// must never silently build a Bouncer whose Approve closure is nil.
func TestApproveSeam_NilEnvClosureFailsToBuild(t *testing.T) {
	env, _ := testEnv(t)
	env.ApprovePlan = nil

	r, err := shedbuild.Parse([]byte(approveSeamFixture("plan")))
	if err != nil {
		t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
	}

	if _, err := shedbuild.Build(r, env); err == nil {
		t.Fatalf("shedbuild.Build() error = nil; want non-nil for approve_seam: plan against a nil Env.ApprovePlan")
	}
}

// TestApproveSeam_UnknownValueFailsToBuild parses a one-row recipe naming an approve_seam value
// bouncerEntry does not recognize, asserting Build fails: approve_seam names env.ApprovePlan and
// nothing else, so any value other than "plan" is a build-time error, not a silently-ignored
// key.
func TestApproveSeam_UnknownValueFailsToBuild(t *testing.T) {
	env, _ := testEnv(t)

	r, err := shedbuild.Parse([]byte(approveSeamFixture("discussion")))
	if err != nil {
		t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
	}

	if _, err := shedbuild.Build(r, env); err == nil {
		t.Fatalf("shedbuild.Build() error = nil; want non-nil for an unrecognized approve_seam value")
	}
}
