// runlevel.go implements Run, webster's `run` verb engine core: the
// run-level exclusive lease, the automatic validation gate (including the
// zero-batch pre-flight refusal), the state-phase entry-time reclaim of
// webster's own two reclaimable substrates (Master's own strand and any
// non-terminal recovery-batch strand — forks die with Master, so there is
// never a third), the plan-fingerprint crash/resume guard with its --fresh
// archive/re-init escape, the never-instantly-re-pause clear, the
// stale-outcome/summary archive, the always-fresh Master spawn
// (fork-authorized, both output files), the shuttle-outcome-to-RunResult
// mapping, and the run-exit whole-session audit cross-check that backstops
// record-batch's own per-batch incremental audit. Named runlevel.go, not
// run.go, mirroring builderengine's own runlevel.go naming note (avoiding a
// clash with any future poll/spawn-style file name).

package websterengine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// runLockName is the exclusive-lease file name inside the webster dir, held
// for the ENTIRE duration of one Run call — builderengine's own run.lock
// discipline (runlevel.go) applied to webster's own dir: without it, two
// concurrent `lyx webster run` invocations would each cold-start from the
// same state.json and reports, then both drive the Master spawn at once.
const runLockName = "run.lock"

// ErrRunBusy marks Run's fail-fast refusal when another invocation already
// holds websterDir's run.lock. It is webster's own sentinel (independent of
// builderengine.ErrRunBusy, per the webster-owns-its-own-domain-types
// decision) because the caller must treat this refusal differently from
// every other hard error: the losing call touched NOTHING on disk — the
// winner is mid-run and owns the state — so webstercli must not run its own
// exit-time weft backstop for it.
var ErrRunBusy = errors.New("webster: run is already in progress")

// OutcomeFileName is outcome.yaml's fixed filename inside a webster dir —
// webster's own copy of builderengine's own unexported outcomeFileName
// constant, kept identical so a webster run's outcome.yaml reads exactly
// like builder's, per the A/B contract-compatibility decision.
const OutcomeFileName = "outcome.yaml"

// OutcomePath returns the path to outcome.yaml inside websterDir.
func OutcomePath(websterDir string) string {
	return filepath.Join(websterDir, OutcomeFileName)
}

// MasterHandle is the started-but-not-yet-finished Master spawn Run blocks
// on: StrandGUID identifies the mux strand Master runs in (available
// immediately after the start, so Run can persist it to state.json BEFORE
// blocking — the record the next run's entry-time reclaim reads), and Wait
// blocks until the spawn reaches a terminal shuttle outcome.
// *shuttleengine.Run satisfies this structurally.
type MasterHandle interface {
	StrandGUID() string
	Wait() (shuttleengine.Result, error)
}

// MasterStarter is the seam Run spawns Master through: builderengine's own
// OrchestratorStarter shape (runlevel.go), webster-named. Production code
// passes an adapter over *shuttleengine.Runner (webstercli's own starter);
// tests pass a local fake.
type MasterStarter interface {
	StartMaster(shuttleengine.Spec) (MasterHandle, error)
}

// RunDeps carries every seam Run needs, so a test can fake each one
// independently: Starter spawns Master and hands back the handle Run blocks
// on; Mux is the live mux query surface the entry-time reclaim consults via
// StrandLive/RemoveStrand; Engine and ShuttleCfg/Layout are what
// shuttleengine.FindRun (Master session-identity resolution) and the
// weft-reference audit pattern need; PlanDir, WebsterDir, ReportsDir, and
// PromptsDir are the hubgeometry-resolved _lyx/plan, _lyx/webster,
// _lyx/webster/reports, and _lyx/webster/prompts directories; WorktreeRoot
// is the host repo checkout Validate's context estimate resolves
// Scope/Where entries against; Config is the loaded webster.yaml; Roles is
// the pre-flight-resolved role->model-spec map (see ResolveRoles).
type RunDeps struct {
	Starter      MasterStarter
	Mux          shuttleengine.MuxOps
	Engine       shuttleengine.Engine
	ShuttleCfg   shuttleengine.Config
	Layout       *hubgeometry.Layout
	Roles        map[Role]modelspec.Resolved
	Config       Config
	PlanDir      string
	WebsterDir   string
	ReportsDir   string
	PromptsDir   string
	WorktreeRoot string
}

// RunOptions carries one `run` invocation's caller-supplied choices. Fresh
// requests the fingerprint-mismatch escape: archive the stale state.json and
// reports dir, clear the re-renderable prompts dir, and re-init, rather than
// refusing with ErrFingerprintMismatch.
type RunOptions struct {
	Fresh bool
}

// RunResult is what one successful Run call hands back to its caller
// (internal/webstercli's `run` verb): the parsed outcome.yaml's own judgment
// (Outcome/StuckReason/BatchesDone) plus, once a valid summary.md has been
// read, its title — the future loom-finalize PR-text source's headline.
type RunResult struct {
	// Outcome is one of builderengine.OutcomeDone, OutcomeStuck, or
	// OutcomePaused, taken verbatim from the parsed outcome.yaml.
	Outcome string
	// StuckReason is the parsed outcome.yaml's stuck_reason, verbatim.
	StuckReason string
	// BatchesDone is the parsed outcome.yaml's batches_done, verbatim.
	BatchesDone int
	// SummaryTitle is the parsed summary.md's title heading. Always
	// populated for Outcome == builderengine.OutcomeDone (ParseSummary is
	// required there — a missing or malformed summary is a hard error);
	// populated best-effort for stuck/paused (empty when summary.md is
	// itself missing or malformed, which is not an error on those two
	// outcomes).
	SummaryTitle string
}

// newRunGUID returns a 128-bit random identifier, hex-encoded, generated
// from crypto/rand — webster's own copy of builderengine's own unexported
// newRunGUID (runlevel.go): minted once at first init, never regenerated
// across a resume.
func newRunGUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("webster: mint run guid: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// MasterAskingError marks Run's mapping of a shuttle OutcomeAsking result
// for Master's own spawn: Master ended its turn asking a question instead
// of ever reaching its own outcome-file final action. Unwrap returns
// ErrMasterAsking so a caller can classify via errors.Is without needing the
// concrete type; the concrete type itself carries the per-call SessionID,
// RunDir, and LastAssistantMessage a caller needs to log or resume from.
type MasterAskingError struct {
	SessionID string
	RunDir    string
	Message   string
}

func (e *MasterAskingError) Error() string {
	return fmt.Sprintf("webster: master asked a question instead of finishing (session %s, kept run dir %s): %s", e.SessionID, e.RunDir, e.Message)
}

// Unwrap lets a caller match this error via errors.Is(err, ErrMasterAsking).
func (e *MasterAskingError) Unwrap() error { return ErrMasterAsking }

// ErrMasterAsking is the sentinel MasterAskingError wraps.
var ErrMasterAsking = errors.New("webster: master asking")

// MasterDiedError marks Run's mapping of a shuttle OutcomeDied result for
// Master's own spawn: its pane died (or it never became ready) before it
// ever reached its own outcome-file final action.
type MasterDiedError struct {
	SessionID string
	RunDir    string
}

func (e *MasterDiedError) Error() string {
	return fmt.Sprintf("webster: master pane died (session %s, kept run dir %s)", e.SessionID, e.RunDir)
}

// Unwrap lets a caller match this error via errors.Is(err, ErrMasterDied).
func (e *MasterDiedError) Unwrap() error { return ErrMasterDied }

// ErrMasterDied is the sentinel MasterDiedError wraps.
var ErrMasterDied = errors.New("webster: master died")

// MasterTimeoutError marks Run's mapping of a shuttle OutcomeTimeout result
// for Master's own spawn: its wall-clock Timeout (MasterTimeoutMin, the
// whole-run analog of builder's orchestrator_timeout_min) elapsed before it
// ever reached its own outcome-file final action.
type MasterTimeoutError struct {
	SessionID string
	RunDir    string
}

func (e *MasterTimeoutError) Error() string {
	return fmt.Sprintf("webster: master run timed out (session %s, kept run dir %s)", e.SessionID, e.RunDir)
}

// Unwrap lets a caller match this error via errors.Is(err, ErrMasterTimeout).
func (e *MasterTimeoutError) Unwrap() error { return ErrMasterTimeout }

// ErrMasterTimeout is the sentinel MasterTimeoutError wraps.
var ErrMasterTimeout = errors.New("webster: master timed out")

// clearRenderedPrompts removes every fork prompt file previously written
// into promptsDir, part of Run's --fresh escape: these are re-renderable
// artifacts (BeginBatch rewrites each batch's own the next time it begins),
// never archived, unlike the fingerprint-mismatch escape's state.json/
// reports treatment — deleting rather than preserving a purely derived,
// cheaply reproduced artifact is the correct posture. Absent dir: a no-op.
func clearRenderedPrompts(promptsDir string) error {
	if err := os.RemoveAll(promptsDir); err != nil {
		return fmt.Errorf("webster: clear rendered prompts dir %s: %w", promptsDir, err)
	}
	return nil
}

// reclaimEntryTimeStrands stops the only two substrates a crashed or killed
// `run` process can ever leave live behind it: Master's own recorded strand
// and any recorded, non-terminal recovery-batch strand. Unlike builder,
// forks die WITH Master (same process) — there is never an orphaned
// in-flight fork implementer to reclaim, which is what makes webster's own
// entry-time reclaim strictly simpler than builder's own, per
// discussion.md's crash-resume-re-drive-first-unreported decision. A nil st
// (no run has ever started) is a no-op.
func reclaimEntryTimeStrands(mux shuttleengine.MuxOps, st *State) error {
	if st == nil {
		return nil
	}

	if st.MasterStrand != "" {
		if err := builderengine.RemoveStrandIfLive(mux, st.MasterStrand); err != nil {
			return err
		}
	}

	for _, bs := range st.Batches {
		if bs != nil && bs.Kind == "recovery" && !bs.Terminal {
			if err := builderengine.RemoveStrandIfLive(mux, bs.StrandGUID); err != nil {
				return err
			}
		}
	}

	return nil
}

// countBegunForkBatches counts st's recorded batches whose Kind is "fork"
// AND whose recorded SessionID matches sessionID — the run-exit audit
// cross-check's begun-batch baseline: every such batch was recorded because
// BeginBatch ran for it under THIS Master session, so its own in-session
// fork MUST be represented in the whole-session audit's transcript count,
// or a fork silently failed to survive audit. The session scoping exists
// because the whole-session audit only ever covers the current session's
// own subagents directory: a crash-resumed run's fresh Master never forked
// the batches a prior session completed, and counting those would fail
// every legitimately completed resume.
func countBegunForkBatches(st *State, sessionID string) int {
	if st == nil {
		return 0
	}
	count := 0
	for _, bs := range st.Batches {
		if bs != nil && bs.Kind == "fork" && bs.SessionID == sessionID {
			count++
		}
	}
	return count
}

// Run drives one `lyx webster run` invocation to completion: the run-level
// mutex, the automatic validation gate (including the zero-batch
// pre-flight refusal), the state-phase entry-time reclaim, the
// plan-fingerprint crash/resume guard (with its --fresh escape), the
// never-instantly-re-pause clear (run only after every refusal gate passes,
// so a refused run leaves a pending pause intact), the stale-outcome/
// summary archive, the always-fresh Master spawn, and the
// shuttle-outcome-to-RunResult mapping (see mapMasterDone). Every returned
// error is "webster: "-prefixed (via the helpers it calls); ErrRunBusy and
// ErrFingerprintMismatch are exported sentinels a caller matches via
// errors.Is, and a non-done shuttle outcome for Master's own spawn returns
// one of the three distinct *Master*Error types above rather than ever
// attempting to parse a (non-existent) outcome.yaml.
func Run(deps RunDeps, opts RunOptions) (RunResult, error) {
	if err := os.MkdirAll(deps.WebsterDir, 0o755); err != nil {
		return RunResult{}, fmt.Errorf("webster: create webster dir %s: %w", deps.WebsterDir, err)
	}

	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(deps.WebsterDir, runLockName))
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: acquire run lock in %s: %w", deps.WebsterDir, err)
	}
	if !locked {
		return RunResult{}, fmt.Errorf("%w: %q (run.lock held); wait for it to finish, or check `lyx webster status`", ErrRunBusy, deps.WebsterDir)
	}
	defer runLock.Release()

	plan, err := builderengine.ParsePlan(deps.PlanDir)
	if err != nil {
		return RunResult{}, err
	}

	// nothing-to-build is a malformed plan, never a vacuous outcome: done —
	// webster's own pre-flight ahead of builderengine.Validate, per
	// discussion.md's run-verb-shape decision.
	if len(plan.Batches) == 0 {
		return RunResult{}, fmt.Errorf("webster: plan %s has zero batches; nothing to build is a malformed plan, never a vacuous outcome: done", deps.PlanDir)
	}

	caps := builderengine.ValidateCaps{
		ContextCapTokens: deps.Config.BatchContextCapTokens,
		CardCap:          deps.Config.BatchCardCap,
	}
	if findings := builderengine.Validate(plan, deps.WorktreeRoot, caps); len(findings) > 0 {
		msgs := make([]string, len(findings))
		for i, f := range findings {
			msgs[i] = f.Error()
		}
		return RunResult{}, fmt.Errorf("webster: plan validation refused this run (%d finding(s)): %s", len(findings), strings.Join(msgs, "; "))
	}

	fingerprint, err := builderengine.Fingerprint(deps.PlanDir)
	if err != nil {
		return RunResult{}, err
	}

	// Serialize the whole state phase — load, entry-time reclaim, fresh
	// archive/re-init, and the post-start strand record — against every
	// other verb's own state read-modify-write, mirroring builderengine's
	// own AcquireStateMutation discipline. Released explicitly right after
	// the strand record lands, never held across Master's own wait.
	mutateLock, err := AcquireStateMutation(deps.WebsterDir)
	if err != nil {
		return RunResult{}, err
	}
	mutateHeld := true
	defer func() {
		if mutateHeld {
			_ = mutateLock.Release()
		}
	}()

	st, err := LoadState(deps.WebsterDir)
	if err != nil {
		return RunResult{}, err
	}

	// Entry-time reclaim BEFORE anything else acts on the loaded state
	// (including the --fresh archive below, which would discard the only
	// record of these strands): a prior run whose process died mid-wait
	// leaves a live Master pane (or a live recovery strand) that keeps
	// driving on its own.
	if err := reclaimEntryTimeStrands(deps.Mux, st); err != nil {
		return RunResult{}, err
	}

	switch {
	case st == nil:
		guid, err := newRunGUID()
		if err != nil {
			return RunResult{}, err
		}
		st = &State{
			RunGUID:         guid,
			PlanFingerprint: fingerprint,
			Batches:         map[int]*BatchState{},
			ChainStartSHAs:  map[int]string{},
		}
		if err := SaveState(deps.WebsterDir, st); err != nil {
			return RunResult{}, err
		}

	case st.PlanFingerprint != fingerprint:
		if !opts.Fresh {
			return RunResult{}, fmt.Errorf("%w: on-disk plan fingerprint %s does not match this run's recorded fingerprint %s; the plan changed since state.json was created — re-run with --fresh to archive the stale state and reports and start over", ErrFingerprintMismatch, fingerprint, st.PlanFingerprint)
		}

		if _, err := builderengine.ArchiveStateFile(deps.WebsterDir, time.Now); err != nil {
			return RunResult{}, err
		}
		if err := builderengine.ArchiveReportsDir(deps.ReportsDir, time.Now); err != nil {
			return RunResult{}, err
		}
		if err := clearRenderedPrompts(deps.PromptsDir); err != nil {
			return RunResult{}, err
		}

		guid, err := newRunGUID()
		if err != nil {
			return RunResult{}, err
		}
		st = &State{
			RunGUID:         guid,
			PlanFingerprint: fingerprint,
			Batches:         map[int]*BatchState{},
			ChainStartSHAs:  map[int]string{},
		}
		if err := SaveState(deps.WebsterDir, st); err != nil {
			return RunResult{}, err
		}
	}

	// Clear any leftover pause flag now that the run has passed every
	// refusal gate (validation, the plan-fingerprint check) and is
	// committed to spawning a fresh Master: a resumed run must not
	// instantly re-pause on the flag that requested the very pause it is
	// now resuming from. Placing the clear HERE — not at the bare entry —
	// means a run that refused above leaves the operator's pending pause
	// intact rather than silently discarding a request it never acted on.
	if err := builderengine.ClearPause(deps.WebsterDir); err != nil {
		return RunResult{}, err
	}

	if _, err := builderengine.ArchiveStaleOutcome(deps.WebsterDir, time.Now); err != nil {
		return RunResult{}, err
	}
	if _, err := ArchiveStaleSummary(deps.WebsterDir, time.Now); err != nil {
		return RunResult{}, err
	}

	outcomePath, err := filepath.Abs(OutcomePath(deps.WebsterDir))
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: resolve outcome path: %w", err)
	}
	summaryPath, err := filepath.Abs(SummaryPath(deps.WebsterDir))
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: resolve summary path: %w", err)
	}

	prompt, err := RenderMasterPrompt(plan, st, outcomePath, summaryPath, deps.Config.SelfFixCap, deps.Config.PollWaitS)
	if err != nil {
		return RunResult{}, err
	}

	resolved, ok := deps.Roles[RoleMaster]
	if !ok {
		return RunResult{}, fmt.Errorf("webster: no resolved model-spec for role %q", RoleMaster)
	}

	spec := shuttleengine.Spec{
		Prompt: string(prompt),
		// Both output files: shuttle classifies this run done only once
		// BOTH land, so a Master that writes outcome.yaml but never
		// reaches its summary.md final action never falsely reads as
		// finished.
		OutputFiles:   []string{outcomePath, summaryPath},
		Model:         resolved.Model,
		Effort:        resolved.Params["effort"],
		Version:       resolved.Params["version"],
		ForkSubagents: true,
		Role:          string(RoleMaster),
		Interactive:   false,
		Timeout:       time.Duration(deps.Config.MasterTimeoutMin) * time.Minute,
	}

	handle, err := deps.Starter.StartMaster(spec)
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: start master: %w", err)
	}

	// Record and persist Master's strand GUID the instant it exists — BEFORE
	// resolving the session ID via FindRun, which itself can fail. The
	// entry-time reclaim is the only thing that can ever stop a still-live
	// Master a dead run process left behind, and it keys off MasterStrand; if
	// FindRun failed after a live pane was already spawned but before the
	// strand was durable, that pane would be invisible to every future
	// reclaim. Two saves, not one, keep the reclaimable record ahead of the
	// fallible resolve.
	st.MasterStrand = handle.StrandGUID()
	if err := SaveState(deps.WebsterDir, st); err != nil {
		return RunResult{}, err
	}

	runState, _, err := shuttleengine.FindRun(deps.ShuttleCfg, deps.Layout, st.MasterStrand)
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: resolve spawned master run: %w", err)
	}
	st.MasterSessionID = runState.SessionID
	// The launch model IS the idempotent-assertion baseline BeginBatch's
	// own per-batch model check consults from batch 1 onward — a batch 1
	// begin-batch call for a non-oversized batch then finds AssertedModel
	// already equal to RoleMaster's model and injects nothing.
	st.AssertedModel = resolved.Model

	// Second save: the session ID and launch-model baseline the verbs read.
	if err := SaveState(deps.WebsterDir, st); err != nil {
		return RunResult{}, err
	}

	// The state phase is over; release the mutation lease before blocking
	// on Master — its own begin-batch/record-batch calls need it free.
	_ = mutateLock.Release()
	mutateHeld = false

	result, err := handle.Wait()
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: run master: %w", err)
	}

	switch result.Outcome {
	case shuttleengine.OutcomeDone:
		return mapMasterDone(deps, plan, outcomePath, summaryPath, result)

	case shuttleengine.OutcomeAsking:
		return RunResult{}, &MasterAskingError{SessionID: result.SessionID, RunDir: result.RunDir, Message: result.LastAssistantMessage}

	case shuttleengine.OutcomeDied:
		return RunResult{}, &MasterDiedError{SessionID: result.SessionID, RunDir: result.RunDir}

	case shuttleengine.OutcomeTimeout:
		return RunResult{}, &MasterTimeoutError{SessionID: result.SessionID, RunDir: result.RunDir}

	default:
		return RunResult{}, fmt.Errorf("webster: master run returned unrecognized shuttle outcome %q", result.Outcome)
	}
}

// mapMasterDone maps a shuttle-level OutcomeDone Master spawn (both contract
// files landed) onto RunResult: strict outcome.yaml parsing, the summary.md
// gate (required content-validity when outcome: done, best-effort
// otherwise), the every-batch-terminal-done cross-check and the run-exit
// whole-session audit cross-check (done outcomes only — the backstops
// behind record-batch's own per-batch incremental audit), and the
// pause-flag clear every non-paused terminal performs. The
// asking/died/timeout shuttle outcomes never reach this function — they are
// mapped directly in Run, before any attempt to parse a file Master never
// wrote.
func mapMasterDone(deps RunDeps, plan *builderengine.Plan, outcomePath, summaryPath string, result shuttleengine.Result) (RunResult, error) {
	outcome, err := builderengine.ParseOutcome(outcomePath)
	if err != nil {
		return RunResult{}, err
	}

	var summaryTitle string
	if outcome.Outcome == builderengine.OutcomeDone {
		// Required: a done run with a missing or malformed summary.md is a
		// hard error, never guessed — the artifact is the future
		// loom-finalize PR-text source.
		summary, err := ParseSummary(summaryPath)
		if err != nil {
			return RunResult{}, fmt.Errorf("webster: run reached outcome: done but summary.md is missing or malformed: %w", err)
		}
		summaryTitle = summary.Title

		// outcome: done is a whole-plan claim: every batch must carry a
		// persisted terminal done record. A Master that wrote done while a
		// batch was begun-but-never-recorded (a fork slipped past
		// record-batch) is caught here, closing the begin-without-record leg
		// of the two-layer bracket enforcement at run exit.
		if err := verifyEveryBatchDone(deps.WebsterDir, plan); err != nil {
			return RunResult{}, err
		}

		if err := runExitAuditCrossCheck(deps, outcomePath, summaryPath, result); err != nil {
			return RunResult{}, err
		}
	} else if summary, err := ParseSummary(summaryPath); err == nil {
		// summary.md's content is optional on stuck/paused: best-effort
		// only, never a hard error, per discussion.md's summary-artifact
		// decision.
		summaryTitle = summary.Title
	}

	if outcome.Outcome != builderengine.OutcomePaused {
		if err := builderengine.ClearPause(deps.WebsterDir); err != nil {
			return RunResult{}, err
		}
	}

	return RunResult{
		Outcome:      outcome.Outcome,
		StuckReason:  outcome.StuckReason,
		BatchesDone:  outcome.BatchesDone,
		SummaryTitle: summaryTitle,
	}, nil
}

// verifyEveryBatchDone reloads the persisted state and confirms every batch
// in plan carries a terminal record whose status is done — the whole-plan
// invariant an outcome: done claims. A batch with no record, a non-terminal
// record, or a terminal non-done record (stuck/dead) means Master wrote
// done while that batch was not actually built — most importantly a batch
// begun but never recorded (its fork slipped past record-batch), which the
// transcript-count cross-check alone cannot catch when the fork transcript
// exists but no record-batch consumed it. State is reloaded fresh because
// the in-memory copy Run captured before Master spawned is stale by run
// exit (begin/record-batch mutated it repeatedly).
func verifyEveryBatchDone(websterDir string, plan *builderengine.Plan) error {
	st, err := LoadState(websterDir)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("webster: run reached outcome: done but no state.json exists — no batch was ever recorded")
	}

	var offenders []string
	for _, b := range plan.Batches {
		bs, ok := st.Batches[b.Number]
		switch {
		case !ok || bs == nil:
			offenders = append(offenders, fmt.Sprintf("%02d-%s (never recorded)", b.Number, b.Slug))
		case !bs.Terminal:
			offenders = append(offenders, fmt.Sprintf("%02d-%s (begun, not recorded terminal)", b.Number, b.Slug))
		case bs.Status != builderengine.DigestStatusDone:
			offenders = append(offenders, fmt.Sprintf("%02d-%s (%s)", b.Number, b.Slug, bs.Status))
		}
	}
	if len(offenders) > 0 {
		return fmt.Errorf("webster: run reached outcome: done but %d batch(es) lack a terminal done record: %s — a batch was begun without being recorded done, or Master claimed done prematurely", len(offenders), strings.Join(offenders, ", "))
	}
	return nil
}

// runExitAuditCrossCheck implements the run-exit whole-session backstop
// behind record-batch's own per-batch incremental audit: a nil
// result.ForkAudit on a done run of Master's ForkSubagents: true spec is
// itself a hard error (the audit could not complete — fail loud, never
// skipped), CheckParent/CheckFork run over the whole-session facts exactly
// as record-batch's own incremental audit does, and the total audited
// fork-transcript count must be >= the number of batches BeginBatch
// recorded with Kind: "fork" under THIS Master session (see
// countBegunForkBatches — a prior crashed session's batches are outside the
// current session's audit by construction) — a shortfall means a batch was
// recorded without its fork surviving audit. Every violation is a hard
// error carried on the run's own error; the outcome file stays on disk for
// diagnosis (Run never removes it).
func runExitAuditCrossCheck(deps RunDeps, outcomePath, summaryPath string, result shuttleengine.Result) error {
	if result.ForkAudit == nil {
		return fmt.Errorf("webster: run reached outcome: done on a fork-authorized master spawn but its whole-session fork audit did not complete (nil ForkAudit) — this is fail-loud, never skipped")
	}

	// Reload state fresh: begin-batch/record-batch mutated and persisted it
	// repeatedly across Master's whole run, so the in-memory copy captured
	// before Master ever spawned is stale by run-exit.
	st, err := LoadState(deps.WebsterDir)
	if err != nil {
		return err
	}

	weftRef := weftReferencePattern(deps.Layout)

	var violations []error
	for _, v := range CheckParent(*result.ForkAudit, outcomePath, summaryPath, weftRef) {
		violations = append(violations, v)
	}
	for _, f := range result.ForkAudit.Forks {
		for _, v := range CheckFork(f, weftRef) {
			violations = append(violations, v)
		}
	}
	if len(violations) > 0 {
		return errors.Join(violations...)
	}

	begun := countBegunForkBatches(st, result.SessionID)
	audited := len(result.ForkAudit.Forks)
	if audited < begun {
		return fmt.Errorf("webster: run-exit audit cross-check: %d audited fork transcript(s) is fewer than %d begun fork batch(es) — a batch was recorded without its fork surviving audit", audited, begun)
	}

	return nil
}
