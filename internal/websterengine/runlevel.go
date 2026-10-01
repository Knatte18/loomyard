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
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
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
// identifies the reed strand Master runs in (available once the start returns, which includes the
// provider's startup window, so Run can persist it to state.json BEFORE blocking on Wait — the
// record the next run's entry-time reclaim reads), and Wait blocks until the spawn reaches a
// terminal shuttle outcome. *shuttleengine.Run satisfies this structurally.
//
// Accepted residuals, both from the spawn now including the startup window under the
// state-mutation lease, and from state.json being persisted only after StartMaster returns:
// a process killed inside the startup window, or a startup mechanism failure (shuttle could not
// get a liveness answer from reed maxStatusRetries times), leaves a live Master pane whose
// MasterStrand was never recorded — invisible to reclaimEntryTimeStrands. A not-ready start no
// longer leaks (shuttle tears the strand down) unless that teardown's own strand removal fails,
// which the returned error then states.
type MasterHandle interface {
	StrandGUID() string
	Wait() (shuttleengine.Result, error)
}

// MasterStarter is the seam Run spawns Master through, webster's own OrchestratorStarter shape.
// Production code passes an adapter over *shuttleengine.Runner (webstercli's own starter);
// tests pass a local fake.
//
// A shuttleengine.GateSpec rides beside the Spec: it is the mechanical validator the Master run's
// own Wait consults before it may finalize, and its zero value (a nil Gate) means ungated, which is
// what the Webster row supplies today. The seam is widened rather than joined by a second
// StartMaster form because it has exactly one call site and two production implementors -- an added
// form would cost every fake a second method with no caller, which is the price shuttleengine's own
// RunGated/AttachGated pay deliberately for a genuinely shared seam and which buys nothing here.
type MasterStarter interface {
	StartMaster(shuttleengine.Spec, shuttleengine.GateSpec) (MasterHandle, error)
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

	// Gate is the mechanical validator Master's own shuttle run is held to: Run hands it to
	// StartMaster beside the Spec, and the run's Wait re-prompts the live Master on a failed verdict
	// until it passes or the re-prompt budget is exhausted. The zero value means ungated, which is
	// what every caller supplies until a Webster validator exists.
	Gate shuttleengine.GateSpec

	// FrictionDir is the told absolute friction directory (see internal/friction), empty when Tier 2
	// is off. It lives here rather than on Geometry because internal/hubgeom and
	// internal/standalonegeom are the Told-Geometry Invariant's only Geometry-struct constructors and
	// this value needs no geometry derivation.
	FrictionDir string

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
// It also discards pending audit findings, on an unchanged plan too, once their suspect paths match the run's start commit.
type RunOptions struct {
	Fresh bool
}

// RunResult is what one successful Run call hands back: the parsed outcome.yaml's judgment
// (Outcome/StuckReason/BatchesDone) plus the summary.md's title.
type RunResult struct {
	// Outcome is one of webster's own outcomeDone, outcomeStuck, or outcomePaused values (outcome.go),
	// taken verbatim from the parsed outcome.yaml — except that the integration stage demotes a done to outcomeStuck when its triage finds a regression.
	Outcome string
	// StuckReason is the parsed outcome.yaml's stuck_reason, verbatim — except for a done demoted by the integration stage,
	// whose reason names the regressing identities and the localized card.
	StuckReason string
	// BatchesDone is the parsed outcome.yaml's batches_done, verbatim.
	BatchesDone int
	// SummaryTitle is the parsed summary.md's title heading. Always
	// populated for Outcome == outcomeDone (summaryparser.Parse is
	// required there — a missing or malformed summary is a hard error);
	// populated best-effort for stuck/paused (empty when summary.md is
	// itself missing or malformed, which is not an error on those two
	// outcomes).
	SummaryTitle string
	// Warnings carries every non-fatal observation the integration stage accumulated this run — never a failure.
	// Mirrors RecordResult.Warnings' shape and contract: the integration stage's triage notices (flaky and pre-existing failures, an unavailable baseline) and a nil OpenBisector's unlocalized-failure notice.
	Warnings []string
	// Cycles carries every non-trivial strongly-connected component
	// SequenceBatches condensed for this run — always informational, never
	// a failure, and empty for the overwhelmingly common acyclic plan.
	Cycles []Cycle
}

// hasBlockingFinding reports whether findings carries at least one planglyph.SeverityBlocking
// entry, mirroring internal/loomshed/planvalidate.go's own hasBlockingFinding and
// internal/loomcli/validate.go's own planFindingsHaveBlocking: severity, not finding count, decides
// the verdict on every side of this parity, and Run's own pre-flight gate is the real gate the
// other three mirror.
func hasBlockingFinding(findings []planglyph.Finding) bool {
	for _, f := range findings {
		if f.Severity == planglyph.SeverityBlocking {
			return true
		}
	}
	return false
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
	return fmt.Sprintf("webster: master asked a question instead of finishing (session %s, kept run dir %s): %s%s", e.SessionID, e.RunDir, e.Message, masterRerunWayForward)
}

// runExitWayForward is the trailing way-forward clause every run-exit refusal carries:
// state.json keeps every terminal batch, so a fresh Master resumes and re-drives each batch without a done record.
const runExitWayForward = "; way forward: re-run `lyx webster run`; a fresh Master resumes from state.json and re-drives every batch without a done record"

// masterRerunWayForward is the trailing way-forward clause of the Master-ended-early errors.
const masterRerunWayForward = "; way forward: re-run `lyx webster run` (re-step the Webster row); a fresh Master resumes from state.json"

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
	return fmt.Sprintf("webster: master pane died (session %s, kept run dir %s)%s", e.SessionID, e.RunDir, masterRerunWayForward)
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
	return fmt.Sprintf("webster: master run timed out (session %s, kept run dir %s)%s", e.SessionID, e.RunDir, masterRerunWayForward)
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
// A done outcome passes through the run-exit audit: policy findings nobody dispositioned become run-level warnings on RunResult.Warnings,
// and an undispositioned correctness finding demotes the outcome to stuck, the same way a regression in the integration stage does.
// Once the integration stage has run, every recorded audit warning is appended to summary.md's "Audit warnings" section when that file exists.
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
		return RunResult{}, fmt.Errorf("%w: %q (run.lock held); way forward: wait for it to finish, or check `lyx webster status`", ErrRunBusy, deps.Geom.ScratchDir)
	}
	defer runLock.Release()

	plan, err := planparser.ParsePlan(deps.Geom.PlanDir)
	if err != nil {
		return RunResult{}, err
	}

	// The approval gate fires here, at entry; the full validation gate runs further down, once the
	// state phase has settled, because its resolve-backed half must be scoped by the completed
	// cards only state.json knows about (see the ValidateDispatch call below).
	if !plan.Approved {
		return RunResult{}, fmt.Errorf("webster: plan %s is not approved (frontmatter approved: is not true); webster never runs an unapproved plan; way forward: approve the plan through its review, then re-run `lyx webster run`", deps.Geom.PlanDir)
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
		return RunResult{}, fmt.Errorf("webster: plan %s produced zero execution batches; nothing to build is a malformed plan, never a vacuous outcome: done; way forward: fix the plan's cards, run `lyx webster rebaseline --card NN` naming each card you edited when state.json already records this run, then re-run `lyx webster run`", deps.Geom.PlanDir)
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
	fileHashes, err := planFileHashes(deps.Geom.PlanDir)
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

	freshDrop, freshWarnings, err := freshPendingDrop(deps.Geom, st, opts)
	if err != nil {
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
			PlanFileHashes:  fileHashes,
			Batches:         map[int]*BatchState{},
		}
		if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
			return RunResult{}, err
		}

	case st.PlanFingerprint != fingerprint, freshDrop:
		if !opts.Fresh {
			return RunResult{}, fmt.Errorf("%w: on-disk plan fingerprint %s does not match this run's recorded fingerprint %s; the plan changed since state.json was created; %s", ErrFingerprintMismatch, fingerprint, st.PlanFingerprint, fingerprintMismatchWayForward(st, deps.Geom.PlanDir))
		}

		if _, err := archiveStateFile(deps.Geom.WebsterDir, time.Now); err != nil {
			return RunResult{}, err
		}
		// The drop is committed once the state is archived.
		// Only a done outcome carries RunResult.Warnings, so each drop is logged here too, where every later refusal, Master outcome and error still leaves it on record.
		for _, w := range freshWarnings {
			logger.Warn("websterengine: --fresh dropped a pending audit finding", "warning", w)
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
			PlanFileHashes:  fileHashes,
			Batches:         map[int]*BatchState{},
		}
		if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
			return RunResult{}, err
		}
	}

	if len(st.PendingAuditFindings) > 0 {
		return RunResult{}, pendingAuditFindingsError(st.PendingAuditFindings)
	}

	// Validation runs HERE — after the state phase settles — rather than at entry, because its
	// resolve-backed half must be scoped to the cards whose work has NOT landed yet: re-validating
	// the whole plan on a resume reported every completed Create/Delete/Rename card as a blocking
	// defect (create-already-exists, glyph-not-found — the plan working exactly as designed) and
	// permanently refused the very resume this verb's own help promises, wedging both the
	// documented `lyx webster run` resume flow and loom's own Webster-row re-drive. The scoping is
	// the same one begin-batch already uses; a fresh run has no completed cards and gets the
	// whole-plan answer unchanged. The plan-unapproved gate, which ValidateDispatch's format-only
	// set deliberately omits, already fired at entry above.
	// The scope here is begunCards, not completedCards: a batch begun but not recorded may already
	// have landed its work (see begunCards).
	findings, err := planglyph.ValidateDispatch(plan, deps.Geom.WorktreeRoot, begunCards(batches, st))
	// The resolve pass canonicalizes handles, rewriting the plan on disk before it reports either a
	// finding or an error, so the staleness re-baseline runs HERE — ahead of both refusals below —
	// and is persisted immediately. Restamping only past the refusals left state.json describing the
	// pre-rewrite bytes, and the first begin-batch then refused this run's own edit as a foreign one.
	// See this package's doc.go. A restamp or save failure never masks err.
	if rebaseErr := restampAndSaveFingerprint(deps.Geom, st); rebaseErr != nil {
		if err == nil {
			return RunResult{}, rebaseErr
		}
		// Both failed. The re-baseline failure is never allowed to MASK err — the validation
		// verdict is what the operator asked for — but it must not be dropped either: the resolve
		// pass has already rewritten the plan on disk, so a state.json still holding the
		// pre-rewrite fingerprint makes the NEXT run refuse this run's own edit as a foreign one,
		// with ErrFingerprintMismatch, whose advised recourse (--fresh) restarts into the same
		// wall. Both are reported, in the same shape webstercli's begin-batch and record-batch
		// already use for this exact coincidence (crucible round opus-medium-r5, R5-3).
		err = fmt.Errorf("%w; additionally, persisting the plan-fingerprint re-baseline this run had already earned failed: %v", err, rebaseErr)
	}
	if err != nil {
		if errors.Is(err, planglyph.ErrQuarryUnavailable) {
			// Its own returned error, named for quarry rather than the plan — a gate that could not
			// read the code has not found a plan defect to refuse the run over, matching
			// internal/loomshed/planvalidate.go's producer-side disposition.
			return RunResult{}, fmt.Errorf("webster: quarry could not answer validating plan %s: %w; way forward: transient, re-run `lyx webster run` once quarry answers", deps.Geom.PlanDir, err)
		}
		return RunResult{}, err
	}
	if hasBlockingFinding(findings) {
		msgs := make([]string, len(findings))
		for i, f := range findings {
			msgs[i] = f.Error()
		}
		return RunResult{}, fmt.Errorf("webster: plan validation refused this run (%d finding(s)): %s; way forward: fix the named cards in the plan, run `lyx webster rebaseline --card NN` naming each card you edited when state.json already records this run, then re-run `lyx webster run`", len(findings), strings.Join(msgs, "; "))
	}
	// No second re-baseline: the one above already ran immediately after the rewriting call, ahead
	// of both refusals, and persisted itself.

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
	summaryPath, err := filepath.Abs(summaryparser.Path(deps.Geom.WebsterDir))
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
	// The integration-report path is resolved unconditionally: the Master prompt renders it as its
	// own {{.integration_report_path}} marker regardless of whether the plan carries a "## verify:"
	// section, with the surrounding prose gating when it matters.
	integrationReportPath, err := filepath.Abs(IntegrationReportPath(deps.Geom.ReportsDir))
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: resolve integration report path: %w", err)
	}
	// A report left by an earlier run describes an earlier head: Master would read it as this run's
	// verdict and end stuck again without respawning the integration fork, so a run resumed after a
	// fix could never re-verify. Every run starts without one.
	if err := os.Remove(integrationReportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return RunResult{}, fmt.Errorf("webster: remove stale integration report %s: %w", integrationReportPath, err)
	}

	integrationPromptPath := ""
	if ShouldRunIntegration(plan) {
		integrationNotePath := friction.NotePath(deps.FrictionDir, "webster-integration")
		integrationLogPath, err := filepath.Abs(IntegrationLogPath(deps.Geom.ScratchDir))
		if err != nil {
			return RunResult{}, fmt.Errorf("webster: resolve integration log path: %w", err)
		}
		// Remove any prior log so a log present at triage time was written by this run's fork.
		if err := os.Remove(integrationLogPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return RunResult{}, fmt.Errorf("webster: remove stale integration log %s: %w", integrationLogPath, err)
		}
		// The fork's shell redirect cannot create the log's parent directory.
		if err := os.MkdirAll(filepath.Dir(integrationLogPath), 0o755); err != nil {
			return RunResult{}, fmt.Errorf("webster: create verify log dir: %w", err)
		}
		integrationPrompt, err := RenderIntegrationPrompt(plan, integrationReportPath, integrationLogPath, deps.Geom.WorktreeRoot, deps.Geom.StencilsDir, integrationNotePath)
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

	masterNotePath := friction.NotePath(deps.FrictionDir, "webster-master")
	prompt, err := RenderMasterPrompt(batches, st, outcomePath, summaryPath, integrationPromptPath, deps.Geom.PlanDir, integrationReportPath, deps.Config.SelfFixCap, deps.Config.PollWaitS, deps.Geom.WorktreeRoot, deps.Geom.AnchorRoot, deps.Geom.StencilsDir, masterNotePath)
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

	// The state-mutation lease acquired above is held across this call, which now includes the
	// provider's startup window (bounded by startup_timeout_s) — see AcquireStateMutation's own
	// contract. At run entry no batch forks exist yet, so the hold stalls nothing in practice.
	handle, err := deps.Starter.StartMaster(spec, deps.Gate)
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: start master: %w; way forward: transient, re-run `lyx webster run`", err)
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
		runResult.Warnings = append(freshWarnings, runResult.Warnings...)
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
		warnings, stuckReason, err := runIntegrationStage(deps, plan, batches, runResult.Outcome)
		if err != nil {
			// The error paths that remain are a done outcome with no integration report and an infrastructure failure in triage, localization, or recording.
			// runIntegrationStage can return warnings ALONGSIDE the latter,
			// and every error return here reports the zero RunResult, so those warnings reach no envelope.
			// They are logged instead rather than dropped: the triage and "could not be localized" notices explain the state the failed stage left behind.
			for _, w := range warnings {
				logger.Warn("websterengine: integration stage warning", "warning", w)
			}
			return RunResult{}, err
		}
		runResult.Warnings = append(runResult.Warnings, warnings...)
		// A regression demotes Master's done to stuck; Master's own stuck keeps its own reason.
		// outcome.yaml is never rewritten.
		if stuckReason != "" && runResult.Outcome == outcomeDone {
			runResult.Outcome = outcomeStuck
			runResult.StuckReason = stuckReason
		}
		// Every warning recorded this run, at record-batch or at run exit, reaches summary.md once, whatever the outcome;
		// a missing summary on a non-done outcome skips the section.
		if err := appendRecordedAuditWarnings(deps, batches, summaryPath); err != nil {
			return RunResult{}, err
		}
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
// The run-exit audit's warnings ride RunResult.Warnings, and its stuck reason demotes a done outcome to outcomeStuck.
func mapMasterDone(deps RunDeps, batches []batcher.Batch, outcomePath, summaryPath string, result shuttleengine.Result) (RunResult, error) {
	outcome, err := parseOutcome(outcomePath)
	if err != nil {
		// Run archives a stale outcome.yaml at entry, so a re-run starts a fresh Master that writes a new one.
		return RunResult{}, fmt.Errorf("%w; way forward: re-run `lyx webster run`; the stale file is archived and a fresh Master writes a new one", err)
	}

	var summaryTitle string
	var auditWarnings []string
	if outcome.Outcome == outcomeDone {
		// Required: a done run with a missing or malformed summary.md is a
		// hard error, never guessed — the artifact is the future
		// loom-finalize PR-text source.
		summary, err := summaryparser.Parse(summaryPath)
		if err != nil {
			return RunResult{}, fmt.Errorf("webster: run reached outcome: done but summary.md is missing or malformed: %w%s", err, runExitWayForward)
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

		var auditStuck string
		auditWarnings, auditStuck, err = runExitAuditCrossCheck(deps, outcomePath, summaryPath, result)
		if err != nil {
			return RunResult{}, err
		}
		// An undispositioned correctness finding demotes Master's done to stuck;
		// outcome.yaml is never rewritten.
		if auditStuck != "" {
			outcome.Outcome = outcomeStuck
			outcome.StuckReason = auditStuck
		}
	} else if summary, err := summaryparser.Parse(summaryPath); err == nil {
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
		Warnings:     auditWarnings,
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
		return fmt.Errorf("webster: run reached outcome: done but no state.json exists — no batch was ever recorded%s", runExitWayForward)
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
		return fmt.Errorf("webster: run reached outcome: done but %d batch(es) lack a terminal done record: %s — a batch was begun without being recorded done, or Master claimed done prematurely%s", len(offenders), strings.Join(offenders, ", "), runExitWayForward)
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
// recorded without its fork surviving audit;
// both stay errors.
//
// Findings are then dispositioned like record-batch's: every identity the ledger already holds is dropped, because the whole-session parent audit repeats every finding an earlier record-batch warned on or failed a batch for.
// A policy finding nobody dispositioned is recorded as a run-level warning (saved to state.json before the lease is released) and its text is returned in warnings.
// A correctness finding nobody dispositioned yields stuckReason, which names each suspect path and the git way forward;
// it is recorded in State.PendingAuditFindings, not dispositioned, and blocks run entry until AcceptPendingAudit clears it.
// The outcome file stays on disk for diagnosis (Run never removes it).
func runExitAuditCrossCheck(deps RunDeps, outcomePath, summaryPath string, result shuttleengine.Result) (warnings []string, stuckReason string, err error) {
	if result.ForkAudit == nil {
		return nil, "", fmt.Errorf("webster: run reached outcome: done on a fork-authorized master spawn but its whole-session fork audit did not complete (nil ForkAudit) — this is fail-loud, never skipped%s", runExitWayForward)
	}

	mutateLock, err := AcquireStateMutation(deps.Geom.ScratchDir)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = mutateLock.Release() }()

	// Reload state fresh: begin-batch/record-batch mutated and persisted it
	// repeatedly across Master's whole run, so the in-memory copy captured
	// before Master ever spawned is stale by run-exit.
	st, err := LoadState(deps.Geom.WebsterDir, deps.Geom.ScratchDir)
	if err != nil {
		return nil, "", err
	}
	if st == nil {
		return nil, "", fmt.Errorf("webster: run-exit audit cross-check: no state.json to disposition findings against%s", runExitWayForward)
	}

	planDirs, err := planDirSpellings(deps.Geom)
	if err != nil {
		return nil, "", err
	}
	var candidates []AuditViolation
	candidates = append(candidates, CheckParent(*result.ForkAudit, outcomePath, summaryPath, deps.Geom.WorktreeRoot, deps.RefMatcher)...)
	for _, f := range result.ForkAudit.Forks {
		candidates = append(candidates, CheckFork(f, outcomePath, summaryPath, deps.Geom.WorktreeRoot, planDirs, deps.RefMatcher)...)
	}

	// Classification is the only fallible step and runs before any mutation.
	var policy, correctness []classifiedFinding
	for _, v := range candidates {
		id := findingIdentity(result.SessionID, v)
		if isDispositioned(st, id) {
			continue
		}
		severity, err := ClassifyViolation(v, deps.Geom)
		if err != nil {
			return nil, "", err
		}
		cf := classifiedFinding{ID: id, Violation: v}
		if severity == AuditSeverityCorrectness {
			correctness = append(correctness, cf)
		} else {
			policy = append(policy, cf)
		}
	}

	begun := countBegunForkBatches(st, result.SessionID)
	audited := len(result.ForkAudit.Forks)
	if audited < begun {
		return nil, "", fmt.Errorf("webster: run-exit audit cross-check: %d audited fork transcript(s) is fewer than %d begun fork batch(es) — a batch was recorded without its fork surviving audit%s", audited, begun, runExitWayForward)
	}

	for _, cf := range policy {
		if text, added := recordRunWarning(st, cf.ID, string(cf.Violation.Class), cf.Violation.Detail); added {
			warnings = append(warnings, text)
		}
	}
	if len(warnings) > 0 {
		if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
			return nil, "", err
		}
	}

	if len(correctness) > 0 {
		details := make([]string, len(correctness))
		var paths []string
		seen := map[string]bool{}
		for i, cf := range correctness {
			details[i] = cf.Violation.Detail
			pending := PendingAuditFinding{ID: cf.ID, Class: string(cf.Violation.Class), Detail: cf.Violation.Detail}
			if p := cf.Violation.Path; p != "" {
				pending.Paths = []string{p}
				if !seen[p] {
					seen[p] = true
					paths = append(paths, p)
				}
			}
			if !hasPendingFinding(st, cf.ID) {
				st.PendingAuditFindings = append(st.PendingAuditFindings, pending)
			}
		}
		if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
			return nil, "", err
		}
		pathList := "none named"
		if len(paths) > 0 {
			pathList = strings.Join(paths, ", ")
		}
		pathless := false
		for _, cf := range correctness {
			pathless = pathless || cf.Violation.Path == ""
		}
		stuckReason = fmt.Sprintf("run-exit audit found %d correctness finding(s): %s; suspect paths: %s; way forward: restore the named paths to the last batch head with git, then run \"lyx webster accept-audit\" and re-step the Webster row (lyx webster run)%s", len(correctness), strings.Join(details, "; "), pathList, pathlessClause(pathless))
	}

	return warnings, stuckReason, nil
}

// hasPendingFinding reports whether st already carries a pending finding with identity id.
func hasPendingFinding(st *State, id string) bool {
	for _, f := range st.PendingAuditFindings {
		if f.ID == id {
			return true
		}
	}
	return false
}

// ErrPendingAuditFindings is the sentinel Run returns while run-exit correctness findings are pending.
var ErrPendingAuditFindings = errors.New("webster: correctness findings from an earlier run exit are pending")

// pendingAuditFindingsError wraps ErrPendingAuditFindings with the pending details and suspect paths.
func pendingAuditFindingsError(pending []PendingAuditFinding) error {
	details := make([]string, len(pending))
	var paths []string
	seen := map[string]bool{}
	for i, f := range pending {
		details[i] = f.Detail
		for _, p := range f.Paths {
			if !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	pathList := "none named"
	if len(paths) > 0 {
		pathList = strings.Join(paths, ", ")
	}
	pathless := false
	for _, f := range pending {
		pathless = pathless || len(f.Paths) == 0
	}
	return fmt.Errorf("%w: %d correctness finding(s) from an earlier run exit are pending: %s; suspect paths: %s; way forward: restore the named paths to the last batch head with git, then run \"lyx webster accept-audit\"%s", ErrPendingAuditFindings, len(pending), strings.Join(details, "; "), pathList, pathlessClause(pathless))
}

// freshPendingDrop decides whether opts.Fresh discards st's pending audit findings, and returns one warning per dropped finding.
// It refuses with ErrPendingAuditFindings while a suspect path outside the plan directory still differs from the run's start commit.
// When no batch recorded a start, the worktree's HEAD stands in for it.
// A plan file differs by design, and an unverifiable path or a pathless finding is dropped with the archived state.
func freshPendingDrop(geom Geometry, st *State, opts RunOptions) (drop bool, warnings []string, err error) {
	if !opts.Fresh || st == nil || len(st.PendingAuditFindings) == 0 {
		return false, nil, nil
	}
	base := runStartCommit(st)
	if base == "" {
		head, err := headSHA(geom.WorktreeRoot)
		if err != nil {
			return false, nil, err
		}
		base = head
	}
	planDir, err := canonicalPath(geom.PlanDir)
	if err != nil {
		return false, nil, err
	}
	var paths []string
	seen := map[string]bool{}
	for _, f := range st.PendingAuditFindings {
		for _, p := range f.Paths {
			canon, err := canonicalPath(resolveWritePath(geom.WorktreeRoot, p))
			if err != nil {
				return false, nil, err
			}
			if !seen[p] && !pathWithin(planDir, canon) {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	differing, _, err := checkSuspectPaths(geom, st, base, paths)
	if err != nil {
		return false, nil, err
	}
	if len(differing) > 0 {
		return false, nil, fmt.Errorf("%w: --fresh would drop pending audit findings while their suspect paths still differ from the run's start commit %s: %s; way forward: reset the branch to %s with git, then re-run \"lyx webster run --fresh\"", ErrPendingAuditFindings, base, strings.Join(differing, ", "), base)
	}
	for _, f := range st.PendingAuditFindings {
		warnings = append(warnings, fmt.Sprintf("--fresh dropped pending audit finding %s: %s", f.ID, f.Detail))
	}
	return true, warnings, nil
}

// pathlessClause is the way-forward clause for a pending finding that names no path, or "" when every finding names one.
func pathlessClause(pathless bool) string {
	if !pathless {
		return ""
	}
	return "; a finding with no path clears only through \"lyx webster run --fresh\" after resetting the branch to the run's start commit"
}

// appendRecordedAuditWarnings reloads state and appends every recorded audit warning to summary.md as its "Audit warnings" section.
// It is a no-op when nothing was recorded or summary.md does not exist.
func appendRecordedAuditWarnings(deps RunDeps, batches []batcher.Batch, summaryPath string) error {
	st, err := LoadState(deps.Geom.WebsterDir, deps.Geom.ScratchDir)
	if err != nil {
		return err
	}
	if st == nil {
		return nil
	}
	recorded := RecordedAuditWarnings(st, batches)
	if len(recorded) == 0 {
		return nil
	}
	if _, err := os.Stat(summaryPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("webster: stat summary %s: %w", summaryPath, err)
	}
	return AppendAuditWarnings(deps.Geom.WebsterDir, recorded)
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
// re-confirmation, mirroring the run-exit audit's own backstop posture).
// On a FAILED report it triages the failure regardless of Master's own outcome: it reruns the verify once at head, compares the remaining failures against the earliest batch start commit,
// and records the classification (flaky, pre-existing, or regression) and the failing identities in the integration report.
// Only a regression is escalated: the in-process SHA-bisect runs over the regressing identities across every batch's own accumulated BatchState.CardSHAs, the reserved -1 record and the summary.md section are written,
// and the returned stuck reason is non-empty so Run demotes a Master done to stuck.
// A flaky or pre-existing verdict leaves Master's outcome untouched and is recorded as warnings, a summary.md triage section, and a friction note.
// The returned error is reserved for infrastructure failures and the missing-report-under-done inconsistency.
// When deps.OpenBisector is nil, this mode has no fabric repo to bisect against: the localization path (BisectAndEscalate/bisect) is bypassed entirely — never pushed down into those functions —
// and a regression records an unlocalized "unknown"/"unknown" failure instead, with the returned warning explaining why;
// triage itself then has no baseline to compare against either.
// This bypass is deliberate: bisect's own empty-SHA fallback is unreachable here because card SHAs accumulate normally, a single accumulated SHA would make it record a real SHA under a "cannot localize" claim,
// and two or more reach repo.CurrentBranch(), which nil-pointer panics on a nil bisector.
func runIntegrationStage(deps RunDeps, plan *planparser.Plan, batches []batcher.Batch, masterOutcome string) (warnings []string, stuckReason string, err error) {
	if !ShouldRunIntegration(plan) {
		return nil, "", nil
	}
	// A run whose batches are not all terminal-done never reached the
	// integration stage in the first place (Master's own bracket
	// instruction only spawns the integration fork after every batch is
	// done) — this is expected, not a failure, so the error here is
	// swallowed rather than propagated.
	if err := verifyEveryBatchDone(deps.Geom.WebsterDir, deps.Geom.ScratchDir, batches); err != nil {
		return nil, "", nil
	}

	clk := deps.Clock
	if clk == nil {
		clk = realClock{}
	}
	await, err := AwaitIntegration(deps.Geom.ReportsDir, time.Duration(DefaultAwaitWaitS)*time.Second, clk)
	if err != nil {
		return nil, "", err
	}
	if !await.ReportPresent {
		// A done outcome CLAIMS a passing integration suite, so a missing
		// report is a real inconsistency — fail loud. A non-done outcome with
		// no report is instead consistent (the integration fork died, or
		// Master stuck out before the stage) — Master's own graceful judgment
		// is the run's result, and erroring here would overwrite it.
		if masterOutcome == outcomeDone {
			return nil, "", fmt.Errorf("webster: run reached outcome: done on a plan with a \"## verify:\" section but its integration report never landed — the integration fork never ran or never reported%s", runExitWayForward)
		}
		return nil, "", nil
	}

	reportPath := IntegrationReportPath(deps.Geom.ReportsDir)
	report, err := ParseIntegrationReport(reportPath)
	if err != nil {
		return nil, "", err
	}
	if report.Status == ReportStatusOK {
		// Master's own "done" outcome already reflects a passing integration
		// suite correctly; nothing further to escalate.
		return nil, "", nil
	}

	// This stage runs in three phases, split exactly the way recover-batch splits its own, and for the same reason:
	// the triage rerun, the baseline run and the localization in the middle run the plan's whole "## verify:" command several times — minutes to tens of minutes —
	// and AcquireStateMutation's contract forbids holding the lease across a long block.
	// Holding it there stalled every concurrent bracket verb behind an unbounded blocking acquire, with no timeout and no diagnostic.
	//
	// Phase 1, unleased: read the card SHAs the search runs over and the baseline commit.
	// LoadState is a plain read, and a fresh one is required — begin-batch/record-batch mutated and persisted state repeatedly across Master's whole run,
	// so the copy captured before Master ever spawned is long stale.
	st, err := LoadState(deps.Geom.WebsterDir, deps.Geom.ScratchDir)
	if err != nil {
		return nil, "", err
	}
	if st == nil {
		return nil, "", fmt.Errorf("webster: integration stage: no state.json to escalate against")
	}
	shas, labels := accumulatedCardSHAs(batches, st)
	// Triage's baseline is the earliest of every batch's start commit, not the first batch's:
	// begin-batch does not enforce execution order, so a later batch may have begun, and landed, first.
	// recover-batch inherits StartSHA and a begin-batch re-begin keeps it, so each survives every retry.
	startSHAs := batchStartSHAs(batches, st)

	// Phase 2, unleased: triage, then localize the offending card of a regression.
	// "unknown" for both is the honest answer when there is no fabric repo to bisect against,
	// and the bypass lives at this call site rather than inside LocalizeIntegrationFailure so a nil bisector is never handed to it — see RunDeps.OpenBisector's own doc comment.
	var bisector FabricBisector
	if deps.OpenBisector != nil {
		bisector, err = deps.OpenBisector()
		if err != nil {
			return nil, "", err
		}
	}
	outcome, err := triageIntegrationFailure(runVerifyCapture, bisector, startSHAs, plan.Verify, deps.Geom.WorktreeRoot, deps.Geom.ScratchDir, IntegrationLogPath(deps.Geom.ScratchDir))
	if err != nil {
		return nil, "", err
	}
	warnings = append(warnings, outcome.Warnings...)
	warnings = append(warnings, triageWarnings(outcome.Triage)...)

	regression := outcome.Triage.Verdict == TriageVerdictRegression
	offendingCard, offendingSHA := "unknown", "unknown"
	if regression {
		if bisector == nil {
			warnings = append(warnings, "the integration suite failed and the offending card could not be localized because this mode has no fabric repo to bisect against")
		} else {
			offendingCard, offendingSHA, err = LocalizeIntegrationFailure(bisector, shas, labels, plan.Verify, deps.Geom.WorktreeRoot, outcome.Triage.Regressions)
			if err != nil {
				return warnings, "", err
			}
		}
	}

	// Phase 3, leased: record the result against a state reloaded fresh under the lease,
	// since the unleased work above gave every concurrent verb room to persist its own mutations.
	regressing := failuresByID(outcome.Failures, outcome.Triage.Regressions)
	if err := recordTriageResult(deps, reportPath, outcome, regressing, offendingCard, offendingSHA); err != nil {
		return warnings, "", err
	}

	if !regression {
		logger.Warn("websterengine: integration verify failed without a regression", "verdict", outcome.Triage.Verdict, "flaky", outcome.Triage.Flaky, "preExisting", outcome.Triage.PreExisting)
		return warnings, "", nil
	}
	return warnings, triageStuckReason(regressing, offendingCard), nil
}

// recordTriageResult is runIntegrationStage's leased phase: it reloads the integration report and writes the triage's failures and classification into it,
// then either escalates a regression (the reserved -1 record and the summary.md section) or records a non-regression (the summary.md triage section and the friction note, with no state record).
// The lease is released on return, so the caller's logging runs outside it.
func recordTriageResult(deps RunDeps, reportPath string, outcome triageOutcome, regressing []IntegrationFailure, offendingCard, offendingSHA string) error {
	mutateLock, err := AcquireStateMutation(deps.Geom.ScratchDir)
	if err != nil {
		return err
	}
	defer func() { _ = mutateLock.Release() }()

	st, err := LoadState(deps.Geom.WebsterDir, deps.Geom.ScratchDir)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("webster: integration stage: no state.json to escalate against")
	}

	report, err := ParseIntegrationReport(reportPath)
	if err != nil {
		return err
	}
	report.Failures = outcome.Failures
	report.Triage = &outcome.Triage
	if err := WriteIntegrationReport(reportPath, report); err != nil {
		return err
	}

	if outcome.Triage.Verdict == TriageVerdictRegression {
		RecordIntegrationFailure(st, offendingCard, offendingSHA)
		if err := AppendIntegrationFailure(deps.Geom.WebsterDir, offendingCard, offendingSHA, regressing); err != nil {
			return err
		}
	} else {
		if err := AppendIntegrationTriage(deps.Geom.WebsterDir, outcome.Triage.Flaky, outcome.Triage.PreExisting); err != nil {
			return err
		}
		// The friction note is best-effort, like the rest of the friction plumbing:
		// the report and summary section already carry the verdict, so a failed note must not fail the run.
		if err := writeTriageFrictionNote(deps.FrictionDir, outcome.Triage); err != nil {
			logger.Warn("websterengine: triage friction note not written", "cause", err)
		}
	}

	return SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st)
}

// failuresByID returns the members of fs whose ID is in ids, in fs order.
func failuresByID(fs []IntegrationFailure, ids []string) []IntegrationFailure {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var out []IntegrationFailure
	for _, f := range fs {
		if want[f.ID] {
			out = append(out, f)
		}
	}
	return out
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

// batchStartSHAs returns every batch's recorded non-empty StartSHA, in batches order.
// A batch with no record or no StartSHA contributes nothing.
func batchStartSHAs(batches []batcher.Batch, st *State) []string {
	var starts []string
	for _, b := range batches {
		number, _ := batchIdentity(b)
		if bs := st.Batches[number]; bs != nil && bs.StartSHA != "" {
			starts = append(starts, bs.StartSHA)
		}
	}
	return starts
}
