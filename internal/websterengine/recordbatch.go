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
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/cardverify"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
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

// ErrCardNotDone is the sentinel RecordBatch returns when a done-check reports a blocking finding.
// Such a finding concerns the batch's own cards.
// It is a Create target that still does not resolve, or a Delete target that still does.
// RecordBatch turns it into a failed batch, with the findings as its reasons.
var ErrCardNotDone = errors.New("webster: record-batch's done-checks reported a blocking finding")

// ErrLaterTargetRemoved is the sentinel record-batch returns when a fork batch removes a symbol that a later card names.
// The batch is handed back to its fork once, while the fork still holds the batch's context.
var ErrLaterTargetRemoved = errors.New("webster: the batch removed a symbol a later card names")

// LaterTargetRemovedError is the refusal record-batch returns, once per bracket.
// It covers a fork batch that removes a symbol a later card names without reporting a rename.
// The batch stays non-terminal with its report in place.
// The next record-batch call records the batch instead of refusing again.
type LaterTargetRemovedError struct {
	// Batch names the batch as NN-<slug>.
	Batch string
	// Number is the batch's number, which Error formats as two digits.
	Number int
	// Reasons holds one line per later card and removed symbol.
	Reasons []string
}

// Error names each removed symbol and the later card that names it.
// It also gives the way forward through the batch's fork.
func (e *LaterTargetRemovedError) Error() string {
	return fmt.Sprintf("%s: batch %s: %s; way forward: send this message to the batch's fork, which either restores each name and commits, or lists each rename under its report's renames, and then rewrites its report with its head; then run `lym webster record-batch %02d` again, which records the batch without handing it back a second time",
		ErrLaterTargetRemoved, e.Batch, strings.Join(e.Reasons, "; "), e.Number)
}

// Unwrap returns the later-target-removed sentinel, so errors.Is matches it.
func (e *LaterTargetRemovedError) Unwrap() error { return ErrLaterTargetRemoved }

// cardNotDoneError is the error the post-batch done-checks return, and it unwraps to the card-not-done sentinel.
// Its message carries the findings' text.
// The flags record which kinds of finding it holds.
// deleteNotDone is set for a Delete target that still resolves.
// glyphRejected is set when quarry rejects a card's target before resolving it.
// createNotDone is set for a Create target that still does not resolve, or for a Rename pair that did not happen.
// signatureMismatch is set when a target's member is declared only with another parameter list.
type cardNotDoneError struct {
	reasons           []string
	deleteNotDone     bool
	glyphRejected     bool
	createNotDone     bool
	signatureMismatch bool
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

// laterDeleteReferenceReasons returns one reason per blocking reference from an unbegun card.
// The referenced symbol is one the cards in own delete.
// A method matched by name alone is informational and gives no reason.
// Its receiver is not resolved, and the later card's verify build catches a real call.
// laterDeleteReferenceReasons returns the blocking later-delete findings as reasons.
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
		if f.Severity == planindex.SeverityBlocking {
			reasons = append(reasons, f.Error())
		}
	}
	return reasons, nil
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
	var notDone *cardNotDoneError
	if errors.As(cause, &notDone) {
		reasons = notDone.reasons
		deleteNotDone = notDone.deleteNotDone
		glyphRejected = notDone.glyphRejected
		createNotDone = notDone.createNotDone
		signatureMismatch = notDone.signatureMismatch
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
		referenced, err := laterDeleteReferenceReasons(in.Plan, in.Batches, in.State, in.Cards, in.Geom)
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
		number, slug := batchIdentity(batch)
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
	number, slug := batchIdentity(batch)

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
		digest := distill(report)
		digest.Batch = polledID
		if err := recordWaiting(bs, &digest, deps.Geom.ReportsDir, number, slug, time.Now); err != nil {
			return nil, err
		}
		return &RecordResult{Digest: &digest, Warnings: warnings}, nil
	}

	deviations, postWarnings, err := postBatchChecks(postBatchInputs{
		Plan:      deps.Plan,
		State:     deps.State,
		Batch:     bs,
		Geom:      deps.Geom,
		Number:    batchNumber,
		Cards:     batch.Cards,
		Completed: completedCards(deps.Batches, deps.State, batchNumber),
		StartSHA:  bs.StartSHA,
		HeadSHA:   report.HeadSHA,
		Label:     polledID,
		Renames:   report.Renames,
		// Only an OK report's fork is waiting to be told: a FAILED one goes to a recovery strand instead.
		HandBackRemovedTargets: report.Status == ReportStatusOK,
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

	departures, err := batchDepartures(deps.Geom, bs.StartSHA, report.HeadSHA)
	if err != nil {
		return nil, err
	}

	digest := distill(report)
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
	number, slug := batchIdentity(batch)

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

// postBatchInputs carries everything the shared post-batch mechanical pass needs.
// Cards are the completed batch's own cards;
// Completed names every card whose work landed before this batch, so drift detection scopes to the remaining work;
// StartSHA is the bracket record's captured start SHA and HeadSHA the reconciled report head;
// Label names the batch in warnings;
// Batch is the record being finished, which later-card drift warnings are recorded onto;
// Renames are the glyph renames the batch's report lists, which drift detection repairs in the cards not yet begun;
// HandBackRemovedTargets asks the pass to refuse with *LaterTargetRemovedError.
// The refusal fires when the batch removed a symbol a later card names;
// it comes before any plan rewrite past the re-sign rebind.
type postBatchInputs struct {
	Plan                   *planparser.Plan
	State                  *State
	Batch                  *BatchState
	Geom                   Geometry
	Number                 int
	Cards                  []planparser.Card
	Completed              []planparser.Card
	StartSHA               string
	HeadSHA                string
	Label                  string
	Renames                []ReportRename
	HandBackRemovedTargets bool
}

// Bind reasons that postBatchChecks decides itself.
const (
	// bindCardNotDone marks a handle of a card the done-checks reported findings for.
	bindCardNotDone planindex.BindReason = "card-not-done"
	// bindBatchNotDone marks a handle of a card whose batch-mate failed the done-checks, so the pass never bound it.
	bindBatchNotDone planindex.BindReason = "batch-not-done"
	// bindDeltaUnavailable marks a handle the pass could not bind because the batch's delta was unavailable.
	bindDeltaUnavailable planindex.BindReason = "delta-unavailable"
	// bindLanguageNone marks a handle of a plan whose language opts out of glyphs.
	bindLanguageNone planindex.BindReason = "language-none"
)

// failedBindOutcomes returns one failed outcome for each handle the cards own, in card and body order.
// reasonFor picks the reason each card's handles fail with.
func failedBindOutcomes(cards []planparser.Card, reasonFor func(planparser.Card) planindex.BindReason) []planindex.BindOutcome {
	var outcomes []planindex.BindOutcome
	for _, c := range cards {
		for _, handle := range planparser.OwnedHandles(c) {
			outcomes = append(outcomes, planindex.BindOutcome{Card: c.Number, Handle: handle, Reason: reasonFor(c)})
		}
	}
	return outcomes
}

// logBindOutcomes writes one Info line per handle outcome, so the bind rate is countable by grep.
func logBindOutcomes(batch int, outcomes []planindex.BindOutcome) {
	for _, o := range outcomes {
		if o.Reason == "" {
			logger.Info("webster: handle bind", "batch", batch, "card", o.Card, "handle", o.Handle, "outcome", "bound", "glyph", o.Glyph)
			continue
		}
		logger.Info("webster: handle bind", "batch", batch, "card", o.Card, "handle", o.Handle, "outcome", "failed", "reason", string(o.Reason))
	}
}

// cardNotDoneFor returns the ErrCardNotDone-wrapped error that findings make against the batch's own cards.
// It returns nil when findings is empty.
// It logs every handle of the batch as failed,
// so the bind trace names the card that was not done and the batch-mates held back with it.
func cardNotDoneFor(in postBatchInputs, findings []planindex.Finding) *cardNotDoneError {
	if len(findings) == 0 {
		return nil
	}
	notDone := &cardNotDoneError{}
	notDoneCards := map[string]bool{}
	for _, f := range findings {
		notDone.reasons = append(notDone.reasons, f.Error())
		notDoneCards[f.Card] = true
		switch f.Check {
		case "delete-not-done":
			notDone.deleteNotDone = true
		case "glyph-rejected":
			notDone.glyphRejected = true
		case "create-not-done", "rename-not-done":
			notDone.createNotDone = true
		case "signature-mismatch":
			notDone.signatureMismatch = true
		}
	}
	logBindOutcomes(in.Number, failedBindOutcomes(in.Cards, func(c planparser.Card) planindex.BindReason {
		if notDoneCards[c.ID()] {
			return bindCardNotDone
		}
		return bindBatchNotDone
	}))
	return notDone
}

// rereadCards returns cards as plan holds them now, matched by card ID; a card plan no longer holds stays as given.
func rereadCards(plan *planparser.Plan, cards []planparser.Card) []planparser.Card {
	reread := make([]planparser.Card, len(cards))
	for i, c := range cards {
		reread[i] = c
		for _, current := range plan.Cards {
			if current.ID() == c.ID() {
				reread[i] = current
				break
			}
		}
	}
	return reread
}

// postBatchChecks runs the mechanical pass every terminal batch owes, whichever verb takes it terminal.
// It runs the card done-checks and carries a recorded C# re-sign into the plan.
// It computes the single delta that the rest of the pass shares.
// It then binds handles, runs the glyph scope guard, and runs drift detection with its exact-tier auto-repair.
// It also runs the plan-staleness re-baseline each of those rewrites requires.
//
// The scope guard's findings come back as deviations.
// They are the deduplicated, sorted files the batch changed outside its targets.
// They are informational and never join the warnings.
// When the delta cannot be computed, there are no deviations, and the could-not-run warning is returned instead.
//
// It is one function rather than two because a batch that reached done through recover-batch is
// exactly as done as one that reached it through record-batch:
// its Create targets have to resolve, its plan: handles have to bind,
// and the drift its work caused has to be repaired or reported.
// Skipping this pass for a recovered batch would leave its plan: handles unbound.
// Every later card would then reference an unbound handle.
// Drift detection would not see those references, since its reference index keys on the ref as the card spells it.
//
// Findings about the batch's own cards (done-checks, blocking bind findings) are returned as an
// ErrCardNotDone-wrapped error.
// Drift findings concern later cards, so they become `later card:` warnings recorded once on in.Batch.
// Informational findings ride out on warnings too.
// Warnings come back alongside any error, so a caller never loses them.
func postBatchChecks(in postBatchInputs) (deviations, warnings []string, err error) {
	// A card's completion has a mechanical verdict.
	// A Create target that still does not resolve, or a Delete target that still does, blocks the card.
	// This runs its own batched Resolve against the post-card tree, distinct from the single delta call below.
	index, err := in.Geom.index()
	if err != nil {
		return nil, nil, err
	}
	doneFindings, err := index.DoneChecks(in.Plan, in.Cards, in.Geom.WorktreeRoot)
	if err != nil {
		return nil, nil, err
	}
	if notDone := cardNotDoneFor(in, doneFindings); notDone != nil {
		return nil, nil, notDone
	}

	// A recorded C# re-sign is carried into the plan once its edit is in the tree:
	// every later card naming the member by its old glyph then names the minted id.
	// This rewrite runs before the delta, so everything after it reads the re-read plan,
	// and the plan is re-baselined before the guard and drift detection run.
	fingerprintBefore, err := fingerprint(in.Geom.PlanDir)
	if err != nil {
		return nil, nil, err
	}
	resignFindings, err := index.RebindResigns(in.Plan, in.Cards, in.Geom.WorktreeRoot)
	if err != nil {
		return nil, nil, err
	}
	if notDone := cardNotDoneFor(in, resignFindings); notDone != nil {
		return nil, nil, notDone
	}
	fingerprintAfter, err := fingerprint(in.Geom.PlanDir)
	if err != nil {
		return nil, nil, err
	}
	recordedCards := in.Cards
	if fingerprintAfter != fingerprintBefore {
		if err := restampFingerprint(in.State, in.Geom.PlanDir, in.Geom.WebsterDir); err != nil {
			return nil, nil, err
		}
		reparsed, err := in.Plan.Reparse()
		if err != nil {
			return nil, nil, fmt.Errorf("re-parse plan %s after re-sign rebinding: %w", in.Geom.PlanDir, err)
		}
		in.Plan = reparsed
		in.Cards = rereadCards(reparsed, in.Cards)
	}

	// One planindex.Delta serves the whole batch: BindHandles, ScopeGuard and DetectDrift all consume it.
	// HeadSHA is the reconciled report head, which the caller has already cross-checked against the worktree's real HEAD.
	// That cross-check is why the delta can be trusted here and nowhere earlier.
	// A DeltaGit infrastructure error does not abort the sequence.
	// The scope guard degrades to an informational notice on this same deltaErr.
	// The done-checks above ran on their own Resolve and are unaffected.
	// delta is the zero value on error, so BindHandles cannot confirm any handle bound.
	// It reports bind-count-mismatch for every card that declared one.
	// An unconfirmed Create is exactly a not-done card.
	// The floor error of a git below lym's minimum is not a quarry degrade,
	// so it stops the batch here like any other non-quarry error.
	delta, deltaErr := index.Delta(in.Geom.WorktreeRoot, in.StartSHA, in.HeadSHA)
	if deltaErr != nil && !errors.Is(deltaErr, planindex.ErrQuarryUnavailable) {
		return nil, nil, deltaErr
	}

	// The report's renames are what drift detection repairs further down,
	// so a removal the fork reported is no hand-back.
	reported := make([]planindex.Rename, len(in.Renames))
	for i, r := range in.Renames {
		reported[i] = planindex.Rename{Old: r.Old, New: r.New}
	}

	// A removed later target is handed back before binding,
	// so the plan carries no bind or drift rewrite of a batch its fork is still to change.
	if in.HandBackRemovedTargets && deltaErr == nil && !in.Batch.LaterTargetsReturned {
		if removed := delta.RemovedTargets(in.Plan, in.Completed, in.Cards, reported); len(removed) > 0 {
			reasons := make([]string, len(removed))
			for i, f := range removed {
				reasons[i] = f.Error()
			}
			in.Batch.LaterTargetsReturned = true
			return nil, nil, &LaterTargetRemovedError{Batch: in.Label, Number: in.Number, Reasons: reasons}
		}
	}

	// Binding runs after the done-checks above, so a card that already failed create-not-done is never bound.
	// It applies its whole batch of substitutions in one RewriteRefs call.
	bindFindings, bindOutcomes, bindErr := delta.BindHandles(in.Plan, in.Geom.PlanDir, in.Cards)
	// The trace is logged before the restamp below, so a restamp failure still reports the bind that landed.
	// The trace names the reason the pass decided.
	// That reason overrides the outcomes BindHandles computed when there is no glyph language or no delta.
	if bindErr == nil {
		if !in.Plan.GlyphLanguage() {
			bindOutcomes = failedBindOutcomes(in.Cards, func(planparser.Card) planindex.BindReason { return bindLanguageNone })
		} else if deltaErr != nil {
			bindOutcomes = failedBindOutcomes(in.Cards, func(planparser.Card) planindex.BindReason { return bindDeltaUnavailable })
		}
		logBindOutcomes(in.Number, bindOutcomes)
	}
	// BindHandles rewrites plan references on disk before it reports a finding or an error.
	// The substitutions come from delta.Created;
	// a bind-count mismatch comes from a card whose handle matched nothing.
	// One call can therefore do both.
	// The staleness re-baseline runs here, not once after every refusal below.
	// See this package's doc.go for what restamping past the refusals costs.
	// A restamp failure never masks bindErr: the caller is already returning for that reason.
	if rebaseErr := restampFingerprint(in.State, in.Geom.PlanDir, in.Geom.WebsterDir); rebaseErr != nil && bindErr == nil {
		return nil, nil, rebaseErr
	}
	if bindErr != nil {
		return nil, nil, bindErr
	}
	var bindBlocking []string
	for _, f := range bindFindings {
		bindBlocking = append(bindBlocking, f.Error())
	}
	if len(bindBlocking) > 0 {
		return nil, nil, fmt.Errorf("%w: %s", ErrCardNotDone, strings.Join(bindBlocking, "; "))
	}

	// The informational glyph scope guard never blocks.
	// It degrades explicitly on the deltaErr above rather than running against a zero-value delta;
	// such a delta would look like a real, empty one.
	// An unavailable diff costs visibility, not correctness,
	// and the done-checks above have already blocked on the same infrastructure error.
	if deltaErr != nil {
		warnings = append(warnings, fmt.Sprintf("glyph scope guard could not run for batch %s: %v", in.Label, deltaErr))
	} else {
		deviationSet := map[string]bool{}
		// The guard's targets are the cards as recorded and as re-read.
		// A re-signed member's old glyph and its minted id both count as the batch's own.
		for _, f := range delta.ScopeGuard(in.Plan, append(slices.Clone(recordedCards), in.Cards...)) {
			if f.File != "" && !deviationSet[f.File] {
				deviationSet[f.File] = true
				deviations = append(deviations, f.File)
			}
		}
		sort.Strings(deviations)
	}

	// Drift detection runs after binding and before the digest is persisted, on the delta's own
	// deleted-symbols-still-referenced signal.
	// HeadSHA is the triggering SHA that every exact-tier repair's own amendment records.
	// Drift runs against the remaining plan:
	// the delta's deleted symbols intersected with what the plan still has to do.
	// Every already-built card is excluded, and so is this batch's own.
	// Without that exclusion, a Delete card's own deletion is reported against the very card that asked for it,
	// and no Delete card could ever be recorded.
	// in.Plan rides along as DetectDrift's fullPlan so gate one can recognize this batch's own declared Rename outcome:
	// the pending view excludes exactly the cards whose renames the delta reports.
	// The report's renames ride along,
	// so a later card naming a glyph the batch renamed is rewritten even where quarry cannot prove the rename.
	driftFindings, driftErr := delta.DetectDrift(in.Plan, in.Completed, reported, in.Geom.PlanDir, in.Geom.WorktreeRoot, in.HeadSHA, time.Now().UTC().Format(time.RFC3339))
	// The exact-tier repair's RewriteRefs writes to disk before this call reports anything.
	// Its blocking plan-references-deleted-symbol finding comes from another part of the same delta,
	// so the re-baseline runs here, for the reason the bind step above gives.
	if rebaseErr := restampFingerprint(in.State, in.Geom.PlanDir, in.Geom.WebsterDir); rebaseErr != nil && driftErr == nil {
		return nil, warnings, rebaseErr
	}
	if driftErr != nil {
		return nil, warnings, driftErr
	}
	// DetectDrift is the one index call in this sequence that returns a mixed severity set:
	// its plan-references-deleted-symbol finding is blocking;
	// the evidence tier's rename-candidate is informational by construction.
	// drift.go's contract makes the rename-versus-genuine-delete decision the reviewer's, never the pipeline's.
	// Failing the batch on it would destroy the very tier it belongs to.
	// A finding that kills the batch never reaches a reviewer at all.
	// So the split is by whose card the finding concerns.
	// A blocking finding concerns a later card;
	// it is recorded as a "later card:" warning rather than failing this batch,
	// and an informational one rides out on warnings.
	for _, f := range driftFindings {
		if f.Severity != planindex.SeverityBlocking {
			warnings = append(warnings, f.Error())
			continue
		}
		// Drift runs over the pending plan, which already excludes this batch's cards.
		// So a blocking finding concerns a card still to be built.
		// It warns here, recorded once in the batch's state, and refuses at that card's own begin-batch.
		detail := "later card: " + f.Error()
		if text, added := recordBatchWarning(in.State, in.Batch, "later-card-drift:"+f.Error(), "later-card-drift", detail); added {
			warnings = append(warnings, text)
		}
	}

	// No third restamp is needed.
	// The two above cover every rewrite this pass performs,
	// and each runs right after its own rewriting call rather than past the refusals between them.
	// The amendment log the exact-tier repair writes is excluded from the fingerprint,
	// so it needs no re-baseline.
	return deviations, warnings, nil
}
