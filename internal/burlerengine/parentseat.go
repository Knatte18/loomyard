// parentseat.go builds the review table's parent seat for a round that holds a parent-review ask:
// the chair's parent consolidation part, and the parent coverage gate entry that holds the chair to every parent finding.

package burlerengine

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ParentOrigin is the origin a review finding kept from the parent's report carries.
const ParentOrigin = "parent"

// parentConsolidateStencil is the chair's parent consolidation part.
// parentRulesMarker is the review step's optional marker it fills.
const (
	parentConsolidateStencil = "burler-parent-consolidation"
	parentRulesMarker        = "parent_rules"
)

// parentCoverageAttempts is the re-prompt budget of the parent coverage entry.
const parentCoverageAttempts = 3

// ParentCoverageEntryName names the parent coverage entry in a gate report.
const ParentCoverageEntryName = "parent-coverage"

// parentRulesBlock renders the chair's parent consolidation part for engine:
// the value of the review step's optional parent_rules marker.
// A round whose opts hold no parent-review ask returns "" without reading anything.
// The value leads with the blank line that separates it from the part before it.
func parentRulesBlock(stencilsDir, engine string, opts RunOpts) (string, error) {
	if opts.ParentAsk == 0 {
		return "", nil
	}
	rules, err := fillInstruction(stencilsDir, engine, parentConsolidateStencil, map[string]string{"parent_origin": ParentOrigin}, nil)
	if err != nil {
		return "", err
	}
	return "\n\n" + strings.TrimRight(string(rules), "\n"), nil
}

// withParentCoverage returns table with the parent coverage entry of the ask opts names appended to its gate.
// The entry follows the review-parse entry, so it runs only at an arrival whose review parses.
// A round whose opts hold no ask returns table unchanged.
func (e *Engine) withParentCoverage(table seatengine.Table, p *Profile, opts RunOpts) seatengine.Table {
	if opts.ParentAsk == 0 {
		return table
	}
	table.Gate = append(append(shuttleengine.GateSpec{}, table.Gate...), ParentCoverageGateEntry(ParentReviewFilesOf(e.geom.AnchorPath), opts.ParentAsk, p.ReviewPath))
	return table
}

// ParentCoverageGateEntry returns the gate entry that holds a round's chair to every finding of its parent's report.
// Each evaluation first sets the ask numbered number timeout when it is still pending-post or open,
// so an ask the chair did not wait for is closed before its review can be accepted.
// Then, when the ask is done with findings, it fails naming each report id that the review leaves uncovered.
// A report id is covered when a finding of the review at reviewPath tagged origin parent lists it in its parent_ids.
// It never passes on its cap, so a review that leaves an id uncovered fails the gate.
func ParentCoverageGateEntry(files ParentReviewFiles, number int, reviewPath string) shuttleengine.GateEntry {
	return shuttleengine.GateEntry{Name: ParentCoverageEntryName, Gate: parentCoverageGate(files, number, reviewPath), Attempts: parentCoverageAttempts}
}

// parentCoverageGate returns the closure of the parent coverage entry; see ParentCoverageGateEntry.
// A done ask whose report cannot be read is a gate error, since the report is lym's own file.
func parentCoverageGate(files ParentReviewFiles, number int, reviewPath string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		ask, found, err := EndParentAsk(files, number, ParentAskTimeout, "")
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if !found || ask.State != ParentAskDone {
			return shuttleengine.GateResult{Passed: true}, nil
		}
		report, err := ReadParentReport(ask.Report)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if len(report.Findings) == 0 {
			return shuttleengine.GateResult{Passed: true}, nil
		}
		content, err := os.ReadFile(reviewPath)
		if err != nil {
			return shuttleengine.GateResult{Findings: fmt.Sprintf("burler: read review file %q: %s\n\nWrite the review file (%s) and keep every finding of the parent's report %s in it.", reviewPath, err, reviewPath, ask.Report)}, nil
		}
		_, findings, err := ParseReview(content)
		if err != nil {
			return shuttleengine.GateResult{Findings: fmt.Sprintf("%s\n\nRewrite the review file (%s) so it parses.", err, reviewPath)}, nil
		}
		missing := uncoveredParentIDs(report, findings)
		if len(missing) == 0 {
			return shuttleengine.GateResult{Passed: true}, nil
		}
		return shuttleengine.GateResult{Findings: fmt.Sprintf(
			"The parent's report %s holds findings %s that no finding tagged `origin: %s` lists in its `parent_ids`.\n\n"+
				"Keep each of them in the review file (%s) as a finding tagged `origin: %s` whose `parent_ids` list names it; never move one to `## Rejected`.",
			ask.Report, strings.Join(missing, ", "), ParentOrigin, reviewPath, ParentOrigin)}, nil
	}
}

// uncoveredParentIDs returns, in report order, the ids of report's findings that no ParentOrigin finding covers.
// A finding covers an id by listing it in its ParentIDs.
func uncoveredParentIDs(report ParentReport, findings []Finding) []string {
	var covered []string
	for _, finding := range findings {
		if finding.Origin == ParentOrigin {
			covered = append(covered, finding.ParentIDs...)
		}
	}
	var missing []string
	for _, item := range report.Findings {
		if !slices.Contains(covered, item.ID) {
			missing = append(missing, item.ID)
		}
	}
	return missing
}
