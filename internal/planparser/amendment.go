// amendment.go implements AppendAmendment, the plan's third write path: an append, distinct from
// both SetApproved (a frontmatter flip) and RewriteRefs (an in-place ref substitution), so it must
// be named as its own path rather than left implicit. The log lives beside the plan because the
// plan directory is the thing that changed, and planparser stays the sole writer of that tree
// because it owns the file. Append-only means append-only: AppendAmendment never rewrites an
// existing entry, never sorts, and never deduplicates. Commit messages were rejected as the record
// for a terminal reason -- a squash-merge erases them, so the history would exist only where
// nothing can read it back.

package planparser

import (
	"fmt"
	"os"
	"path/filepath"
)

// AmendmentsFileName is the fixed filename of a plan's append-only amendment log, joined onto a
// plan directory the same way overviewFileName is. This package is the sole declarer of that name.
const AmendmentsFileName = "amendments.md"

// amendmentsFileHeading is the fixed heading AppendAmendment writes when it creates the amendments
// file for the first time.
const amendmentsFileHeading = "# Amendments\n"

// Amendment is one append-only amendment log entry: a resolved glyph substitution recorded beside
// the plan it amends, for a human reading the log rather than the commit history a squash-merge
// erases.
type Amendment struct {
	// Timestamp is when the amendment was made.
	Timestamp string
	// Card is the "N-<slug>" card identity the amendment concerns.
	Card string
	// OldGlyph is the pre-amendment glyph or handle string.
	OldGlyph string
	// NewGlyph is the post-amendment glyph string.
	NewGlyph string
	// Tier names which check tier surfaced the amendment.
	Tier string
	// SHA is the commit SHA the amendment corresponds to.
	SHA string
}

// AppendAmendment renders one markdown list item carrying all six of a's fields, in declared
// order, and appends it to filepath.Join(planDir, AmendmentsFileName), creating the file with
// amendmentsFileHeading when it does not yet exist. It never rewrites, sorts, or deduplicates any
// existing entry -- append-only means append-only. Errors are wrapped with this package's
// "planparser:" prefix convention, matching SetApproved's own.
func AppendAmendment(planDir string, a Amendment) error {
	path := filepath.Join(planDir, AmendmentsFileName)

	existing, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("planparser: read amendments file %s: %w", path, err)
		}
		existing = []byte(amendmentsFileHeading)
	}

	entry := fmt.Sprintf(
		"- Timestamp: %s, Card: %s, OldGlyph: %s, NewGlyph: %s, Tier: %s, SHA: %s\n",
		a.Timestamp, a.Card, a.OldGlyph, a.NewGlyph, a.Tier, a.SHA,
	)

	newContent := append(existing, []byte(entry)...)
	if err := os.WriteFile(path, newContent, 0o644); err != nil {
		return fmt.Errorf("planparser: write amendments file %s: %w", path, err)
	}

	return nil
}
