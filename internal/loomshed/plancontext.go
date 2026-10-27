// plancontext.go holds the plan gate's context checks: rename-tool-missing, pattern-changes-mismatch, verify-raw-command, verify-gate-no-wait and plan-verify-retired, and calls the PATTERN checks of patternplan.go.
// Each compares the plan with something outside the plan directory, such as the hub's config or the decision record, so they live here rather than in planparser.

package loomshed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// renameToolMissingCheck names the finding planContextFindings appends for a symbol Rename pair when the hub lists no rename tool.
const renameToolMissingCheck = "rename-tool-missing"

// patternChangesMismatchCheck names the finding planContextFindings appends for a PATTERN change the plan and the decision record do not both declare.
const patternChangesMismatchCheck = "pattern-changes-mismatch"

// verifyRawCommandCheck names the finding planContextFindings appends for a card Verify command the hub's refuse_raw list names.
const verifyRawCommandCheck = "verify-raw-command"

// verifyGateNoWaitCheck names the finding planContextFindings appends for a `lym gate test` call in a card Verify line without `--wait`.
const verifyGateNoWaitCheck = "verify-gate-no-wait"

// planVerifyRetiredCheck names the informational finding planContextFindings appends for an overview carrying a `## verify:` section.
const planVerifyRetiredCheck = "plan-verify-retired"

// PlanContext is what the plan gate checks the plan against besides the code tree.
// The zero value checks nothing: no rename tool is required to be missing, no record is compared and no Verify command is refused.
type PlanContext struct {
	// RenameTools is the hub's language.yaml rename_tools list.
	// A symbol Rename pair needs it non-empty.
	RenameTools []string
	// RefuseRaw is the hub's gate.yaml refuse_raw list.
	// A card Verify command opening with an entry is refused.
	RefuseRaw []string
	// DecisionRecordPath is the decision record whose `## PATTERN changes` section the plan must carry.
	// Empty skips the comparison.
	DecisionRecordPath string
	// PatternBase returns the PATTERN.md overview at the task branch's merge base with its parent, or at HEAD with no parent recorded; nil bytes mean the file is absent there.
	// Nil skips the PATTERN checks.
	PatternBase func() ([]byte, error)
}

// planContextFindings returns the context checks' findings over cards, in this order: one rename-tool-missing per symbol Rename pair when pc.RenameTools is empty, one pattern-changes-mismatch per bullet the plan and the decision record do not both declare, then per card the verify-raw-command and verify-gate-no-wait findings of its Verify line, then an informational plan-verify-retired when the overview carries a `## verify:` section.
// The PATTERN findings of patternPlanFindings follow the mismatch findings; they read worktreeRoot's PATTERN.md.
// The record comparison and the retired-section check are over plan, not cards.
// An absent record adds nothing; a read error is returned, and so is a record whose section is malformed, since the plan writer cannot fix the record.
func planContextFindings(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string, pc PlanContext) ([]planindex.Finding, error) {
	var findings []planindex.Finding
	if len(pc.RenameTools) == 0 {
		for _, card := range cards {
			for _, pair := range planparser.SymbolRenamePairs(plan, []planparser.Card{card}) {
				findings = append(findings, planindex.Finding{
					Check:    renameToolMissingCheck,
					Card:     card.ID(),
					Detail:   fmt.Sprintf("the symbol rename %s -> %s needs the hub's structure-aware rename tool, and language.yaml lists none; way forward: set rename_tools with \"lym config language\" from the prime, or recast the card", pair.Old, pair.New),
					Severity: planindex.SeverityBlocking,
				})
			}
		}
	}

	mismatches, err := patternChangeMismatches(plan, pc.DecisionRecordPath)
	if err != nil {
		return nil, err
	}
	findings = append(findings, mismatches...)
	patternFindings, err := patternPlanFindings(plan, cards, worktreeRoot, pc)
	if err != nil {
		return nil, err
	}
	findings = append(findings, patternFindings...)
	findings = append(findings, verifyLineFindings(cards, pc.RefuseRaw)...)
	if plan.Verify != "" {
		findings = append(findings, planindex.Finding{
			Check:    planVerifyRetiredCheck,
			Detail:   "the overview's `## verify:` section is no longer run by any gate; the parser still accepts it, and the hub's gates carry the verification",
			Severity: planindex.SeverityInformational,
		})
	}
	return findings, nil
}

// verifyLauncherWords are the command prefixes the verify checks read past to find the command they launch.
var verifyLauncherWords = []string{"env", "exec", "xargs", "sudo", "time"}

// verifyLineFindings returns, per card, a verify-raw-command finding for each command of its Verify line that opens with an entry of refuseRaw, then a verify-gate-no-wait finding for each `lym gate test` call without `--wait`.
// A command is read in command position: past a leading environment assignment and launcher word.
func verifyLineFindings(cards []planparser.Card, refuseRaw []string) []planindex.Finding {
	var findings []planindex.Finding
	for _, card := range cards {
		commands := planparser.VerifyCommands(card.Verify)
		for _, command := range commands {
			fields := commandFields(command)
			for _, raw := range refuseRaw {
				if hasFieldPrefix(fields, strings.Fields(raw)) {
					findings = append(findings, planindex.Finding{
						Check:    verifyRawCommandCheck,
						Card:     card.ID(),
						Detail:   fmt.Sprintf("card %d Verify: `%s` runs a command the hub's gate.yaml refuse_raw lists, and Go re-runs this line outside any agent hook; way forward: replace it with a `lym gate test --wait` call naming a tier list and a scope form, such as `lym gate test --wait --tier 1 --card <card id>`", card.Number, command),
						Severity: planindex.SeverityBlocking,
					})
					break
				}
			}
		}
		for _, command := range commands {
			fields := commandFields(command)
			if hasFieldPrefix(fields, []string{"lym", "gate", "test"}) && !slices.Contains(fields, "--wait") {
				findings = append(findings, planindex.Finding{
					Check:    verifyGateNoWaitCheck,
					Card:     card.ID(),
					Detail:   fmt.Sprintf("card %d Verify: `%s` lacks `--wait`, so a busy gate slot fails the line instead of waiting; way forward: add `--wait`", card.Number, command),
					Severity: planindex.SeverityBlocking,
				})
			}
		}
	}
	return findings
}

// commandFields splits command into fields and drops the leading launcher words and environment assignments, so the first field is the launched command.
func commandFields(command string) []string {
	fields := strings.Fields(command)
	for len(fields) > 0 && (slices.Contains(verifyLauncherWords, fields[0]) || isEnvironmentAssignment(fields[0])) {
		fields = fields[1:]
	}
	return fields
}

// isEnvironmentAssignment reports whether field is a NAME=value environment assignment.
func isEnvironmentAssignment(field string) bool {
	name, _, found := strings.Cut(field, "=")
	return found && name != "" && !strings.ContainsFunc(name, func(r rune) bool {
		return r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
}

// hasFieldPrefix reports whether fields opens with every field of prefix, and prefix is non-empty.
func hasFieldPrefix(fields, prefix []string) bool {
	return len(prefix) > 0 && len(fields) >= len(prefix) && slices.Equal(fields[:len(prefix)], prefix)
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
		return nil, fmt.Errorf("decision record %s: %s: %s; fix the section and confirm it with \"lym loom validate-discussion\"", recordPath, "## PATTERN changes", formatDiscussionFindings(recordFindings))
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
