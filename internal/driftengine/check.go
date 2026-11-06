// check.go — the drift check every writer gate and Drift-Verify runs: containment of the current code's captured changes by the sealed expectation, and the gate entry over it.
//
// Check writes no tracked record and decides nothing; its report is what a caller maps to an outcome.

package driftengine

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// noExpectationSealedYet is the reason a check skips while the run has not passed Drift-Expect.
const noExpectationSealedYet = "no expectation is sealed yet"

// amendmentKindEntries is the Kind of an amendment that adds entries to the sealed expectation.
const amendmentKindEntries = "entries"

// The way forward a failing drift gate names for an unexpected cell and for a failed current capture.
const (
	unexpectedCellWayForward = "way forward: fix the code so the value returns to its baseline, or, when the change is intended, leave it for the parent's lym drift amend at the Stuck that follows"
	captureFaultWayForward   = "way forward: fix the code when the change broke the capture, otherwise leave it, since the stop that follows goes to the parent, whose way forward is a fix of the environment the capture names, then lym loom resume"
)

// ClassifiedCell is one cell of the diff and what the expectation says about it.
type ClassifiedCell struct {
	// Cell is the difference between the baseline and the current capture.
	Cell Cell
	// Coverage is the covering entry's why and whether it is an open range; it is meaningful only when Expected.
	Coverage Coverage
	// Expected is true when an entry of the expectation covers the cell.
	Expected bool
	// Repeated is true when the cell's key occurs more than once in its case, so a listing names the occurrence.
	Repeated bool
}

// CheckReport is what a drift check found.
type CheckReport struct {
	// Skip is the reason nothing was checked, and empty when the check ran past its first steps.
	Skip string
	// Mode is DiffLines or DiffCommand, the mode the check ran under.
	Mode string
	// BaselineFault is the reason the baseline cannot be compared, and ends the check.
	BaselineFault string
	// RecordsFault is the reason the records on disk differ from their committed copies; the check went on over the committed copies.
	RecordsFault string
	// SealFault is the reason the sealed expectation cannot be trusted; the check went on over the sealed copy, or an empty expectation when there is none.
	SealFault string
	// CaptureFault is the reason the current capture did not pass, and ends the check.
	CaptureFault string
	// Expectation is the sealed expectation merged with every amendment that adds entries.
	Expectation Expectation
	// Capture is how the current capture ended; the zero value under DiffCommand.
	Capture CurrentCapture
	// Cells is every cell of the diff under DiffLines, in the diff's order, each classified by the expectation.
	Cells []ClassifiedCell
	// Uncovered names each expectation entry that covers no cell.
	Uncovered []string
	// Compared is true when the declared compare ran, under DiffCommand, and Compare is how it ended.
	Compared bool
	Compare  CompareResult
}

// Unexpected returns the cells no entry of the expectation covers.
func (r CheckReport) Unexpected() []ClassifiedCell {
	var unexpected []ClassifiedCell
	for _, cell := range r.Cells {
		if !cell.Expected {
			unexpected = append(unexpected, cell)
		}
	}
	return unexpected
}

// Check runs the drift check over d's seams and returns what it found; it ends at the first step that ends the check:
// no scope record, scope off, a deferred capture and a waiver skip it;
// a run not past Drift-Expect skips it before any baseline check, since a real run has no awaited_at before Drift-Await counts the capture;
// a capture that never counted, a missing baseline directory and a changed baseline digest are a baseline fault;
// a records mismatch is a records fault and the check goes on over the committed copies;
// a failed seal check is a seal fault and the check goes on against the sealed copy, or an empty expectation when there is none.
// Under DiffLines it takes a current capture, whose failure is a capture fault, diffs it against the baseline, drops the cells of the volatile lists and classifies each cell left by the merged expectation.
// Under DiffCommand it runs the declared compare and takes no capture.
// Whether the capture counted is read from the tracked scope record alone, never from the capture record.
func Check(ctx context.Context, d Deps) (CheckReport, error) {
	if err := requireSeams("the drift check", seam{"Config", d.Config != nil}, seam{"Status", d.Status != nil}); err != nil {
		return CheckReport{}, err
	}
	var report CheckReport
	rec, err := ReadRecords(d)
	if err != nil {
		return report, err
	}
	if report.Skip = skipReason(rec.ScopeFound, rec.Scope); report.Skip != "" {
		return report, nil
	}

	mode, patterns, volatile := rec.Scope.Diff, rec.Scope.KeyPatterns, rec.Scope.Volatile
	if mode == "" {
		cfg, err := d.Config()
		if err != nil {
			return report, fmt.Errorf("drift: load the drift config for the check: %w", err)
		}
		mode, patterns, volatile = cfg.Diff, cfg.KeyPatterns, cfg.Volatile
	}
	report.Mode = mode
	compiled, err := CompileKeyPatterns(patterns)
	if err != nil {
		return report, fmt.Errorf("drift: %w", err)
	}

	status, err := d.Status()
	if err != nil {
		return report, fmt.Errorf("drift: read the run status for the check: %w", err)
	}
	if !PastRow(d.Rows, status.CurrentProducer, expectRow) {
		report.Skip = noExpectationSealedYet
		return report, nil
	}

	if rec.Scope.AwaitedAt == "" {
		report.BaselineFault = DeferHaltPrefix + "the baseline capture never counted, so there is no baseline to compare; way forward: the parent runs lym drift defer, then lym loom resume"
		return report, nil
	}
	baseline, err := CheckBaseline(d)
	if err != nil {
		return report, err
	}
	if baseline.Fault != "" {
		report.BaselineFault = baseline.Fault
		return report, nil
	}

	if rec.Mismatch {
		report.RecordsFault = "the drift records on disk differ from their newest committed copy, which the check read instead; way forward: the parent runs lym drift amend <slug> --restore"
	}
	head, err := baselineHead(d)
	if err != nil {
		return report, err
	}
	seal, err := CheckSeal(d, rec, head)
	if err != nil {
		return report, err
	}
	if seal.NotYet {
		report.Skip = noExpectationSealedYet
		return report, nil
	}
	report.SealFault = seal.Fault

	var baselineListings []CaseListing
	if mode == DiffLines {
		if baselineListings, err = ListCaptures(d.Paths.BaselineDir, compiled); err != nil {
			return report, err
		}
	}
	if report.Expectation, err = sealedAndAmended(seal.Sealed, rec.Scope.Amendments, mode, baselineListings); err != nil {
		return report, err
	}

	switch mode {
	case DiffLines:
		return report, classifyCurrent(ctx, d, rec.Scope, compiled, volatile, baselineListings, &report)
	case DiffCommand:
		if err := requireSeams("the drift check under diff command", seam{"Compare", d.Compare != nil}); err != nil {
			return report, err
		}
		if report.Compare, err = d.Compare(ctx); err != nil {
			return report, fmt.Errorf("drift: run the compare for the check: %w", err)
		}
		report.Compared = true
		return report, nil
	}
	return report, fmt.Errorf("drift: the check ran under diff %q, which is neither %q nor %q", mode, DiffLines, DiffCommand)
}

// skipReason returns why a scope record needs no check, and empty when it does.
func skipReason(found bool, scope ScopeRecord) string {
	switch {
	case !found:
		return "no scope record exists, so no regression scope was recorded"
	case scope.Scope == ScopeOff:
		return "the recorded scope is off"
	case scope.Deferred:
		return "the baseline capture was deferred, so there is no baseline to compare"
	case scope.WaivedAt != "":
		return "the check was waived: " + scope.WaivedReason
	}
	return ""
}

// baselineHead returns the code head the counted baseline was captured at, from the capture record while it exists and otherwise from the oldest commit that recorded the baseline's digest.
func baselineHead(d Deps) (string, error) {
	capture, found, err := ReadCapture(d.Paths)
	if err != nil {
		return "", err
	}
	if found {
		return capture.Head, nil
	}
	commit, _, err := d.TrailerCommit(BaselineTrailerKey, false)
	if err != nil {
		return "", fmt.Errorf("drift: find the oldest %s commit: %w", BaselineTrailerKey, err)
	}
	return commit.CodeHead, nil
}

// sealedAndAmended parses the sealed expectation and every amendment that adds entries under mode against the baseline's listings, and merges them in order.
func sealedAndAmended(sealed []byte, amendments []Amendment, mode string, baseline []CaseListing) (Expectation, error) {
	expectation, err := ParseExpectation(sealed, mode, baseline)
	if err != nil {
		return Expectation{}, fmt.Errorf("drift: the sealed expectation does not parse: %w", err)
	}
	for _, amendment := range amendments {
		if amendment.Kind != amendmentKindEntries {
			continue
		}
		amended, err := ParseExpectation([]byte(amendment.Entries), mode, baseline)
		if err != nil {
			return Expectation{}, fmt.Errorf("drift: the amendment of %s does not parse: %w", amendment.At, err)
		}
		expectation = expectation.Merge(amended)
	}
	return expectation, nil
}

// classifyCurrent takes the current capture, diffs it against the baseline and classifies each cell volatile does not name into report.
// A capture that did not pass is a capture fault and ends the check with no error.
func classifyCurrent(ctx context.Context, d Deps, scope ScopeRecord, patterns []*regexp.Regexp, volatile Volatile, baseline []CaseListing, report *CheckReport) error {
	if err := requireSeams("the drift check under diff lines", seam{"CaptureCurrent", d.CaptureCurrent != nil}); err != nil {
		return err
	}
	capture, err := d.CaptureCurrent(ctx)
	if err != nil {
		return fmt.Errorf("drift: take the current capture for the check: %w", err)
	}
	report.Capture = capture
	if !capture.Passed {
		report.CaptureFault = fmt.Sprintf("the current capture did not pass: %s; log %s; %s", capture.Reason, capture.Log, captureFaultWayForward)
		return nil
	}

	var leaveOut []string
	if scope.Scope == ScopePartial {
		if leaveOut, err = skippedCases(scope.Skip, d.Paths.BaselineDir, capture.Dir); err != nil {
			return err
		}
	}
	cells, err := DiffCaptures(d.Paths.BaselineDir, capture.Dir, patterns, leaveOut)
	if err != nil {
		return err
	}
	cells = volatile.withoutCells(cells)
	current, err := ListCaptures(capture.Dir, patterns)
	if err != nil {
		return err
	}
	repeated := repeatedKeys(baseline, current)
	for _, cell := range cells {
		coverage, expected := report.Expectation.Cover(cell)
		report.Cells = append(report.Cells, ClassifiedCell{Cell: cell, Coverage: coverage, Expected: expected, Repeated: repeated[caseKey{cell.Case, cell.Identity.Key}]})
	}
	report.Uncovered = report.Expectation.Uncovering(cells)
	return nil
}

// skippedCases returns the cases of dirs that hold a skipped test's profile.
func skippedCases(skip []string, dirs ...string) ([]string, error) {
	var cases []string
	for _, dir := range dirs {
		for _, test := range skip {
			files, err := testFiles(dir, test)
			if err != nil {
				return nil, err
			}
			cases = append(cases, files...)
		}
	}
	return cases, nil
}

// caseKey names a key within a case.
type caseKey struct {
	caseName, key string
}

// repeatedKeys returns the keys that occur more than once in a case of any of the listings.
func repeatedKeys(listings ...[]CaseListing) map[caseKey]bool {
	repeated := map[caseKey]bool{}
	for _, side := range listings {
		for _, listing := range side {
			counts := map[string]int{}
			for _, line := range listing.Lines {
				if line.Identity.Key == "" {
					continue
				}
				counts[line.Identity.Key]++
				if counts[line.Identity.Key] > 1 {
					repeated[caseKey{listing.Case, line.Identity.Key}] = true
				}
			}
		}
	}
	return repeated
}

// Findings renders the report: a line for the skip, each fault with its way forward, each unexpected cell with its case, identity, old value and new value, and the compare's exit and log under DiffCommand.
// It is empty when the check found nothing to report.
func (r CheckReport) Findings() string {
	var lines []string
	if r.Skip != "" {
		lines = append(lines, "skipped: "+r.Skip)
	}
	for _, fault := range []struct{ label, text string }{
		{"baseline fault", r.BaselineFault},
		{"records fault", r.RecordsFault},
		{"seal fault", r.SealFault},
		{"capture fault", r.CaptureFault},
	} {
		if fault.text != "" {
			lines = append(lines, fault.label+": "+fault.text)
		}
	}
	unexpected := r.Unexpected()
	for _, cell := range unexpected {
		lines = append(lines, "unexpected cell: "+describeCell(cell))
	}
	if len(unexpected) > 0 {
		lines = append(lines, unexpectedCellWayForward)
	}
	if r.Compared {
		lines = append(lines, fmt.Sprintf("the compare exited %d; log %s", r.Compare.ExitCode, r.Compare.Log))
	}
	return strings.Join(lines, "\n")
}

// describeCell names a cell by its kind, case, identity, old value and new value.
func describeCell(c ClassifiedCell) string {
	cell := c.Cell
	if cell.Kind == CellAddedCase || cell.Kind == CellRemovedCase {
		return fmt.Sprintf("%s %q", cell.Kind, cell.Case)
	}
	return fmt.Sprintf("%s in case %q at %s: old %q, new %q", cell.Kind, cell.Case, identityText(cell.Identity, c.Repeated), cell.Old, cell.New)
}

// identityText names an identity: a key, with its occurrence only when the key repeats in its case, or `#` and the line position of an unkeyed line.
func identityText(identity Identity, repeated bool) string {
	if identity.Key == "" {
		return fmt.Sprintf("#%d", identity.Line)
	}
	if repeated {
		return fmt.Sprintf("key %q n %d", identity.Key, identity.N)
	}
	return fmt.Sprintf("key %q", identity.Key)
}

// NewDriftGate returns the gate the `drift` entry of a writer's gate list runs: it runs Check over d and judges the report.
// It passes on a skip and on a baseline fault, which no writer session can clear, and under DiffCommand whatever the compare's exit; its findings are logged at Info when they remain.
// It fails on a capture fault and, under DiffLines, on any unexpected cell, with the report's findings, which also name a records or seal fault found beside them.
// It writes no tracked record, only the current capture's scratch directory.
func NewDriftGate(d Deps) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		report, err := Check(context.Background(), d)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("drift: the drift gate: %w", err)
		}
		findings := report.Findings()
		clearable := report.Skip == "" && report.BaselineFault == ""
		if clearable && (report.CaptureFault != "" || len(report.Unexpected()) > 0) {
			return shuttleengine.GateResult{Findings: findings}, nil
		}
		if findings != "" {
			logger.Info("drift: the drift gate passed with findings no writer session can clear", "findings", findings)
		}
		return shuttleengine.GateResult{Passed: true}, nil
	}
}
