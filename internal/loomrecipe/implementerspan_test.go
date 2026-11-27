// implementerspan_test.go — tests for the span between a plan's approval and a recipe's last implementer row.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestInImplementerSpan pins which rows of each recipe stand past the plan's approval and not past the last Webster row.
func TestInImplementerSpan(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		recipe string
		row    string
		want   bool
	}{
		{"BeforeThePlan_DriftBaseline", shedrun.RecipeTapestry, "Drift-Baseline", false},
		{"ThePlanWriteRow", shedrun.RecipeTapestry, "Plan-Write", false},
		{"TheApprovalRow", shedrun.RecipeTapestry, "Plan-Bouncer", false},
		{"InsideTheApprovalSegment", shedrun.RecipeTapestry, "Plan-Burler-Fix", false},
		{"BetweenApprovalAndWebster_DriftAwait", shedrun.RecipeTapestry, "Drift-Await", true},
		{"BetweenApprovalAndWebster_DriftExpect", shedrun.RecipeTapestry, "Drift-Expect", true},
		{"TheWebsterRow", shedrun.RecipeTapestry, "Webster", true},
		{"PastTheLastWebsterRow", shedrun.RecipeTapestry, "Parent-Align", false},
		{"RetiredRowPastWebster", shedrun.RecipeTapestry, "Merge-In", false},
		{"NoStatusFile", shedrun.RecipeTapestry, "", false},
		{"RowTheRecipeLacks", shedrun.RecipeTapestry, "No-Such-Row", false},
		{"DarnHasNoPlan", shedrun.RecipeDarn, "Darn-Write", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := InImplementerSpan(tt.recipe, tt.row)
			if err != nil || got != tt.want {
				t.Errorf("InImplementerSpan(%q, %q) = %v, %v; want %v, nil", tt.recipe, tt.row, got, err, tt.want)
			}
		})
	}
	if _, err := InImplementerSpan("no-such-recipe", "Webster"); err == nil {
		t.Error("InImplementerSpan(unknown recipe) error = nil; want a refusal")
	}
}
