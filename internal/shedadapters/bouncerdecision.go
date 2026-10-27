// bouncerdecision.go routes a judged round whose review holds a finding of class decision to the run's parent:
// the escalation with cause decision, the parent's recorded answer, the refusal check that answer meets, and the snapshot of the decision record the carry-over compares against.

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// DecisionHaltPrefix opens the Reason of a halt that only the run's parent may settle.
// Two producers open a reason with it: the Bouncer, for the halt a recorded accept over a held refusal meets at the budget and the hard cap, and Drift-Verify, for its decision halts.
// A caller that must tell such a halt from any other Stuck reads the Reason for it, and arbiterstop hands every one of them to the parent.
const DecisionHaltPrefix = "decision halt: "

// decisionSnapshot mirrors a decision snapshot file's YAML.
type decisionSnapshot struct {
	Titles []string `yaml:"titles"`
}

// decisionEscalationOpen reports whether round carries an escalation with cause decision that no settled accept has closed.
// It is false while EscalateDecisions is off.
// An unreadable or malformed decision file leaves the escalation open, since it is not a settled accept.
func (b *Bouncer) decisionEscalationOpen(round int) bool {
	if !b.cfg.EscalateDecisions {
		return false
	}
	cause, _, escalated, err := readEscalation(b.cfg.RunDir, round)
	if err != nil || !escalated || cause != EscalationDecision {
		return false
	}
	decision, _, settled, exists, err := readCirclingDecision(b.cfg.RunDir, round)
	if err != nil {
		return true
	}
	return !(exists && settled && decision == CirclingAccept)
}

// reviewFindings returns the findings of round's review.
func (b *Bouncer) reviewFindings(round int) ([]burlerengine.Finding, error) {
	raw, err := os.ReadFile(filepath.Join(b.cfg.RunDir, b.cfg.ReportName(round)))
	if err != nil {
		return nil, err
	}
	_, findings, err := burlerengine.ParseReview(raw)
	return findings, err
}

// decisionFindings returns the findings of class decision in round's review.
// A review that cannot be read or does not parse is an error.
func (b *Bouncer) decisionFindings(round int) ([]burlerengine.Finding, error) {
	findings, err := b.reviewFindings(round)
	if err != nil {
		return nil, err
	}
	var decisions []burlerengine.Finding
	for _, finding := range findings {
		if finding.Class == burlerengine.ClassDecision {
			decisions = append(decisions, finding)
		}
	}
	return decisions, nil
}

// routeDecisions settles a judged round whose review holds a finding of class decision, whatever the judge ruled, and reports handled true when it took the round.
// It reports handled false and takes nothing when EscalateDecisions is off or the review does not parse or holds no such finding.
// It does the same when the round carries an escalation record of another cause, a decision file of another cause or a settled accept.
// By the round's decision file, none escalates with cause decision after writing the snapshot of the decision record.
// A pending continue is re-judged when the dropped-key check refuses it, and otherwise runs another round, or at the hard cap is retired and escalated again.
// A pending accept passes the round when the refusal check admits it, with the decision findings left out of its BLOCKING checks.
// A refused accept is re-judged while budget is left, retired and escalated again at the budget below the hard cap, and halts the run at the budget at or past it.
func (b *Bouncer) routeDecisions(ctx context.Context, round int, verdict bouncerVerdict, ledger ledgerFile, spawned bool) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if !b.cfg.EscalateDecisions {
		return "", shedengine.OutputPointer{}, nil, false
	}
	findings, err := b.decisionFindings(round)
	if err != nil || len(findings) == 0 {
		return "", shedengine.OutputPointer{}, nil, false
	}
	cause, _, escalated, err := readEscalation(b.cfg.RunDir, round)
	if err != nil || (escalated && cause != EscalationDecision) {
		return "", shedengine.OutputPointer{}, nil, false
	}
	decision, decisionCause, settled, decided, err := readCirclingDecision(b.cfg.RunDir, round)
	if err != nil {
		if !escalated {
			return "", shedengine.OutputPointer{}, nil, false
		}
		outcome, ptr, derr := b.degrade(ctx, fmt.Sprintf("shedadapters: bouncer circling decision for round %d is unreadable: %v; way forward: fix or delete %s, then run `lym loom start` to resume the round", round, err, circlingDecisionPath(b.cfg.RunDir, round)), "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
		return outcome, ptr, derr, true
	}
	if decided && (decisionCause != EscalationDecision || (decision == CirclingAccept && settled)) {
		return "", shedengine.OutputPointer{}, nil, false
	}

	ptr := shedengine.OutputPointer{Path: ledgerPath(b.cfg.RunDir, round)}
	var outcome shedengine.Outcome
	switch {
	case !decided:
		if !escalated {
			logger.Info("shedadapters: bouncer escalating a decision-class finding to the parent", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "verdict", verdict, "spawned", spawned)
		}
		outcome, ptr, err = b.escalateUndecided(round, verdict, ledger, ptr)
	case decision == CirclingContinue:
		outcome, ptr, err = b.settleDecisionContinue(ctx, round, verdict, ledger, ptr)
	default:
		outcome, ptr, err = b.settleDecisionAccept(ctx, round, verdict, ledger, ptr)
	}
	return outcome, ptr, err, true
}

// escalateUndecided escalates round with cause decision after writing its snapshot.
// A round that already carries the escalation record returns the same Awaiting.
func (b *Bouncer) escalateUndecided(round int, verdict bouncerVerdict, ledger ledgerFile, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := writeDecisionSnapshot(b.cfg.RunDir, round, b.cfg.DecisionRecordPath); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): %w; way forward: make the named path writable, then run `lym loom start`, which re-escalates the round", b.cfg.Name, bouncerEngineLabel, err)
	}
	return b.escalateDecision(round, b.decisionRefusal(round, verdict, ledger, CirclingAccept), false, ptr)
}

// settleDecisionContinue acts on a pending continue recorded for a decision escalation.
// A continue passes nothing, so only the dropped-key check applies: a refusal re-judges the round.
// Otherwise a continue at or past the hard cap is retired and the round escalates again, and below it the next round starts.
func (b *Bouncer) settleDecisionContinue(ctx context.Context, round int, verdict bouncerVerdict, ledger ledgerFile, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if refusal := b.decisionRefusal(round, verdict, ledger, CirclingContinue); refusal != "" {
		return b.rejudge(ctx, round, verdict, refusal)
	}
	if b.atHardCap(round) {
		if err := b.retireContinue(round); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): retire the continue decision of round %d at the hard cap: %w; way forward: make the named path writable, then run `lym loom start`, which re-escalates the round", b.cfg.Name, bouncerEngineLabel, round, err)
		}
		return b.escalateDecision(round, b.decisionRefusal(round, verdict, ledger, CirclingAccept), false, ptr)
	}
	b.ensureFocus(round + 1)
	ptr.BudgetExempt = b.budgetReached(round)
	return shedengine.Stuck, ptr, nil
}

// settleDecisionAccept acts on a pending accept recorded for a decision escalation.
// An admitted accept passes the round.
// A refused one is re-judged while budget is left, and at the budget below the hard cap it is retired and the round escalates again.
// At the budget at or past the hard cap the judge outputs are archived and the run halts, so a resume re-judges the round under the recorded accept.
func (b *Bouncer) settleDecisionAccept(ctx context.Context, round int, verdict bouncerVerdict, ledger ledgerFile, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	refusal := b.decisionRefusal(round, verdict, ledger, CirclingAccept)
	switch {
	case refusal == "":
		return b.acceptRound(round, ptr)
	case !b.budgetReached(round):
		return b.rejudge(ctx, round, verdict, refusal)
	case !b.atHardCap(round):
		if err := b.retireContinue(round); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): retire the accept decision of round %d held by a refusal: %w; way forward: make the named path writable, then run `lym loom start`, which re-escalates the round", b.cfg.Name, bouncerEngineLabel, round, err)
		}
		return b.escalateDecision(round, refusal, true, ptr)
	}
	if err := archiveStaleOutputs(judgeOutputs(b.cfg.RunDir, round), b.cfg.Now); err != nil {
		return b.degrade(ctx, "shedadapters: bouncer failed to archive a held accept's judge outputs", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
	}
	reason := fmt.Sprintf("%sbouncer %s: round %d's recorded accept is held by a refusal at the budget and the perch's hard cap (%s); way forward: run `lym loom resume`, whose next Call re-judges the round under the recorded accept", DecisionHaltPrefix, b.cfg.Name, round, refusal)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason, Halt: true}, nil
}

// escalateDecision renders the decision detail of round and escalates it with cause decision.
// A detail that cannot be rendered warns and escalates without it.
func (b *Bouncer) escalateDecision(round int, refusal string, held bool, ptr shedengine.OutputPointer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	detail, err := b.renderDecisionDetail(round, refusal, held)
	if err != nil {
		logger.Warn("shedadapters: bouncer decision detail render failed; escalating without it", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
		detail = ""
	}
	return b.escalateWith(round, EscalationDecision, detail, ptr)
}

// decisionRefusal returns why the parent's recorded decision does not pass round yet, or "" when it does.
// A continue passes nothing, so only the dropped-key check applies to it.
// An accept faces judgmentRefusal's whole check with the round's decision-class findings and ledger entries left out of both BLOCKING checks.
func (b *Bouncer) decisionRefusal(round int, verdict bouncerVerdict, ledger ledgerFile, decision CirclingDecision) string {
	if decision == CirclingContinue {
		return b.droppedKeyRefusal(round, ledger)
	}
	return b.refusalLeavingOut(round, verdict, ledger, burlerengine.ClassDecision)
}

// renderDecisionDetail renders the decision fragment for round: its decision-class findings, every BLOCKING finding of its review, the refusal and the verbs that record the answer.
// held tells that the refusal already retired an accept at the budget, which adds the sentence that an accept escalates the round again while the refusal holds.
// A refusal over a dropped ledger key adds the sentence that a continue meets the same check.
func (b *Bouncer) renderDecisionDetail(round int, refusal string, held bool) (string, error) {
	template, err := stencilstore.Read(b.cfg.StencilsDir, "bouncer-fragment-decision")
	if err != nil {
		return "", fmt.Errorf("read decision fragment: %w", err)
	}
	findings, err := b.reviewFindings(round)
	if err != nil {
		return "", fmt.Errorf("read the round's review: %w", err)
	}
	var decisionLines, blockingLines []string
	for _, finding := range findings {
		if finding.Class == burlerengine.ClassDecision {
			decisionLines = append(decisionLines, findingLine(finding))
		}
		if finding.Severity == burlerengine.SeverityBlocking {
			blockingLines = append(blockingLines, findingLine(finding))
		}
	}
	blocking := "none"
	if len(blockingLines) > 0 {
		blocking = strings.Join(blockingLines, "\n")
	}
	refusalText := "none"
	if refusal != "" {
		refusalText = refusal
		if held {
			refusalText += "\n\nAn `accept` escalates the round again while the refusal holds, since no budget is left to re-judge it; `continue` is the way forward."
		}
		if strings.HasPrefix(refusal, droppedKeyRefusalPrefix) {
			refusalText += "\n\nA `continue` meets the same check, halts the segment at its budget, and `lym loom resume` then re-judges the round."
		}
	}
	filled, err := stencil.Fill(template, map[string]string{
		"slug":              b.cfg.Slug,
		"decision_findings": strings.Join(decisionLines, "\n"),
		"blocking_findings": blocking,
		"refusal":           refusalText,
		"decide_verbs":      b.decideVerbs(round),
	})
	if err != nil {
		return "", fmt.Errorf("fill decision fragment: %w", err)
	}
	return strings.TrimSpace(string(filled)), nil
}

// findingLine renders finding as one bullet line: its id, severity, summary and fix.
func findingLine(finding burlerengine.Finding) string {
	line := fmt.Sprintf("- `%s` (%s): %s", finding.ID, finding.Severity, oneLine(finding.Summary))
	if fix := oneLine(finding.Fix); fix != "" {
		line += " Fix: " + fix
	}
	return line
}

// oneLine collapses every run of whitespace in text to a single space.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// decideVerbs renders the verb lines that record the segment's outcome for round: continue and accept below the hard cap, accept alone at or past it.
func (b *Bouncer) decideVerbs(round int) string {
	verbSuffix := ""
	if b.cfg.Slug != "" {
		verbSuffix = " " + b.cfg.Slug
	}
	accept := fmt.Sprintf("- `lym loom circling accept%s` to proceed", verbSuffix)
	if b.atHardCap(round) {
		return accept
	}
	return fmt.Sprintf("- `lym loom circling continue%s` for a round over the amended record\n%s", verbSuffix, accept)
}

// writeDecisionSnapshot writes round's decision snapshot, the titles of the decision record's `Added after Discussion` entries, when none exists yet.
// An empty path or an absent record writes an empty list.
// An existing snapshot is left as it is, so a round escalated again keeps its first one.
func writeDecisionSnapshot(runDir string, round int, decisionRecordPath string) error {
	path := decisionSnapshotPath(runDir, round)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat decision snapshot %s: %w", path, err)
	}
	titles, err := addedDecisionTitles(decisionRecordPath)
	if err != nil {
		return err
	}
	content, err := yaml.Marshal(decisionSnapshot{Titles: titles})
	if err != nil {
		return fmt.Errorf("render decision snapshot: %w", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write decision snapshot %s: %w", path, err)
	}
	return nil
}

// addedSinceSnapshot returns the titles of the decision record's `Added after Discussion` entries that round's snapshot lacks, in record order.
// A round with no snapshot has none.
func addedSinceSnapshot(runDir string, round int, decisionRecordPath string) ([]string, error) {
	path := decisionSnapshotPath(runDir, round)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read decision snapshot %s: %w", path, err)
	}
	var snapshot decisionSnapshot
	if err := yaml.Unmarshal(raw, &snapshot); err != nil {
		return nil, fmt.Errorf("decision snapshot %s is not valid YAML: %w", path, err)
	}
	titles, err := addedDecisionTitles(decisionRecordPath)
	if err != nil {
		return nil, err
	}
	recorded := map[string]int{}
	for _, title := range snapshot.Titles {
		recorded[title]++
	}
	var since []string
	for _, title := range titles {
		if recorded[title] > 0 {
			recorded[title]--
			continue
		}
		since = append(since, title)
	}
	return since, nil
}

// addedDecisionTitles returns the titles of the `Added after Discussion` entries of the decision record at path, in record order.
// An empty path and an absent record have none.
func addedDecisionTitles(path string) ([]string, error) {
	if path == "" {
		return []string{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read decision record %s: %w", path, err)
	}
	added, err := discussionparser.ParseAddedDecisions(raw)
	if err != nil {
		return nil, fmt.Errorf("parse decision record %s: %w", path, err)
	}
	titles := make([]string, len(added))
	for i, entry := range added {
		titles[i] = entry.Title
	}
	return titles, nil
}
