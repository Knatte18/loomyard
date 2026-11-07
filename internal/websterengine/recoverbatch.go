// recoverbatch.go implements webster's blocking recovery verb for a batch whose fork reported stuck or never reported.
// It runs as three lease-scoped phases: RecoverSpawnOrAttach, RecoverAwait and PersistRecoveryTerminal.
// RecoverSpawnOrAttach is the only place webster spawns a separate process:
// a cold recovery strand, given the full cold-start prompt rather than the thin fork prompt.
// The caller holds the state-mutation lease across the first and last phases, never across the wait between them.
// This file never touches fabric; the caller commits state.json after the spawn and again at terminal persistence.

package websterengine

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrRecoveryNeedsFresh marks a refusal to recover a failed batch whose findings recovery cannot check.
var ErrRecoveryNeedsFresh = errors.New("webster: recovery cannot check the batch's findings")

// ErrRecoveryDeleteReferenced marks a refusal to clear a delete that a later, unbegun card still references.
var ErrRecoveryDeleteReferenced = errors.New("webster: recovery cannot clear a delete a later card still references")

// ErrRecoveryExhausted marks a refusal to recover a batch that has used every counted recovery spawn.
var ErrRecoveryExhausted = errors.New("webster: the batch's recoveries are exhausted")

// maxRecoveries is how many counted recovery spawns one batch gets.
const maxRecoveries = 2

// recoveryNeedsFreshError carries the refusal text verbatim and unwraps to ErrRecoveryNeedsFresh.
type recoveryNeedsFreshError struct{ msg string }

func (e *recoveryNeedsFreshError) Error() string { return e.msg }

func (e *recoveryNeedsFreshError) Unwrap() error { return ErrRecoveryNeedsFresh }

// Clock abstracts time.Now and time.Sleep, so a bounded wait runs instantly under test.
// Any type with these two methods satisfies it, so a Clock goes straight into the polling loop without an adapter.
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

// RecoverDeps carries the seams and state a recovery reads.
// Geom is the told Geometry every path is read from.
// Reed answers the liveness read in `awaitTerminal`, and Stopper stops a leftover strand.
// Batches is the execution order from the batchifier.
// The predecessor digest lookup in `predecessorDigestLine` depends on Batches being in that order.
type RecoverDeps struct {
	Starter    Starter
	Plan       *planparser.Plan
	Batches    []batcher.Batch
	State      *State
	Roles      map[Role]shuttleengine.ModelChoice
	Config     Config
	Engines    AuditEngines
	Reed       shuttleengine.ReedOps
	Stopper    StrandStopper
	ShuttleCfg shuttleengine.Config
	Geom       Geometry
	// ReadOnly reports whether a fabric-referencing command is read-only, which makes its finding a policy warning.
	// A nil ReadOnly accepts nothing.
	ReadOnly func(cmd string) bool

	// FrictionDir is the told absolute friction directory (see internal/friction), empty when Tier 2 is off.
	// It sits here rather than on Geometry, since it needs no geometry derivation.
	FrictionDir string

	// ParentBranch names the run's parent branch for the head cross-check's clean-parent-merge rule.
	// A nil ParentBranch (standalone mode) accepts no merge commit between the report's head_sha and HEAD.
	ParentBranch ParentBranchFunc

	// LanguageSkills are the hub's language skills the recovery strand loads after roleSkills; empty for none.
	LanguageSkills []string

	// GateTiers are the hub's rendered tier and gate-list lines that fill the recovery prompt's gate_tiers marker.
	// It reads "none" when no tier is declared.
	GateTiers string

	// RenameTools are the hub's structure-aware rename tools the recovery prompt renders; empty for none.
	RenameTools []string

	// Gate is the hub's gate config, from which the recovery prompt renders its card and batch gate calls.
	Gate gateslot.Config

	// CardFixer sends a failed card gate call through a cold fixer first.
	// The recovery prompt states this.
	CardFixer bool
}

// RecoverResult is what one RecoverAwait call returns.
// Digest is nil while Running.
// Running reports whether the wait elapsed before a terminal digest.
// ElapsedS counts seconds since the spawn.
// Warnings holds non-fatal substrate-cleanup failures.
// A moved-HEAD notice joins them when only merge commits sit between the report's head_sha and the worktree's HEAD.
type RecoverResult struct {
	Digest   *Digest
	Running  bool
	ElapsedS int
	Warnings []string
}

// archiveStaleReport renames a stale report to free its path, keeping it auditable rather than deleting it.
// It returns ("", nil) when the report is absent.
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

// refuseRecoveringDoneReport refuses to recover a batch whose report already has status OK.
// record-batch is the verb that consumes such a report, so recovery never archives finished work.
// It lets recovery proceed when prior is terminal dead or terminal failed.
// It also lets recovery proceed when the report is missing or unparseable.
func refuseRecoveringDoneReport(reportsDir string, number int, slug string, prior *BatchState) error {
	// A late report written after a dead or failed terminal classification gets archived, not refused.
	if prior != nil && prior.Terminal && (prior.Status == DigestStatusDead || prior.Status == DigestStatusFailed) {
		return nil
	}

	reportPath := filepath.Join(reportsDir, ReportFileName(number, slug))
	report, err := ParseReport(reportPath)
	if err != nil {
		// A missing or unparseable report leaves recovery as the path.
		return nil
	}
	if report.Status == ReportStatusOK {
		return fmt.Errorf("webster: batch %02d-%s already has a report with status: OK at %s — recover-batch never archives finished work; record it with `lym webster record-batch %d` instead", number, slug, reportPath, number)
	}
	return nil
}

// failureDigestBlock renders a prior record's failure for the recovery prompt.
// It lists the reasons of a failed digest, which failBatch already ends with the suspect paths.
// It then adds one instruction per card in AmendedCards: re-read the card and bring the committed work in line with it.
// The amended instructions come from AmendedCards, not from the reasons;
// so a stuck, dead or failed recovery does not erase them.
// It returns "" when prior is neither a failed batch nor holds an amended card.
func failureDigestBlock(prior *BatchState) string {
	if prior == nil {
		return ""
	}
	var lines []string
	if prior.Status == DigestStatusFailed && prior.Digest != nil {
		for _, r := range prior.Digest.Reasons {
			lines = append(lines, "- "+r)
		}
	}
	for _, a := range prior.AmendedCards {
		lines = append(lines, fmt.Sprintf("- card %s was amended after the previous attempt began: re-read the card file and bring the work already committed in line with it", a.Card))
	}
	return strings.Join(lines, "\n")
}

// uncommittedPathsBlock renders the uncommitted worktree paths for the recovery prompt.
// It groups them by whether this run wrote them.
// A path the run's write evidence cannot attribute counts as not written by this run;
// a collapsed untracked directory counts as written when any written path lies under it.
// A clean tree renders "none".
func uncommittedPathsBlock(deps RecoverDeps) (string, error) {
	uncommitted, err := UncommittedPaths(deps.Geom)
	if err != nil {
		return "", err
	}
	if len(uncommitted) == 0 {
		return "none", nil
	}
	var written []string
	if deps.Engines != nil {
		writes, err := loadRunWrites(deps.Engines, deps.State, deps.Geom.PaneCwd)
		if err != nil {
			return "", err
		}
		if written, err = writtenWorktreePaths(writes, deps.Geom.PaneCwd, deps.Geom.WorktreeRoot); err != nil {
			return "", err
		}
	}
	var own, foreign []string
	for _, p := range uncommitted {
		if writtenUnder(written, p) {
			own = append(own, p)
		} else {
			foreign = append(foreign, p)
		}
	}
	var sections []string
	for _, group := range []struct {
		heading string
		paths   []string
	}{{"Written by this run:", own}, {"Not written by this run:", foreign}} {
		if len(group.paths) > 0 {
			sections = append(sections, group.heading+"\n- "+strings.Join(group.paths, "\n- "))
		}
	}
	return strings.Join(sections, "\n\n"), nil
}

// writtenUnder reports whether the uncommitted path is one of written, or a directory holding one.
// A path with a trailing slash names a collapsed untracked directory.
func writtenUnder(written []string, uncommitted string) bool {
	if dir, isDir := strings.CutSuffix(uncommitted, "/"); isDir {
		return slices.ContainsFunc(written, func(w string) bool { return strings.HasPrefix(w, dir+"/") })
	}
	return slices.Contains(written, uncommitted)
}

// rendersNewAmendment reports whether prior holds an amendment no earlier recovery spawn rendered into its prompt.
func rendersNewAmendment(prior *BatchState) bool {
	return prior != nil && slices.ContainsFunc(prior.AmendedCards, func(a AmendedCard) bool { return !a.Rendered })
}

// recoverSpawn archives any stale report and stops a live prior strand.
// It then renders the recovery prompt, starts the recovery strand, and returns a fresh BatchState.
// clk stamps SpawnedAt so elapsed-since-spawn is measured against the same clock.
// The record appends the spawned run's session id to the prior record's RecoverySessions.
// It records that run's engine under the same id in RecoveryEngines.
// The record counts the spawn in Recoveries unless a new amendment is rendered or prior waits on its parent.
// The record carries HEAD at the spawn as RecoveryStartSHA.
// HEAD is read before anything is stopped or started, so a failed read starts nothing and counts nothing.
func recoverSpawn(deps RecoverDeps, batch batcher.Batch, prior *BatchState, prevDigest string, clk Clock) (*BatchState, error) {
	number, slug := batchIdentity(batch)

	if err := renameToolRefusal(deps.Plan, batch, deps.RenameTools, "recover-batch"); err != nil {
		return nil, err
	}

	patternChanges, err := discussionparser.DeclaredPatternChanges(deps.Geom.DecisionRecordPath)
	if err != nil {
		return nil, fmt.Errorf("%w re-run `lym webster recover-batch %d`", err, number)
	}

	head, err := deps.Geom.git().HeadSHA(deps.Geom.WorktreeRoot)
	if err != nil {
		return nil, fmt.Errorf("webster: read HEAD before spawning the recovery of batch %02d: %w; way forward: transient, re-run `lym webster recover-batch %d`", number, err, number)
	}

	cardHashes, err := batchCardHashes(batch, deps.Geom.PlanDir)
	if err != nil {
		return nil, fmt.Errorf("%w; way forward: transient, re-run `lym webster recover-batch %d`", err, number)
	}

	if err := refuseRecoveringDoneReport(deps.Geom.ReportsDir, number, slug, prior); err != nil {
		return nil, err
	}

	// The gate calls render before the first mutation, so a gate list the config lacks refuses with nothing changed.
	cardGates, err := renderCardGates(batch.Cards, masterPlanDirDisplay(deps.Geom.PaneCwd, deps.Geom.PlanDir), deps.Gate)
	if err != nil {
		return nil, err
	}
	batchGate, err := renderBatchGate(batch.Cards, deps.Gate)
	if err != nil {
		return nil, err
	}

	// The recovery strand writes its report into this directory, so it must exist.
	if err := os.MkdirAll(deps.Geom.ReportsDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create reports dir %s: %w", deps.Geom.ReportsDir, err)
	}

	if _, err := archiveStaleReport(deps.Geom.ReportsDir, number, slug, clk.Now); err != nil {
		return nil, err
	}

	// A fork batch's record has an empty StrandGUID, which the reclaim skips.
	if prior != nil && prior.StrandGUID != "" {
		if err := deps.Stopper.StopStrand(prior.StrandGUID); err != nil {
			return nil, fmt.Errorf("websterengine: stop prior recovery strand %s before respawn: %w", prior.StrandGUID, err)
		}
	}

	batchName := fmt.Sprintf("%02d-%s", number, slug)
	reportPath, err := filepath.Abs(filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug)))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve report path: %w", err)
	}

	notePath := friction.NotePath(deps.FrictionDir, batchName+"-recovery")
	uncommittedPaths, err := uncommittedPathsBlock(deps)
	if err != nil {
		return nil, fmt.Errorf("webster: list the worktree's uncommitted paths for batch %s: %w; way forward: transient, re-run `lym webster recover-batch %d`", batchName, err, number)
	}
	prompt, err := RenderRecoveryPrompt(batch, cardGates, batchGate, prevDigest, failureDigestBlock(prior), uncommittedPaths, reportPath, deps.Geom.WorktreeRoot, deps.Geom.PlanDir, deps.Geom.PaneCwd, deps.Geom.StencilsDir, deps.Geom.SpecsDir, deps.GateTiers, deps.RenameTools, patternChanges, deps.Config.SelfFixCap, notePath, deps.Geom.ParentName, deps.CardFixer)
	if err != nil {
		return nil, err
	}

	choice, ok := deps.Roles[RoleRecovery]
	if !ok {
		return nil, fmt.Errorf("webster: no resolved model-spec for role %q", RoleRecovery)
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{reportPath},
		ModelChoice: choice,
		Skills:      languageconfig.AppendSkills(roleSkills, deps.LanguageSkills),
		Role:        string(RoleRecovery),
		Segment:     segmentcolor.Webster,
		Round:       batchName,
		Timeout:     time.Duration(deps.Config.RecoveryTimeoutMin) * time.Minute,
	}

	run, err := deps.Starter.Start(spec)
	if err != nil {
		return nil, fmt.Errorf("webster: start recovery strand for batch %s: %w; way forward: transient, re-run `lym webster recover-batch %d`", batchName, err, number)
	}

	runState, runDir, err := shuttleengine.FindRun(deps.ShuttleCfg, deps.Geom.AnchorRoot, run.StrandGUID())
	if err != nil {
		// The strand is live but no record of it will be saved, so nothing later could stop it.
		stopErr := deps.Stopper.StopStrand(run.StrandGUID())
		return nil, errors.Join(fmt.Errorf("webster: resolve spawned recovery run: %w", err), stopErr)
	}

	// The recovery record inherits the original bracket's start SHA when there is one.
	// It does not re-capture HEAD at spawn.
	// A recovery exists because the batch's own fork got stuck, frequently after committing part of its work,
	// so a start SHA captured here would exclude that part.
	// The post-batch checks this record's terminal path runs (`postBatchChecks`) take their delta from the start SHA.
	// A narrower delta reports a symbol the stuck fork already created as never created.
	// The whole bracket is the base commit for this batch's work.
	start := head
	if prior != nil && prior.StartSHA != "" {
		start = prior.StartSHA
	}

	// Recorded audit warnings carry over too.
	// Their identities stay dispositioned in State.AuditDispositions (the once-per-identity rule),
	// so no later call records them again.
	// The batch's fork transcripts carry over too,
	// so the run-exit audit still knows which report each of those forks owns.
	var priorWarnings []AuditWarning
	var priorSuspects []SuspectPath
	var priorTranscripts []string
	// The prompt above rendered every amended card, so the fresh record marks each entry rendered.
	// Only an edit made after this spawn leaves an entry unrendered.
	var amended []AmendedCard
	recoveries := 0
	var recoverySessions []string
	recoveryEngines := map[string]string{}
	if prior != nil {
		recoverySessions = slices.Clone(prior.RecoverySessions)
		maps.Copy(recoveryEngines, prior.RecoveryEngines)
		for _, a := range prior.AmendedCards {
			amended = append(amended, AmendedCard{Card: a.Card, Rendered: true})
		}
		priorWarnings = prior.AuditWarnings
		priorSuspects = prior.SuspectPaths
		priorTranscripts = prior.ForkTranscripts
		recoveries = prior.Recoveries
	}
	// A spawn resuming a batch that waited on its parent continues the recovery that stopped, so it costs none.
	if !rendersNewAmendment(prior) && !waitsOnParent(prior) {
		recoveries++
	}
	recoverySessions = append(recoverySessions, runState.SessionID)
	recoveryEngines[runState.SessionID] = runState.Engine

	return &BatchState{
		Slug:             slug,
		Cards:            batchCardIDs(batch),
		CardHashes:       cardHashes,
		AmendedCards:     amended,
		StartSHA:         start,
		AuditWarnings:    priorWarnings,
		SuspectPaths:     priorSuspects,
		ForkTranscripts:  priorTranscripts,
		Kind:             "recovery",
		SpawnedAt:        clk.Now().UTC().Format(time.RFC3339),
		StrandGUID:       run.StrandGUID(),
		ShuttleRunDir:    runDir,
		EventsPath:       runState.EventsPath,
		EventsOffset:     runState.PromptOffset,
		Recoveries:       recoveries,
		RecoverySessions: recoverySessions,
		RecoveryEngines:  recoveryEngines,
		RecoveryStartSHA: head,
	}, nil
}

// RecoveryRetry reports whether batch n's dead recovery earns one more recovery.
// It also reports how many counted recoveries the batch has.
// It is true for a terminal dead recovery record with no counted recovery, the state a `reset --to batch-start` leaves.
// That record still owes the batch its first counted recovery.
// Below the cap it is otherwise true only when the recovery committed work of its own:
// HEAD descends from the record's non-empty RecoveryStartSHA,
// and the first-parent range from it to HEAD holds a commit that is not a merge.
// An empty start, a HEAD that a reset moved off it, and a range holding only merge commits each give false.
// A failed git read, including a start the store does not hold, gives false and is logged at Warn.
// The callers share this rule, so one rule decides the retry.
func RecoveryRetry(geom Geometry, st *State, n int) (retry bool, recoveries int) {
	retry, recoveries, err := recoveryRetryRead(geom, st, n)
	if err != nil {
		logger.Warn("websterengine: could not tell whether the dead recovery committed, so no retry is offered", "batch", n, "error", err)
		return false, recoveries
	}
	return retry, recoveries
}

// recoveryRetryRead reports whether batch n's dead recovery earns one more recovery.
// A failed git read comes back as err with retry false, rather than folded into the verdict.
func recoveryRetryRead(geom Geometry, st *State, n int) (retry bool, recoveries int, err error) {
	var bs *BatchState
	if st != nil {
		bs = st.Batches[n]
	}
	if bs == nil {
		return false, 0, nil
	}
	recoveries = bs.Recoveries
	if bs.Kind != "recovery" || !bs.Terminal || bs.Status != DigestStatusDead || recoveries >= maxRecoveries {
		return false, recoveries, nil
	}
	if recoveries == 0 {
		return true, recoveries, nil
	}
	if bs.RecoveryStartSHA == "" {
		return false, recoveries, nil
	}
	committed, err := committedSince(geom, bs.RecoveryStartSHA)
	if err != nil {
		return false, recoveries, fmt.Errorf("webster: read whether the dead recovery of batch %02d committed since %s: %w", n, bs.RecoveryStartSHA, err)
	}
	return committed, recoveries, nil
}

// committedSince reports whether HEAD descends from start, and whether that first-parent range holds a non-merge commit.
func committedSince(geom Geometry, start string) (bool, error) {
	git := geom.git()
	head, err := git.HeadSHA(geom.WorktreeRoot)
	if err != nil {
		return false, err
	}
	descends, err := git.IsAncestor(geom.WorktreeRoot, start, head)
	if err != nil || !descends {
		return false, err
	}
	return git.NonMergeCommitsBetween(geom.WorktreeRoot, start, head)
}

// RecoverSpawnOrAttach attaches to a recorded, non-terminal recovery BatchState when one exists.
// Otherwise it spawns a fresh recovery.
// The caller persists deps.State via SaveState when spawned is true.
//
// The spawn call includes the provider's startup window under the state-mutation lease.
// Two residuals are accepted: one from that window, and one from state.json persisted after the call returns:
//
//  1. A process killed inside the startup window leaves a live recovery strand whose guid was never persisted.
//     The next call's prior.StrandGUID reclaim (StopStrand) cannot see that strand, so the next spawn runs beside it.
//  2. Shuttle could not get a liveness answer from reed maxStatusRetries times during startup.
//     The spawn then returns an error with the strand left live and no guid persisted.
//     That is accepted, because tearing the strand down could kill a working agent;
//     a reed in that state usually fails the next `AddStrand` too.
//     The error names the strand guid so an operator can remove it by hand.
//  3. A not-ready start does not leak the strand: shuttle tears it down.
//     The exception is a failed strand removal in that teardown, which the returned error states.
//
// Before spawning, it refuses with ErrRecoveryDeleteReferenced.
// The refusal applies while an unbegun later card's Edit code references a symbol the batch's own cards delete.
// A recovery cannot change the plan, so it would fail the same done-check again.
// The bound: it refuses only while that check fires for the batch's own cards against the current plan and tree;
// moving the delete and rebaselining clears it.
// A record waiting on its parent is never attached to and never refused as exhausted: the spawn resumes it uncounted.
func RecoverSpawnOrAttach(deps RecoverDeps, batchNumber int, clk Clock) (bs *BatchState, spawned bool, err error) {
	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, false, err
	}

	prior := deps.State.Batches[batchNumber]
	if prior != nil && prior.Kind == "recovery" && !prior.Terminal && prior.StrandGUID != "" && !waitsOnParent(prior) {
		return prior, false, nil
	}
	// The recovery key can change mid-run, so the spawn re-checks it ahead of anything that archives or stops.
	if choice, ok := deps.Roles[RoleRecovery]; ok {
		if err := roleAuditEngine(deps.Engines, RoleRecovery, choice); err != nil {
			return nil, false, err
		}
	}
	if prior != nil && prior.Recoveries >= maxRecoveries && !rendersNewAmendment(prior) && !waitsOnParent(prior) {
		return nil, false, fmt.Errorf("%w: batch %02d has had %d recoveries; %s", ErrRecoveryExhausted, batchNumber, prior.Recoveries, exhaustedRecoveryWayForward(batchNumber))
	}
	if prior != nil && prior.Terminal && prior.Status == DigestStatusFailed && allReadOnlyFabricReferences(prior.Uncheckable, deps.ReadOnly) {
		// An evidence failure returns the wrap unchanged: its way forward names this verb for the re-run.
		if _, err := AcceptBatchFabricReference(deps.State, deps.Geom, batchNumber, deps.ReadOnly, "recover-batch"); err != nil {
			return nil, false, err
		}
	}
	if prior != nil && prior.Terminal && prior.Status == DigestStatusFailed && len(prior.Uncheckable) > 0 {
		writes, err := contractWritesFor(deps.Engines, deps.State, deps.Geom, prior.Uncheckable)
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
			for _, u := range contracts.Uncleared {
				what = append(what, fmt.Sprintf("%s (%s)", u.Path, noteWroteLast(u.Writer)))
			}
			wayForward := resetToStartSteps(stepRun)
			if len(contracts.Uncleared) == 0 && allPathlessFabricReference(contracts.Rest) {
				if wayForward, err = acceptAuditWayForward(deps, batchNumber); err != nil {
					return nil, false, err
				}
			}
			return nil, false, &recoveryNeedsFreshError{msg: fmt.Sprintf("webster: batch %02d failed on findings recovery cannot check: %s; %s", batchNumber, strings.Join(what, ", "), wayForward)}
		}
		if len(contracts.Uncleared) > 0 {
			return nil, false, fmt.Errorf("webster: batch %02d failed on contract file(s) a fork, recovery or %s session wrote last: %s", batchNumber, gatefixer.RoleVerifyFixer, contractDeleteClause(contracts.Uncleared, fmt.Sprintf("lym webster recover-batch %d", batchNumber)))
		}
	}

	referenced, err := laterDeleteReferenceReasons(deps.Plan, deps.Batches, deps.State, batch.Cards, deps.Geom)
	if err != nil {
		return nil, false, fmt.Errorf("%w; way forward: transient, re-run `lym webster recover-batch %d`", err, batchNumber)
	}
	if len(referenced) > 0 {
		return nil, false, fmt.Errorf("%w: batch %02d: %s; way forward: %s", ErrRecoveryDeleteReferenced, batchNumber, strings.Join(referenced, "; "), deleteReferencedWayForward(batchNumber, prior != nil && len(prior.Uncheckable) > 0, false))
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

// acceptAuditWayForward returns the way forward for a refusal over a failed batch.
// It applies when every uncheckable entry of that batch is a pathless fabric reference.
// It runs the evidence check `lym webster accept-audit --batch` applies, mutating nothing,
// so it names that verb only on a route the check admits.
// When the evidence holds, the way forward is accept-audit then recover-batch.
// Otherwise it is the evidence refusal's reason and its own steps, which re-run both verbs.
// Either way it also names the reset-to-start route.
// A git read failure is returned.
func acceptAuditWayForward(deps RecoverDeps, batchNumber int) (string, error) {
	acceptThenRecover := fmt.Sprintf("\"lym webster accept-audit --batch %d\" then \"lym webster recover-batch %d\"", batchNumber, batchNumber)
	_, err := batchFabricReferenceEvidence(deps.State, deps.Geom, batchNumber, deps.ReadOnly, "re-run "+acceptThenRecover)
	if err == nil {
		return fmt.Sprintf("way forward: %s; otherwise %s", acceptThenRecover, freshRestartSteps(stepRun)), nil
	}
	if !errors.Is(err, ErrAuditNotAcceptable) {
		return "", err
	}
	refusal := fmt.Sprintf("accept-audit --batch %d refuses it: %s", batchNumber, strings.TrimPrefix(err.Error(), ErrAuditNotAcceptable.Error()+": "))
	if !strings.Contains(refusal, stepResetToStart) {
		refusal += "; otherwise " + freshRestartSteps(stepRun)
	}
	return refusal, nil
}

// RecoveryWaitBudget is the wait one recover-batch call blocks for by default: recovery_timeout_min plus one poll tick.
// The budget always outlasts the timeout measured from spawn.
// So a call returns at a terminal digest, or once the strand is marked dead on its timeout.
// It never returns a running snapshot.
func RecoveryWaitBudget(cfg Config) time.Duration {
	return time.Duration(cfg.RecoveryTimeoutMin)*time.Minute + pollTick
}

// RecoverAwait drives the bounded wait for a recovery strand.
// It runs the long-poll classification loop, then releases the substrate on terminal.
// The caller runs it with the state-mutation lease released.
func RecoverAwait(deps RecoverDeps, batchNumber int, bs *BatchState, wait time.Duration, clk Clock) (*RecoverResult, error) {
	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	return awaitTerminal(deps, batch, bs, wait, clk)
}

// PersistRecoveryTerminal runs the shared post-batch mechanical pass and merges a terminal digest into st.
// The caller loads st fresh under the lease, after the unleased wait.
// It marks the batch terminal and clears the in-flight cursor.
// It returns the pass's informational warnings alongside any error, so a caller never loses them.
//
// The mechanical pass is required here.
// A batch that reaches done through recover-batch is exactly as done as one that reaches it through record-batch.
// Master's own failure ladder treats a terminal recovery digest as "move on to the next batch".
// Skipping the pass would mark the batch terminal on the recovery strand's self-reported status alone.
// It would run no done-checks, handle binding, scope guard, drift detection or plan-staleness re-baseline.
// Such a card leaves the plan's handles unbound.
// Every later card then keeps referencing an unbound handle for the rest of the plan's life.
//
// Before that pass, every suspect path the failed batch recorded is checked at the report's head.
// A plan file must match the plan as the run recorded it, a tracked path must not differ from the head,
// and a tracked path must not still hold the flagged blob unless the start commit held it too.
// A batch recording a finding the check cannot verify never reaches it:
// the spawn refuses it with ErrRecoveryNeedsFresh.
// A recovery that leaves any of them fails the batch again with the same suspect paths,
// so the next recover-batch checks them again.
// Accepted residual: a strand whose re-derivation is byte-identical to the flagged content is failed again;
// the operator's way forward is to revert the path and edit its card.
//
// A blocking finding fails the batch through failBatch: the recovery strand said done,
// but the tree says the card is not, and webster believes the tree.
// The report is archived and the record is terminal failed.
// The *BatchFailedError is returned with the pass's warnings.
// The next recover-batch spawns a fresh strand rather than re-attaching to the finished one.
// Re-attaching would fail the same checks forever.
//
// A waiting digest, from a strand that reported NEEDS_PARENT, runs neither the suspect check nor the pass:
// the batch is recorded waiting with its need and its report archived, and stays in flight.
func PersistRecoveryTerminal(deps RecoverDeps, st *State, batchNumber int, digest *Digest) (warnings []string, err error) {
	if st == nil {
		return nil, fmt.Errorf("webster: recovery terminal persistence requires a loaded state; State is nil")
	}
	if deps.Plan == nil {
		return nil, fmt.Errorf("webster: recovery terminal persistence requires a parsed plan; RecoverDeps.Plan is nil")
	}
	bs, ok := st.Batches[batchNumber]
	if !ok || bs == nil {
		return nil, fmt.Errorf("webster: no recorded state for batch %d at recovery terminal persistence — state.json changed underneath the recovery wait; way forward: re-run `lym webster recover-batch %d`", batchNumber, batchNumber)
	}

	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	number, slug := batchIdentity(batch)

	// A recovery waiting on its parent has not finished,
	// so the suspect check and the pass wait for the strand that resumes it.
	if digest.Status == DigestStatusWaiting {
		return nil, recordWaiting(bs, digest, deps.Geom.ReportsDir, number, slug, time.Now)
	}

	// The recovery strand's own report carries the head it committed at, already parsed into the digest.
	// Both verbs reconcile that head against the worktree's HEAD under the merge-only rule.
	// They record the batch at the report's head, so the pass below is fed the same pair of SHAs on either path.
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
			PaneCwd:      deps.Geom.PaneCwd,
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

	// The pass below restamps the plan hashes,
	// so a plan edited since the run recorded it, or since this batch was begun, is refused first.
	if err := PlanEditError(st, deps.Geom.PlanDir, deps.Geom.WebsterDir); err != nil {
		return nil, err
	}
	if err := batchCardEditError(st, bs, batch, deps.Geom.PlanDir, deps.Geom.WebsterDir); err != nil {
		return nil, err
	}

	deviations, warnings, err := postBatchChecks(postBatchInputs{
		Plan:      deps.Plan,
		State:     st,
		Batch:     bs,
		Geom:      deps.Geom,
		Number:    batchNumber,
		Cards:     batch.Cards,
		Completed: completedCards(deps.Batches, st, batchNumber),
		StartSHA:  bs.StartSHA,
		HeadSHA:   head,
		Label:     fmt.Sprintf("%02d-%s", number, slug),
		Renames:   digest.Renames,
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

	// An amendment accepted while this recovery ran fails it whatever its report says,
	// so the next recover-batch re-runs the batch on the amended card.
	if len(amendedReasons(bs)) > 0 {
		bfe, ferr := failBatch(failBatchInput{
			State:        st,
			Batch:        bs,
			Number:       number,
			Slug:         slug,
			ReportsDir:   deps.Geom.ReportsDir,
			PaneCwd:      deps.Geom.PaneCwd,
			WorktreeRoot: deps.Geom.WorktreeRoot,
			Git:          deps.Geom.Git,
			HeadSHA:      head,
			Now:          time.Now,
		})
		if ferr != nil {
			return warnings, ferr
		}
		return warnings, bfe
	}

	departures, err := batchDepartures(deps.Geom, bs.StartSHA, head)
	if err != nil {
		return warnings, err
	}

	digest.Deviations = deviations
	digest.Departures = departures
	bs.Digest = digest
	bs.Terminal = true
	bs.Status = digest.Status
	// A recovery recorded done has built every amended card, so none stays to be rendered again.
	if digest.Status == DigestStatusDone {
		bs.AmendedCards = nil
	}
	// Record CardSHAs like record-batch does, so the verify gate's card hint has no gaps.
	if digest.HeadSHA != "" {
		bs.CardSHAs = []string{digest.HeadSHA}
	}
	st.CurrentBatch = 0
	return warnings, nil
}

// awaitTerminal drives one bounded long-poll wait for bs's recovery strand.
// Each poll classifies the batch report, the turn ends and the strand's liveness into a digest.
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

	// The turn ends are read through the engine the live recovery session ran under;
	// a record that lists no session, or no engine for its session, reads as an engine-less entry.
	var session string
	if n := len(bs.RecoverySessions); n > 0 {
		session = bs.RecoverySessions[n-1]
	}
	engine, err := sessionAuditEngine(deps.Engines, session, bs.RecoveryEngines[session])
	if err != nil {
		return nil, err
	}

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

		turnEnded, err := gatefixer.TurnEndedAfter(bs.EventsPath, bs.EventsOffset, engine, gatefixer.TurnEndRead{Now: clk.Now()})
		if err != nil {
			return Digest{}, false, err
		}
		strandLive, err := gatefixer.StrandLive(deps.Reed, bs.StrandGUID)
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
	if err := deps.Stopper.StopStrand(bs.StrandGUID); err != nil {
		warnings = append(warnings, fmt.Sprintf("recover-batch: stop strand %s: %v", bs.StrandGUID, err))
	}

	// A merge in progress leaves the batch non-terminal and retryable.
	if err := refuseMidMerge(deps.Geom.git(), deps.Geom.WorktreeRoot); err != nil {
		return nil, err
	}

	// An abbreviated head_sha is resolved to the full SHA the run record holds.
	// It is then cross-checked against the worktree's actual HEAD under the merge-only rule.
	if digest.HeadSHA != "" {
		if digest.HeadSHA, err = resolveReportHead(deps.Geom.git(), deps.Geom.WorktreeRoot, reportPath, digest.HeadSHA); err != nil {
			return nil, err
		}
		moved, err := reconcileReportHead(deps.Geom.git(), deps.Geom.WorktreeRoot, digest.HeadSHA, fmt.Sprintf("recovery report for batch %02d-%s", number, slug), deps.ParentBranch, number)
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
