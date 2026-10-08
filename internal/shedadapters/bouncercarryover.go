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
		return fmt.Errorf("shedadapters: %s (%s): carry over round %d's open findings: %w; way forward: fix the file the error names, keeping its `## Open risks` carry-over marker lines well-formed, then run `lyx loom start` to re-run the settle", b.cfg.Name, bouncerEngineLabel, round, err)
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
