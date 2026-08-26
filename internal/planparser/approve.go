// approve.go implements SetApproved, this package's first plan-format write path. The write must
// live here, not in any caller, because the Planparser Sole-Parser Invariant reserves every
// plan-format write, the same way it reserves every plan-format read, to this one package.

package planparser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// approvedLinePrefix is the exact frontmatter line prefix SetApproved rewrites or inserts.
const approvedLinePrefix = "approved:"

// SetApproved flips the plan overview's `approved:` frontmatter key to true, leaving every other
// byte of the file untouched: the remaining frontmatter keys, their order, the "---" fences, the
// framing paragraph, the Card Index, and every plan-level body section.
// It resolves the overview path as filepath.Join(planDir, overviewFileName), reusing the
// package's own overviewFileName constant rather than a new literal, and never composes the
// `_lyx` literal itself, per the Lyxdirs Single-Declarer Invariant.
// It reads the file, separates the leading frontmatter block with splitFrontmatter, rewrites only
// the approved: line's value, and writes the file back -- it never round-trips the frontmatter
// through the YAML decoder, since that would not preserve the body and would reorder or requote
// the keys.
// SetApproved is idempotent: an overview already carrying approved: true is a successful no-op
// leaving the file byte-identical. When the frontmatter block carries no approved: key at all,
// SetApproved inserts approved: true into the block rather than failing, so the seam is total over
// every plan ParsePlan accepts.
// A missing overview file, an unreadable overview file, and a file with no "---"-fenced
// frontmatter block are each an error, wrapped with this package's "planparser:" error prefix
// convention.
func SetApproved(planDir string) error {
	overviewPath := filepath.Join(planDir, overviewFileName)

	data, err := os.ReadFile(overviewPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("planparser: plan overview not found: %s", overviewPath)
		}
		return fmt.Errorf("planparser: read plan overview %s: %w", overviewPath, err)
	}

	fmBlock, body, found, err := splitFrontmatter(string(data))
	if err != nil {
		return fmt.Errorf("planparser: plan overview %s: %w", overviewPath, err)
	}
	if !found {
		return fmt.Errorf("planparser: plan overview %s: missing required frontmatter", overviewPath)
	}

	newBlock, changed := setApprovedLine(fmBlock)
	if !changed {
		return nil
	}

	newContent := "---\n" + newBlock + "\n---\n" + body
	if err := os.WriteFile(overviewPath, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("planparser: write plan overview %s: %w", overviewPath, err)
	}

	return nil
}

// setApprovedLine rewrites block's approved: line to "approved: true", inserting that line at the
// end of the block when none is present. It reports whether it made any change, so SetApproved can
// treat an already-approved overview as a no-op that never rewrites the file.
func setApprovedLine(block string) (newBlock string, changed bool) {
	lines := strings.Split(block, "\n")

	for i, line := range lines {
		if !strings.HasPrefix(line, approvedLinePrefix) {
			continue
		}
		if strings.TrimSpace(line) == "approved: true" {
			return block, false
		}
		lines[i] = "approved: true"
		return strings.Join(lines, "\n"), true
	}

	lines = append(lines, "approved: true")
	return strings.Join(lines, "\n"), true
}
