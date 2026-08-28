// runlevel.go implements Run, webster's `run` verb engine core: the run-level exclusive lease, the
// automatic validation gate (including the zero-batch pre-flight refusal), the state-phase
// entry-time reclaim of webster's own two reclaimable substrates (Master's own strand and any
// non-terminal recovery-batch strand — forks die with Master, so there is never a third), the
// plan-fingerprint crash/resume guard with its --fresh archive/re-init escape, the
// never-instantly-re-pause clear, the stale-outcome/summary archive, the always-fresh Master spawn
// (fork-authorized, both output files), the shuttle-outcome-to-RunResult mapping, and the run-exit
// whole-session audit cross-check that backstops record-batch's own per-batch incremental audit.
// Named runlevel.go, not run.go, to avoid a clash with any future poll/spawn-style file name.

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

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// runLockName is the exclusive-lease file name inside the webster scratch
// dir, held for the ENTIRE duration of one Run call:
// without it, two concurrent `lyx webster run` invocations would each
// cold-start from the same state.json and reports, then both drive the
// Master spawn at once.
const runLockName = "run.lock"

// ErrRunBusy marks Run's fail-fast refusal when another invocation already holds scratchDir's
// run.lock.
// It is webster's own sentinel (per the
// webster-owns-its-own-domain-types decision) because the caller must treat this refusal
// differently from every other hard error: the losing call touched NOTHING on disk — the winner is
// mid-run and owns the state — so webstercli must not run its own exit-time fabric backstop for it.
var ErrRunBusy = errors.New("webster: run is already in progress")

// ErrNilBatcher marks Run's refusal when RunDeps.Batcher was never populated
// by the caller. Population is webstercli's obligation (PersistentPreRunE
// resolves it via batcher.Active); a nil interface here would panic inside
// Batch rather than surface a diagnosable error.
var ErrNilBatcher = errors.New("webster: RunDeps.Batcher not populated")

// RunActive reports whether a live `lyx webster run` currently holds scratchDir's run.lock.
// It probes non-blocking: if the lock can be acquired, no run owns it and the probe returns false;
// otherwise it returns true.
// Probe errors (filesystem failures) are returned so the caller can decide.
func RunActive(scratchDir string) (bool, error) {
	fl, acquired, err := lock.TryAcquireWriteLock(filepath.Join(scratchDir, runLockName))
	if err != nil {
		return false, fmt.Errorf("webster: probe run.lock in %s: %w", scratchDir, err)
	}
	if acquired {
		// Nobody held it — release the probe lock immediately so a real run
		// starting right after is never blocked by the probe.
		_ = fl.Release()
		return false, nil
	}
	return true, nil
}

// OutcomeFileName is outcome.yaml's fixed filename inside a webster dir.
// The file's schema is webster's own (outcome.go), owned outright rather than shared with any
// sibling module.
const OutcomeFileName = "outcome.yaml"

// OutcomePath returns the path to outcome.yaml inside websterDir.
func OutcomePath(websterDir string) string {
	return filepath.Join(websterDir, OutcomeFileName)
}

// MasterHandle is the started-but-not-yet-finished Master spawn Run blocks on: StrandGUID
// identifies the reed strand Master runs in (available immediately after the start, so Run can
// persist it to state.json BEFORE blocking — the record the next run's entry-time reclaim reads),
// and Wait blocks until the spawn reaches a terminal shuttle outcome. *shuttleengine.Run satisfies
// this structurally.
type MasterHandle interface {
	StrandGUID() string
	Wait() (shuttleengine.Result, error)
}

// MasterStarter is the seam Run spawns Master through, webster's own OrchestratorStarter shape.
// Production code passes an adapter over *shuttleengine.Runner (webstercli's own starter);
// tests pass a local fake.
type MasterStarter interface {
	StartMaster(shuttleengine.Spec) (MasterHandle, error)
}

// RunDeps carries every seam Run needs for testing.
// Starter spawns Master; Reed, Engine, and ShuttleCfg support session resolution and audit;
// Geom is the told Geometry every path (PlanDir, WebsterDir, ReportsDir, PromptsDir, ScratchDir,
// WorktreeRoot, AnchorRoot, StencilsDir) is read from;
// RefMatcher is the injected fabric-reference class matcher CheckParent/CheckFork consult, never nil
// in either mode;
// Config, Roles, and Batcher carry the loaded configuration, pre-flight-resolved role->model-spec
// map, and CLI-pre-resolved active batchifier.
type RunDeps struct {
	Starter    MasterStarter
	Reed       shuttleengine.ReedOps
	Engine     shuttleengine.Engine
	ShuttleCfg shuttleengine.Config
	Roles      map[Role]modelspec.Resolved
	Config     Config
	// Batcher is the CLI-resolved active batchifier (batcher.Active),
	// populated by webstercli's PersistentPreRunE before Run is called. Run
	// refuses with ErrNilBatcher when it is nil.
	Batcher    batcher.Batcher
	Geom       Geometry
	RefMatcher RefMatcher

	// Clock is the integration stage's bounded-wait clock seam: nil selects
	// the production realClock, and a test injects a fake so the
	// missing-integration-report wait replays instantly instead of blocking
	// a real DefaultAwaitWaitS window.
	Clock Clock

	// OpenBisector is the integration stage's bisect-repo opener: a caller-supplied closure,
	// never a defaulted construction. It stays lazy so a run that never reaches a failing
	// integration suite never opens a fabric handle; a test injects a fake by supplying an
	// opener that returns it. A nil OpenBisector means "no fabric in this mode" — the
	// integration-failure bypass at the runIntegrationStage call site, not "construct the
	// production default".
	OpenBisector func() (FabricBisector, error)
}

// RunOptions carries one `run` invocation's caller-supplied choices.
// Fresh requests the fingerprint-mismatch escape: archive the stale state.json and reports dir,
// clear the re-renderable prompts dir, and re-init, rather than refusing with
// ErrFingerprintMismatch.
type RunOptions struct {
	Fresh bool
}

// RunResult is what one successful Run call hands back: the parsed outcome.yaml's judgment
// (Outcome/StuckReason/BatchesDone) plus the summary.md's title.
type RunResult struct {
	// Outcome is one of webster's own outcomeDone, outcomeStuck, or
	// outcomePaused values (outcome.go), taken verbatim from the parsed
	// outcome.yaml.
	Outcome string
	// StuckReason is the parsed outcome.yaml's stuck_reason, verbatim.
	StuckReason string
	// BatchesDone is the parsed outcome.yaml's batches_done, verbatim.
	BatchesDone int
	// SummaryTitle is the parsed summary.md's title heading. Always
	// populated for Outcome == outcomeDone (ParseSummary is
	// required there — a missing or malformed summary is a hard error);
	// populated best-effort for stuck/paused (empty when summary.md is
	// itself missing or malformed, which is not an error on those two
	// outcomes).
	SummaryTitle string
	// Warnings carries every non-fatal observation the integration stage
	// accumulated this run — never a failure. Mirrors RecordResult.Warnings'
	// shape and contract: a nil OpenBisector's unlocalized-failure notice is
	// the one warning this stage currently produces.
	Warnings []string
	// Cycles carries every non-trivial strongly-connected component
	// SequenceBatches condensed for this run — always informational, never
	// a failure, and empty for the overwhelmingly common acyclic plan.
	Cycles []Cycle
}

// newRunGUID returns a 128-bit random identifier, hex-encoded, generated
// from crypto/rand: minted once at first init, never regenerated
// across a resume.
func newRunGUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("webster: mint run guid: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// MasterAskingError marks Run's mapping of a shuttle OutcomeAsking result for Master's own spawn:
// Master ended its turn asking a question instead of ever reaching its own outcome-file final
// action.
// Unwrap returns ErrMasterAsking so a caller can classify via errors.Is without needing the
// concrete type;
// the concrete type itself carries the per-call SessionID, RunDir, and LastAssistantMessage a
// caller needs to log or resume from.
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

// MasterDiedError marks Run's mapping of a shuttle OutcomeDied result for Master's own spawn: its
// pane died (or it never became ready) before it ever reached its own outcome-file final action.
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

// MasterTimeoutError marks Run's mapping of a shuttle OutcomeTimeout result for Master's own spawn:
// its wall-clock Timeout (MasterTimeoutMin, webster's own whole-run timeout config key)
// elapsed before it ever reached its own outcome-file final action.
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
// and any recorded, non-terminal recovery-batch strand.
// Forks die WITH Master (same process) — there is never an orphaned
// in-flight fork implementer to reclaim, which is what keeps webster's own
// entry-time reclaim simple, per
// discussion.md's crash-resume-re-drive-first-unreported decision. A nil st
// (no run has ever started) is a no-op.
func reclaimEntryTimeStrands(reed shuttleengine.ReedOps, st *State) error {
	if st == nil {
		return nil
	}

	if st.MasterStrand != "" {
		if err := removeStrandIfLive(reed, st.MasterStrand); err != nil {
			return err
		}
	}

	for _, bs := range st.Batches {
		if bs != nil && bs.Kind == "recovery" && !bs.Terminal {
			if err := removeStrandIfLive(reed, bs.StrandGUID); err != nil {
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

// Run drives one `lyx webster run` invocation to completion: the run-level mutex, validation gate,
// state-phase entry-time reclaim, plan-fingerprint crash/resume guard with its --fresh escape, and
// Master spawn to outcome.
// ErrRunBusy and ErrFingerprintMismatch are exported sentinels;
// non-done shuttle outcomes return *Master*Error types.
func Run(deps RunDeps, opts RunOptions) (RunResult, error) {
	if err := os.MkdirAll(deps.Geom.WebsterDir, 0o755); err != nil {
		return RunResult{}, fmt.Errorf("webster: create webster dir %s: %w", deps.Geom.WebsterDir, err)
	}
	if err := os.MkdirAll(deps.Geom.ScratchDir, 0o755); err != nil {
		return RunResult{}, fmt.Errorf("webster: create webster scratch dir %s: %w", deps.Geom.ScratchDir, err)
	}

	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(deps.Geom.ScratchDir, runLockName))
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: acquire run lock in %s: %w", deps.Geom.ScratchDir, err)
	}
	if !locked {
		return RunResult{}, fmt.Errorf("%w: %q (run.lock held); wait for it to finish, or check `lyx webster status`", ErrRunBusy, deps.Geom.ScratchDir)
	}
	defer runLock.Release()

	plan, err := planparser.ParsePlan(deps.Geom.PlanDir)
	if err != nil {
		return RunResult{}, err
	}

	if findings := planparser.Validate(plan, deps.Geom.WorktreeRoot); len(findings) > 0 {
		msgs := make([]string, len(findings))
		for i, f := range findings {
			msgs[i] = f.Error()
		}
		return RunResult{}, fmt.Errorf("webster: plan validation refused this run (%d finding(s)): %s", len(findings), strings.Join(msgs, "; "))
	}

	if deps.Batcher == nil {
		return RunResult{}, ErrNilBatcher
	}
	batches := deps.Batcher.Batch(plan.Cards)

	// nothing-to-build is a malformed plan, never a vacuous outcome: done —
	// webster's own pre-flight over the batchifier's own output, per
	// discussion.md's run-verb-shape decision. This refusal runs against the
	// batchifier's own output, still naming the batchifier, because
	// SequenceBatches below is length-preserving and can neither create nor
	// remove this condition.
	if len(batches) == 0 {
		return RunResult{}, fmt.Errorf("webster: plan %s produced zero execution batches; nothing to build is a malformed plan, never a vacuous outcome: done", deps.Geom.PlanDir)
	}

	// Re-bind batches through the sequencer: every later use in this
	// function (the Master prompt, mapMasterDone, runIntegrationStage) then
	// sees the derived execution order rather than the batchifier's own
	// declared order.
	var cycles []Cycle
	batches, cycles = SequenceBatches(batches)

	fingerprint, err := fingerprint(deps.Geom.PlanDir)
	if err != nil {
		return RunResult{}, err
	}

	// Serialize the whole state phase — load, entry-time reclaim, fresh
	// archive/re-init, and the post-start strand record — against every
	// other verb's own state read-modify-write, webster's own
	// AcquireStateMutation discipline. Released explicitly right after
	// the strand record lands, never held across Master's own wait.
	mutateLock, err := AcquireStateMutation(deps.Geom.ScratchDir)
	if err != nil {
		return RunResult{}, err
	}
	mutateHeld := true
	defer func() {
		if mutateHeld {
			_ = mutateLock.Release()
		}
	}()

	st, err := LoadState(deps.Geom.WebsterDir, deps.Geom.ScratchDir)
	if err != nil {
		return RunResult{}, err
	}

	// Entry-time reclaim BEFORE anything else acts on the loaded state
	// (including the --fresh archive below, which would discard the only
	// record of these strands): a prior run whose process died mid-wait
	// leaves a live Master pane (or a live recovery strand) that keeps
	// driving on its own.
	if err := reclaimEntryTimeStrands(deps.Reed, st); err != nil {
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
		}
		if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
			return RunResult{}, err
		}

	case st.PlanFingerprint != fingerprint:
		if !opts.Fresh {
			return RunResult{}, fmt.Errorf("%w: on-disk plan fingerprint %s does not match this run's recorded fingerprint %s; the plan changed since state.json was created — re-run with --fresh to archive the stale state and reports and start over", ErrFingerprintMismatch, fingerprint, st.PlanFingerprint)
		}

		if _, err := archiveStateFile(deps.Geom.WebsterDir, time.Now); err != nil {
			return RunResult{}, err
		}
		if err := archiveReportsDir(deps.Geom.ReportsDir, time.Now); err != nil {
			return RunResult{}, err
		}
		if err := clearRenderedPrompts(deps.Geom.PromptsDir); err != nil {
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
		}
		if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
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
	if err := ClearPause(deps.Geom.ScratchDir); err != nil {
		return RunResult{}, err
	}

	if _, err := archiveStaleOutcome(deps.Geom.WebsterDir, time.Now); err != nil {
		return RunResult{}, err
	}
	if _, err := ArchiveStaleSummary(deps.Geom.WebsterDir, time.Now); err != nil {
		return RunResult{}, err
	}

	outcomePath, err := filepath.Abs(OutcomePath(deps.Geom.WebsterDir))
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: resolve outcome path: %w", err)
	}
	summaryPath, err := filepath.Abs(SummaryPath(deps.Geom.WebsterDir))
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: resolve summary path: %w", err)
	}

	// The integration fork's prompt is Go-rendered and Go-written, up front,
	// exactly like a batch's own fork prompt: Master may write nothing but its
	// two contract files (a hand-synthesized prompt file would be a
	// parent-write audit violation), so a plan with a "## verify:" section
	// must find its integration prompt already on disk — found live in
	// crucible round fable-r1, where a Master correctly refused to improvise
	// one and the stage was unreachable.
	integrationPromptPath := ""
	if ShouldRunIntegration(plan) {
		integrationReportPath, err := filepath.Abs(IntegrationReportPath(deps.Geom.ReportsDir))
		if err != nil {
			return RunResult{}, fmt.Errorf("webster: resolve integration report path: %w", err)
		}
		integrationPrompt, err := RenderIntegrationPrompt(plan, integrationReportPath, deps.Geom.WorktreeRoot, deps.Geom.StencilsDir)
		if err != nil {
			return RunResult{}, err
		}
		if err := os.MkdirAll(deps.Geom.PromptsDir, 0o755); err != nil {
			return RunResult{}, fmt.Errorf("webster: create prompts dir %s: %w", deps.Geom.PromptsDir, err)
		}
		integrationPromptPath, err = filepath.Abs(filepath.Join(deps.Geom.PromptsDir, integrationPromptFileName))
		if err != nil {
			return RunResult{}, fmt.Errorf("webster: resolve integration prompt path: %w", err)
		}
		if err := os.WriteFile(integrationPromptPath, integrationPrompt, 0o644); err != nil {
			return RunResult{}, fmt.Errorf("webster: write integration prompt %s: %w", integrationPromptPath, err)
		}
	}

	prompt, err := RenderMasterPrompt(batches, st, outcomePath, summaryPath, integrationPromptPath, deps.Config.SelfFixCap, deps.Config.PollWaitS, deps.Geom.AnchorRoot, deps.Geom.StencilsDir)
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
	if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
		return RunResult{}, err
	}

	runState, _, err := shuttleengine.FindRun(deps.ShuttleCfg, deps.Geom.AnchorRoot, st.MasterStrand)
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: resolve spawned master run: %w", err)
	}
	st.MasterSessionID = runState.SessionID
	// The launch model IS the idempotent-assertion baseline BeginBatch's
	// own per-batch model check consults from batch 1 onward — a batch 1
	// begin-batch call then finds AssertedModel already equal to
	// RoleMaster's model and injects nothing.
	st.AssertedModel = resolved.Model

	// Second save: the session ID and launch-model baseline the verbs read.
	if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
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
		runResult, mapErr := mapMasterDone(deps, batches, outcomePath, summaryPath, result)
		if mapErr != nil {
			return RunResult{}, mapErr
		}
		// Cycles are always informational: prepend one warning per cycle
		// ahead of the integration stage's own per-failure warnings below,
		// so the sequencing observations, which describe the whole run,
		// read first. Non-done outcomes (asking/died/timeout) return an
		// error rather than a RunResult, so a cycle observed on a run that
		// ends stuck/paused/died reaches the operator through that error
		// path's own message rather than through Cycles — an accepted,
		// stated limitation, not an oversight.
		runResult.Cycles = cycles
		if len(cycles) > 0 {
			cycleWarnings := make([]string, len(cycles))
			for i, c := range cycles {
				cycleWarnings[i] = c.Warning()
			}
			runResult.Warnings = append(cycleWarnings, runResult.Warnings...)
		}
		// The integration-suite stage is a minimal call-site addition at the
		// end of the run, not a rewrite of the batch loop above: it re-derives
		// everything it needs from disk (the plan's own ShouldRunIntegration,
		// every batch's own persisted terminal record) rather than trusting
		// whatever Master's own outcome.yaml/summary.md already said, so it
		// runs the same way regardless of whether Master itself reported done
		// or stuck for this plan.
		warnings, err := runIntegrationStage(deps, plan, batches, runResult.Outcome)
		if err != nil {
			return RunResult{}, err
		}
		runResult.Warnings = append(runResult.Warnings, warnings...)
		return runResult, nil

	case shuttleengine.OutcomeAsking:
		logger.Warn("websterengine: master run is asking", "outcome", result.Outcome, "sessionID", result.SessionID, "runDir", result.RunDir, "lastAssistantMessage", result.LastAssistantMessage)
		return RunResult{}, &MasterAskingError{SessionID: result.SessionID, RunDir: result.RunDir, Message: result.LastAssistantMessage}

	case shuttleengine.OutcomeDied:
		logger.Warn("websterengine: master run died", "outcome", result.Outcome, "sessionID", result.SessionID, "runDir", result.RunDir)
		return RunResult{}, &MasterDiedError{SessionID: result.SessionID, RunDir: result.RunDir}

	case shuttleengine.OutcomeTimeout:
		logger.Warn("websterengine: master run timed out", "outcome", result.Outcome, "sessionID", result.SessionID, "runDir", result.RunDir)
		return RunResult{}, &MasterTimeoutError{SessionID: result.SessionID, RunDir: result.RunDir}

	default:
		logger.Warn("websterengine: master run returned unrecognized shuttle outcome", "outcome", result.Outcome, "sessionID", result.SessionID, "runDir", result.RunDir)
		return RunResult{}, fmt.Errorf("webster: master run returned unrecognized shuttle outcome %q", result.Outcome)
	}
}

// mapMasterDone maps a shuttle-level OutcomeDone Master spawn onto RunResult:
// strict outcome.yaml parsing, summary.md validation (required for done,
// best-effort otherwise), every-batch-terminal-done and run-exit audit
// cross-checks (done outcomes only), and pause-flag clear for non-paused
// terminals.
func mapMasterDone(deps RunDeps, batches []batcher.Batch, outcomePath, summaryPath string, result shuttleengine.Result) (RunResult, error) {
	outcome, err := parseOutcome(outcomePath)
	if err != nil {
		return RunResult{}, err
	}

	var summaryTitle string
	if outcome.Outcome == outcomeDone {
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
		if err := verifyEveryBatchDone(deps.Geom.WebsterDir, deps.Geom.ScratchDir, batches); err != nil {
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

	if outcome.Outcome != outcomePaused {
		if err := ClearPause(deps.Geom.ScratchDir); err != nil {
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

// batchIdentity returns b's own number/slug identity, taken from its first
// card. This is a v0 identity-batcher assumption — one card per batch, so
// the batch's own number/slug coincide with its sole card's — documented
// the same way state.go's BatchState.CardSHAs is: the multi-card
// enumeration path is dormant until a grouping batchifier ships, and needs
// its own identity scheme then, not a change here.
func batchIdentity(b batcher.Batch) (number int, slug string) {
	if len(b.Cards) == 0 {
		return 0, ""
	}
	return b.Cards[0].Number, b.Cards[0].Slug
}

// verifyEveryBatchDone reloads the persisted state and confirms every batch
// in batches carries a terminal record whose status is done — the
// whole-plan invariant an outcome: done claims. A batch with no record, a
// non-terminal record, or a terminal non-done record (stuck/dead) means
// Master wrote done while that batch was not actually built — most
// importantly a batch begun but never recorded (its fork slipped past
// record-batch), which the transcript-count cross-check alone cannot catch
// when the fork transcript exists but no record-batch consumed it. State is
// reloaded fresh because the in-memory copy Run captured before Master
// spawned is stale by run exit (begin/record-batch mutated it repeatedly).
// scratchDir locates state.json's lock, which now lives outside websterDir.
func verifyEveryBatchDone(websterDir, scratchDir string, batches []batcher.Batch) error {
	st, err := LoadState(websterDir, scratchDir)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("webster: run reached outcome: done but no state.json exists — no batch was ever recorded")
	}

	var offenders []string
	for _, b := range batches {
		number, slug := batchIdentity(b)
		bs, ok := st.Batches[number]
		switch {
		case !ok || bs == nil:
			offenders = append(offenders, fmt.Sprintf("%02d-%s (never recorded)", number, slug))
		case !bs.Terminal:
			offenders = append(offenders, fmt.Sprintf("%02d-%s (begun, not recorded terminal)", number, slug))
		case bs.Status != DigestStatusDone:
			offenders = append(offenders, fmt.Sprintf("%02d-%s (%s)", number, slug, bs.Status))
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
	st, err := LoadState(deps.Geom.WebsterDir, deps.Geom.ScratchDir)
	if err != nil {
		return err
	}

	var violations []error
	for _, v := range CheckParent(*result.ForkAudit, outcomePath, summaryPath, deps.Geom.WorktreeRoot, deps.RefMatcher) {
		violations = append(violations, v)
	}
	for _, f := range result.ForkAudit.Forks {
		for _, v := range CheckFork(f, outcomePath, summaryPath, deps.Geom.WorktreeRoot, deps.RefMatcher) {
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

// runIntegrationStage drives the plan-level integration-suite stage after
// every batch has reached a terminal-done state, wired at the very end of
// Run per the integration-suite-fork-with-bisect decision: a no-op when the
// plan carries no plan-level "## verify:" section (ShouldRunIntegration) or
// when the whole-plan batch set is not (yet) all terminal-done — a run that
// ended stuck for an ordinary batch reason never reaches the integration
// stage at all. Otherwise it confirms the one dedicated integration fork's
// report has landed (RunIntegration — Master itself spawned that fork and
// waited for its report per webster-template-master.md's own integration-fork
// bracket instruction, so the report is normally already on disk by the
// time Run reaches this call; the bounded await here is a defensive
// re-confirmation, mirroring the run-exit audit's own backstop posture)
// and, on a FAILED report, runs the in-process SHA-bisect over every
// batch's own accumulated BatchState.CardSHAs and escalates. The persisted
// terminal record and summary.md extension this produces are authoritative
// regardless of what Master's own outcome.yaml/summary.md already said,
// since webster's own localized finding is strictly more precise than
// Master's own best-effort report.
// When deps.OpenBisector is nil, this mode has no fabric repo to bisect against: the localization
// path (BisectAndEscalate/bisect) is bypassed entirely — never pushed down into those functions —
// and the escalation records an unlocalized "unknown"/"unknown" failure instead, with the returned
// warning explaining why. This bypass is deliberate: bisect's own empty-SHA fallback is unreachable
// here because card SHAs accumulate normally, a single accumulated SHA would make it record a real
// SHA under a "cannot localize" claim, and two or more reach repo.CurrentBranch(), which nil-pointer
// panics on a nil bisector.
func runIntegrationStage(deps RunDeps, plan *planparser.Plan, batches []batcher.Batch, masterOutcome string) ([]string, error) {
	if !ShouldRunIntegration(plan) {
		return nil, nil
	}
	// A run whose batches are not all terminal-done never reached the
	// integration stage in the first place (Master's own bracket
	// instruction only spawns the integration fork after every batch is
	// done) — this is expected, not a failure, so the error here is
	// swallowed rather than propagated.
	if err := verifyEveryBatchDone(deps.Geom.WebsterDir, deps.Geom.ScratchDir, batches); err != nil {
		return nil, nil
	}

	clk := deps.Clock
	if clk == nil {
		clk = realClock{}
	}
	await, err := AwaitIntegration(deps.Geom.ReportsDir, time.Duration(DefaultAwaitWaitS)*time.Second, clk)
	if err != nil {
		return nil, err
	}
	if !await.ReportPresent {
		// A done outcome CLAIMS a passing integration suite, so a missing
		// report is a real inconsistency — fail loud. A non-done outcome with
		// no report is instead consistent (the integration fork died, or
		// Master stuck out before the stage) — Master's own graceful judgment
		// is the run's result, and erroring here would overwrite it.
		if masterOutcome == outcomeDone {
			return nil, fmt.Errorf("webster: run reached outcome: done on a plan with a \"## verify:\" section but its integration report never landed — the integration fork never ran or never reported")
		}
		return nil, nil
	}

	report, err := ParseReport(IntegrationReportPath(deps.Geom.ReportsDir))
	if err != nil {
		return nil, err
	}
	if report.Status == ReportStatusOK {
		// Master's own "done" outcome already reflects a passing integration
		// suite correctly; nothing further to escalate.
		return nil, nil
	}

	mutateLock, err := AcquireStateMutation(deps.Geom.ScratchDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = mutateLock.Release() }()

	// Reload state fresh: begin-batch/record-batch mutated and persisted it
	// repeatedly across Master's whole run, so the in-memory copy captured
	// before Master ever spawned is stale by the time this stage runs.
	st, err := LoadState(deps.Geom.WebsterDir, deps.Geom.ScratchDir)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("webster: integration stage: no state.json to escalate against")
	}

	var warnings []string
	if deps.OpenBisector == nil {
		// No fabric repo in this mode: bypass BisectAndEscalate/bisect
		// entirely rather than feeding them a nil bisector — see the func
		// doc comment for why the bypass must live at this call site.
		RecordIntegrationFailure(st, "unknown", "unknown")
		if err := AppendIntegrationFailure(deps.Geom.WebsterDir, "unknown", "unknown"); err != nil {
			return nil, err
		}
		warnings = append(warnings, "the integration suite failed and the offending card could not be localized because this mode has no fabric repo to bisect against")
	} else {
		bisector, err := deps.OpenBisector()
		if err != nil {
			return nil, err
		}
		shas, labels := accumulatedCardSHAs(batches, st)
		if err := BisectAndEscalate(bisector, shas, labels, plan.Verify, deps.Geom.WorktreeRoot, deps.Geom.WebsterDir, st); err != nil {
			return nil, err
		}
	}

	if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
		return nil, err
	}

	// A Master that claimed outcome: done while the plan-level integration
	// suite actually FAILED is the same contradiction the missing-report-under-
	// done branch above already fails loud on: a done outcome CLAIMS a passing
	// suite. Fail loud here too, symmetric with that branch — otherwise the run
	// would report done over a suite webster itself just localized as failing,
	// leaving the CLI envelope's outcome: done contradicting summary.md's own
	// "integration suite failed" section. The escalation above (the reserved
	// -1 record and summary.md's localized card) is already persisted and is
	// fabric-committed by webstercli's run backstop, so a resume or a human still
	// sees the offending card despite this loud return. A non-done master
	// outcome (the template-correct stuck) keeps its own judgment: the
	// escalation merely sharpened its summary, so it returns nil below.
	if masterOutcome == outcomeDone {
		return warnings, fmt.Errorf("webster: run reached outcome: done but the plan-level \"## verify:\" suite FAILED and was escalated (see summary.md for the SHA-bisect-localized offending card) — a done outcome requires a passing integration suite")
	}

	return warnings, nil
}

// accumulatedCardSHAs walks batches in execution order and collects every
// terminal batch's own CardSHAs alongside a matching "NN-slug" label — the
// ordered per-card SHA trail and parallel label set bisect and its
// escalation search over. A batch with no persisted record (should never
// happen once verifyEveryBatchDone has already confirmed every batch is
// terminal-done, but handled defensively rather than assumed) contributes
// nothing.
func accumulatedCardSHAs(batches []batcher.Batch, st *State) (shas, labels []string) {
	for _, b := range batches {
		number, slug := batchIdentity(b)
		bs, ok := st.Batches[number]
		if !ok || bs == nil {
			continue
		}
		for _, sha := range bs.CardSHAs {
			shas = append(shas, sha)
			labels = append(labels, fmt.Sprintf("%02d-%s", number, slug))
		}
	}
	return shas, labels
}
