// discussionapproval_test.go — tests for a recipe's discussion approval row and the past-the-approval judgment.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPastDiscussionApproval pins the approval row each recipe answers and which rows of a run count as past it.
func TestPastDiscussionApproval(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("ApprovalRow", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			recipe  string
			wantRow string
			wantHas bool
		}{
			{shedrun.RecipeTapestry, "Discussion-Bouncer", true},
			{"", "Discussion-Bouncer", true},
			{shedrun.RecipeDarn, "", false},
		}
		for _, tt := range tests {
			row, has, err := DiscussionApprovalRow(tt.recipe)
			if err != nil || row != tt.wantRow || has != tt.wantHas {
				t.Errorf("DiscussionApprovalRow(%q) = %q, %v, %v; want %q, %v, nil", tt.recipe, row, has, err, tt.wantRow, tt.wantHas)
			}
		}
	})

	t.Run("PastTheApproval", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			recipe  string
			row     string
			want    bool
			wantHas bool
		}{
			{"BeforeTheSegment_Preflight", shedrun.RecipeTapestry, "Preflight", false, true},
			{"BeforeTheSegment_DiscussionWrite", shedrun.RecipeTapestry, "Discussion-Write", false, true},
			{"TheApprovalRow", shedrun.RecipeTapestry, "Discussion-Bouncer", false, true},
			{"InsideTheSegment_Review", shedrun.RecipeTapestry, "Discussion-Burler-Review", false, true},
			{"InsideTheSegment_Fix", shedrun.RecipeTapestry, "Discussion-Burler-Fix", false, true},
			{"NoStatusFile", shedrun.RecipeTapestry, "", false, true},
			{"RowTheRecipeLacks", shedrun.RecipeTapestry, "No-Such-Row", false, true},
			{"AfterTheSegment_DriftBaseline", shedrun.RecipeTapestry, "Drift-Baseline", true, true},
			{"AfterTheSegment_PlanWrite", shedrun.RecipeTapestry, "Plan-Write", true, true},
			{"AfterTheSegment_CodeBouncer", shedrun.RecipeTapestry, "Code-Bouncer", true, true},
			{"RetiredRowMapsToItsReplacement", shedrun.RecipeTapestry, "Merge-In", true, true},
			{"DarnHasNoApprovalRow", shedrun.RecipeDarn, "Darn-Write", false, false},
			{"PawlHasNoApprovalRow", shedrun.RecipePawl, "Finalize", false, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				past, has, err := PastDiscussionApproval(tt.recipe, tt.row)
				if err != nil || past != tt.want || has != tt.wantHas {
					t.Errorf("PastDiscussionApproval(%q, %q) = %v, %v, %v; want %v, %v, nil", tt.recipe, tt.row, past, has, err, tt.want, tt.wantHas)
				}
			})
		}
	})

	t.Run("UnknownRecipeRefused", func(t *testing.T) {
		t.Parallel()
		if _, _, err := PastDiscussionApproval("no-such-recipe", "Preflight"); err == nil {
			t.Errorf("PastDiscussionApproval(unknown recipe) error = nil; want a refusal")
		}
		if _, _, err := DiscussionApprovalRow("no-such-recipe"); err == nil {
			t.Errorf("DiscussionApprovalRow(unknown recipe) error = nil; want a refusal")
		}
	})

	t.Run("RecipeWithoutAnApprovingBouncerAnswersNoRow", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			yaml string
		}{
			{"DiscussionWriteHandsToANonBouncer", "version: 1\nentry: Write\nterminals: [Next]\nproducers:\n" +
				"  - name: Write\n    engine: DiscussionWrite\n    on_done: Next\n" +
				"  - name: Next\n    engine: Preflight\n"},
			{"NoDiscussionWriteRow", "version: 1\nentry: Only\nterminals: [Only]\nproducers:\n" +
				"  - name: Only\n    engine: Preflight\n"},
		}
		for _, tt := range tests {
			parsed, err := shedbuild.Parse([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("%s: shedbuild.Parse: %v", tt.name, err)
			}
			if row, has := discussionApprovalRow(parsed); has {
				t.Errorf("%s: discussionApprovalRow = %q, true; want false", tt.name, row)
			}
		}
	})
}
