// scopeline.go — where a recipe closes a task's scope: the span from its first Code-Review row to its Finalize row.
//
// The answer is read from the recipe's own rows, never from its name.

package loomrecipe

import "github.com/Knatte18/loomyard/internal/shedrecipe"

// InClosedScope reports whether a run of the named recipe at currentRow stands where its task's scope is closed.
// The scope closes when the run reaches the first row of the recipe's Code-Review segment.
// The last Code-Review round is the last step that can change the code against the record.
// A row at or past the recipe's Finalize row is outside the closed span.
// Finalize's merge and test fixes are no change of scope.
// reviewRan reports whether the run's Code-Review segment has already run;
// a run sent back before the segment afterwards is then still inside the closed span.
// It answers false for a recipe with no Code-Review segment.
// Unless reviewRan is set, it also answers false for an empty currentRow or one the recipe does not hold.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func InClosedScope(recipe, currentRow string, reviewRan bool) (bool, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return false, err
	}
	firstReviewIndex, finalizeIndex, currentIndex := -1, -1, -1
	for i, row := range parsed.Producers {
		if row.Segment == codeReviewSegment && firstReviewIndex < 0 {
			firstReviewIndex = i
		}
		if row.Engine == shedrecipe.LoomFinalizeEngine && finalizeIndex < 0 {
			finalizeIndex = i
		}
		if currentRow != "" && row.Name == currentRow {
			currentIndex = i
		}
	}
	if firstReviewIndex < 0 {
		return false, nil
	}
	if finalizeIndex >= 0 && currentIndex >= finalizeIndex {
		return false, nil
	}
	return reviewRan || currentIndex >= firstReviewIndex, nil
}

// CodeReviewRunSubdir returns the `run_subdir` of the named recipe's Code-Review Bouncer row.
// The bool is false for a recipe with no such row or one whose row names no run_subdir.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func CodeReviewRunSubdir(recipe string) (string, bool, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return "", false, err
	}
	for _, row := range parsed.Producers {
		if row.Engine != bouncerEngine || row.Segment != codeReviewSegment {
			continue
		}
		subdir, ok := row.Config["run_subdir"].(string)
		return subdir, ok && subdir != "", nil
	}
	return "", false, nil
}
