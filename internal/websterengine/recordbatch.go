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
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrNoBeginRecord is the sentinel RecordBatch returns when deps.State.Batches[batchNumber] is
// absent or already Terminal — a record call with no matching (or already-consumed) begin-batch
// record.
// This is the bracket-discipline fail-loud check: a fork's own report, however legitimate it looks,
// is never trusted without Go's own record that begin-batch actually opened this batch first.
var ErrNoBeginRecord = errors.New("webster: record-batch called with no begin-batch record for this batch")

// ErrCardNotDone is the sentinel RecordBatch returns when card 33's DoneChecks report a blocking
// finding against the just-completed batch's own cards — a Create target that still does not
// resolve, or a Delete target that still does — meaning the batch is not done and no terminal
// digest is persisted. webster's own sentinel, per the webster-owns-its-own-domain-types decision.
var ErrCardNotDone = errors.New("webster: record-batch's done-checks reported a blocking finding")

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

	postWarnings, err := postBatchChecks(postBatchInputs{
		Plan:      deps.Plan,
		State:     deps.State,
		Geom:      deps.Geom,
		Cards:     batch.Cards,
		Completed: completedCards(deps.Batches, deps.State, batchNumber),
		StartSHA:  bs.StartSHA,
		HeadSHA:   actualHead,
		Label:     polledID,
	})
	warnings = append(warnings, postWarnings...)
	if err != nil {
		return nil, err
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

// postBatchInputs carries everything the shared post-batch mechanical pass needs.
// Cards are the completed batch's own cards; Completed names every card whose work landed BEFORE
// this batch, so drift detection can scope itself to the plan's remaining work; StartSHA is the
// bracket record's captured start SHA and HeadSHA the already-verified head; Label names the batch
// in warnings.
type postBatchInputs struct {
	Plan      *planparser.Plan
	State     *State
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
// Blocking findings are returned as an ErrCardNotDone-wrapped error; informational ones ride out on
// warnings, which are returned alongside any error so a caller never loses them.
func postBatchChecks(in postBatchInputs) (warnings []string, err error) {
	// A card's completion has a mechanical verdict: a Create target that still does not resolve,
	// or a Delete target that still does, blocks — neither is a judgment call. This runs its own
	// batched Resolve against the post-card tree, distinct from the single delta call below.
	doneFindings, err := planglyph.DoneChecks(in.Plan, in.Cards, in.Geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}
	var doneChecks []string
	for _, f := range doneFindings {
		doneChecks = append(doneChecks, f.Error())
	}
	if len(doneChecks) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrCardNotDone, strings.Join(doneChecks, "; "))
	}

	// The batch's single delta call: BindHandles, ScopeGuard and DetectDrift all consume this one
	// quarry.GitDeltaAnswer rather than each spawning their own. HeadSHA is already cross-checked
	// against the worktree's real HEAD by the caller — that cross-check is why the delta can be
	// trusted here and nowhere earlier.
	// A DeltaGit infrastructure error does not abort the sequence: the scope guard degrades to an
	// informational notice on this same deltaErr, while the done-checks above ran on their own
	// Resolve and are unaffected. delta itself is the zero value on error, so BindHandles correctly
	// cannot confirm any handle bound and reports bind-count-mismatch for every card that declared
	// one — an unconfirmed Create is exactly a not-done card.
	delta, deltaErr := planglyph.Delta(in.Geom.WorktreeRoot, in.StartSHA, in.HeadSHA)
	if deltaErr != nil && !errors.Is(deltaErr, planglyph.ErrQuarryUnavailable) {
		return nil, deltaErr
	}

	// Binding runs after the done-checks above, so a card that already failed create-not-done is
	// never bound, and applies its whole batch of substitutions in this one RewriteRefs call.
	bindFindings, bindErr := planglyph.BindHandles(in.Plan, in.Geom.PlanDir, delta, in.Cards)
	// BindHandles' plan-wide RewriteRefs lands on disk BEFORE it reports either a finding or an
	// error — the substitutions come from delta.Created while a bind-count-mismatch comes from a
	// card whose handle matched nothing, so one call routinely does both — which is why the
	// staleness re-baseline runs HERE rather than once past every refusal below. See this package's
	// doc.go for what restamping past the refusals cost.
	// A restamp failure never masks bindErr: the caller is already returning for that reason.
	if rebaseErr := restampFingerprint(in.State, in.Geom.PlanDir); rebaseErr != nil && bindErr == nil {
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
		for _, f := range planglyph.ScopeGuard(in.Cards, delta) {
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
	pending := planglyph.PendingPlan(in.Plan, in.Completed)
	driftFindings, driftErr := planglyph.DetectDrift(in.Plan, pending, in.Geom.PlanDir, in.Geom.WorktreeRoot, delta, in.HeadSHA, time.Now().UTC().Format(time.RFC3339))
	// The exact-tier repair's own RewriteRefs lands on disk before this call reports anything, and
	// its blocking plan-references-deleted-symbol finding is computed from a different part of the
	// same delta, so re-baseline here for exactly the reason BindHandles does above.
	if rebaseErr := restampFingerprint(in.State, in.Geom.PlanDir); rebaseErr != nil && driftErr == nil {
		return warnings, rebaseErr
	}
	if driftErr != nil {
		return warnings, driftErr
	}
	// DetectDrift is the one planglyph call in this sequence that returns a MIXED severity set:
	// plan-references-deleted-symbol is blocking, while the evidence tier's rename-candidate is
	// informational by construction — drift.go's own contract is that the rename-versus-genuine-delete
	// decision is the reviewer's, never the pipeline's. Failing the batch on it would destroy the very
	// tier it belongs to, since a finding that kills the batch never reaches a reviewer at all.
	// So the split here mirrors BeginBatch's own: blocking fails, informational rides out on warnings
	// exactly as ScopeGuard's findings already do.
	var driftBlocking []string
	for _, f := range driftFindings {
		if f.Severity == planglyph.SeverityBlocking {
			driftBlocking = append(driftBlocking, f.Error())
			continue
		}
		warnings = append(warnings, f.Error())
	}
	if len(driftBlocking) > 0 {
		return warnings, fmt.Errorf("%w: %s", ErrCardNotDone, strings.Join(driftBlocking, "; "))
	}

	// No third restamp: the two above already cover every rewrite this pass can perform, and each
	// runs immediately after its own rewriting call rather than past the refusals between them.
	// The amendment log the exact-tier repair also writes is deliberately excluded from the
	// fingerprint (see fingerprint's own doc comment), so it needs no re-baseline of its own.
	return warnings, nil
}
