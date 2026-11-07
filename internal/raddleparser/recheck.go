// recheck.go implements RecheckBoard, the proposal's second look at the board at approval and at filing.

package raddleparser

import (
	"fmt"
	"slices"
	"time"
)

// Recheck is the outcome of checking a proposal against the board as it stands now.
// Failures lists the rules the board now breaks.
// Landed lists the board_checked slugs that are now done or gone from the board; they fail nothing.
// Dropped lists, only for a check made for filing, the board_checked slugs gone from the board that a part depends on.
// Raddle-File drops them.
type Recheck struct {
	Failures []Finding
	Landed   []string
	Dropped  []string
}

// RecheckBoard judges a proposal against the board as it is now.
// It is the second look at the board, at approval and at filing, for changes since the proposal was read.
// It fails a board_checked entry that is still open with another `revised` stamp.
// It fails an open task or project that carries a module label of the Sketch and is absent from board_checked.
// The run's own entry is exempt from that rule.
// It fails a filed slug that the board or the raddle store already holds.
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

// sameStamp reports whether two RFC 3339 stamps name one instant.
// When either stamp does not parse, it compares the texts instead.
func sameStamp(a, b string) bool {
	first, firstErr := time.Parse(time.RFC3339, a)
	second, secondErr := time.Parse(time.RFC3339, b)
	if firstErr != nil || secondErr != nil {
		return a == b
	}
	return first.Equal(second)
}
