// plancontext.go holds the plan gate's two context checks, rename-tool-missing and pattern-changes-mismatch.
// Both compare the plan with something outside the plan directory, so they live here rather than in planparser.

package loomshed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// renameToolMissingCheck names the finding planContextFindings appends for a symbol Rename pair when the hub lists no rename tool.
const renameToolMissingCheck = "rename-tool-missing"

// patternChangesMismatchCheck names the finding planContextFindings appends for a PATTERN change the plan and the decision record do not both declare.
const patternChangesMismatchCheck = "pattern-changes-mismatch"

// PlanContext is what the plan gate checks the plan against besides the code tree.
// The zero value checks nothing: no rename tool is required to be missing, and no record is compared.
type PlanContext struct {
	// RenameTools is the hub's language.yaml rename_tools list.
	// A symbol Rename pair needs it non-empty.
	RenameTools []string
	// DecisionRecordPath is the decision record whose `## PATTERN changes` section the plan must carry.
	// Empty skips the comparison.
	DecisionRecordPath string
}

// planContextFindings returns the context checks' blocking findings over cards, in this order: one rename-tool-missing per symbol Rename pair when pc.RenameTools is empty, then one pattern-changes-mismatch per bullet the plan and the decision record do not both declare.
// The comparison is over plan, not cards.
// An absent record adds nothing; a read error is returned, and so is a record whose section is malformed, since the plan writer cannot fix the record.
func planContextFindings(plan *planparser.Plan, cards []planparser.Card, pc PlanContext) ([]planindex.Finding, error) {
	var findings []planindex.Finding
	if len(pc.RenameTools) == 0 {
		for _, card := range cards {
			for _, pair := range planparser.SymbolRenamePairs(plan, []planparser.Card{card}) {
				findings = append(findings, planindex.Finding{
					Check:    renameToolMissingCheck,
					Card:     card.ID(),
					Detail:   fmt.Sprintf("the symbol rename %s -> %s needs the hub's structure-aware rename tool, and language.yaml lists none; way forward: set rename_tools with \"lyx config language\" from the prime, or recast the card", pair.Old, pair.New),
					Severity: planindex.SeverityBlocking,
				})
			}
		}
	}

	mismatches, err := patternChangeMismatches(plan, pc.DecisionRecordPath)
	if err != nil {
		return nil, err
	}
	return append(findings, mismatches...), nil
}

// patternChangeMismatches compares the plan's PATTERN changes with those of the decision record at recordPath.
// An empty recordPath or an absent record returns nil.
func patternChangeMismatches(plan *planparser.Plan, recordPath string) ([]planindex.Finding, error) {
	if recordPath == "" {
		return nil, nil
	}
	data, err := os.ReadFile(recordPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	recorded, recordFindings := discussionparser.ParsePatternChanges(data)
	if len(recordFindings) > 0 {
		return nil, fmt.Errorf("decision record %s: %s: %s; fix the section and confirm it with \"lyx loom validate-discussion\"", recordPath, "## PATTERN changes", formatDiscussionFindings(recordFindings))
	}

	type entry struct{ id, action, text string }
	declared := make(map[entry]bool, len(plan.PatternChanges))
	for _, c := range plan.PatternChanges {
		declared[entry{c.ID, c.Action, c.Text}] = true
	}
	recordedSet := make(map[entry]bool, len(recorded))
	for _, c := range recorded {
		recordedSet[entry{c.ID, c.Action, c.Text}] = true
	}

	const wayForward = "way forward: copy the decision record's `## PATTERN changes` bullets into `00-overview.md`'s `## PATTERN changes` section, one per entry, without the added-after-Discussion suffix"
	var findings []planindex.Finding
	for _, c := range recorded {
		if !declared[entry{c.ID, c.Action, c.Text}] {
			findings = append(findings, planindex.Finding{
				Check:    patternChangesMismatchCheck,
				Detail:   fmt.Sprintf("the decision record declares %s and the plan's PATTERN changes section lacks it; %s", c.Bullet(), wayForward),
				Severity: planindex.SeverityBlocking,
			})
		}
	}
	for _, c := range plan.PatternChanges {
		if !recordedSet[entry{c.ID, c.Action, c.Text}] {
			findings = append(findings, planindex.Finding{
				Check:    patternChangesMismatchCheck,
				Detail:   fmt.Sprintf("the plan's PATTERN changes section declares %s and the decision record lacks it; %s", planBullet(c), wayForward),
				Severity: planindex.SeverityBlocking,
			})
		}
	}
	return findings, nil
}

// planBullet renders a plan bullet for a finding: the bullet shape when the line fit it, else the line as written.
func planBullet(c planparser.PatternChange) string {
	if c.ID == "" {
		return "`" + c.Text + "`"
	}
	return discussionparser.PatternChange{ID: c.ID, Action: c.Action, Text: c.Text}.Bullet()
}
