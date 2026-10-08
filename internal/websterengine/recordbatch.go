// recordbatch.go implements RecordBatch, the second of webster's two bracket verbs Master calls around each in-session fork, immediately after a fork returns: the bracket-discipline fail-loud check (a record without a matching begin-batch record is refused), the incremental fork audit with its bounded settle retry, webster's fork-audit findings and their once-per-run dispositions (a policy finding warns, a correctness finding fails the batch), the unconditional transcript-attribution advance, the batch-report presence check and parse, the head-SHA cross-check against the fork's own self-reported head_sha, and the distilled digest's persistence.
// RecordBatch never touches the fabric repo — the caller fabric-commits state.json and the batch
// report once RecordBatch returns successfully, webster's own fabric-commit-boundary discipline.

package websterengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrNoBeginRecord is the cause RecordBatch reports when deps.State.Batches[batchNumber] is absent — a record call with no matching begin-batch record.
// This is the bracket-discipline fail-loud check: a fork's own report, however legitimate it looks,
// is never trusted without Go's own record that begin-batch actually opened this batch first.
var ErrNoBeginRecord = errors.New("webster: record-batch called with no begin-batch record for this batch")

// ErrReportArchived is the sentinel a *ReportArchivedError unwraps to.
var ErrReportArchived = errors.New("webster: report archived")

// ReportArchivedError reports a batch report record-batch could not attribute to a fork of its bracket.
// The report is archived, so `begin-batch` no longer refuses over it, and the batch is re-driven through that verb.
// Cause is the attribution failure (ErrNoBeginRecord, ErrNoForkTranscripts, or a missing transcript directory).
type ReportArchivedError struct {
	Number     int
	Batch      string
	ArchivedTo string
	Cause      error
}

// Error states why the report could not be attributed, where it was archived, and the way forward.
func (e *ReportArchivedError) Error() string {
	return fmt.Sprintf("webster: batch %s's report could not be attributed: %v; report archived to %s; way forward: lyx webster begin-batch %02d re-drives the batch", e.Batch, e.Cause, e.ArchivedTo, e.Number)
}

// Unwrap returns ErrReportArchived and Cause, so errors.Is matches either.
func (e *ReportArchivedError) Unwrap() []error { return []error{ErrReportArchived, e.Cause} }

// ErrCardNotDone is the sentinel RecordBatch returns when card 33's DoneChecks report a blocking
// finding against the just-completed batch's own cards — a Create target that still does not
// resolve, or a Delete target that still does — meaning the batch is not done.
// RecordBatch converts it into a failed batch (see failBatch) with the findings as its reasons.
// webster's own sentinel, per the webster-owns-its-own-domain-types decision.
var ErrCardNotDone = errors.New("webster: record-batch's done-checks reported a blocking finding")

// cardNotDoneError is the error the post-batch done-checks return: it unwraps to ErrCardNotDone and its message carries the findings' text.
// deleteNotDone is set when a finding is a Delete target that still resolves.
type cardNotDoneError struct {
	reasons       []string
	deleteNotDone bool
}

func (e *cardNotDoneError) Error() string {
	return fmt.Sprintf("%s: %s", ErrCardNotDone, strings.Join(e.reasons, "; "))
}

func (e *cardNotDoneError) Unwrap() error { return ErrCardNotDone }

// unbegunCards returns the cards of every batch with no record in st, in batches' own order.
func unbegunCards(batches []batcher.Batch, st *State) []planparser.Card {
	var cards []planparser.Card
	for _, b := range batches {
		number, _ := batchIdentity(b)
		if bs, ok := st.Batches[number]; ok && bs != nil {
			continue
		}
		cards = append(cards, b.Cards...)
	}
	return cards
}

// deleteReferencedWayForward is the way forward for a batch whose Delete target an unbegun later card still references.
// Recovery cannot change the plan, so the way forward edits it; a record recovery would refuse as uncheckable restarts the run instead.
func deleteReferencedWayForward(number int, uncheckable bool) string {
	if uncheckable {
		return freshRestartSteps(stepRun)
	}
	return fmt.Sprintf("move the delete to a card after the one that still references it, run `lyx webster rebaseline --card NN` naming each card you edited, then `lyx webster recover-batch %02d`", number)
}

// laterDeleteReferenceReasons returns one reason per reference an unbegun card still has to a symbol the cards in own delete.
// An infrastructure error is wrapped in planindex.ErrQuarryUnavailable.
func laterDeleteReferenceReasons(plan *planparser.Plan, batches []batcher.Batch, st *State, own []planparser.Card, geom Geometry) ([]string, error) {
	index, err := geom.index()
	if err != nil {
		return nil, err
	}
	findings, err := index.LaterDeleteReferences(plan, own, unbegunCards(batches, st), geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}
	reasons := make([]string, 0, len(findings))
	for _, f := range findings {
		reasons = append(reasons, f.Error())
	}
	return reasons, nil
}

// cardNotDoneInputs carries everything failCardNotDone needs to fail a batch on its post-batch findings.
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

// failCardNotDone takes the batch terminal-failed on cause, an ErrCardNotDone-wrapped error from the post-batch pass.
// When a finding is a Delete target that still resolves and an unbegun later card still references it, the reasons also name that reference,
// and the way forward is the plan edit rather than recover-batch, which would only repeat the same failure.
func failCardNotDone(in cardNotDoneInputs, cause error) (*BatchFailedError, error) {
	var reasons []string
	var deleteNotDone bool
	var notDone *cardNotDoneError
	if errors.As(cause, &notDone) {
		reasons = notDone.reasons
		deleteNotDone = notDone.deleteNotDone
	} else {
		reasons = strings.Split(strings.TrimPrefix(cause.Error(), ErrCardNotDone.Error()+": "), "; ")
	}

	var wayForward string
	if deleteNotDone {
		referenced, err := laterDeleteReferenceReasons(in.Plan, in.Batches, in.State, in.Cards, in.Geom)
		if err != nil {
			return nil, fmt.Errorf("%w; way forward: transient, re-run `lyx webster %s %d`", err, in.Verb, in.Number)
		}
		if len(referenced) > 0 {
			reasons = append(append([]string(nil), reasons...), referenced...)
			wayForward = deleteReferencedWayForward(in.Number, len(in.Batch.Uncheckable) > 0)
		}
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
	// Plan is the already-parsed plan — mirroring the field BeginDeps already carries. DoneChecks
	// here, and BindHandles/DetectDrift in cards 34 and 36, all need the parsed plan, and RecordDeps
	// carried none before this field: deps.Geom.PlanDir reaches the directory but nothing reached
	// the plan itself.
	Plan *planparser.Plan
	// ParentBranch names the run's parent branch for the head cross-check's clean-parent-merge rule;
	// nil (standalone mode) accepts no merge commit between the report's head_sha and HEAD.
	ParentBranch ParentBranchFunc
	// VerifyTimeout bounds each card verify command the policy-warning evidence re-run executes;
	// zero means DefaultCardVerifyTimeout.
	VerifyTimeout time.Duration
}

// RecordResult is what one successful RecordBatch call hands back to its caller
// (internal/webstercli's record-batch verb): Digest is the distilled digest once the batch reaches
// a terminal classification (nil when NoReport is true);
// NoReport reports whether the batch-report file was still absent this call (the batch stays
// non-terminal and State.CurrentBatch stays unchanged — Master's ladder re-forks once);
// Warnings carries every non-fatal fork-audit-policy warning observed this call (a multi-new-transcript notice, a fork that never returned a final report, a dirty worktree after the batch's own commits, a moved-HEAD notice when a parent merge-in landed after the fork's commit, or a recorded policy audit warning), never treated as a failure;
// Failed is set when the batch was taken terminal-failed on its audit findings, with Digest the failed digest;
// ArchivedReport is the path an unattributable report was archived to, returned alongside a *ReportArchivedError so the caller's fabric sync can commit it.
type RecordResult struct {
	Digest         *Digest
	NoReport       bool
	Failed         bool
	ArchivedReport string
	Warnings       []string
}

// archiveUnattributable archives batch number's report, when one exists, and returns the error a record-batch attribution refusal ends with:
// a *ReportArchivedError when a report was archived, otherwise cause wrapped with the same begin-batch way forward.
// The batch record is left as it is.
func archiveUnattributable(deps RecordDeps, number int, slug string, cause error) (*RecordResult, error) {
	archived, err := archiveStaleReport(deps.Geom.ReportsDir, number, slug, time.Now)
	if err != nil {
		return nil, err
	}
	if archived == "" {
		return nil, fmt.Errorf("%w; way forward: lyx webster begin-batch %02d re-drives the batch", cause, number)
	}
	return &RecordResult{ArchivedReport: archived}, &ReportArchivedError{
		Number:     number,
		Batch:      fmt.Sprintf("%02d-%s", number, slug),
		ArchivedTo: archived,
		Cause:      cause,
	}
}

// RecordBatch drives one record-batch call: the bracket-discipline check, incremental fork audit, fork-audit finding dispositions, transcript-attribution advance, report parse, and digest persistence.
// Each audit finding is dispositioned once per run: a policy finding is recorded as a warning (after its batch's card verify commands pass, when the report is OK),
// and a correctness finding, or a policy finding whose evidence re-run fails, fails the batch and returns a *BatchFailedError naming `lyx webster recover-batch`, alongside a RecordResult carrying the failed digest.
// The caller persists deps.State via SaveState once RecordBatch returns, whether or not it returned a *BatchFailedError.
func RecordBatch(deps RecordDeps, batchNumber int) (*RecordResult, error) {
	// The plan is a hard precondition, refused loudly rather than dereferenced several frames down
	// inside planglyph. Every production caller parses it (internal/webstercli's record-batch verb),
	// so a nil here is a wiring mistake in a test or a new caller, and a nil-pointer panic out of
	// DoneChecks names neither the missing field nor the verb that failed to supply it.
	if deps.Plan == nil {
		return nil, fmt.Errorf("webster: record-batch requires a parsed plan; RecordDeps.Plan is nil")
	}
	// State is the same kind of hard precondition, and was the half-applied one: it is dereferenced
	// on the very next line (crucible round opus-medium-r6, R6-21).
	if deps.State == nil {
		return nil, fmt.Errorf("webster: record-batch requires loaded run state; RecordDeps.State is nil")
	}

	bs, ok := deps.State.Batches[batchNumber]
	if !ok || bs == nil {
		batch, err := findBatch(deps.Batches, batchNumber)
		if err != nil {
			return nil, err
		}
		number, slug := batchIdentity(batch)
		return archiveUnattributable(deps, number, slug, ErrNoBeginRecord)
	}
	if bs.Terminal {
		if bs.Kind == "fork" {
			if res, err := auditTerminalFork(deps, bs, batchNumber); res != nil || err != nil {
				return res, err
			}
		}
		way := fmt.Sprintf("lyx webster recover-batch %02d", batchNumber)
		if bs.Status == DigestStatusDone {
			way = "continue with the next batch"
		}
		return nil, fmt.Errorf("webster: batch %02d is already terminal (%s), so record-batch has nothing to record; way forward: %s", batchNumber, bs.Status, way)
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
			return archiveUnattributable(deps, number, slug, fmt.Errorf("no transcript exists on this machine for the session that opened batch %02d-%s's bracket (%s): %w — session transcripts are machine-local, so a crash window resumed on a different machine cannot re-attribute its report", number, slug, bs.SessionID, err))
		}
		return nil, err
	}

	// Check transcripts before report presence so a fake (unfakeable) report is caught.
	warning, err := ClassifyAttribution(newReports)
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
	ownReport := filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug))
	var candidates []AuditViolation
	candidates = append(candidates, CheckParent(audit, deps.OutcomePath, deps.SummaryPath, deps.Geom.WorktreeRoot, deps.RefMatcher)...)
	for _, f := range newReports {
		candidates = append(candidates, CheckFork(f, deps.OutcomePath, deps.SummaryPath, deps.Geom.WorktreeRoot, planDirs, websterDirs, ownReport, deps.RefMatcher)...)
		forkWarnings = append(forkWarnings, ForkWarnings(f)...)
	}

	// A finding dispositioned by an earlier call is dropped: the whole-session parent audit repeats every earlier finding on each record-batch,
	// and a finding is reported once per run.
	// Classification is the only fallible step and runs before any mutation.
	var policy, correctness []classifiedFinding
	for _, v := range candidates {
		id := findingIdentity(bs.SessionID, v)
		if isDispositioned(deps.State, id) {
			continue
		}
		severity, err := ClassifyViolation(v, deps.Geom)
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

	// A plan edited since the run recorded it, or a begun card edited since its batch began, is refused before anything mutates or any card verify runs:
	// every restamp further down exists to adopt webster's own rewrites, so a difference seen here is someone else's edit.
	if err := PlanEditError(deps.State, deps.Geom.PlanDir); err != nil {
		return nil, err
	}
	if err := batchCardEditError(deps.State, bs, batch, deps.Geom.PlanDir); err != nil {
		return nil, err
	}

	// Attribution advances before report-presence check so a retry sees only its own new transcript.
	deps.State.SeenForkTranscripts = append(deps.State.SeenForkTranscripts, newPaths...)
	bs.ForkTranscripts = append(bs.ForkTranscripts, newPaths...)

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
		return nil, fmt.Errorf("%w; way forward: `lyx webster recover-batch %d` archives the malformed report and re-drives the batch", err, number)
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

	// An OK report that carries policy findings, or whose batch already holds warnings from an earlier no-report call, must show its cards' own verify commands still pass:
	// a policy warning is only safe to carry once the work is evidenced.
	// A FAILED report takes its policy findings as warnings with no re-run.
	if report.Status == ReportStatusOK && (len(policy) > 0 || len(bs.AuditWarnings) > 0) {
		if failures := rerunCardVerifies(batch.Cards, deps.Geom.WorktreeRoot, deps.VerifyTimeout); len(failures) > 0 {
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

	postWarnings, err := postBatchChecks(postBatchInputs{
		Plan:      deps.Plan,
		State:     deps.State,
		Batch:     bs,
		Geom:      deps.Geom,
		Cards:     batch.Cards,
		Completed: completedCards(deps.Batches, deps.State, batchNumber),
		StartSHA:  bs.StartSHA,
		HeadSHA:   report.HeadSHA,
		Label:     polledID,
	})
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

	// An amendment accepted while this attempt ran means the attempt built the old card, so the batch fails whatever the report says and recovery re-runs it.
	if len(amendedReasons(bs)) > 0 {
		bfe, ferr := failBatch(failBatchInput{
			State:        deps.State,
			Batch:        bs,
			Number:       number,
			Slug:         slug,
			ReportsDir:   deps.Geom.ReportsDir,
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

	digest := distill(report)
	digest.Batch = polledID

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
			uncheckable = append(uncheckable, fmt.Sprintf("%s: %s", cf.Violation.Class, cf.Violation.Detail))
		}
	}
	return failFromFindings(deps, bs, number, slug, headSHA, all, nil, nil, suspects, uncheckable, newPaths, warnings)
}

// auditTerminalFork audits the fork transcripts a terminal fork batch has not consumed yet, once and without the settle wait,
// so a fork that forged its own batch record cannot land behind the "already terminal" refusal.
// An undispositioned correctness finding replaces the terminal record with a failed one and returns its *BatchFailedError;
// otherwise nothing is mutated and both results are nil, leaving the caller to refuse the batch as already terminal.
// A session whose transcripts are not on this machine has nothing to audit and is not an error.
// It audits nothing while another fork batch of the session is begun and not terminal or the verify-gate report exists:
// every fork of a Master session shares its session id, so an unseen transcript may then be that fork's.
// That batch's own record-batch, or the run-exit audit for the verify-gate fixer fork, audits it instead.
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
	number, slug := batchIdentity(batch)

	seenSet := make(map[string]bool, len(deps.State.SeenForkTranscripts))
	for _, p := range deps.State.SeenForkTranscripts {
		seenSet[p] = true
	}
	audit, err := deps.Engine.AuditForksIncremental(bs.SessionID, deps.Geom.WorktreeRoot, seenSet)
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
		for _, v := range CheckFork(f, deps.OutcomePath, deps.SummaryPath, deps.Geom.WorktreeRoot, planDirs, websterDirs, ownReport, deps.RefMatcher) {
			id := findingIdentity(bs.SessionID, v)
			if isDispositioned(deps.State, id) {
				continue
			}
			severity, err := ClassifyViolation(v, deps.Geom)
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
// suspects are the correctness paths, uncheckable the entries recovery cannot verify, newTranscripts the transcripts this call consumes.
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

// postBatchInputs carries everything the shared post-batch mechanical pass needs.
// Cards are the completed batch's own cards;
// Completed names every card whose work landed BEFORE this batch, so drift detection can scope itself to the plan's remaining work;
// StartSHA is the bracket record's captured start SHA and HeadSHA the reconciled report head;
// Label names the batch in warnings;
// Batch is the record being finished, which later-card drift warnings are recorded onto.
type postBatchInputs struct {
	Plan      *planparser.Plan
	State     *State
	Batch     *BatchState
	Geom      Geometry
	Cards     []planparser.Card
	Completed []planparser.Card
	StartSHA  string
	HeadSHA   string
	Label     string
}

// postBatchChecks runs the mechanical pass every terminal batch owes, whichever verb takes it
// terminal: the card done-checks, the single delta the rest of the pass shares, handle binding, the
// informational glyph scope guard, and drift detection with its exact-tier auto-repair — plus the
// plan-staleness re-baseline each of those rewrites requires.
//
// It is one function rather than two because a batch that reached done through recover-batch is
// exactly as done as one that reached it through record-batch: its Create targets have to resolve,
// its plan: handles have to bind, and the drift its work caused has to be repaired or reported.
// recover-batch used to skip the whole pass, so a recovered card's handles were never bound and
// every later card kept referencing an unbound plan: handle for the rest of the plan's life —
// invisible to drift detection too, since its reference index keys on the ref as the card spells it.
//
// Findings about the batch's own cards (done-checks, blocking bind findings) are returned as an
// ErrCardNotDone-wrapped error.
// Drift findings concern later cards, so they become `later card:` warnings recorded once on in.Batch;
// informational findings ride out on warnings too, which are returned alongside any error so a caller never loses them.
func postBatchChecks(in postBatchInputs) (warnings []string, err error) {
	// A card's completion has a mechanical verdict: a Create target that still does not resolve,
	// or a Delete target that still does, blocks — neither is a judgment call. This runs its own
	// batched Resolve against the post-card tree, distinct from the single delta call below.
	index, err := in.Geom.index()
	if err != nil {
		return nil, err
	}
	doneFindings, err := index.DoneChecks(in.Plan, in.Cards, in.Geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}
	notDone := &cardNotDoneError{}
	for _, f := range doneFindings {
		notDone.reasons = append(notDone.reasons, f.Error())
		if f.Check == "delete-not-done" {
			notDone.deleteNotDone = true
		}
	}
	if len(notDone.reasons) > 0 {
		return nil, notDone
	}

	// The batch's single delta call: BindHandles, ScopeGuard and DetectDrift all consume this one planindex.Delta rather than each spawning their own.
	// HeadSHA is the reconciled report head, already cross-checked against the worktree's real HEAD by the caller —
	// that cross-check is why the delta can be trusted here and nowhere earlier.
	// A DeltaGit infrastructure error does not abort the sequence: the scope guard degrades to an
	// informational notice on this same deltaErr, while the done-checks above ran on their own
	// Resolve and are unaffected. delta itself is the zero value on error, so BindHandles correctly
	// cannot confirm any handle bound and reports bind-count-mismatch for every card that declared
	// one — an unconfirmed Create is exactly a not-done card.
	delta, deltaErr := index.Delta(in.Geom.WorktreeRoot, in.StartSHA, in.HeadSHA)
	if deltaErr != nil && !errors.Is(deltaErr, planindex.ErrQuarryUnavailable) {
		return nil, deltaErr
	}

	// Binding runs after the done-checks above, so a card that already failed create-not-done is
	// never bound, and applies its whole batch of substitutions in this one RewriteRefs call.
	bindFindings, bindErr := delta.BindHandles(in.Plan, in.Geom.PlanDir, in.Cards)
	// BindHandles' plan-wide RewriteRefs lands on disk BEFORE it reports either a finding or an
	// error — the substitutions come from delta.Created while a bind-count-mismatch comes from a
	// card whose handle matched nothing, so one call routinely does both — which is why the
	// staleness re-baseline runs HERE rather than once past every refusal below. See this package's
	// doc.go for what restamping past the refusals cost.
	// A restamp failure never masks bindErr: the caller is already returning for that reason.
	if rebaseErr := restampFingerprint(in.State, in.Geom.PlanDir, in.Geom.WebsterDir); rebaseErr != nil && bindErr == nil {
		return nil, rebaseErr
	}
	if bindErr != nil {
		return nil, bindErr
	}
	var bindBlocking []string
	for _, f := range bindFindings {
		bindBlocking = append(bindBlocking, f.Error())
	}
	if len(bindBlocking) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrCardNotDone, strings.Join(bindBlocking, "; "))
	}

	// The informational glyph scope guard: it stays informational and never blocks, and degrades
	// explicitly on the same deltaErr above rather than running against a zero-value delta that
	// would otherwise look like a real, empty one — an unavailable diff costs visibility, not
	// correctness, and the done-checks above have already blocked on the same infrastructure error.
	if deltaErr != nil {
		warnings = append(warnings, fmt.Sprintf("glyph scope guard could not run for batch %s: %v", in.Label, deltaErr))
	} else {
		for _, f := range delta.ScopeGuard(in.Cards) {
			warnings = append(warnings, f.Error())
		}
	}

	// Drift detection runs after binding and before the digest is persisted, on the delta's own
	// deleted-symbols-still-referenced signal. HeadSHA is threaded through as the triggering SHA
	// every exact-tier repair's own amendment records.
	// Drift runs against the REMAINING plan, which is DetectDrift's own stated signal: the delta's
	// deleted symbols intersected with what the plan still has to do. Every already-built card is
	// excluded, and so is THIS batch's own — its work is exactly what the delta reports, so without
	// the exclusion a Delete card's own successful deletion came back as
	// plan-references-deleted-symbol against the very card that asked for it, and no Delete card
	// could ever be recorded.
	// in.Plan rides along as DetectDrift's fullPlan so gate one can recognize THIS batch's own
	// declared Rename outcome — the pending view excludes exactly the cards whose renames the
	// delta reports.
	driftFindings, driftErr := delta.DetectDrift(in.Plan, in.Completed, in.Geom.PlanDir, in.Geom.WorktreeRoot, in.HeadSHA, time.Now().UTC().Format(time.RFC3339))
	// The exact-tier repair's own RewriteRefs lands on disk before this call reports anything, and
	// its blocking plan-references-deleted-symbol finding is computed from a different part of the
	// same delta, so re-baseline here for exactly the reason BindHandles does above.
	if rebaseErr := restampFingerprint(in.State, in.Geom.PlanDir, in.Geom.WebsterDir); rebaseErr != nil && driftErr == nil {
		return warnings, rebaseErr
	}
	if driftErr != nil {
		return warnings, driftErr
	}
	// DetectDrift is the one index call in this sequence that returns a MIXED severity set:
	// plan-references-deleted-symbol is blocking, while the evidence tier's rename-candidate is
	// informational by construction — drift.go's own contract is that the rename-versus-genuine-delete
	// decision is the reviewer's, never the pipeline's. Failing the batch on it would destroy the very
	// tier it belongs to, since a finding that kills the batch never reaches a reviewer at all.
	// So the split here is by whose card the finding concerns: a blocking finding is about a later card, so it is recorded as a "later card:" warning rather than failing this batch,
	// and an informational one rides out on warnings exactly as ScopeGuard's findings already do.
	for _, f := range driftFindings {
		if f.Severity != planindex.SeverityBlocking {
			warnings = append(warnings, f.Error())
			continue
		}
		// Drift runs over the pending plan, which already excludes this batch's cards, so a blocking finding concerns a card still to be built:
		// it warns here, recorded once in the batch's state, and refuses at that card's own begin-batch.
		detail := "later card: " + f.Error()
		if text, added := recordBatchWarning(in.State, in.Batch, "later-card-drift:"+f.Error(), "later-card-drift", detail); added {
			warnings = append(warnings, text)
		}
	}

	// No third restamp: the two above already cover every rewrite this pass can perform, and each
	// runs immediately after its own rewriting call rather than past the refusals between them.
	// The amendment log the exact-tier repair also writes is deliberately excluded from the
	// fingerprint (see fingerprint's own doc comment), so it needs no re-baseline of its own.
	return warnings, nil
}
