// bouncercarryover.go builds a judged round's carry-over entry from the round's ledger and hands it to the Bouncer's CarryOver seam.
// The entry lists the findings still open when the segment closed, so the sessions that read the decision record next see what no fresh review has confirmed fixed.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
)

// carryOverUnlabelled is the label a carry-over finding carries when its ledger entry has no class or no severity.
const carryOverUnlabelled = "unlabelled"

// writeCarryOver calls the CarryOver seam for round with closing, when one is configured.
// A failure comes back as the caller's own error, naming the file the seam's error names and the command that re-runs the settle.
func (b *Bouncer) writeCarryOver(round int, closing discussionparser.CarryOverClosing) error {
	if b.cfg.CarryOver == nil {
		return nil
	}
	entry, err := b.buildCarryOver(round, closing)
	if err == nil {
		err = b.cfg.CarryOver(entry)
	}
	if err != nil {
		return fmt.Errorf("shedadapters: %s (%s): carry over round %d's open findings: %w; way forward: fix the file the error names, keeping its `## Open risks` carry-over marker lines well-formed, then run `lym loom start` to re-run the settle", b.cfg.Name, bouncerEngineLabel, round, err)
	}
	return nil
}

// buildCarryOver reads round's ledger and returns the carry-over entry for it.
// The findings are every open entry at MEDIUM or worse and every open entry missing its class or severity, sorted by key.
// Each finding's rounds are the ledger rounds its key was open in, never the judge-written rounds list.
func (b *Bouncer) buildCarryOver(round int, closing discussionparser.CarryOverClosing) (discussionparser.CarryOver, error) {
	raw, err := os.ReadFile(ledgerPath(b.cfg.RunDir, round))
	if err != nil {
		return discussionparser.CarryOver{}, err
	}
	ledger, err := parseLedger(raw)
	if err != nil {
		return discussionparser.CarryOver{}, err
	}

	reviewPath, err := anchorRelativePath(b.cfg.AnchorPath, filepath.Join(b.cfg.RunDir, b.cfg.ReportName(round)))
	if err != nil {
		return discussionparser.CarryOver{}, err
	}
	fixerReportPath, err := anchorRelativePath(b.cfg.AnchorPath, roundFixerReportPath(b.cfg.RunDir, round))
	if err != nil {
		return discussionparser.CarryOver{}, err
	}

	histories := ledgerHistories(b.cfg.RunDir, round)
	seen := map[string]bool{}
	var findings []discussionparser.CarryOverFinding
	for _, e := range ledger.Entries {
		if e.Status != "open" || seen[e.Key] || !carriesOver(e) {
			continue
		}
		seen[e.Key] = true
		findings = append(findings, discussionparser.CarryOverFinding{
			Key:      e.Key,
			Class:    labelOrUnlabelled(string(e.Class)),
			Severity: labelOrUnlabelled(string(e.Severity)),
			Rounds:   histories[e.Key].openRounds,
		})
	}
	if b.closesDecisionAccept(round) {
		findings, err = b.addDecisionAcceptFindings(round, findings)
		if err != nil {
			return discussionparser.CarryOver{}, err
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Key < findings[j].Key })

	return discussionparser.CarryOver{
		Segment:         b.cfg.Segment,
		Round:           round,
		Closing:         closing,
		ReviewPath:      reviewPath,
		FixerReportPath: fixerReportPath,
		Findings:        findings,
	}, nil
}

// closesDecisionAccept reports whether round's escalation has cause decision and its decision is an accept.
func (b *Bouncer) closesDecisionAccept(round int) bool {
	cause, _, escalated, err := readEscalation(b.cfg.RunDir, round)
	if err != nil || !escalated || cause != EscalationDecision {
		return false
	}
	decision, _, _, exists, err := readCirclingDecision(b.cfg.RunDir, round)
	return err == nil && exists && decision == CirclingAccept
}

// addDecisionAcceptFindings adds to findings, the ledger's carried entries, what a decision accept passes the round over, whatever the ledger says.
// Every decision-class finding of round's review is added at any severity, keyed by its id, with the `Added after Discussion` titles recorded since the escalation;
// a ledger entry of the same key carries those titles instead of being repeated.
// Every other BLOCKING finding of the review not already carried by key is added too, so one the judge closed still reaches `## Open risks`.
// Both match the ledger by exact key only, since a ledger entry records no review id.
func (b *Bouncer) addDecisionAcceptFindings(round int, findings []discussionparser.CarryOverFinding) ([]discussionparser.CarryOverFinding, error) {
	reviewFindings, err := b.reviewFindings(round)
	if err != nil {
		return nil, fmt.Errorf("read round %d's review: %w", round, err)
	}
	addedSince, err := addedSinceSnapshot(b.cfg.RunDir, round, b.cfg.DecisionRecordPath)
	if err != nil {
		return nil, err
	}
	carried := make(map[string]int, len(findings))
	for i, finding := range findings {
		carried[finding.Key] = i
	}
	for _, finding := range reviewFindings {
		if finding.Class != burlerengine.ClassDecision {
			continue
		}
		if i, ok := carried[finding.ID]; ok {
			findings[i].AddedSince = addedSince
			continue
		}
		carried[finding.ID] = len(findings)
		findings = append(findings, discussionparser.CarryOverFinding{
			Key:        finding.ID,
			Class:      labelOrUnlabelled(string(finding.Class)),
			Severity:   labelOrUnlabelled(string(finding.Severity)),
			Rounds:     []int{round},
			AddedSince: addedSince,
		})
	}
	for _, finding := range reviewFindings {
		if finding.Severity != burlerengine.SeverityBlocking {
			continue
		}
		if _, ok := carried[finding.ID]; ok {
			continue
		}
		carried[finding.ID] = len(findings)
		findings = append(findings, discussionparser.CarryOverFinding{
			Key:      finding.ID,
			Class:    labelOrUnlabelled(string(finding.Class)),
			Severity: labelOrUnlabelled(string(finding.Severity)),
			Rounds:   []int{round},
		})
	}
	return findings, nil
}

// carriesOver reports whether an open ledger entry belongs in the carry-over: MEDIUM or BLOCKING, or missing a class or severity label.
func carriesOver(e ledgerEntry) bool {
	if e.Class == "" || e.Severity == "" {
		return true
	}
	return e.Severity == burlerengine.SeverityBlocking || e.Severity == burlerengine.SeverityMedium
}

func labelOrUnlabelled(label string) string {
	if label == "" {
		return carryOverUnlabelled
	}
	return label
}

// anchorRelativePath returns path relative to anchor, slash-separated; a path that does not lie under anchor is an error.
func anchorRelativePath(anchor, path string) (string, error) {
	rel, err := filepath.Rel(anchor, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s does not lie under the anchor %s", path, anchor)
	}
	return filepath.ToSlash(rel), nil
}
