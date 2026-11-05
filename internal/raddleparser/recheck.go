// recheck.go implements RecheckBoard, the proposal's second look at the board at approval and at filing.

package raddleparser

import (
	"fmt"
	"slices"
	"time"
)

// Recheck is the result of RecheckBoard.
// Failures are the rules the board now breaks.
// Landed names the board_checked entries now done or gone from the board; they fail nothing.
// Dropped names, only when BoardContext.Filing is set, the board_checked entries gone from the board that a part depends on, which Raddle-File drops.
type Recheck struct {
	Failures []Finding
	Landed   []string
	Dropped  []string
}

// RecheckBoard judges p against the board as it is now.
// It fails a board_checked entry still open with another `revised` stamp, an open task or project that carries a module label of the Sketch and is absent from board_checked, whenever it was created or promoted, and a filed slug now taken.
func RecheckBoard(p Proposal, board BoardContext) Recheck {
	var result Recheck
	for _, checked := range p.BoardChecked {
		entryAt := slices.IndexFunc(board.Entries, func(entry BoardEntry) bool { return entry.Slug == checked.Slug })
		if entryAt < 0 || !board.Entries[entryAt].Open {
			result.Landed = append(result.Landed, checked.Slug)
			continue
		}
		if !sameStamp(board.Entries[entryAt].Revised, checked.Revised) {
			result.Failures = append(result.Failures, Finding{Check: CheckRevisedMoved, Detail: fmt.Sprintf("the entry %q was revised at %s after the proposal read %s", checked.Slug, board.Entries[entryAt].Revised, checked.Revised)})
		}
	}
	for _, slug := range uncheckedEntries(p, board) {
		result.Failures = append(result.Failures, Finding{Check: CheckBoardUnchecked, Detail: fmt.Sprintf("the open entry %q carries a module label of the Sketch and is not in board_checked", slug)})
	}
	for _, slug := range filedSlugs(p) {
		if takenSlug(slug, board) {
			result.Failures = append(result.Failures, Finding{Check: CheckSlugTaken, Part: slug, Detail: fmt.Sprintf("the slug %q is already on the board or in the raddle store", slug)})
		}
	}

	if board.Filing {
		for _, part := range p.Parts {
			for _, dep := range part.DependsOn {
				gone := slices.ContainsFunc(p.BoardChecked, func(checked BoardCheck) bool { return checked.Slug == dep }) &&
					!slices.ContainsFunc(board.Entries, func(entry BoardEntry) bool { return entry.Slug == dep })
				if gone && !slices.Contains(result.Dropped, dep) {
					result.Dropped = append(result.Dropped, dep)
				}
			}
		}
	}
	return result
}

// sameStamp reports whether two RFC 3339 stamps name one instant, and falls back to comparing the text when either does not parse.
func sameStamp(a, b string) bool {
	first, firstErr := time.Parse(time.RFC3339, a)
	second, secondErr := time.Parse(time.RFC3339, b)
	if firstErr != nil || secondErr != nil {
		return a == b
	}
	return first.Equal(second)
}
