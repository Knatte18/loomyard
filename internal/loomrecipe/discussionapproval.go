// discussionapproval.go — a recipe's discussion approval row and the test of whether a run stands past it.
//
// Both answers are read from the recipe's own rows, never from its name.

package loomrecipe

import "github.com/Knatte18/loomyard/internal/shedbuild"

// discussionWriteEngine is the engine of the row that writes a recipe's discussion.
const discussionWriteEngine = "DiscussionWrite"

// discussionApprovalRow returns the name of the row that approves parsed's discussion: the row its DiscussionWrite row's on_done names, when that row's engine is the Bouncer.
// It reports false for a recipe with no DiscussionWrite row, and for one whose DiscussionWrite row hands to a row that is not a Bouncer.
func discussionApprovalRow(parsed shedbuild.Recipe) (string, bool) {
	for _, row := range parsed.Producers {
		if row.Engine != discussionWriteEngine {
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

// DiscussionApprovalRow returns the name of the row that approves the named recipe's discussion, and reports whether the recipe has one.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func DiscussionApprovalRow(recipe string) (string, bool, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return "", false, err
	}
	row, hasRow := discussionApprovalRow(parsed)
	return row, hasRow, nil
}

// PastDiscussionApproval reports whether a run of the named recipe standing at currentRow is past its discussion approval, and whether the recipe has an approval row at all.
// hasRow is false, and past false, for a recipe with no approval row.
// Otherwise a run is past when currentRow, mapped through RetiredRows, is a row of the recipe after every row of the approval row's segment.
// An empty currentRow, a row before or inside that segment, and a row the recipe does not hold are not past.
// The recipe's rows are read expanded, so an approval row inside a block, as the tapestry recipe's Motif block holds it, is found where the run meets it.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func PastDiscussionApproval(recipe, currentRow string) (past, hasRow bool, err error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return false, false, err
	}
	approvalRow, hasRow := discussionApprovalRow(parsed)
	if !hasRow {
		return false, false, nil
	}

	segment := ""
	for _, row := range parsed.Producers {
		if row.Name == approvalRow {
			segment = row.Segment
		}
	}
	lastApprovalIndex, currentIndex := -1, -1
	if mapped, retired := RetiredRows(recipe)[currentRow]; retired {
		currentRow = mapped
	}
	for i, row := range parsed.Producers {
		if row.Name == approvalRow || (segment != "" && row.Segment == segment) {
			lastApprovalIndex = i
		}
		if currentRow != "" && row.Name == currentRow {
			currentIndex = i
		}
	}
	return currentIndex > lastApprovalIndex, true, nil
}
