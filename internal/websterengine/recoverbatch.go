// recoverbatch.go implements webster's re-entrant, blocking exception-path verb as three
// lease-scoped phases: RecoverSpawnOrAttach (the only place webster spawns a genuinely separate
// process — escalating a batch a fork reported stuck, or never reported at all, to a cold
// implementer strand at the recovery role, rendering the SEPARATE, full cold-start recovery prompt
// via RenderRecoveryPrompt — deliberately distinct from RenderForkPrompt's thin in-session fork
// prompt, since the recovery strand inherits no session context, per the fork-context-hygiene
// Shared Decision), RecoverAwait (the bounded wait, over webster's own classification machinery —
// Classify/PollUntilTerminal/ TurnEnded/StrandLive), and PersistRecoveryTerminal (the terminal
// digest merge into a freshly reloaded state).
// First call spawns and records;
// the call that spawns the recovery strand first waits for its provider to come up (normally
// seconds, bounded by startup_timeout_s), and every call then blocks for RecoveryWaitBudget and
// returns the terminal digest;
// recovery_timeout_min bounds every call, since the strand classifies dead on it.
// A running snapshot comes back only when an operator passes a shorter --wait.
//
// The three-phase split exists for the state-mutation lease: the caller holds it across
// spawn-or-attach — now including the spawn's startup window — and across the terminal persist,
// but NEVER across the bounded wait between them (see AcquireStateMutation's contract, held across
// the spawn's startup window but never across a long block).
// Nothing here touches fabric: the caller fabric-commits state.json after the spawn record and again at
// terminal persistence, webster's own fabric-commit-boundary discipline.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrRecoveryNeedsFresh is the sentinel RecoverSpawnOrAttach's refusal of an uncheckable failed batch unwraps to.
var ErrRecoveryNeedsFresh = errors.New("webster: recovery cannot check the batch's findings")

// ErrRecoveryDeleteReferenced is the sentinel RecoverSpawnOrAttach's refusal of a batch whose Delete target an unbegun later card still references unwraps to.
var ErrRecoveryDeleteReferenced = errors.New("webster: recovery cannot clear a delete a later card still references")

// recoveryNeedsFreshError carries the refusal text verbatim and unwraps to ErrRecoveryNeedsFresh.
type recoveryNeedsFreshError struct{ msg string }

func (e *recoveryNeedsFreshError) Error() string { return e.msg }

func (e *recoveryNeedsFreshError) Unwrap() error { return ErrRecoveryNeedsFresh }

// Clock abstracts time.Now/time.Sleep so RecoverBatch's bounded wait runs instantly under test,
// mirroring shuttleengine's wait.go seam and webster's own poll.go clock.
// Clock is deliberately a plain, exported webster-local interface — it structurally satisfies
// poll.go's unexported clock interface (identical Now/Sleep method set), which is what lets
// RecoverBatch hand a Clock value straight to PollUntilTerminal without any adapter: Go interface
// satisfaction is structural, not by declared type identity.
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

// RecoverDeps carries seams RecoverBatch needs: Starter, Plan, Batches, State, Roles, Config,
// Engine, Reed, ShuttleCfg, and Geom, the told Geometry every path is read from.
// Batches is the execution order, the batchifier's own order (ExecutionBatches);
// predecessorDigestLine's lookup depends on Batches already being in that order.
type RecoverDeps struct {
	Starter    Starter
	Plan       *planparser.Plan
	Batches    []batcher.Batch
	State      *State
	Roles      map[Role]modelspec.Resolved
	Config     Config
	Engine     shuttleengine.Engine
	Reed       shuttleengine.ReedOps
	ShuttleCfg shuttleengine.Config
	Geom       Geometry

	// FrictionDir is the told absolute friction directory (see internal/friction), empty when Tier 2
	// is off. It lives here rather than on Geometry because internal/hubgeom and
	// internal/standalonegeom are the Told-Geometry Invariant's only Geometry-struct constructors and
	// this value needs no geometry derivation.
	FrictionDir string

	// ParentBranch names the run's parent branch for the head cross-check's clean-parent-merge rule;
	// nil (standalone mode) accepts no merge commit between the report's head_sha and HEAD.
	ParentBranch ParentBranchFunc
}

// RecoverResult is what one RecoverAwait call hands back: Digest (nil while Running), Running (true if wait elapsed non-terminal), ElapsedS (since spawn), and Warnings (non-fatal substrate-cleanup failures, plus the moved-HEAD notice when only merge commits sit between the report's head_sha and the worktree's HEAD).
type RecoverResult struct {
	Digest   *Digest
	Running  bool
	ElapsedS int
	Warnings []string
}

// archiveStaleReport renames a stale report to free the path, keeping it
// auditable rather than deleting. Absent file returns ("", nil).
func archiveStaleReport(reportsDir string, number int, slug string, now func() time.Time) (string, error) {
	path := filepath.Join(reportsDir, ReportFileName(number, slug))
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("webster: stat batch report %s: %w", path, err)
	}

	const ext = ".yaml"
	base := strings.TrimSuffix(ReportFileName(number, slug), ext)
	stamp := now().UTC().Format(archiveTimestampFormat)
	target, err := firstFreeArchivePath(func(suffix string) string {
		return filepath.Join(reportsDir, fmt.Sprintf("%s-%s%s%s", base, stamp, suffix, ext))
	})
	if err != nil {
		return "", fmt.Errorf("webster: find archive target for batch report %s: %w", path, err)
	}

	if err := os.Rename(path, target); err != nil {
		return "", fmt.Errorf("webster: archive stale batch report %s: %w", path, err)
	}
	return target, nil
}

// refuseRecoveringDoneReport refuses to recover a batch whose report already
// has status: OK (record-batch is the consuming verb), except when prior is
// terminal dead (a late orphan report), terminal failed (a still-running fork's late report,
// or one left over after the failure), or missing/unparseable.
func refuseRecoveringDoneReport(reportsDir string, number int, slug string, prior *BatchState) error {
	// Dead-orphan and failed exceptions: archive a late report written after the terminal classification.
	if prior != nil && prior.Terminal && (prior.Status == DigestStatusDead || prior.Status == DigestStatusFailed) {
		return nil
	}

	reportPath := filepath.Join(reportsDir, ReportFileName(number, slug))
	report, err := ParseReport(reportPath)
	if err != nil {
		// Absent or malformed report: recovery is the path for this.
		return nil
	}
	if report.Status == ReportStatusOK {
		return fmt.Errorf("webster: batch %02d-%s already has a report with status: OK at %s — recover-batch never archives finished work; record it with `lyx webster record-batch %d` instead", number, slug, reportPath, number)
	}
	return nil
}

// failureDigestBlock renders a failed prior record's digest for the recovery prompt: the reasons, which failBatch already ends with the suspect paths.
// It returns "" when prior is not a failed batch.
func failureDigestBlock(prior *BatchState) string {
	if prior == nil || prior.Status != DigestStatusFailed || prior.Digest == nil || len(prior.Digest.Reasons) == 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range prior.Digest.Reasons {
		fmt.Fprintf(&b, "- %s\n", r)
	}
	return strings.TrimRight(b.String(), "\n")
}

// recoverSpawn archives any stale report, stops a live prior strand, renders
// the recovery prompt, and starts the recovery strand, returning a fresh BatchState.
// clk stamps SpawnedAt so elapsed-since-spawn is measured against the same clock.
func recoverSpawn(deps RecoverDeps, batch batcher.Batch, prior *BatchState, prevDigest string, clk Clock) (*BatchState, error) {
	number, slug := batchIdentity(batch)

	cardHashes, err := batchCardHashes(batch, deps.Geom.PlanDir)
	if err != nil {
		return nil, fmt.Errorf("%w; way forward: transient, re-run `lyx webster recover-batch %d`", err, number)
	}

	if err := refuseRecoveringDoneReport(deps.Geom.ReportsDir, number, slug, prior); err != nil {
		return nil, err
	}

	// Ensure reports dir exists so the recovery strand's report write succeeds.
	if err := os.MkdirAll(deps.Geom.ReportsDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create reports dir %s: %w", deps.Geom.ReportsDir, err)
	}

	if _, err := archiveStaleReport(deps.Geom.ReportsDir, number, slug, clk.Now); err != nil {
		return nil, err
	}

	if prior != nil {
		if err := removeStrandIfLive(deps.Reed, prior.StrandGUID); err != nil {
			return nil, err
		}
	}

	batchName := fmt.Sprintf("%02d-%s", number, slug)
	reportPath, err := filepath.Abs(filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug)))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve report path: %w", err)
	}

	notePath := friction.NotePath(deps.FrictionDir, batchName+"-recovery")
	cardGates := renderCardGates(deps.Plan, batch.Cards, masterPlanDirDisplay(deps.Geom.WorktreeRoot, deps.Geom.PlanDir), deps.Geom.WorktreeRoot)
	prompt, err := RenderRecoveryPrompt(batch, cardGates, prevDigest, failureDigestBlock(prior), reportPath, deps.Geom.RepoRoot, deps.Geom.PlanDir, deps.Geom.WorktreeRoot, deps.Geom.StencilsDir, deps.Geom.SpecsDir, deps.Config.SelfFixCap, notePath, deps.Geom.ParentName)
	if err != nil {
		return nil, err
	}

	resolved, ok := deps.Roles[RoleRecovery]
	if !ok {
		return nil, fmt.Errorf("webster: no resolved model-spec for role %q", RoleRecovery)
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{reportPath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Skills:      roleSkills,
		Role:        string(RoleRecovery),
		Segment:     segmentcolor.Webster,
		Round:       batchName,
		Timeout:     time.Duration(deps.Config.RecoveryTimeoutMin) * time.Minute,
	}

	run, err := deps.Starter.Start(spec)
	if err != nil {
		return nil, fmt.Errorf("webster: start recovery strand for batch %s: %w; way forward: transient, re-run `lyx webster recover-batch %d`", batchName, err, number)
	}

	runState, runDir, err := shuttleengine.FindRun(deps.ShuttleCfg, deps.Geom.AnchorRoot, run.StrandGUID())
	if err != nil {
		return nil, fmt.Errorf("webster: resolve spawned recovery run: %w", err)
	}

	head, err := deps.Geom.git().HeadSHA(deps.Geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}

	// The recovery record inherits the ORIGINAL bracket's start SHA when there is one, rather than
	// re-capturing the head at spawn time. A recovery exists because the batch's own fork got stuck,
	// frequently after committing part of its work, so a start SHA captured here would exclude
	// exactly that part — and the post-batch mechanical pass this record's own terminal path runs
	// (see postBatchChecks) computes its single delta from it. A narrower delta reports a symbol the
	// stuck fork already created as never created, which is a bind-count-mismatch against a card
	// whose handle did land. The whole bracket is the honest base commit for this batch's work.
	start := head
	if prior != nil && prior.StartSHA != "" {
		start = prior.StartSHA
	}

	// Recorded audit warnings carry over as well: their identities stay dispositioned in
	// State.AuditDispositions (the once-per-identity rule), so no later call would record them again.
	// The batch's fork transcripts carry over too, so the run-exit audit still knows which report each of those forks owns.
	var priorWarnings []AuditWarning
	var priorSuspects []SuspectPath
	var priorTranscripts []string
	if prior != nil {
		priorWarnings = prior.AuditWarnings
		priorSuspects = prior.SuspectPaths
		priorTranscripts = prior.ForkTranscripts
	}

	return &BatchState{
		Slug:            slug,
		Cards:           batchCardIDs(batch),
		CardHashes:      cardHashes,
		StartSHA:        start,
		AuditWarnings:   priorWarnings,
		SuspectPaths:    priorSuspects,
		ForkTranscripts: priorTranscripts,
		Kind:            "recovery",
		SpawnedAt:       clk.Now().UTC().Format(time.RFC3339),
		StrandGUID:      run.StrandGUID(),
		ShuttleRunDir:   runDir,
		EventsPath:      runState.EventsPath,
		EventsOffset:    runState.PromptOffset,
	}, nil
}

// RecoverSpawnOrAttach decides spawn-or-attach: if a recorded, non-terminal recovery BatchState
// exists, ATTACH and return it;
// otherwise SPAWN fresh.
// Caller persists deps.State via SaveState when spawned is true.
//
// Accepted residuals, both from the spawn call now including the provider's startup window under
// the state-mutation lease, and from state.json being persisted only after that call returns:
//
//  1. A process killed inside the startup window leaves a live recovery strand whose guid was
//     never persisted, so the next call's prior.StrandGUID reclaim (removeStrandIfLive) cannot
//     see it, and the next spawn runs beside it.
//  2. A startup mechanism failure (shuttle could not get a liveness answer from reed
//     maxStatusRetries times) returns an error with the strand left live and no guid persisted,
//     with the same consequence — accepted because tearing it down could kill a working agent, and
//     a reed in that state usually fails the next AddStrand too. The error names the strand guid so
//     an operator can remove it by hand.
//  3. A not-ready start no longer leaks (shuttle tears the strand down) unless that teardown's own
//     strand removal fails, which the returned error then states.
//
// Before spawning, it refuses with ErrRecoveryDeleteReferenced while an unbegun later card's Edit code still references a symbol the batch's own cards delete, since a recovery cannot change the plan and would fail the same done-check again.
// The bound: it refuses only while that check fires for the batch's own cards against the current plan and tree;
// moving the delete and rebaselining clears it.
func RecoverSpawnOrAttach(deps RecoverDeps, batchNumber int, clk Clock) (bs *BatchState, spawned bool, err error) {
	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, false, err
	}

	prior := deps.State.Batches[batchNumber]
	if prior != nil && prior.Kind == "recovery" && !prior.Terminal && prior.StrandGUID != "" {
		return prior, false, nil
	}
	if prior != nil && prior.Terminal && prior.Status == DigestStatusFailed && len(prior.Uncheckable) > 0 {
		writes, err := contractWritesFor(deps.Engine, deps.State, deps.Geom, prior.Uncheckable)
		if err != nil {
			return nil, false, err
		}
		contracts, err := splitContractPaths(deps.Geom, writes, prior.Uncheckable)
		if err != nil {
			return nil, false, err
		}
		if len(contracts.Rest) > 0 {
			var what []string
			for _, entry := range contracts.Rest {
				reason, _, err := uncheckableReason(deps.Geom, deps.State, entry)
				if err != nil {
					return nil, false, err
				}
				if reason == "" {
					reason = reasonNoPath
				}
				what = append(what, fmt.Sprintf("%s (%s)", entry, reason))
			}
			for _, p := range contracts.Uncleared {
				what = append(what, fmt.Sprintf("%s (%s)", p, noteForkWroteLast))
			}
			wayForward := resetToStartSteps(stepRunFresh)
			if len(contracts.Uncleared) == 0 && allPathlessFabricReference(contracts.Rest) {
				wayForward = fmt.Sprintf("way forward: when the worktree is clean and HEAD is the batch's start commit (git reset --keep to it if the batch committed), or the batch's commits are kept and every recorded command is read-only, \"lyx webster accept-audit --batch %d\" then \"lyx webster recover-batch %d\"; otherwise %s", batchNumber, batchNumber, strings.TrimPrefix(resetToStartSteps(stepRunFresh), "way forward: "))
			}
			return nil, false, &recoveryNeedsFreshError{msg: fmt.Sprintf("webster: batch %02d failed on findings recovery cannot check: %s; %s", batchNumber, strings.Join(what, ", "), wayForward)}
		}
		if len(contracts.Uncleared) > 0 {
			return nil, false, fmt.Errorf("webster: batch %02d failed on contract file(s) a fork wrote last: %s", batchNumber, contractDeleteClause(contracts.Uncleared, fmt.Sprintf("lyx webster recover-batch %d", batchNumber)))
		}
	}

	referenced, err := laterDeleteReferenceReasons(deps.Plan, deps.Batches, deps.State, batch.Cards, deps.Geom)
	if err != nil {
		return nil, false, fmt.Errorf("%w; way forward: transient, re-run `lyx webster recover-batch %d`", err, batchNumber)
	}
	if len(referenced) > 0 {
		return nil, false, fmt.Errorf("%w: batch %02d: %s; way forward: %s", ErrRecoveryDeleteReferenced, batchNumber, strings.Join(referenced, "; "), deleteReferencedWayForward(batchNumber, prior != nil && len(prior.Uncheckable) > 0))
	}

	prevDigest := predecessorDigestLine(deps.Batches, deps.State, batchNumber)

	fresh, err := recoverSpawn(deps, batch, prior, prevDigest, clk)
	if err != nil {
		return nil, false, err
	}
	if deps.State.Batches == nil {
		deps.State.Batches = map[int]*BatchState{}
	}
	deps.State.Batches[batchNumber] = fresh
	deps.State.CurrentBatch = batchNumber
	return fresh, true, nil
}

// RecoveryWaitBudget is the wait one recover-batch call blocks for by default: recovery_timeout_min plus one poll tick.
// The budget always outlasts the timeout measured from spawn, so a call returns at a terminal digest, or once Classify marks the strand dead on its timeout, and never as a running snapshot.
func RecoveryWaitBudget(cfg Config) time.Duration {
	return time.Duration(cfg.RecoveryTimeoutMin)*time.Minute + pollTick
}

// RecoverAwait drives the bounded wait for a recovery strand: the long-poll classification loop
// (see awaitTerminal) plus substrate release on terminal.
// Caller runs this with the state-mutation lease RELEASED.
func RecoverAwait(deps RecoverDeps, batchNumber int, bs *BatchState, wait time.Duration, clk Clock) (*RecoverResult, error) {
	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	return awaitTerminal(deps, batch, bs, wait, clk)
}

// PersistRecoveryTerminal runs the shared post-batch mechanical pass and then merges a terminal
// digest into st (loaded fresh under the lease after the unleased wait).
// Marks batch terminal and clears the in-flight cursor, and returns the pass's informational
// warnings alongside any error so a caller never loses them.
//
// The mechanical pass is not optional here. A batch that reaches done through recover-batch is
// exactly as done as one that reaches it through record-batch, and Master's own failure ladder
// treats a terminal recovery digest as "move on to the next batch" — but this path used to mark the
// batch terminal on the recovery strand's self-reported status alone, running no done-checks, no
// handle binding, no scope guard, no drift detection and no plan-staleness re-baseline. A card
// recovered that way never bound its plan: handles, so every later card kept referencing an unbound
// handle for the rest of the plan's life.
//
// Before that pass, every suspect path the failed batch recorded is checked at the report's head.
// A plan file must match the plan as the run recorded it, a tracked path must not differ from the head,
// and a tracked path must not still hold the flagged blob unless the start commit held it too.
// A batch recording a finding the check cannot verify never reaches it: RecoverSpawnOrAttach refuses it with ErrRecoveryNeedsFresh.
// A recovery that leaves any of them fails the batch again with the same suspect paths, so the next recover-batch checks them again.
// Accepted residual: a strand whose re-derivation is byte-identical to the flagged content is failed again;
// the operator's way forward is to revert the path and edit its card.
//
// A blocking finding fails the batch through failBatch: the recovery strand said done,
// but the tree says the card is not, and webster believes the tree.
// The report is archived, the record is terminal failed, and the *BatchFailedError is returned with the pass's warnings,
// so the next recover-batch spawns a fresh strand instead of re-attaching to the finished one and failing the same checks forever.
func PersistRecoveryTerminal(deps RecoverDeps, st *State, batchNumber int, digest *Digest) (warnings []string, err error) {
	if st == nil {
		return nil, fmt.Errorf("webster: recovery terminal persistence requires a loaded state; State is nil")
	}
	if deps.Plan == nil {
		return nil, fmt.Errorf("webster: recovery terminal persistence requires a parsed plan; RecoverDeps.Plan is nil")
	}
	bs, ok := st.Batches[batchNumber]
	if !ok || bs == nil {
		return nil, fmt.Errorf("webster: no recorded state for batch %d at recovery terminal persistence — state.json changed underneath the recovery wait; way forward: re-run `lyx webster recover-batch %d`", batchNumber, batchNumber)
	}

	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	number, slug := batchIdentity(batch)

	// The recovery strand's own report carries the head it committed at, already parsed into the digest.
	// Both verbs reconcile that head against the worktree's HEAD under the merge-only rule and record the batch at the report's head,
	// so the pass below is fed the same pair of SHAs on either path.
	head := digest.HeadSHA
	if head == "" {
		head, err = deps.Geom.git().HeadSHA(deps.Geom.WorktreeRoot)
		if err != nil {
			return nil, err
		}
	}

	suspectReasons, err := checkRecoveredSuspects(deps.Geom, st, bs, number, head)
	if err != nil {
		return nil, err
	}
	if len(suspectReasons) > 0 {
		var paths []string
		for _, sp := range bs.SuspectPaths {
			paths = append(paths, sp.Path)
		}
		bfe, ferr := failBatch(failBatchInput{
			State:        st,
			Batch:        bs,
			Number:       number,
			Slug:         slug,
			ReportsDir:   deps.Geom.ReportsDir,
			WorktreeRoot: deps.Geom.WorktreeRoot,
			Git:          deps.Geom.Git,
			HeadSHA:      head,
			Reasons:      suspectReasons,
			SuspectPaths: paths,
			Now:          time.Now,
		})
		if ferr != nil {
			return nil, ferr
		}
		return nil, bfe
	}

	// The pass below restamps the plan hashes, so a plan edited since the run recorded it or since this batch was begun is refused first.
	if err := PlanEditError(st, deps.Geom.PlanDir); err != nil {
		return nil, err
	}
	if err := batchCardEditError(st, bs, batch, deps.Geom.PlanDir); err != nil {
		return nil, err
	}

	warnings, err = postBatchChecks(postBatchInputs{
		Plan:      deps.Plan,
		State:     st,
		Batch:     bs,
		Geom:      deps.Geom,
		Cards:     batch.Cards,
		Completed: completedCards(deps.Batches, st, batchNumber),
		StartSHA:  bs.StartSHA,
		HeadSHA:   head,
		Label:     fmt.Sprintf("%02d-%s", number, slug),
	})
	if err != nil {
		if !errors.Is(err, ErrCardNotDone) {
			return warnings, err
		}
		bfe, ferr := failCardNotDone(cardNotDoneInputs{
			Plan:    deps.Plan,
			Batches: deps.Batches,
			State:   st,
			Batch:   bs,
			Cards:   batch.Cards,
			Number:  number,
			Slug:    slug,
			Geom:    deps.Geom,
			HeadSHA: head,
			Verb:    "recover-batch",
		}, err)
		if ferr != nil {
			return warnings, ferr
		}
		return warnings, bfe
	}

	bs.Digest = digest
	bs.Terminal = true
	bs.Status = digest.Status
	// Record CardSHAs like record-batch does, so the verify gate's card hint has no gaps.
	if digest.HeadSHA != "" {
		bs.CardSHAs = []string{digest.HeadSHA}
	}
	st.CurrentBatch = 0
	return warnings, nil
}

// awaitTerminal drives one bounded long-poll wait for bs's recovery strand,
// assembling ClassifyInputs and releasing substrate on terminal.
// Every terminal digest removes the recovery strand if still live, even when the call then refuses;
// only a done digest also removes the run dir.
// Cleanup failures are warnings, not fatal errors.
func awaitTerminal(deps RecoverDeps, batch batcher.Batch, bs *BatchState, wait time.Duration, clk Clock) (*RecoverResult, error) {
	number, slug := batchIdentity(batch)

	spawnedAt, err := time.Parse(time.RFC3339, bs.SpawnedAt)
	if err != nil {
		return nil, fmt.Errorf("webster: parse recorded spawnedAt %q for batch %d: %w", bs.SpawnedAt, number, err)
	}

	reportPath := filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug))
	timeout := time.Duration(deps.Config.RecoveryTimeoutMin) * time.Minute

	gather := func() (Digest, bool, error) {
		var report *Report
		if _, statErr := os.Stat(reportPath); statErr == nil {
			r, err := ParseReport(reportPath)
			if err != nil {
				return Digest{}, false, err
			}
			report = r
		} else if !os.IsNotExist(statErr) {
			return Digest{}, false, fmt.Errorf("webster: stat batch report %s: %w", reportPath, statErr)
		}

		turnEnded, err := TurnEndedAfter(bs.EventsPath, bs.EventsOffset, deps.Engine)
		if err != nil {
			return Digest{}, false, err
		}
		strandLive, err := StrandLive(deps.Reed, bs.StrandGUID)
		if err != nil {
			return Digest{}, false, err
		}

		in := ClassifyInputs{
			BatchNumber:  number,
			BatchSlug:    slug,
			ReportPath:   reportPath,
			Report:       report,
			TurnEnded:    turnEnded,
			StrandLive:   strandLive,
			Elapsed:      clk.Now().Sub(spawnedAt),
			BatchTimeout: timeout,
		}
		digest, terminal := Classify(in)
		return digest, terminal, nil
	}

	digest, err := PollUntilTerminal(gather, wait, clk)
	if err != nil {
		return nil, err
	}

	elapsedS := int(clk.Now().Sub(spawnedAt).Seconds())

	if digest.Status == DigestStatusRunning {
		return &RecoverResult{Running: true, ElapsedS: elapsedS}, nil
	}

	var warnings []string

	// The strand is removed before the refusals below,
	// so a terminal digest that then refuses leaves no live strand.
	if err := removeStrandIfLive(deps.Reed, bs.StrandGUID); err != nil {
		warnings = append(warnings, fmt.Sprintf("recover-batch: remove strand %s: %v", bs.StrandGUID, err))
	}

	// A merge in progress leaves the batch non-terminal and retryable, like RecordBatch.
	if err := refuseMidMerge(deps.Geom.git(), deps.Geom.WorktreeRoot); err != nil {
		return nil, err
	}

	// Cross-check report's head_sha against worktree's actual HEAD under RecordBatch's merge-only rule.
	if digest.HeadSHA != "" {
		moved, err := reconcileReportHead(deps.Geom.git(), deps.Geom.WorktreeRoot, digest.HeadSHA, fmt.Sprintf("recovery report for batch %02d-%s", number, slug), deps.ParentBranch)
		if err != nil {
			return nil, err
		}
		if moved != "" {
			warnings = append(warnings, moved)
		}
	}

	// Only a done digest drops the run dir;
	// stuck and dead keep it for diagnosis.
	if digest.Status == DigestStatusDone && bs.ShuttleRunDir != "" {
		if err := os.RemoveAll(bs.ShuttleRunDir); err != nil {
			warnings = append(warnings, fmt.Sprintf("recover-batch: remove run dir %s: %v", bs.ShuttleRunDir, err))
		}
	}

	return &RecoverResult{Digest: &digest, ElapsedS: elapsedS, Warnings: warnings}, nil
}
