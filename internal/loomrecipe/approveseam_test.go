// approveseam_test.go pins that the approval seam is wired as shipped, and that a mis-wiring is
// rejected rather than silently accepted.
// The sequence fixture cannot drive the removal of approve_seam from the shipped Plan-Bouncer row:
// the recipe is parsed from the embedded document,
// and a nil Env.ApprovePlan fails at requireSeam before the run starts.
// The negative case is therefore expressed statically, by parsing hand-authored recipe YAML through shedbuild.Parse.
// A seam wired nil fails loudly at requireSeam, which the tests below cover.
// sequence_test.go's trailing planparser.ParsePlan assertion guards that the seam ran on a clean pass.
// No row re-checks the approval flag after Plan-Bouncer's settle writes it.

package loomrecipe

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestShippedRecipe_SeamsWiredOnPlanBouncerOnly asserts each seam key's shipped shape in the embedded tapestry recipe:
// approve_seam: plan and skip_seam: rework-exempt sit on the Plan-Bouncer row;
// approve_seam: discussion sits on the Discussion-Bouncer row, and no other row carries either key.
// require_approved is recognized by no registry entry;
// a row carrying it fails construction rather than silently doing nothing.
//
//testtiming:keep pins which shipped rows carry approve_seam and skip_seam, which TestApproveSeam_FailsToBuild never reads off the shipped recipe
func TestShippedRecipe_SeamsWiredOnPlanBouncerOnly(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	r, err := shedbuild.Parse(recipes.TapestryRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.TapestryRecipe) error = %v; want nil", err)
	}

	for _, row := range r.Producers {
		if _, hasRequireApproved := row.Config["require_approved"]; hasRequireApproved {
			t.Errorf("row %q: carries an unexpected \"require_approved\" key; want it absent from every row -- the key is no longer recognized by any registry entry", row.Name)
		}
	}

	seams := []struct {
		key string
		// want maps each row that carries the key to its shipped value.
		want map[string]string
	}{
		{"approve_seam", map[string]string{loomshed.NamePlanBouncer: "plan", loomshed.NameDiscussionBouncer: "discussion"}},
		{"skip_seam", map[string]string{loomshed.NamePlanBouncer: "rework-exempt"}},
	}
	for _, seam := range seams {
		t.Run(seam.key, func(t *testing.T) {
			t.Parallel()
			for _, row := range r.Producers {
				got, has := row.Config[seam.key]
				if want, carries := seam.want[row.Name]; carries {
					if !has || got != want {
						t.Errorf("row %q: config[%q] = %v (present=%v); want %q", row.Name, seam.key, got, has, want)
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
		"        - _lym/fixture\n" +
		"      rubric_stencil: loom-rubric-plan-review\n" +
		approveSeamLine
}

// TestApproveSeam_FailsToBuild asserts Build fails for a one-row recipe that names approve_seam in two cases:
// a nil Env.ApprovePlan under approve_seam: plan: requireSeam guards that key as commit_seam guards its seams;
// so such a document never builds a Bouncer whose Approve closure is nil;
// or a value bouncerEntry does not recognize, since approve_seam accepts only "plan" and "discussion".
func TestApproveSeam_FailsToBuild(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		approveSeam string
		nilClosure  bool
	}{
		{"nil Env.ApprovePlan", "plan", true},
		{"unrecognized value", "webster", false},
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

// discussionApproveSeamFault returns the first way parsed breaks the discussion approve-seam rule.
// It returns the empty string when parsed keeps the rule.
// The rule: the row that approves the discussion carries approve_seam: discussion, and no other Bouncer row does.
func discussionApproveSeamFault(parsed shedbuild.Recipe) string {
	approvalRow, _ := discussionApprovalRow(parsed)
	for _, row := range parsed.Producers {
		if row.Engine != bouncerEngine {
			continue
		}
		carries := row.Config["approve_seam"] == "discussion"
		switch {
		case row.Name == approvalRow && !carries:
			return fmt.Sprintf("row %q approves the discussion but carries no approve_seam: discussion", row.Name)
		case row.Name != approvalRow && carries:
			return fmt.Sprintf("row %q carries approve_seam: discussion but does not approve the discussion", row.Name)
		}
	}
	return ""
}

// TestDiscussionApprovalRowsCarryApproveSeam checks the rule with discussionApproveSeamFault.
// It runs the check on every embedded loom recipe and on two fixtures that break the rule.
func TestDiscussionApprovalRowsCarryApproveSeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, recipe := range []string{shedrun.RecipeTapestry, shedrun.RecipeDarn, shedrun.RecipePawl, shedrun.RecipeSampler} {
		isLoom, err := IsLoom(recipe)
		if err != nil || !isLoom {
			continue
		}
		parsed, err := parseEmbeddedRecipe(recipe)
		if err != nil {
			t.Fatalf("parseEmbeddedRecipe(%q) error = %v; want nil", recipe, err)
		}
		if fault := discussionApproveSeamFault(parsed); fault != "" {
			t.Errorf("recipe %q: %s; want the rule kept", recipe, fault)
		}
	}

	faults := []struct {
		name    string
		recipe  string
		wantHas string
	}{
		{"ApprovalRowWithoutTheKey", "version: 1\n" +
			"entry: Fixture-Write\n" +
			"terminals: [Fixture-Bouncer]\n" +
			"producers:\n" +
			"  - name: Fixture-Write\n" +
			"    engine: DiscussionWrite\n" +
			"    on_done: Fixture-Bouncer\n" +
			"  - name: Fixture-Bouncer\n" +
			"    engine: Bouncer\n" +
			"    segment: Fixture-Review\n", "approves the discussion but carries no"},
		{"CodeBouncerWithTheKey", approveSeamFixture("discussion"), "does not approve the discussion"},
	}
	for _, tt := range faults {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := shedbuild.Parse([]byte(tt.recipe))
			if err != nil {
				t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
			}
			if fault := discussionApproveSeamFault(parsed); !strings.Contains(fault, tt.wantHas) {
				t.Errorf("discussionApproveSeamFault() = %q; want it to contain %q", fault, tt.wantHas)
			}
		})
	}
}
