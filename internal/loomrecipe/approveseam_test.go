// approveseam_test.go's single subject: the approval seam is wired as shipped, and a mis-wiring is
// rejected rather than silently accepted. Removing approve_seam from the shipped Plan-Bouncer row is
// not reachable through the sequence fixture -- the recipe is parsed unconditionally from the
// embedded document, and a nil Env.ApprovePlan fails at requireSeam before the run starts -- so the
// negative case is expressed statically instead, by parsing hand-authored recipe YAML through
// shedbuild.Parse the way overlay_seam_guard_test.go already does for its own fixtures.
//
// This file used to also carry a dynamic negative case, substituting env.ApprovePlan with a non-nil
// no-op closure and driving a real run to prove Plan-Revalidate's require_approved: true key caught
// the resulting no-op seam. That row -- and the whole class of standalone post-segment mechanical
// re-check it belonged to -- is deleted by the row-removal batch, and nothing re-checks the approval
// flag after Plan-Bouncer's settle writes it any more. The property this file's deleted case pinned
// now rests on two things instead: the approve seam failing loudly at requireSeam if it is ever
// wired nil (still covered below), and sequence_test.go's own trailing planparser.ParsePlan
// approved-flag assertion, the only standing guard left anywhere that the seam genuinely ran on a
// clean pass -- by design, no row re-checks the flag.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// TestShippedRecipe_SeamsWiredOnPlanBouncerOnly parses the real embedded recipes.LoomRecipe and asserts each seam key's shipped shape: approve_seam: plan and skip_seam: rework-exempt each sit on the Plan-Bouncer row and no other row, and require_approved is present on no row at all -- the key is no longer recognized by any registry entry now that both rows sharing the old PlanValidate engine are deleted, and a row carrying it would fail construction outright rather than silently doing nothing.
//
//testtiming:keep pins which shipped rows carry approve_seam and skip_seam, which TestApproveSeam_FailsToBuild never reads off the shipped recipe
func TestShippedRecipe_SeamsWiredOnPlanBouncerOnly(t *testing.T) {
	t.Parallel()
	r, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.LoomRecipe) error = %v; want nil", err)
	}

	for _, row := range r.Producers {
		if _, hasRequireApproved := row.Config["require_approved"]; hasRequireApproved {
			t.Errorf("row %q: carries an unexpected \"require_approved\" key; want it absent from every row -- the key is no longer recognized by any registry entry", row.Name)
		}
	}

	seams := []struct{ key, want string }{
		{"approve_seam", "plan"},
		{"skip_seam", "rework-exempt"},
	}
	for _, seam := range seams {
		t.Run(seam.key, func(t *testing.T) {
			t.Parallel()
			for _, row := range r.Producers {
				got, has := row.Config[seam.key]
				if row.Name == loomshed.NamePlanBouncer {
					if !has || got != seam.want {
						t.Errorf("row %q: config[%q] = %v (present=%v); want %q", row.Name, seam.key, got, has, seam.want)
					}
					continue
				}
				if has {
					t.Errorf("row %q: carries an unexpected %q key = %v; want it absent", row.Name, seam.key, got)
				}
			}
		})
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

// TestApproveSeam_FailsToBuild parses a one-row recipe naming approve_seam on a Bouncer row and builds it, asserting Build fails for a nil Env.ApprovePlan under approve_seam: plan -- a present approve_seam key is guarded by requireSeam on env.ApprovePlan exactly as commit_seam is guarded on env.CommitPlan/env.CommitDiscussion, so a document naming the key against a nil seam must never silently build a Bouncer whose Approve closure is nil -- and for a value bouncerEntry does not recognize: approve_seam names env.ApprovePlan and nothing else, so any value other than "plan" is a build-time error, not a silently-ignored key.
func TestApproveSeam_FailsToBuild(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		approveSeam string
		nilClosure  bool
	}{
		{"nil Env.ApprovePlan", "plan", true},
		{"unrecognized value", "discussion", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env, _ := testEnv(t)
			if tt.nilClosure {
				env.ApprovePlan = nil
			}

			r, err := shedbuild.Parse([]byte(approveSeamFixture(tt.approveSeam)))
			if err != nil {
				t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
			}

			if _, err := shedbuild.Build(r, env); err == nil {
				t.Fatalf("shedbuild.Build() error = nil; want non-nil for approve_seam: %s", tt.approveSeam)
			}
		})
	}
}
