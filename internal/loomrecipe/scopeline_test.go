// scopeline_test.go — tests for the span of a recipe's rows where a task's scope is closed.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestInClosedScope pins which rows of each recipe close a task's scope, with and without a Code-Review segment that ran.
func TestInClosedScope(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		recipe    string
		row       string
		reviewRan bool
		want      bool
	}{
		{"DarnWriterBeforeTheLine", shedrun.RecipeDarn, "Darn-Write", false, false},
		{"DarnDriftVerifyBeforeTheLine", shedrun.RecipeDarn, "Drift-Verify", false, false},
		{"DarnFirstCodeReviewRow", shedrun.RecipeDarn, "Code-Bouncer", false, true},
		{"DarnInsideTheSegment", shedrun.RecipeDarn, "Code-Burler-Fix", false, true},
		{"DarnDriftRecheck", shedrun.RecipeDarn, "Drift-Recheck", false, true},
		{"DarnPRReview", shedrun.RecipeDarn, "PR-Review", false, true},
		{"DarnFinalizeIsOutside", shedrun.RecipeDarn, "Finalize", true, false},
		{"DarnPastFinalize", shedrun.RecipeDarn, "Friction-Reflect", false, false},
		{"DarnWriterAfterAStartedRound", shedrun.RecipeDarn, "Darn-Write", true, true},
		{"TapestryWebsterBeforeTheLine", shedrun.RecipeTapestry, "Webster", false, false},
		{"TapestryPlanReviewIsNoCodeReview", shedrun.RecipeTapestry, "Plan-Bouncer", false, false},
		{"TapestryDescribe", shedrun.RecipeTapestry, "Describe", false, true},
		{"NoStatusFile", shedrun.RecipeDarn, "", false, false},
		{"RowTheRecipeLacks", shedrun.RecipeDarn, "No-Such-Row", false, false},
		{"ArrasHasNoCodeReview", shedrun.RecipeArras, "Raddle-Bouncer", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := InClosedScope(tt.recipe, tt.row, tt.reviewRan)
			if err != nil || got != tt.want {
				t.Errorf("InClosedScope(%q, %q, %v) = %v, %v; want %v, nil", tt.recipe, tt.row, tt.reviewRan, got, err, tt.want)
			}
		})
	}
	if _, err := InClosedScope("no-such-recipe", "Code-Bouncer", false); err == nil {
		t.Error("InClosedScope(unknown recipe) error = nil; want a refusal")
	}
}

// TestCodeReviewRunSubdir pins the run directory of each recipe's Code-Review perch, and none for a recipe without one.
func TestCodeReviewRunSubdir(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		recipe  string
		want    string
		wantRow bool
	}{
		{"Darn", shedrun.RecipeDarn, "code", true},
		{"Tapestry", shedrun.RecipeTapestry, "code", true},
		{"Arras", shedrun.RecipeArras, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, hasRow, err := CodeReviewRunSubdir(tt.recipe)
			if err != nil || got != tt.want || hasRow != tt.wantRow {
				t.Errorf("CodeReviewRunSubdir(%q) = %q, %v, %v; want %q, %v, nil", tt.recipe, got, hasRow, err, tt.want, tt.wantRow)
			}
		})
	}
}
