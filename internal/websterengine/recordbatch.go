// recordbatch.go implements RecordBatch, the second of webster's two bracket verbs Master calls
// around each in-session fork, immediately after a fork returns: the bracket-discipline fail-loud
// check (a record without a matching begin-batch record is refused), the incremental fork audit
// with its bounded settle retry, webster's fork-audit policy checks, the unconditional
// transcript-attribution advance, the batch-report presence check and parse, the head-SHA
// cross-check against the fork's own self-reported head_sha, and the distilled digest's
// persistence.
// RecordBatch never touches the fabric repo — the caller fabric-commits state.json and the batch
// report once RecordBatch returns successfully, webster's own fabric-commit-boundary discipline.

package websterengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrNoBeginRecord is the sentinel RecordBatch returns when deps.State.Batches[batchNumber] is
// absent or already Terminal — a record call with no matching (or already-consumed) begin-batch
// record.
// This is the bracket-discipline fail-loud check: a fork's own report, however legitimate it looks,
// is never trusted without Go's own record that begin-batch actually opened this batch first.
var ErrNoBeginRecord = errors.New("webster: record-batch called with no begin-batch record for this batch")

// RecordDeps carries every seam RecordBatch needs, so a test can fake each one independently:
// Batches is the batchifier-derived execution batches (see RunDeps.Batcher) `run` computed
// once at entry;
// State is the already-loaded run state RecordBatch reads and mutates;
// Config is the loaded webster.yaml;
// Engine supplies the incremental fork audit (AuditForksIncremental);
// Geom is the told Geometry the audit and the dirty-worktree/head-SHA checks read: the audit workdir
// is Geom.WorktreeRoot (not Geom.AnchorRoot) — the audit resolves transcript-relative recorded write
// paths against this directory, so it must be the directory the fork actually ran in, which is the
// worktree root in every mode (the two coincide in hub mode, so nothing changes there);
// RefMatcher is the injected fabric-reference class matcher (a real *fabricengine.RefScanner in hub
// mode, NeverMatches in standalone) CheckParent/CheckFork consult, never nil in either mode;
// OutcomePath and SummaryPath are the run's two Master contract files CheckParent's write-policy
// exempts;
// Sleeper is the clock seam SettleRetry's bounded wait uses.
type RecordDeps struct {
	Batches     []batcher.Batch
	State       *State
	Config      Config
	Engine      shuttleengine.Engine
	Geom        Geometry
	RefMatcher  RefMatcher
	OutcomePath string
	SummaryPath string
	Sleeper     Sleeper
}

// RecordResult is what one successful RecordBatch call hands back to its caller
// (internal/webstercli's record-batch verb): Digest is the distilled digest once the batch reaches
// a terminal classification (nil when NoReport is true);
// NoReport reports whether the batch-report file was still absent this call (the batch stays
// non-terminal and State.CurrentBatch stays unchanged — Master's ladder re-forks once);
// Warnings carries every non-fatal fork-audit-policy warning observed this call (a
// multi-new-transcript notice, a fork that never returned a final report, or a dirty worktree after
// the batch's own commits), never treated as a failure.
type RecordResult struct {
	Digest   *Digest
	NoReport bool
	Warnings []string
}

// RecordBatch drives one record-batch call: the bracket-discipline check, incremental fork audit,
// fork-audit policy checks, transcript-attribution advance, report parse, and digest persistence.
// The caller persists deps.State via SaveState once RecordBatch returns successfully.
func RecordBatch(deps RecordDeps, batchNumber int) (*RecordResult, error) {
	bs, ok := deps.State.Batches[batchNumber]
	if !ok || bs == nil || bs.Terminal {
		return nil, ErrNoBeginRecord
	}

	// Recovery batches are consumed by recover-batch, not record-batch.
	if bs.Kind != "fork" {
		return nil, fmt.Errorf("webster: batch %d is a %s batch, not a fork batch — its report is consumed by `lyx webster recover-batch %d`, never record-batch", batchNumber, bs.Kind, batchNumber)
	}

	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	number, slug := batchIdentity(batch)

	seenSet := make(map[string]bool, len(deps.State.SeenForkTranscripts))
	for _, p := range deps.State.SeenForkTranscripts {
		seenSet[p] = true
	}

	// Audit the session that opened this batch's bracket (bs.SessionID),
	// not the current Master session, so a resumed Master can consume a
	// report whose transcript lives under the crashed session's directory.
	fetch := func() (shuttleengine.ForkAudit, error) {
		return deps.Engine.AuditForksIncremental(bs.SessionID, deps.Geom.WorktreeRoot, seenSet)
	}

	audit, newReports, err := SettleRetry(fetch, deps.State.SeenForkTranscripts, DefaultSettleWindow, DefaultSettleTick, deps.Sleeper)
	if err != nil {
		// Session transcripts are machine-local, so a cross-machine resume
		// fails here with the documented operator recourse.
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("webster: no transcript exists on this machine for the session that opened batch %02d-%s's bracket (%s): %w — session transcripts are machine-local, so a crash window resumed on a different machine cannot re-attribute its report; an operator resolves that by moving the batch's report file out of the reports dir and re-driving the batch", number, slug, bs.SessionID, err)
		}
		return nil, err
	}

	// Check transcripts before report presence so a fake (unfakeable) report is caught.
	warning, err := ClassifyAttribution(newReports)
	if err != nil {
		return nil, err
	}

	var warnings []string
	if warning != "" {
		warnings = append(warnings, warning)
	}

	var violations []error
	for _, v := range CheckParent(audit, deps.OutcomePath, deps.SummaryPath, deps.Geom.WorktreeRoot, deps.RefMatcher) {
		violations = append(violations, v)
	}
	for _, f := range newReports {
		for _, v := range CheckFork(f, deps.OutcomePath, deps.SummaryPath, deps.Geom.WorktreeRoot, deps.RefMatcher) {
			violations = append(violations, v)
		}
		warnings = append(warnings, ForkWarnings(f)...)
	}
	if len(violations) > 0 {
		return nil, errors.Join(violations...)
	}

	// Attribution advances before report-presence check so a retry sees only its own new transcript.
	newPaths := make([]string, 0, len(newReports))
	for _, f := range newReports {
		newPaths = append(newPaths, f.TranscriptPath)
	}
	deps.State.SeenForkTranscripts = append(deps.State.SeenForkTranscripts, newPaths...)
	bs.ForkTranscripts = append(bs.ForkTranscripts, newPaths...)

	polledID := fmt.Sprintf("%02d-%s", number, slug)

	reportPath := filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug))
	if _, statErr := os.Stat(reportPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return &RecordResult{NoReport: true, Warnings: warnings}, nil
		}
		return nil, fmt.Errorf("webster: stat batch report %s: %w", reportPath, statErr)
	}

	report, err := ParseReport(reportPath)
	if err != nil {
		return nil, err
	}

	if isDirty, err := dirty(deps.Geom.WorktreeRoot); err != nil {
		return nil, err
	} else if isDirty {
		warnings = append(warnings, fmt.Sprintf("worktree is dirty after batch %s's own commits (uncommitted or untracked changes remain)", polledID))
	}

	// Cross-check report's head_sha against the worktree's actual HEAD.
	actualHead, err := headSHA(deps.Geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}
	if actualHead != report.HeadSHA {
		return nil, fmt.Errorf("webster: batch report %s: head_sha %q does not match the worktree's actual HEAD %q", reportPath, report.HeadSHA, actualHead)
	}

	digest := distill(report)
	digest.Batch = polledID

	bs.Digest = &digest
	bs.CardSHAs = []string{actualHead}
	bs.Terminal = true
	bs.Status = digest.Status
	deps.State.CurrentBatch = 0

	return &RecordResult{Digest: &digest, Warnings: warnings}, nil
}
