// patternplan.go holds the plan gate's PATTERN checks: pattern-removes-prescribed, pattern-headroom and pattern-line-unprescribed.
// They compare the plan's fenced entry lines and its declared PATTERN changes with the overview in the worktree and at the base, so they live beside the other context checks rather than in planparser.

package loomshed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// patternHeadroomCheck names the finding patternPlanFindings appends when the overview the plan's fenced entry lines project grows past the headroom line.
const patternHeadroomCheck = "pattern-headroom"

// patternRemovesPrescribedCheck names the finding patternPlanFindings appends when a card fences a line for an entry the plan removes.
const patternRemovesPrescribedCheck = "pattern-removes-prescribed"

// patternLineUnprescribedCheck names the finding patternPlanFindings appends for a declared added or changed entry that no card fences a line for.
const patternLineUnprescribedCheck = "pattern-line-unprescribed"

// patternPlanFindings returns the PATTERN checks' findings, in this order.
// First one pattern-removes-prescribed per removed entry that a card in cards fences a line for.
// Then a pattern-headroom when the overview projected from every card of plan grows past the headroom line.
// Last, only with pc.DecisionRecordPath set, one pattern-line-unprescribed per added or changed entry that no card fences a line for and whose bullet the worktree overview still holds as the base does.
// It reads nothing and returns nil when pc.PatternBase is nil or worktreeRoot holds no PATTERN.md; a read error of either overview is returned.
func patternPlanFindings(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string, pc PlanContext) ([]planindex.Finding, error) {
	if pc.PatternBase == nil {
		return nil, nil
	}
	overview, err := os.ReadFile(pattern.File(worktreeRoot))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	base, err := pc.PatternBase()
	if err != nil {
		return nil, fmt.Errorf("read PATTERN.md at the base: %w", err)
	}

	var removed []string
	for _, change := range plan.PatternChanges {
		if change.ID != "" && change.Action == "removes" {
			removed = append(removed, change.ID)
		}
	}

	var findings []planindex.Finding
	for _, change := range plan.PatternChanges {
		if change.ID == "" || change.Action != "removes" {
			continue
		}
		for _, card := range cards {
			if !fencesEntry(card, change.ID) {
				continue
			}
			findings = append(findings, planindex.Finding{
				Check:    patternRemovesPrescribedCheck,
				Card:     card.ID(),
				Detail:   fmt.Sprintf("card %d fences a line for %s, which the plan's PATTERN changes removes; way forward: drop the fenced line from the card, since the plan both drops and rewrites the entry", card.Number, planBullet(change)),
				Severity: planindex.SeverityBlocking,
			})
			break
		}
	}

	var prescribed []string
	for _, card := range plan.Cards {
		for _, entry := range planparser.CardPatternEntries(card) {
			prescribed = append(prescribed, entry.Line)
		}
	}
	if finding, over := pattern.Headroom(base, pattern.ProjectOverview(overview, prescribed, removed)); over {
		findings = append(findings, planindex.Finding{
			Check:    patternHeadroomCheck,
			Detail:   finding.Message + "; way forward: shorten the prescribed lines or move detail into the entries' background files, and when that is not enough ask the parent to record a `changes` decision that shortens other entries with `lyx loom decision add --pattern-entry ... --pattern-action changes --pattern-text ...` and prescribe those lines on a card; once the plan is approved, the parent passes `--amends-plan` as well",
			Severity: planindex.SeverityBlocking,
		})
	}

	if pc.DecisionRecordPath == "" {
		return findings, nil
	}
	for _, change := range plan.PatternChanges {
		if change.ID == "" || (change.Action != "adds" && change.Action != "changes") {
			continue
		}
		if anyCardFencesEntry(plan.Cards, change.ID) {
			continue
		}
		baseBullet, inBase := pattern.EntryBullet(base, change.ID)
		worktreeBullet, inWorktree := pattern.EntryBullet(overview, change.ID)
		if inBase != inWorktree || baseBullet != worktreeBullet {
			continue
		}
		findings = append(findings, planindex.Finding{
			Check:    patternLineUnprescribedCheck,
			Detail:   fmt.Sprintf("the plan declares %s and no card fences its overview line; way forward: prescribe the entry's whole overview line in a fenced block on a pending card that edits PATTERN.md, or, when that card is done, on a card inserted with `lyx loom insert-card` after the last begun batch", planBullet(change)),
			Severity: planindex.SeverityBlocking,
		})
	}
	return findings, nil
}

// fencesEntry reports whether card fences a line for the entry id.
func fencesEntry(card planparser.Card, id string) bool {
	for _, entry := range planparser.CardPatternEntries(card) {
		if entry.ID == id {
			return true
		}
	}
	return false
}

// anyCardFencesEntry reports whether any of cards fences a line for the entry id.
func anyCardFencesEntry(cards []planparser.Card, id string) bool {
	for _, card := range cards {
		if fencesEntry(card, id) {
			return true
		}
	}
	return false
}
