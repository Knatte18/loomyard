// recordbatch.go implements RecordBatch, the record-batch step that Master calls after each in-session fork returns.
// RecordBatch refuses a record with no matching begin-batch record.
// It runs the incremental fork audit with a bounded settle retry, and it applies each finding once per run.
// A policy finding warns; a correctness finding fails the batch.
// It always advances the transcript attribution.
// It checks that the batch report is present and parses it.
// It cross-checks the report's head SHA against the fork's self-reported head_sha.
// It persists the distilled digest.
// RecordBatch never touches the fabric repo.
// The caller fabric-commits state.json and the batch report once RecordBatch returns successfully.

package websterengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/cardverify"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrNoBeginRecord is the cause RecordBatch reports when a batch has no begin-batch record.
// It is the bracket-discipline check that fails loud.
// A fork's own report is never trusted without a record that begin-batch opened the batch first.
var ErrNoBeginRecord = errors.New("webster: record-batch called with no begin-batch record for this batch")

// ErrReportArchived marks a batch report that was archived because record-batch could not attribute it to a fork.
var ErrReportArchived = errors.New("webster: report archived")

// ReportArchivedError reports a batch report that record-batch could not attribute to a fork of its bracket.
// The report is archived, and begin-batch re-drives the batch.
// Cause is the attribution failure: no begin-batch record, no fork transcripts, or a missing transcript directory.
type ReportArchivedError struct {
	Number     int
	Batch      string
	ArchivedTo string
	Cause      error
}

// Error states why the report could not be attributed, where it was archived, and the way forward.
func (e *ReportArchivedError) Error() string {
	return fmt.Sprintf("webster: batch %s's report could not be attributed: %v; report archived to %s; way forward: lym webster begin-batch %02d re-drives the batch", e.Batch, e.Cause, e.ArchivedTo, e.Number)
}

// Unwrap returns the archived-report sentinel and the attribution cause, so errors.Is matches either.
func (e *ReportArchivedError) Unwrap() []error { return []error{ErrReportArchived, e.Cause} }

// deleteReferencedWayForward is the way forward for a batch whose Delete target a later, unbegun card still references.
// Recovery cannot change the plan, so the way forward edits it.
// A record that recovery would refuse as uncheckable restarts the run instead.
// A batch that has used its counted recoveries takes the exhausted-batch steps.
// A failed batch records no amendment, so those steps apply.
func deleteReferencedWayForward(number int, uncheckable, exhausted bool) string {
	if uncheckable {
		return freshRestartSteps(stepRun)
	}
	if exhausted {
		return strings.TrimPrefix(exhaustedRecoveryWayForward(number), "way forward: ")
	}
	return fmt.Sprintf("move the delete to a card after the one that still references it, run `lym webster rebaseline --card NN` naming each card you edited, then `lym webster recover-batch %02d`", number)
}

// glyphRejectedWayForward is the way forward for a batch whose card names a target quarry rejects.
// Recovery re-runs the same check on the same plan, so the way forward edits the card.
// A record that recovery would refuse as uncheckable restarts the run instead.
// A batch that has used its counted recoveries takes the exhausted-batch steps.
// A failed batch records no amendment, so those steps apply.
func glyphRejectedWayForward(number int, uncheckable, exhausted bool) string {
	if uncheckable {
		return freshRestartSteps(stepRun)
	}
	if exhausted {
		return strings.TrimPrefix(exhaustedRecoveryWayForward(number), "way forward: ")
	}
	return fmt.Sprintf("amend the card's target to a spelling `lym quarry resolve` answers, run `lym webster rebaseline --card NN` naming each card you edited, then `lym webster recover-batch %02d`", number)
}

// createTargetWayForward is the way forward for a batch whose Create target or Rename pair is not done.
// It also covers a batch whose card names a member that its unit declares only with another parameter list.
// A recovery checks the same target again.
// So the way forward amends the target whenever the member may live under another unit or signature.
// The way forward leads with recover-batch only when no recovery has tried the batch and no declared signature differs.
// A batch whose counted recoveries are used up takes the exhausted-batch steps.
// A failed batch records no amendment, so those steps apply.
// A record that recovery would refuse as uncheckable restarts the run instead.
func createTargetWayForward(number int, uncheckable, recovered, signatureMismatch, exhausted bool) string {
	if uncheckable {
		return freshRestartSteps(stepRun)
	}
	if exhausted {
		return strings.TrimPrefix(exhaustedRecoveryWayForward(number), "way forward: ")
	}
	amend := fmt.Sprintf("amend the card's Create target or Rename new side to the unit, owner or parameter list the finding names the member declared with, run `lym webster rebaseline --card NN` naming each card you edited, then `lym webster recover-batch %02d`", number)
	if recovered || signatureMismatch {
		return amend
	}
	return fmt.Sprintf("lym webster recover-batch %02d to build the target as the card names it; or, when the member is declared where the finding names it, %s", number, amend)
}

// exhaustedRecoveryWayForward is the way forward for a batch that has used its counted recoveries.
// It resets the batch to its start and recovers it again, or it restarts the run.
func exhaustedRecoveryWayForward(number int) string {
	return fmt.Sprintf("%s; or %s", wayForwardSteps(resetVerb(ResetToBatchStart, number), fmt.Sprintf("lym webster recover-batch %d", number)), freshRestartSteps(stepRun))
}

// cardNotDoneInputs carries what a batch needs to be failed on its post-batch findings.
// Verb names the calling verb, which a transient error tells the operator to re-run.
type cardNotDoneInputs struct {
	Plan    *planparser.Plan
	Batches []batcher.Batch
	State   *State
	Batch   *BatchState
	Cards   []planparser.Card
	Number  int
	Slug    string
	Geom    Geometry
	HeadSHA string
	Verb    string
}

// failCardNotDone fails the batch terminal on cause, an ErrCardNotDone-wrapped error from the post-batch pass.
// The reasons open with the batch's id and its card list, so a finding keyed on a later card reads as the batch's own.
// A Delete target that still resolves and an unbegun later card still references adds that reference to the reasons,
// and the way forward is then the plan edit, since recover-batch would only repeat the failure.
// A glyph-rejected finding, and a Create target that does not resolve, offer the card amendment.
// A Rename pair that did not happen does too.
// The amendment comes alone once a recover-batch call has tried the batch.
// It also comes alone when the member is declared with another parameter list.
// An exhausted batch takes the exhausted-batch steps instead.
// A batch is exhausted once its counted recoveries are used and no amendment is left to render.
func failCardNotDone(in cardNotDoneInputs, cause error) (*BatchFailedError, error) {
	var reasons []string
	var deleteNotDone, glyphRejected, createNotDone, signatureMismatch bool
	var notDone *runplan.CardNotDoneError
	if errors.As(cause, &notDone) {
		reasons = notDone.Reasons
		deleteNotDone = notDone.DeleteNotDone
		glyphRejected = notDone.GlyphRejected
		createNotDone = notDone.CreateNotDone
		signatureMismatch = notDone.SignatureMismatch
	} else {
		reasons = strings.Split(strings.TrimPrefix(cause.Error(), ErrCardNotDone.Error()+": "), "; ")
	}

	cardIDs := make([]string, 0, len(in.Cards))
	for _, c := range in.Cards {
		cardIDs = append(cardIDs, fmt.Sprintf("%d-%s", c.Number, c.Slug))
	}
	reasons = append([]string{fmt.Sprintf("batch %02d-%s holds cards %s", in.Number, in.Slug, strings.Join(cardIDs, ", "))}, reasons...)

	exhausted := in.Batch.Recoveries >= maxRecoveries && !rendersNewAmendment(in.Batch)
	var wayForward string
	if deleteNotDone {
		referenced, err := runplan.LaterDeleteReferenceReasons(in.Plan, in.Batches, &in.State.State, in.Cards, in.Geom.runPlan())
		if err != nil {
			return nil, fmt.Errorf("%w; way forward: transient, re-run `lym webster %s %d`", err, in.Verb, in.Number)
		}
		if len(referenced) > 0 {
			reasons = append(append([]string(nil), reasons...), referenced...)
			wayForward = deleteReferencedWayForward(in.Number, len(in.Batch.Uncheckable) > 0, exhausted)
		}
	}
	if wayForward == "" && glyphRejected {
		wayForward = glyphRejectedWayForward(in.Number, len(in.Batch.Uncheckable) > 0, exhausted)
	}
	if wayForward == "" && (createNotDone || signatureMismatch) {
		wayForward = createTargetWayForward(in.Number, len(in.Batch.Uncheckable) > 0, in.Verb == "recover-batch", signatureMismatch, exhausted)
	}
	return failBatch(failBatchInput{
		State:      in.State,
		Batch:      in.Batch,
		Number:     in.Number,
		Slug:       in.Slug,
		ReportsDir: in.Geom.ReportsDir,
		HeadSHA:    in.HeadSHA,
		Reasons:    reasons,
		WayForward: wayForward,
		Now:        time.Now,
	})
}

// RecordDeps carries every seam RecordBatch needs, so a test can fake each one independently.
// Batches holds the batcher-derived execution batches, computed once when the run starts.
// State is the loaded run state that RecordBatch reads and mutates.
// Config is the loaded webster.yaml.
// Engines supplies the incremental fork audit, which reads each session through the engine its batch record names.
// Geom is the Geometry that the audit and the dirty-worktree and head-SHA checks read.
// The audit resolves transcript-relative recorded write paths against Geom.PaneCwd, the directory the fork ran in,
// and the git checks read Geom.WorktreeRoot.
// RefMatcher classifies fabric references, and it is never nil in either mode:
// a *fabricengine.RefScanner in hub mode, or NeverMatches in standalone mode.
// OutcomePath and SummaryPath are the run's two Master contract files, which the parent write policy exempts.
// Sleeper is the clock seam for the bounded settle-retry wait.
type RecordDeps struct {
	Batches    []batcher.Batch
	State      *State
	Config     Config
	Engines    AuditEngines
	Geom       Geometry
	RefMatcher RefMatcher
	// ReadOnly classifies a fabric-referencing command as read-only, which makes its finding a policy warning; nil accepts nothing.
	ReadOnly    func(cmd string) bool
	OutcomePath string
	SummaryPath string
	Sleeper     Sleeper
	// Plan is the already-parsed plan that the done checks read.
	// RecordBatch refuses a nil Plan, since every production caller parses it first.
	// The done checks, handle binding and drift detection all need the parsed plan.
	// Geom.PlanDir gives the directory, and this field gives the parsed content.
	Plan *planparser.Plan
	// ParentBranch names the run's parent branch for the head cross-check's clean-parent-merge rule;
	// nil (standalone mode) accepts no merge commit between the report's head_sha and HEAD.
	ParentBranch ParentBranchFunc
	// VerifyTimeout bounds each card verify command the policy-warning evidence re-run executes;
	// zero means cardverify.DefaultTimeout.
	VerifyTimeout time.Duration
}

// RecordResult is what a successful record-batch call returns.
// Digest is the distilled digest once the batch reaches a terminal classification, and nil when NoReport is true.
// NoReport reports whether the batch report file was still absent.
// The batch then stays non-terminal and State.CurrentBatch is unchanged, and the caller re-forks once.
// Warnings carries the non-fatal fork-audit warnings observed this call; they never fail the batch.
// Failed is set when the batch was taken terminal-failed on its audit findings, and Digest is then the failed digest.
// ArchivedReport is the path an unattributable report was archived to.
// It is returned alongside a *ReportArchivedError, so the caller's fabric sync can commit it.
type RecordResult struct {
	Digest         *Digest
	NoReport       bool
	Failed         bool
	ArchivedReport string
	Warnings       []string
}

// archiveUnattributable archives batch number's report when one exists.
// It returns a *ReportArchivedError when it archived one; otherwise cause wrapped with a begin-batch way forward.
// The batch record is left unchanged.
func archiveUnattributable(deps RecordDeps, number int, slug string, cause error) (*RecordResult, error) {
	archived, err := archiveStaleReport(deps.Geom.ReportsDir, number, slug, time.Now)
	if err != nil {
		return nil, err
	}
	if archived == "" {
		return nil, fmt.Errorf("%w; way forward: lym webster begin-batch %02d re-drives the batch", cause, number)
	}
	return &RecordResult{ArchivedReport: archived}, &ReportArchivedError{
		Number:     number,
		Batch:      fmt.Sprintf("%02d-%s", number, slug),
		ArchivedTo: archived,
		Cause:      cause,
	}
}

// RecordBatch drives one record-batch call.
// It checks the bracket discipline, runs the incremental fork audit, and dispositions each finding.
// It also advances transcript attribution, parses the report, and persists the digest.
// Each audit finding is dispositioned once per run.
// A policy finding is recorded as a warning once its batch's card verify commands pass, when the report is OK.
// A correctness finding, or a policy finding whose evidence re-run fails, fails the batch.
// Such a failure returns a *BatchFailedError naming `lym webster recover-batch`.
// It comes with a RecordResult carrying the failed digest.
// A NEEDS_PARENT report records the batch as waiting with its need and archives the report.
// A call on a waiting batch refuses, naming the resume step.
// An OK report returns *LaterTargetRemovedError once per bracket:
// its batch removed a symbol that a later card names, and no rename is reported for it.
// The batch then stays open for its fork.
// The caller persists deps.State via SaveState once RecordBatch returns, whether or not it returned a *BatchFailedError.
func RecordBatch(deps RecordDeps, batchNumber int) (*RecordResult, error) {
	// The plan is a hard precondition, so it is refused here rather than dereferenced several frames down in planglyph.
	// Every production caller parses it (internal/webstercli's record-batch verb), so a nil here is a wiring mistake.
	// A nil-pointer panic out of DoneChecks would name neither the missing field nor the verb that failed to supply it.
	if deps.Plan == nil {
		return nil, fmt.Errorf("webster: record-batch requires a parsed plan; RecordDeps.Plan is nil")
	}
	// The run state is a hard precondition too, refused here before the batch lookup dereferences it.
	if deps.State == nil {
		return nil, fmt.Errorf("webster: record-batch requires loaded run state; RecordDeps.State is nil")
	}

	bs, ok := deps.State.Batches[batchNumber]
	if !ok || bs == nil {
		batch, err := findBatch(deps.Batches, batchNumber)
		if err != nil {
			return nil, err
		}
		number, slug := runplan.BatchIdentity(batch)
		return archiveUnattributable(deps, number, slug, ErrNoBeginRecord)
	}
	if bs.Terminal {
		if bs.Kind == "fork" {
			if res, err := auditTerminalFork(deps, bs, batchNumber); res != nil || err != nil {
				return res, err
			}
		}
		way := fmt.Sprintf("lym webster recover-batch %02d", batchNumber)
		if bs.Status == DigestStatusDone {
			way = "continue with the next batch"
		}
		return nil, fmt.Errorf("webster: batch %02d is already terminal (%s), so record-batch has nothing to record; way forward: %s", batchNumber, bs.Status, way)
	}
	if waitsOnParent(bs) {
		return nil, waitingRefusal("record-batch", bs, batchNumber)
	}

	// Recovery batches are consumed by recover-batch, not record-batch.
	if bs.Kind != "fork" {
		return nil, fmt.Errorf("webster: batch %d is a %s batch, not a fork batch — its report is consumed by `lym webster recover-batch %d`, never record-batch", batchNumber, bs.Kind, batchNumber)
	}

	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	number, slug := runplan.BatchIdentity(batch)

	// The bracket's own transcripts are left out of the seen set, so the engine re-reads them in full.
	seenSet := make(map[string]bool, len(deps.State.SeenForkTranscripts))
	for _, p := range deps.State.SeenForkTranscripts {
		if !slices.Contains(bs.BracketTranscripts, p) {
			seenSet[p] = true
		}
	}

	// Audit the session that opened this batch's bracket (bs.SessionID).
	// Its transcript holds the batch report even when a later, resumed session consumes it.
	engine, err := sessionAuditEngine(deps.Engines, bs.SessionID, bs.Engine)
	if err != nil {
		return nil, err
	}
	fetch := func() (shuttleengine.ForkAudit, error) {
		return engine.AuditForksIncremental(bs.SessionID, deps.Geom.PaneCwd, seenSet)
	}

	audit, newReports, err := SettleRetry(fetch, deps.State.SeenForkTranscripts, DefaultSettleWindow, DefaultSettleTick, deps.Sleeper)
	if err != nil {
		// Session transcripts are machine-local, so a resume on another machine fails here.
		if errors.Is(err, fs.ErrNotExist) {
			return archiveUnattributable(deps, number, slug, fmt.Errorf("no transcript exists on this machine for the session that opened batch %02d-%s's bracket (%s): %w — session transcripts are machine-local, so a crash window resumed on a different machine cannot re-attribute its report", number, slug, bs.SessionID, err))
		}
		return nil, err
	}

	ownReport := filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug))

	// A transcript the bracket attributed earlier counts only when it wrote this batch's report;
	// a fork stopped and resumed across a no-report call appends to such a transcript, so the next call sees nothing new.
	var bracketForks, counted []shuttleengine.ForkReport
	for _, f := range audit.Forks {
		if slices.Contains(bs.BracketTranscripts, f.TranscriptPath) {
			bracketForks = append(bracketForks, f)
			if slices.ContainsFunc(f.WritePaths, func(w string) bool { return isOwnReportWrite(deps.Geom.PaneCwd, w, ownReport) }) {
				counted = append(counted, f)
			}
		}
	}
	counted = append(counted, newReports...)

	// Check transcripts before report presence, so a report that no transcript wrote is caught.
	warning, err := ClassifyAttribution(counted)
	if err != nil {
		return archiveUnattributable(deps, number, slug, err)
	}

	var warnings []string
	if warning != "" {
		warnings = append(warnings, warning)
	}

	// forkWarnings are held back and appended only on the no-report path: once the report file
	// exists, the report is the fork's contract and "never returned a final report" is false noise.
	var forkWarnings []string
	planDirs, err := planDirSpellings(deps.Geom)
	if err != nil {
		return nil, err
	}
	websterDirs, err := websterDirSpellings(deps.Geom)
	if err != nil {
		return nil, err
	}
	var candidates []AuditViolation
	candidates = append(candidates, CheckParent(audit, deps.OutcomePath, deps.SummaryPath, deps.Geom.PaneCwd, deps.RefMatcher)...)
	for _, f := range append(slices.Clone(newReports), bracketForks...) {
		candidates = append(candidates, CheckFork(f, deps.OutcomePath, deps.SummaryPath, deps.Geom.PaneCwd, planDirs, websterDirs, ownReport, deps.RefMatcher)...)
		forkWarnings = append(forkWarnings, ForkWarnings(f)...)
	}

	// A finding dispositioned by an earlier call is dropped.
	// The whole-session parent audit repeats every earlier finding on each record-batch,
	// and a finding is reported once per run.
	// Classification is the only fallible step and runs before any mutation.
	var policy, correctness []classifiedFinding
	for _, v := range candidates {
		id := findingIdentity(bs.SessionID, v)
		if isDispositioned(deps.State, id) {
			continue
		}
		severity, err := ClassifyViolation(v, deps.Geom, deps.ReadOnly)
		if err != nil {
			return nil, err
		}
		cf := classifiedFinding{ID: id, Violation: v}
		if severity == AuditSeverityCorrectness {
			correctness = append(correctness, cf)
		} else {
			policy = append(policy, cf)
		}
	}

	newPaths := make([]string, 0, len(newReports))
	for _, f := range newReports {
		newPaths = append(newPaths, f.TranscriptPath)
	}

	polledID := fmt.Sprintf("%02d-%s", number, slug)
	reportPath := filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug))

	// A correctness finding fails the batch whatever the report says, and with no report at all:
	// it is a halt with a way forward, and failing the batch dispositions every finding once.
	if len(correctness) > 0 {
		all := append(append([]classifiedFinding(nil), correctness...), policy...)
		headSHA := ""
		if r, perr := ParseReport(reportPath); perr == nil {
			resolved, rerr := resolveReportHead(deps.Geom.git(), deps.Geom.WorktreeRoot, reportPath, r.HeadSHA)
			switch {
			case rerr == nil:
				headSHA = resolved
			case !errors.Is(rerr, ErrHeadSHAUnresolved):
				return nil, rerr
			}
		}
		return failOnCorrectness(deps, bs, number, slug, headSHA, all, correctness, newPaths, warnings)
	}

	// A plan edited since the run recorded it, or a begun card edited since its batch began, is refused.
	// The check runs before anything mutates or any card verify runs:
	// every restamp further down adopts webster's own rewrites,
	// so a difference seen here is someone else's edit.
	if err := PlanEditError(deps.State, deps.Geom.PlanDir, deps.Geom.WebsterDir); err != nil {
		return nil, err
	}
	if err := batchCardEditError(deps.State, bs, batch, deps.Geom.PlanDir, deps.Geom.WebsterDir); err != nil {
		return nil, err
	}

	// Attribution advances before report-presence check so a retry sees only its own new transcript.
	// Each list advances by the new transcripts only, without duplicates.
	for _, p := range newPaths {
		if !slices.Contains(deps.State.SeenForkTranscripts, p) {
			deps.State.SeenForkTranscripts = append(deps.State.SeenForkTranscripts, p)
		}
		if !slices.Contains(bs.ForkTranscripts, p) {
			bs.ForkTranscripts = append(bs.ForkTranscripts, p)
		}
		if !slices.Contains(bs.BracketTranscripts, p) {
			bs.BracketTranscripts = append(bs.BracketTranscripts, p)
		}
	}

	if _, statErr := os.Stat(reportPath); statErr != nil {
		if os.IsNotExist(statErr) {
			for _, cf := range policy {
				if text, added := recordBatchWarning(deps.State, bs, cf.ID, string(cf.Violation.Class), cf.Violation.Detail); added {
					warnings = append(warnings, text)
				}
			}
			return &RecordResult{NoReport: true, Warnings: append(warnings, forkWarnings...)}, nil
		}
		return nil, fmt.Errorf("webster: stat batch report %s: %w", reportPath, statErr)
	}

	report, err := ParseReport(reportPath)
	if err != nil {
		return nil, fmt.Errorf("%w; way forward: `lym webster recover-batch %d` archives the malformed report and re-drives the batch", err, number)
	}

	// The run record holds full SHAs only, so an abbreviated head_sha is resolved before its first use.
	if report.HeadSHA, err = resolveReportHead(deps.Geom.git(), deps.Geom.WorktreeRoot, reportPath, report.HeadSHA); err != nil {
		return nil, err
	}

	// A merge in progress leaves the batch non-terminal and retryable.
	if err := refuseMidMerge(deps.Geom.git(), deps.Geom.WorktreeRoot); err != nil {
		return nil, err
	}

	if isDirty, err := deps.Geom.git().Dirty(deps.Geom.WorktreeRoot); err != nil {
		return nil, err
	} else if isDirty {
		warnings = append(warnings, fmt.Sprintf("worktree is dirty after batch %s's own commits (uncommitted or untracked changes remain)", polledID))
	}

	// Cross-check report's head_sha against the worktree's actual HEAD, tolerating a parent merge-in.
	moved, err := reconcileReportHead(deps.Geom.git(), deps.Geom.WorktreeRoot, report.HeadSHA, "batch report "+reportPath, deps.ParentBranch, number)
	if err != nil {
		return nil, err
	}
	if moved != "" {
		warnings = append(warnings, moved)
	}

	// An OK report with policy findings, or a batch with earlier warnings, needs its cards' verify commands to pass:
	// a policy warning is only safe to carry once the work is evidenced.
	// A FAILED report takes its policy findings as warnings with no re-run.
	if report.Status == ReportStatusOK && (len(policy) > 0 || len(bs.AuditWarnings) > 0) {
		runner := cardverify.Runner{Worktree: deps.Geom.PaneCwd, Slots: deps.Geom.GateSlots, WaitDir: deps.Geom.GateWaitDir, Timeout: deps.VerifyTimeout}
		if failures := rerunCardVerifies(batch.Cards, runner); len(failures) > 0 {
			var earlier []string
			for _, w := range bs.AuditWarnings {
				earlier = append(earlier, auditWarningText(w))
			}
			return failFromFindings(deps, bs, number, slug, report.HeadSHA, policy, earlier, failures, nil, nil, nil, warnings)
		}
	}
	for _, cf := range policy {
		if text, added := recordBatchWarning(deps.State, bs, cf.ID, string(cf.Violation.Class), cf.Violation.Detail); added {
			warnings = append(warnings, text)
		}
	}

	// A batch waiting on its parent has not finished, so it runs no post-batch pass and spends no recovery.
	if report.Status == ReportStatusNeedsParent {
		digest := runplan.Distill(report)
		digest.Batch = polledID
		if err := recordWaiting(bs, &digest, deps.Geom.ReportsDir, number, slug, time.Now); err != nil {
			return nil, err
		}
		return &RecordResult{Digest: &digest, Warnings: warnings}, nil
	}

	// Only an OK report's fork is waiting to be told: a FAILED one goes to a recovery strand instead.
	deviations, postWarnings, err := runplan.PostBatchChecks(postBatchInputs(deps.Geom, deps.Plan, deps.Batches, deps.State, bs, batch.Cards, report.HeadSHA, report.Renames, report.Status == ReportStatusOK))
	warnings = append(warnings, postWarnings...)
	if err != nil {
		if !errors.Is(err, ErrCardNotDone) {
			return nil, err
		}
		// The findings concern this batch's own cards, so it fails on its merits.
		bfe, ferr := failCardNotDone(cardNotDoneInputs{
			Plan:    deps.Plan,
			Batches: deps.Batches,
			State:   deps.State,
			Batch:   bs,
			Cards:   batch.Cards,
			Number:  number,
			Slug:    slug,
			Geom:    deps.Geom,
			HeadSHA: report.HeadSHA,
			Verb:    "record-batch",
		}, err)
		if ferr != nil {
			return nil, ferr
		}
		return &RecordResult{Digest: bs.Digest, Failed: true, Warnings: warnings}, bfe
	}

	// An amendment accepted while this attempt ran means the attempt built the old card,
	// so the batch fails whatever the report says and recovery re-runs it.
	if len(amendedReasons(bs)) > 0 {
		bfe, ferr := failBatch(failBatchInput{
			State:        deps.State,
			Batch:        bs,
			Number:       number,
			Slug:         slug,
			ReportsDir:   deps.Geom.ReportsDir,
			PaneCwd:      deps.Geom.PaneCwd,
			WorktreeRoot: deps.Geom.WorktreeRoot,
			Git:          deps.Geom.Git,
			HeadSHA:      report.HeadSHA,
			Now:          time.Now,
		})
		if ferr != nil {
			return nil, ferr
		}
		return &RecordResult{Digest: bs.Digest, Failed: true, Warnings: warnings}, bfe
	}

	departures, err := runplan.BatchDepartures(deps.Geom.runPlan(), bs.StartSHA, report.HeadSHA)
	if err != nil {
		return nil, err
	}

	digest := runplan.Distill(report)
	digest.Batch = polledID
	digest.Deviations = deviations
	digest.Departures = departures

	bs.Digest = &digest
	bs.CardSHAs = []string{report.HeadSHA}
	bs.Terminal = true
	bs.Status = digest.Status
	deps.State.CurrentBatch = 0

	return &RecordResult{Digest: &digest, Warnings: warnings}, nil
}

// failOnCorrectness fails the batch on its audit findings, naming the correctness findings' paths as suspects.
// A correctness finding with no path is recorded as uncheckable, since recovery cannot verify it.
func failOnCorrectness(deps RecordDeps, bs *BatchState, number int, slug, headSHA string, all, correctness []classifiedFinding, newPaths, warnings []string) (*RecordResult, error) {
	var suspects []string
	for _, cf := range correctness {
		if cf.Violation.Path != "" {
			suspects = append(suspects, cf.Violation.Path)
		}
	}
	uncheckable, err := uncheckableSuspects(deps.Geom, deps.State, suspects)
	if err != nil {
		return nil, err
	}
	for _, cf := range correctness {
		if cf.Violation.Path == "" {
			uncheckable = append(uncheckable, pathlessEntry(cf.Violation.Class, cf.Violation.Detail))
		}
	}
	return failFromFindings(deps, bs, number, slug, headSHA, all, nil, nil, suspects, uncheckable, newPaths, warnings)
}

// uncheckableSuspects returns the paths that cannot be checked:
// neither under geom.PlanDir with st.PlanFileHashes recorded, nor in the task worktree's tracked tree.
// The error is a link-resolution or git probe failure.
func uncheckableSuspects(geom Geometry, st *State, paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		_, uncheckable, err := uncheckableReason(geom, st, p)
		if err != nil {
			return nil, err
		}
		if uncheckable {
			out = append(out, p)
		}
	}
	return out, nil
}

// pathlessViolationClasses is every class a pathless correctness finding can carry.
var pathlessViolationClasses = []AuditViolationClass{
	ClassNestedAgent, ClassFabricReference, ClassNamedSpawn, ClassParentWrite,
	ClassForkContractWrite, ClassForkPlanWrite, ClassForkStateWrite,
}

// pathlessEntry renders the Uncheckable entry recorded for a correctness finding that names no path: "<class>: <detail>".
func pathlessEntry(class AuditViolationClass, detail string) string {
	return fmt.Sprintf("%s: %s", class, detail)
}

// pathlessEntryClass reports the class a pathless entry opens with.
// It reports true only for an entry starting with a known class followed by ": ", so a path never matches.
func pathlessEntryClass(entry string) (AuditViolationClass, bool) {
	for _, class := range pathlessViolationClasses {
		if strings.HasPrefix(entry, string(class)+": ") {
			return class, true
		}
	}
	return "", false
}

// auditTerminalFork audits the fork transcripts that a terminal fork batch has not consumed yet.
// It runs once and without the settle wait,
// so a fork that forged its own batch record cannot land behind the "already terminal" refusal.
// An undispositioned correctness finding replaces the terminal record with a failed one.
// The call then returns the *BatchFailedError;
// otherwise nothing is mutated and both results are nil, leaving the caller to refuse the batch as already terminal.
// A session whose transcripts are not on this machine has nothing to audit and is not an error.
// It audits nothing while the verify-gate report exists.
// Nor does it audit while another fork batch of the session is begun and not terminal.
// Every fork of a Master session shares its session id, so an unseen transcript may then be that fork's.
// That batch's own record-batch, or the run-exit audit for the verify-gate fixers, audits it instead.
func auditTerminalFork(deps RecordDeps, bs *BatchState, batchNumber int) (*RecordResult, error) {
	for n, other := range deps.State.Batches {
		if n != batchNumber && other != nil && other.Kind == "fork" && !other.Terminal && other.SessionID == bs.SessionID {
			return nil, nil
		}
	}
	if _, err := os.Stat(VerifyGateReportPath(deps.Geom.ReportsDir)); err == nil {
		return nil, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("webster: stat verify-gate report: %w", err)
	}

	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	number, slug := runplan.BatchIdentity(batch)

	seenSet := make(map[string]bool, len(deps.State.SeenForkTranscripts))
	for _, p := range deps.State.SeenForkTranscripts {
		seenSet[p] = true
	}
	engine, err := sessionAuditEngine(deps.Engines, bs.SessionID, bs.Engine)
	if err != nil {
		return nil, err
	}
	audit, err := engine.AuditForksIncremental(bs.SessionID, deps.Geom.PaneCwd, seenSet)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	newReports := NewTranscripts(audit, deps.State.SeenForkTranscripts)
	if len(newReports) == 0 {
		return nil, nil
	}

	planDirs, err := planDirSpellings(deps.Geom)
	if err != nil {
		return nil, err
	}
	websterDirs, err := websterDirSpellings(deps.Geom)
	if err != nil {
		return nil, err
	}
	ownReport := filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug))
	var correctness []classifiedFinding
	for _, f := range newReports {
		for _, v := range CheckFork(f, deps.OutcomePath, deps.SummaryPath, deps.Geom.PaneCwd, planDirs, websterDirs, ownReport, deps.RefMatcher) {
			id := findingIdentity(bs.SessionID, v)
			if isDispositioned(deps.State, id) {
				continue
			}
			severity, err := ClassifyViolation(v, deps.Geom, deps.ReadOnly)
			if err != nil {
				return nil, err
			}
			if severity == AuditSeverityCorrectness {
				correctness = append(correctness, classifiedFinding{ID: id, Violation: v})
			}
		}
	}
	if len(correctness) == 0 {
		return nil, nil
	}

	newPaths := make([]string, 0, len(newReports))
	for _, f := range newReports {
		newPaths = append(newPaths, f.TranscriptPath)
	}
	headSHA := ""
	if bs.Digest != nil {
		headSHA = bs.Digest.HeadSHA
	}
	return failOnCorrectness(deps, bs, number, slug, headSHA, correctness, correctness, newPaths, nil)
}

// classifiedFinding is one audit finding with its ledger identity.
type classifiedFinding struct {
	ID        string
	Violation AuditViolation
}

// failFromFindings takes the batch terminal-failed on its audit findings.
// Every finding is marked failed in the ledger,
// and the reasons are the findings' own text, then the earlier recorded warnings, then the verify failures.
// suspects are the correctness paths.
// uncheckable are the entries recovery cannot verify.
// newTranscripts are the transcripts this call consumes.
// It returns the failed digest with Failed set, together with the *BatchFailedError.
func failFromFindings(deps RecordDeps, bs *BatchState, number int, slug, headSHA string, findings []classifiedFinding, earlier, verifyFailures, suspects, uncheckable, newTranscripts, warnings []string) (*RecordResult, error) {
	var reasons []string
	for _, cf := range findings {
		reasons = append(reasons, cf.Violation.Error())
	}
	reasons = append(reasons, earlier...)
	reasons = append(reasons, verifyFailures...)
	bfe, err := failBatch(failBatchInput{
		State:          deps.State,
		Batch:          bs,
		Number:         number,
		Slug:           slug,
		ReportsDir:     deps.Geom.ReportsDir,
		PaneCwd:        deps.Geom.PaneCwd,
		WorktreeRoot:   deps.Geom.WorktreeRoot,
		Git:            deps.Geom.Git,
		HeadSHA:        headSHA,
		Reasons:        reasons,
		SuspectPaths:   suspects,
		Uncheckable:    uncheckable,
		NewTranscripts: newTranscripts,
		Now:            time.Now,
	})
	if err != nil {
		return nil, err
	}
	for _, cf := range findings {
		recordFailedFinding(deps.State, cf.ID)
	}
	return &RecordResult{Digest: bs.Digest, Failed: true, Warnings: warnings}, bfe
}

// postBatchInputs returns the shared post-batch checks' inputs for batch record bs, whichever verb takes it terminal.
// headSHA is the reconciled report head and renames the glyph renames its report lists.
// handBack asks the checks to hand a batch that removed a later card's symbol back to its fork.
// The refusal names record-batch again.
// A drift finding on a later card is recorded once on bs through webster's audit ledger.
func postBatchInputs(geom Geometry, plan *planparser.Plan, batches []batcher.Batch, st *State, bs *BatchState, cards []planparser.Card, headSHA string, renames []ReportRename, handBack bool) runplan.PostBatchInputs {
	number, slug := runplan.BatchIdentity(batcher.Batch{Cards: cards})
	return runplan.PostBatchInputs{
		Plan:                   plan,
		State:                  &st.State,
		Batch:                  bs,
		Geom:                   geom.runPlan(),
		Batches:                batches,
		Number:                 number,
		Cards:                  cards,
		StartSHA:               bs.StartSHA,
		HeadSHA:                headSHA,
		Label:                  runplan.CardID(planparser.Card{Number: number, Slug: slug}),
		Renames:                renames,
		HandBackRemovedTargets: handBack,
		HandBackWayForward:     laterTargetHandBackWayForward(number),
		RecordWarning: func(id, class, detail string) (string, bool) {
			return recordBatchWarning(st, bs, id, class, detail)
		},
	}
}
