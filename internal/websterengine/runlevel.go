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
	"slices"
	"sort"
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

// masterAwaitedShellPrefix is the background shell Master's wait treats like a fork:
// the backgrounded recovery verb of the failure ladder.
const masterAwaitedShellPrefix = "lyx webster recover-batch"

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
// A shuttleengine.GateSpec rides beside the Spec: it is the list of mechanical validators the Master run's own Wait consults before it may finalize, and an empty list means ungated, which is what the Webster row supplies today.
// The seam is widened rather than joined by a second
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

	// Gate is the list of mechanical validators Master's own shuttle run is held to:
	// Run hands it to StartMaster beside the Spec, and the run's Wait re-prompts the live Master on a failed verdict until it passes or the failing entry's re-prompt budget is exhausted.
	// An empty list means ungated, which is what every caller supplies until a Webster validator exists.
	Gate shuttleengine.GateSpec

	// FrictionDir is the told absolute friction directory (see internal/friction), empty when Tier 2
	// is off. It lives here rather than on Geometry because internal/hubgeom and
	// internal/standalonegeom are the Told-Geometry Invariant's only Geometry-struct constructors and
	// this value needs no geometry derivation.
	FrictionDir string

	// ParentBranch names the branch the run merges its parent in from, for the fix commit check's clean-parent-merge rule.
	// It is nil in standalone mode, so no merge commit is accepted there.
	ParentBranch ParentBranchFunc
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
	// taken verbatim from the parsed outcome.yaml — except that a done whose verify gate did not pass is demoted to outcomeStuck.
	Outcome string
	// StuckReason is the parsed outcome.yaml's stuck_reason, verbatim — except for a done demoted by the verify gate,
	// whose reason names the failing identities and the attempts spent.
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
	// Warnings carries every non-fatal observation accumulated this run — never a failure.
	// Mirrors RecordResult.Warnings' shape and contract: the verify gate's flaky notice, and the sequencing and audit warnings.
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

// reclaimEntryTimeStrands stops the only substrates a crashed or killed `run` process can ever leave live behind it:
// Master's own recorded strand and any recorded, non-terminal recovery-batch strand.
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
// and an undispositioned correctness finding demotes the outcome to stuck, the same way a done whose verify gate did not pass does.
// Run hands Merriam's spawn the plan-level verify as a must-pass gate entry named `verify` (NewVerifyGate), so a red tree re-prompts Merriam's live session and a gate that never passes ends the run stuck.
// Every recorded audit warning is appended to summary.md's "Audit warnings" section when that file exists.
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
	// Run adds the `verify` entry itself, so a caller's gate naming one would run beside it.
	for _, e := range deps.Gate {
		if e.Name == verifyGateName {
			return RunResult{}, fmt.Errorf("webster: RunDeps.Gate already names an entry %q; Run adds the plan-level verify gate itself; way forward: drop the %q entry from the Webster row's gates", verifyGateName, verifyGateName)
		}
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
	// function (the Master prompt, mapMasterDone) then
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

	freshDrop, freshWarnings, err := freshPendingDrop(deps.Engine, deps.Geom, st, opts)
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
		if err := storePlanBaseline(deps.Geom.WebsterDir, deps.Geom.PlanDir, fileHashes); err != nil {
			return RunResult{}, err
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
		if err := storePlanBaseline(deps.Geom.WebsterDir, deps.Geom.PlanDir, fileHashes); err != nil {
			return RunResult{}, err
		}
		if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
			return RunResult{}, err
		}
	}

	if len(st.PendingAuditFindings) > 0 {
		return RunResult{}, pendingAuditFindingsError(deps.Engine, st, deps.Geom)
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
	// The scope here is DispatchScope, not completedCards: a batch begun but not recorded may already have landed its work, or not yet,
	// and its forthcoming Create targets stay out of the status check.
	begun, forthcoming := DispatchScope(batches, st)
	findings, err := planglyph.ValidateDispatch(plan, deps.Geom.WorktreeRoot, begun, forthcoming)
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

	// A verify-gate report left by an earlier run describes an earlier attempt and would be read as this run's.
	verifyGateReportPath, err := filepath.Abs(VerifyGateReportPath(deps.Geom.ReportsDir))
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: resolve verify-gate report path: %w", err)
	}
	if err := os.Remove(verifyGateReportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return RunResult{}, fmt.Errorf("webster: remove stale verify-gate report: %w", err)
	}

	// The fixer fork's prompt is Go-rendered and Go-written up front for the same reason as a batch fork's:
	// Merriam may write nothing but its two contract files.
	verifyFixNotePath := friction.NotePath(deps.FrictionDir, "webster-verify-fix")
	verifyFixPrompt, err := RenderVerifyFixPrompt(verifyGateReportPath, deps.Geom.WorktreeRoot, deps.Geom.PlanDir, deps.Geom.StencilsDir, outcomePath, summaryPath, verifyFixNotePath)
	if err != nil {
		return RunResult{}, err
	}
	if err := os.MkdirAll(deps.Geom.PromptsDir, 0o755); err != nil {
		return RunResult{}, fmt.Errorf("webster: create prompts dir %s: %w", deps.Geom.PromptsDir, err)
	}
	verifyFixPromptPath, err := filepath.Abs(filepath.Join(deps.Geom.PromptsDir, verifyFixPromptFileName))
	if err != nil {
		return RunResult{}, fmt.Errorf("webster: resolve verify-fix prompt path: %w", err)
	}
	if err := os.WriteFile(verifyFixPromptPath, verifyFixPrompt, 0o644); err != nil {
		return RunResult{}, fmt.Errorf("webster: write verify-fix prompt %s: %w", verifyFixPromptPath, err)
	}

	masterNotePath := friction.NotePath(deps.FrictionDir, "webster-master")
	prompt, err := RenderMasterPrompt(batches, st, outcomePath, summaryPath, verifyFixPromptPath, deps.Geom.PlanDir, deps.Config.SelfFixCap, deps.Geom.WorktreeRoot, deps.Geom.RepoRoot, deps.Geom.StencilsDir, masterNotePath)
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
		// The failure ladder backgrounds recover-batch and recovery_timeout_min bounds it, so the gate waits on it like a fork.
		AwaitedShellPrefixes: []string{masterAwaitedShellPrefix},
		Role:                 MerriamStrandRole,
		Interactive:          false,
		Timeout:              time.Duration(deps.Config.MasterTimeoutMin) * time.Minute,
	}

	// The state-mutation lease acquired above is held across this call, which now includes the
	// provider's startup window (bounded by startup_timeout_s) — see AcquireStateMutation's own
	// contract. At run entry no batch forks exist yet, so the hold stalls nothing in practice.
	verifyGate, gateNotes := NewVerifyGate(deps.Geom, deps.Config.VerifyGateAttempts, batches, deps.ParentBranch, deps.FrictionDir)
	gate := append(slices.Clone(deps.Gate), shuttleengine.GateEntry{Name: verifyGateName, Gate: verifyGate, Attempts: deps.Config.VerifyGateAttempts})
	handle, err := deps.Starter.StartMaster(spec, gate)
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

	// The note is best-effort and written whatever the outcome, so a hang's evidence outlives a non-done run.
	if err := writeBackgroundShellFrictionNote(deps.FrictionDir, result.ExpiredShells, deps.ShuttleCfg.BackgroundShellWaitMin); err != nil {
		logger.Warn("websterengine: background shell friction note not written", "err", err)
	}

	switch result.Outcome {
	case shuttleengine.OutcomeDone:
		runResult, mapErr := mapMasterDone(deps, batches, outcomePath, summaryPath, result)
		if mapErr != nil {
			return RunResult{}, mapErr
		}
		// Cycles are always informational: prepend one warning per cycle
		// ahead of the verify gate's own warnings below,
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
		// The plan-level verify ran as a gate on Master's own session, so a flaky pass reaches the run here, after the wait.
		flakyWarnings, err := gateNotes.Apply(deps.Geom.WebsterDir)
		if err != nil {
			return RunResult{}, err
		}
		runResult.Warnings = append(runResult.Warnings, flakyWarnings...)
		// A done outcome reports each shell the wait counted a turn end past;
		// mapMasterDone has already required its summary.md.
		if runResult.Outcome == outcomeDone {
			for _, label := range result.ExpiredShells {
				runResult.Warnings = append(runResult.Warnings, expiredShellWarning(label))
			}
			if err := AppendBackgroundShells(deps.Geom.WebsterDir, result.ExpiredShells); err != nil {
				return RunResult{}, err
			}
		}
		// A done whose verify gate did not pass ends stuck;
		// Master's own stuck keeps its own reason.
		// outcome.yaml is never rewritten.
		if result.Gate != nil && !result.Gate.Passed && runResult.Outcome == outcomeDone {
			runResult.Outcome = outcomeStuck
			runResult.StuckReason = verifyGateStuckReason(deps.Geom.ReportsDir, result.Gate)
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
// A correctness finding nobody dispositioned yields stuckReason, which names each suspect path and the way forward pendingPathsWayForward builds;
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
	websterDirs, err := websterDirSpellings(deps.Geom)
	if err != nil {
		return nil, "", err
	}
	var candidates []AuditViolation
	candidates = append(candidates, CheckParent(*result.ForkAudit, outcomePath, summaryPath, deps.Geom.WorktreeRoot, deps.RefMatcher)...)
	for _, f := range result.ForkAudit.Forks {
		candidates = append(candidates, CheckFork(f, outcomePath, summaryPath, deps.Geom.WorktreeRoot, planDirs, websterDirs, forkOwnReport(st, deps.Geom, f.TranscriptPath), deps.RefMatcher)...)
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
		writes, err := contractWritesFor(deps.Engine, st, deps.Geom, paths)
		if err != nil {
			return nil, "", err
		}
		wayForward, err := pendingPathsWayForward(deps.Geom, writes, paths, pathless, " and re-step the Webster row (lyx webster run)")
		if err != nil {
			return nil, "", err
		}
		stuckReason = fmt.Sprintf("run-exit audit found %d correctness finding(s): %s; suspect paths: %s; way forward: %s", len(correctness), strings.Join(details, "; "), pathList, wayForward)
	}

	return warnings, stuckReason, nil
}

// forkOwnReport returns the report the fork behind transcript may write:
// the report of the batch whose ForkTranscripts holds it, or the verify-gate report when no batch does,
// since only the verify-gate fixer fork runs outside a batch bracket.
func forkOwnReport(st *State, geom Geometry, transcript string) string {
	for number, bs := range st.Batches {
		if bs != nil && slices.Contains(bs.ForkTranscripts, transcript) {
			return filepath.Join(geom.ReportsDir, ReportFileName(number, bs.Slug))
		}
	}
	return VerifyGateReportPath(geom.ReportsDir)
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
// Its way forward is pendingPathsWayForward's, judged over the write history engine's audit of st's sessions yields.
// The error from sorting the paths is returned as is.
func pendingAuditFindingsError(engine shuttleengine.Engine, st *State, geom Geometry) error {
	pending := st.PendingAuditFindings
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
	writes, err := contractWritesFor(engine, st, geom, paths)
	if err != nil {
		return err
	}
	wayForward, err := pendingPathsWayForward(geom, writes, paths, pathless, "")
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: %d correctness finding(s) from an earlier run exit are pending: %s; suspect paths: %s; way forward: %s", ErrPendingAuditFindings, len(pending), strings.Join(details, "; "), pathList, wayForward)
}

// pendingPathsWayForward is the way-forward text for pending findings naming paths, followed by tail.
// A finding with no path, or a path outside the plan directory that is not in the task worktree's tracked tree (see trackedRel), clears only through run --fresh,
// so that route is then the whole way forward:
// accept-audit refuses every finding while any one of them cannot be checked.
// Otherwise the text ends in "lyx webster accept-audit".
// A plan path never gets the git clause, which cannot restore it:
// it gets planPathClause instead,
// and the git clause covers only the other paths.
// The error is a link-resolution or git probe failure.
func pendingPathsWayForward(geom Geometry, writes RunWrites, paths []string, pathless bool, tail string) (string, error) {
	contracts, err := splitContractPaths(geom, writes, paths)
	if err != nil {
		return "", err
	}
	plan, rest, err := splitPlanPaths(geom, contracts.Rest)
	if err != nil {
		return "", err
	}
	var unchecked []string
	for _, p := range rest {
		_, ok, err := trackedRel(geom.WorktreeRoot, p)
		if err != nil {
			return "", err
		}
		if !ok {
			unchecked = append(unchecked, p)
		}
	}
	if pathless || len(unchecked) > 0 {
		var why []string
		if len(unchecked) > 0 {
			why = append(why, "nothing the run recorded can check "+strings.Join(unchecked, ", "))
		}
		if pathless {
			why = append(why, "a finding names no path")
		}
		return fmt.Sprintf("reset the branch to the run's start commit with git and run \"lyx webster run --fresh\"%s, since %s", tail, strings.Join(why, " and ")), nil
	}
	var clauses []string
	if len(contracts.Uncleared) > 0 {
		clauses = append(clauses, fmt.Sprintf("delete %s with rm, since a fork wrote it after Master's last write, then run \"lyx webster accept-audit\"%s", strings.Join(contracts.Uncleared, ", "), tail))
	}
	if len(rest) > 0 {
		if len(plan) == 0 {
			clauses = append(clauses, "restore the named paths to the last batch head with git, then run \"lyx webster accept-audit\""+tail)
		} else {
			clauses = append(clauses, "restore the paths other than the plan files to the last batch head with git, then run \"lyx webster accept-audit\""+tail)
		}
	}
	if len(plan) > 0 {
		clauses = append(clauses, fmt.Sprintf("for the plan file(s) %s, %s", strings.Join(plan, ", "), planPathClause("\"lyx webster accept-audit\""+tail)))
	}
	if len(clauses) == 0 {
		return "run \"lyx webster accept-audit\"" + tail, nil
	}
	return strings.Join(clauses, "; "), nil
}

// freshPendingDrop decides whether opts.Fresh discards st's pending audit findings, and returns one warning per dropped finding.
// It refuses with ErrPendingAuditFindings, before anything is archived, in three cases:
// a suspect path outside the plan directory still differs from the run's start commit;
// the worktree's HEAD is not the start commit, so an unaudited commit would become the new run's base;
// a plan path differs from the plan the run recorded and restore-plan can undo that, either because the store holds the recorded copy or because the file was never recorded.
// The start commit is picked by git ancestry,
// and a recorded commit missing from the repository refuses with the fetch way forward.
// When no batch recorded a start, the worktree's HEAD stands in for it.
// When starts are recorded but none is an ancestor of all the others, HEAD stands in only while it is an ancestor of every recorded start (headBeforeEveryStart).
// An unverifiable path, a pathless finding and a differing plan path whose recorded copy is missing from the store are dropped with the archived state,
// since no verb could restore the last;
// its warning says so.
// A batch record with Uncheckable entries counts as a pending finding: its SuspectPaths join the path check,
// and it adds its own drop warning.
func freshPendingDrop(engine shuttleengine.Engine, geom Geometry, st *State, opts RunOptions) (drop bool, warnings []string, err error) {
	if !opts.Fresh || st == nil {
		return false, nil, nil
	}
	var uncheckableBatches []int
	for n, bs := range st.Batches {
		if bs != nil && len(bs.Uncheckable) > 0 {
			uncheckableBatches = append(uncheckableBatches, n)
		}
	}
	sort.Ints(uncheckableBatches)
	if len(st.PendingAuditFindings) == 0 && len(uncheckableBatches) == 0 {
		return false, nil, nil
	}
	bases, err := runEvidenceBases(geom.WorktreeRoot, st)
	if err != nil {
		return false, nil, err
	}
	if len(bases.Missing) > 0 {
		return false, nil, fmt.Errorf("%w: %s", ErrPendingAuditFindings, missingCommitsClause(bases.Missing))
	}
	head, err := headSHA(geom.WorktreeRoot)
	if err != nil {
		return false, nil, err
	}
	base := bases.Start
	if base == "" {
		if err := headBeforeEveryStart(geom.WorktreeRoot, head, bases.Starts); err != nil {
			return false, nil, err
		}
		base = head
	}
	var allPaths []string
	seen := map[string]bool{}
	for _, f := range st.PendingAuditFindings {
		for _, p := range f.Paths {
			if !seen[p] {
				seen[p] = true
				allPaths = append(allPaths, p)
			}
		}
	}
	for _, n := range uncheckableBatches {
		for _, sp := range st.Batches[n].SuspectPaths {
			if !seen[sp.Path] {
				seen[sp.Path] = true
				allPaths = append(allPaths, sp.Path)
			}
		}
	}
	writes, err := contractWritesFor(engine, st, geom, allPaths)
	if err != nil {
		return false, nil, err
	}
	contracts, err := splitContractPaths(geom, writes, allPaths)
	if err != nil {
		return false, nil, err
	}
	if len(contracts.Uncleared) > 0 {
		return false, nil, fmt.Errorf("%w: --fresh would drop pending audit findings while %s", ErrPendingAuditFindings, contractDeleteClause(contracts.Uncleared, "lyx webster run --fresh"))
	}
	planPaths, paths, err := splitPlanPaths(geom, contracts.Rest)
	if err != nil {
		return false, nil, err
	}
	differing, _, err := checkSuspectPaths(geom, st, base, paths)
	if err != nil {
		return false, nil, err
	}
	if len(differing) > 0 {
		return false, nil, fmt.Errorf("%w: --fresh would drop pending audit findings while their suspect paths still differ from the run's start commit %s: %s; way forward: reset the branch to %s with git, then re-run \"lyx webster run --fresh\"", ErrPendingAuditFindings, base, strings.Join(differing, ", "), base)
	}
	if head != base {
		return false, nil, fmt.Errorf("%w: --fresh would drop pending audit findings while HEAD %s is not the run's start commit %s; way forward: reset the branch to %s with git, then re-run \"lyx webster run --fresh\"", ErrPendingAuditFindings, head, base, base)
	}
	planDiffering, _, err := checkSuspectPaths(geom, st, base, planPaths)
	if err != nil {
		return false, nil, err
	}
	var restorable []string
	noCopy := map[string]bool{}
	for _, p := range planDiffering {
		name, err := planFileName(geom, p)
		if err != nil {
			return false, nil, err
		}
		hash, recorded := st.PlanFileHashes[name]
		if !recorded {
			restorable = append(restorable, p)
			continue
		}
		has, err := planBaselineHas(geom.WebsterDir, hash)
		if err != nil {
			return false, nil, err
		}
		if has {
			restorable = append(restorable, p)
		} else {
			noCopy[p] = true
		}
	}
	if len(restorable) > 0 {
		return false, nil, fmt.Errorf("%w: --fresh would drop pending audit findings while plan file(s) differ from the plan the run recorded: %s; way forward: %s", ErrPendingAuditFindings, strings.Join(restorable, ", "), planPathClause("\"lyx webster run --fresh\""))
	}
	for _, f := range st.PendingAuditFindings {
		w := fmt.Sprintf("--fresh dropped pending audit finding %s: %s", f.ID, f.Detail)
		var lost []string
		for _, p := range f.Paths {
			if noCopy[p] {
				lost = append(lost, p)
			}
		}
		if len(lost) > 0 {
			w += fmt.Sprintf("; plan file(s) %s differ from the recorded plan and their recorded copy is missing from the plan baseline store, so no verb could restore them", strings.Join(lost, ", "))
		}
		warnings = append(warnings, w)
	}
	for _, n := range uncheckableBatches {
		warnings = append(warnings, fmt.Sprintf("--fresh dropped batch %02d's uncheckable findings: %s", n, strings.Join(st.Batches[n].Uncheckable, ", ")))
	}
	return true, warnings, nil
}

// headBeforeEveryStart refuses --fresh with ErrPendingAuditFindings unless head is an ancestor of, or equal to, every one of starts.
// It is the HEAD rule for recorded starts that share no single oldest commit, as after a branch rewritten mid-run:
// no recorded start can be named as the run's,
// but a HEAD that every one of them descends from carries no commit the run made.
// An empty starts passes, since nothing was recorded to compare with.
// The error is an IsAncestor failure or the refusal.
func headBeforeEveryStart(worktree, head string, starts []string) error {
	for _, start := range starts {
		ok, err := isAncestor(worktree, head, start)
		if err != nil {
			return err
		}
		if !ok {
			list := strings.Join(starts, " ")
			return fmt.Errorf("%w: --fresh would drop pending audit findings while the batches' recorded start commits %s share no single oldest commit and HEAD %s is not an ancestor of every one of them; way forward: reset the branch to a commit every recorded start descends from (git merge-base --octopus %s) with git, then re-run \"lyx webster run --fresh\"", ErrPendingAuditFindings, strings.Join(starts, ", "), head, list)
		}
	}
	return nil
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

// accumulatedCardSHAs walks batches in execution order and collects every
// terminal batch's own CardSHAs alongside a matching "NN-slug" label — the
// ordered per-card SHA trail and parallel label set cardHint reads to name
// the cards that touched a failing package. A batch with no persisted record (should never
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
