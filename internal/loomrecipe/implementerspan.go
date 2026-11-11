// implementerspan.go — whether a run stands between its plan's approval and its recipe's last implementer row.
//
// The answer is read from the recipe's own rows, never from its name.

package loomrecipe

import "github.com/Knatte18/loomyard/internal/shedbuild"

const (
	// planWriteEngine is the engine of the row that writes a recipe's plan.
	planWriteEngine = "PlanWrite"
	// implementerEngine is the engine of a recipe's Webster rows.
	implementerEngine = "Implementer"
)

// planApprovalRow returns the name of the row that approves parsed's plan:
// the row its PlanWrite row's on_done names, when that row's engine is the Bouncer.
// It reports false for a recipe with no PlanWrite row, or one whose PlanWrite row's on_done names no Bouncer row.
func planApprovalRow(parsed shedbuild.Recipe) (string, bool) {
	for _, row := range parsed.Producers {
		if row.Engine != planWriteEngine {
			continue
		}
		for _, next := range parsed.Producers {
			if next.Name == row.OnDone && next.Engine == bouncerEngine {
				return next.Name, true
			}
		}
		return "", false
	}
	return "", false
}

// InImplementerSpan reports whether a run of the named recipe at currentRow stands past its plan's approval.
// It answers false for a row past the recipe's last Implementer row.
// A run there has an approved plan its Webster sessions build from, and each fresh Webster prompt is rendered from it.
// currentRow is mapped through RetiredRows first.
// The approval counts as passed after the last row of the approval row's segment.
// It answers false for a recipe with no plan approval row or no Implementer row.
// It also answers false for an empty currentRow or one the recipe does not hold.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func InImplementerSpan(recipe, currentRow string) (bool, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return false, err
	}
	approvalRow, hasRow := planApprovalRow(parsed)
	if !hasRow || currentRow == "" {
		return false, nil
	}
	if mapped, retired := RetiredRows(recipe)[currentRow]; retired {
		currentRow = mapped
	}

	segment := ""
	for _, row := range parsed.Producers {
		if row.Name == approvalRow {
			segment = row.Segment
		}
	}
	lastApprovalIndex, lastImplementerIndex, currentIndex := -1, -1, -1
	for i, row := range parsed.Producers {
		if row.Name == approvalRow || (segment != "" && row.Segment == segment) {
			lastApprovalIndex = i
		}
		if row.Engine == implementerEngine {
			lastImplementerIndex = i
		}
		if row.Name == currentRow {
			currentIndex = i
		}
	}
	return currentIndex > lastApprovalIndex && currentIndex <= lastImplementerIndex, nil
}
