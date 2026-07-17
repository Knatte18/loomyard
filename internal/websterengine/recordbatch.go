// recordbatch.go implements RecordBatch, the second of webster's two
// bracket verbs Master calls around each in-session fork, immediately after
// a fork returns: the bracket-discipline fail-loud check (a record without a
// matching begin-batch record is refused), the incremental fork audit with
// its bounded settle retry, webster's fork-audit policy checks, the
// unconditional transcript-attribution advance, the batch-report presence
// check and parse, and the distilled digest's persistence. RecordBatch never
// touches weft — the caller weft-commits state.json and the batch report
// once RecordBatch returns successfully, mirroring builder's own
// weft-commit-boundary discipline.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrNoBeginRecord is the sentinel RecordBatch returns when
// deps.State.Batches[batchNumber] is absent or already Terminal — a record
// call with no matching (or already-consumed) begin-batch record. This is
// the bracket-discipline fail-loud check: a fork's own report, however
// legitimate it looks, is never trusted without Go's own record that
// begin-batch actually opened this batch first.
var ErrNoBeginRecord = errors.New("webster: record-batch called with no begin-batch record for this batch")

// RecordDeps carries every seam RecordBatch needs, so a test can fake each
// one independently: Plan and State are the already-parsed/loaded plan and
// run state RecordBatch reads and mutates; Config is the loaded
// webster.yaml; Engine supplies the incremental fork audit
// (AuditForksIncremental); Layout resolves the pane's actual process cwd the
// audit reads against and the weft-reference pattern CheckFork/CheckParent
// consult; WorktreeRoot is the host repo checkout the drift computation
// diffs against; OutcomePath and SummaryPath are the run's two Master
// contract files CheckParent's write-policy exempts; Sleeper is the clock
// seam SettleRetry's bounded wait uses.
type RecordDeps struct {
	Plan         *builderengine.Plan
	State        *State
	Config       Config
	Engine       shuttleengine.Engine
	Layout       *hubgeometry.Layout
	WorktreeRoot string
	ReportsDir   string
	OutcomePath  string
	SummaryPath  string
	Sleeper      Sleeper
}

// RecordResult is what one successful RecordBatch call hands back to its
// caller (internal/webstercli's record-batch verb): Digest is the distilled
// digest once the batch reaches a terminal classification (nil when
// NoReport is true); NoReport reports whether the batch-report file was
// still absent this call (the batch stays non-terminal and
// State.CurrentBatch stays unchanged — Master's ladder re-forks once);
// Warnings carries every non-fatal fork-audit-policy warning observed this
// call (a multi-new-transcript notice, or a fork that never returned a
// final report), never treated as a failure.
type RecordResult struct {
	Digest   *builderengine.Digest
	NoReport bool
	Warnings []string
}

// RecordBatch drives one record-batch call to completion, immediately after
// Master's fork for batchNumber returns: the bracket-discipline check, the
// incremental fork audit (with its bounded settle retry against a
// zero-new-transcript miss), webster's fork-audit policy (CheckParent on the
// parent session's facts, CheckFork on every new fork transcript), the
// unconditional transcript-attribution advance (BEFORE the report-presence
// check, so a no_report retry sees only its own new transcript), the
// batch-report presence check and parse, and — once a report has landed —
// the distilled digest's persistence. The caller holds the state-mutation
// lease across this whole call and is responsible for persisting
// deps.State (and, once terminal, the batch report) via SaveState once
// RecordBatch returns successfully.
func RecordBatch(deps RecordDeps, batchNumber int) (*RecordResult, error) {
	bs, ok := deps.State.Batches[batchNumber]
	if !ok || bs == nil || bs.Terminal {
		return nil, ErrNoBeginRecord
	}

	batch, err := findBatch(deps.Plan, batchNumber)
	if err != nil {
		return nil, err
	}

	seenSet := make(map[string]bool, len(deps.State.SeenForkTranscripts))
	for _, p := range deps.State.SeenForkTranscripts {
		seenSet[p] = true
	}

	fetch := func() (shuttleengine.ForkAudit, error) {
		return deps.Engine.AuditForksIncremental(deps.State.MasterSessionID, deps.Layout.Cwd, seenSet)
	}

	audit, newReports, err := SettleRetry(fetch, deps.State.SeenForkTranscripts, DefaultSettleWindow, DefaultSettleTick, deps.Sleeper)
	if err != nil {
		return nil, err
	}

	// Transcript count is decided BEFORE report presence — a report with no
	// fork behind it means Master wrote it itself, and this check is what
	// makes that defect unfakeable regardless of whether a report file
	// exists on disk.
	warning, err := ClassifyAttribution(newReports)
	if err != nil {
		return nil, err
	}

	var warnings []string
	if warning != "" {
		warnings = append(warnings, warning)
	}

	weftRef := weftReferencePattern(deps.Layout)

	var violations []error
	for _, v := range CheckParent(audit, deps.OutcomePath, deps.SummaryPath, deps.Layout.Cwd, weftRef) {
		violations = append(violations, v)
	}
	for _, f := range newReports {
		for _, v := range CheckFork(f, weftRef) {
			violations = append(violations, v)
		}
		warnings = append(warnings, ForkWarnings(f)...)
	}
	if len(violations) > 0 {
		return nil, errors.Join(violations...)
	}

	// Attribution advances unconditionally, before the report-presence
	// check below: a no_report retry then sees only its own new
	// transcript(s) on the next call, never re-counting the ones already
	// attributed here.
	newPaths := make([]string, 0, len(newReports))
	for _, f := range newReports {
		newPaths = append(newPaths, f.TranscriptPath)
	}
	deps.State.SeenForkTranscripts = append(deps.State.SeenForkTranscripts, newPaths...)
	bs.ForkTranscripts = append(bs.ForkTranscripts, newPaths...)

	reportPath := filepath.Join(deps.ReportsDir, builderengine.BatchReportFileName(batch.Number, batch.Slug))
	if _, statErr := os.Stat(reportPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return &RecordResult{NoReport: true, Warnings: warnings}, nil
		}
		return nil, fmt.Errorf("webster: stat batch report %s: %w", reportPath, statErr)
	}

	report, err := builderengine.ParseReport(reportPath)
	if err != nil {
		return nil, err
	}

	polledID := fmt.Sprintf("%02d-%s", batch.Number, batch.Slug)
	if report.Batch != polledID {
		return nil, fmt.Errorf("webster: batch report %s: batch field %q does not match the polled batch %q", reportPath, report.Batch, polledID)
	}

	changed, err := builderengine.ChangedFiles(deps.WorktreeRoot, bs.StartSHA)
	if err != nil {
		return nil, err
	}
	dirty, err := builderengine.Dirty(deps.WorktreeRoot)
	if err != nil {
		return nil, err
	}

	digest := builderengine.Distill(report, changed, batch.Scope, dirty)

	bs.Digest = &digest
	bs.Terminal = true
	bs.Status = digest.Status
	deps.State.CurrentBatch = 0

	return &RecordResult{Digest: &digest, Warnings: warnings}, nil
}
