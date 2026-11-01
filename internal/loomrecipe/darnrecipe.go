// darnrecipe.go implements the darn recipe's verify-budget helpers: the rows that carry the budget, its check and its stamp.
// The budget is not declared in the recipe: New stamps the verify budget `darn.yaml` supplies onto those rows at every build.

package loomrecipe

import (
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// verifyGateName is the gate entry whose attempts the verify budget sets.
const verifyGateName = "verify"

// codeReviewSegment is the review segment of the code review perch, the darn recipe's only one.
const codeReviewSegment = "Code-Review"

// treeScopeVerifySource is the verify_source value of a recipe whose verify targets come from the hub's configured trees.
const treeScopeVerifySource = "tree_scope"

// burlerFixEngine is the recipe engine name of a review segment's fix row.
const burlerFixEngine = "BurlerFix"

// verifyGatedRows returns the names of the rows whose verify gate carries the verify budget: the build row and the Code-Review segment's BurlerFix row, each only when it declares a verify gate entry.
// A recipe whose verify_source is not tree_scope has none, and neither has one with no build row.
func verifyGatedRows(recipe shedbuild.Recipe) []string {
	if recipe.VerifySource != treeScopeVerifySource {
		return nil
	}
	var names []string
	hasBuildRow := false
	for _, row := range recipe.Producers {
		if !slices.Contains(shedrecipe.LoomBuildEngines, row.Engine) {
			continue
		}
		hasBuildRow = true
		if hasVerifyGate(row) {
			names = append(names, row.Name)
		}
	}
	if !hasBuildRow {
		return nil
	}
	for _, row := range recipe.Producers {
		if row.Engine == burlerFixEngine && row.Segment == codeReviewSegment && hasVerifyGate(row) {
			names = append(names, row.Name)
		}
	}

	return names
}

// hasVerifyGate reports whether the recipe row declares a verify gate entry.
func hasVerifyGate(row shedbuild.Row) bool {
	gates, _ := row.Config["gates"].([]any)
	for _, raw := range gates {
		if entry, ok := raw.(map[string]any); ok && entry["name"] == verifyGateName {
			return true
		}
	}

	return false
}

// checkVerifyAttempts refuses a verify attempt budget below 1.
func checkVerifyAttempts(attempts int) error {
	if attempts < 1 {
		return fmt.Errorf("loomrecipe: verify_attempts = %d; want at least 1; way forward: run \"lym config darn --set verify_attempts=<n>\" from the prime, then \"lym loom resume\"", attempts)
	}

	return nil
}

// applyVerifyAttempts sets the attempts of the verify gate entry of the recipe row named row.
// A missing row, a row with no gates and a row with no verify entry are each an error.
func applyVerifyAttempts(recipe *shedbuild.Recipe, row string, attempts int) error {
	for i := range recipe.Producers {
		if recipe.Producers[i].Name != row {
			continue
		}
		gates, _ := recipe.Producers[i].Config["gates"].([]any)
		for _, raw := range gates {
			if entry, ok := raw.(map[string]any); ok && entry["name"] == verifyGateName {
				entry["attempts"] = attempts
				return nil
			}
		}
		return fmt.Errorf("loomrecipe: row %q has no %q gate entry to stamp the verify budget onto", row, verifyGateName)
	}

	return fmt.Errorf("loomrecipe: recipe has no row %q to stamp the verify budget onto", row)
}
