// run.go holds Run, the row's producer body: every batch in Go, then the plan-level verify and its fixers.
// A stop of any kind is returned as a stuck result, which carries the reason shed's stop chain reads.

package weaverengine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// runLockName is the run lock's filename inside the scratch directory.
const runLockName = "run.lock"

// The two outcomes a run returns, and the values outcome.yaml's outcome key takes.
const (
	resultDone  = "done"
	resultStuck = "stuck"
)

// The kinds of stop a StopRecord holds.
const (
	stopKindBatch       = "batch"
	stopKindNeedsParent = "needs_parent"
	stopKindVerify      = "verify"
)

// maxLostPerRung is how many lost attempts or fixers one rung takes before the next one counts as a failure.
const maxLostPerRung = 2

// fixerWaitSlice is how long one wait on a verify fixer blocks before Run asks again.
const fixerWaitSlice = time.Minute

// stuckReasonBytes bounds each of the leading text and the way forward of the cause a stuck reason quotes,
// so the one-line reason stays readable.
const stuckReasonBytes = 600

// causeWayForwardMarker opens the way forward a cause ends with.
const causeWayForwardMarker = "; way forward: "

// ErrRunBusy is returned by Run when another call already holds the scratch directory's run lock.
var ErrRunBusy = errors.New("weaver: run is already in progress")

// RunResult is what one Run call returns.
type RunResult struct {
	// Outcome is resultDone or resultStuck.
	Outcome string
	// StuckReason is the one-line account of the stop, set when Outcome is resultStuck.
	StuckReason string
	// BatchesDone counts the batches recorded done, set when Outcome is resultDone.
	BatchesDone int
	// VerifySpans are the time spans of this call's plan-level verify, in the order they started:
	// one gate span per run of the verify and one rework span per verify fixer, from its spawn to its end.
	// They are nil when the call ran no verify.
	VerifySpans []shuttleengine.GateSpan
}

// outcomeRecord is outcome.yaml's one shape, with the keys Webster's outcome file holds so the two read alike.
type outcomeRecord struct {
	Outcome     string `yaml:"outcome"`
	StuckReason string `yaml:"stuck_reason"`
	BatchesDone int    `yaml:"batches_done"`
}

// writeOutcome writes rec to path atomically.
func writeOutcome(path string, rec outcomeRecord) error {
	data, err := yaml.Marshal(rec)
	if err != nil {
		return fmt.Errorf("weaver: marshal the outcome record: %w", err)
	}
	if err := runplan.WriteFileAtomic(filepath.Dir(path), path, data); err != nil {
		return fmt.Errorf("weaver: write %s: %w", path, err)
	}
	return nil
}

// batchEnd is how runBatch ended: the batch recorded done, or the run stopped with a reason.
type batchEnd struct {
	Done   bool
	Reason string
}

// checkDeps refuses a nil seam Run needs as a wiring error naming the field.
func checkDeps(deps RunDeps) error {
	missing := ""
	switch {
	case deps.Batcher == nil:
		missing = "Batcher"
	case deps.Runner == nil:
		missing = "Runner"
	case deps.Fixer == nil:
		missing = "Fixer"
	case deps.Reed == nil:
		missing = "Reed"
	case deps.CommitRecords == nil:
		missing = "CommitRecords"
	case deps.RecordsHead == nil:
		missing = "RecordsHead"
	case deps.Geom.GateConfig == nil:
		missing = "Geom.GateConfig"
	case len(deps.Ladder) == 0:
		missing = "Ladder"
	}
	if missing != "" {
		return fmt.Errorf("weaver: RunDeps.%s was not told; way forward: the CLI layer must set it before it calls Run", missing)
	}
	return nil
}

// Run drives one step of Weaver's row to a stop or to Done.
// It holds the run lock for its whole call, and a held lock returns ErrRunBusy wrapped with the way forward.
// It removes a stale plan-conflict record, and the outcome and summary of an earlier call, on entry.
// It then begins each batch under the state-mutation lease and runs its attempts up the model ladder.
// After the last batch it runs the plan-level verify and its fixers.
// A pass writes the summary and the outcome and commits.
// Every stop is returned as a stuck result.
// A seam that was never told is an error.
func Run(deps RunDeps) (RunResult, error) {
	if err := checkDeps(deps); err != nil {
		return RunResult{}, err
	}
	geom := deps.Geom
	for _, dir := range []string{geom.RunDir, geom.ReportsDir, geom.ScratchDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return RunResult{}, fmt.Errorf("weaver: create %s: %w", dir, err)
		}
	}
	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(geom.ScratchDir, runLockName))
	if err != nil {
		return RunResult{}, fmt.Errorf("weaver: acquire the run lock in %s: %w", geom.ScratchDir, err)
	}
	if !locked {
		return RunResult{}, fmt.Errorf("%w: %q (%s held); way forward: wait for the running row to stop, then %s", ErrRunBusy, geom.ScratchDir, runLockName, stepOrDefault(deps.ReentryStep))
	}
	defer func() { _ = runLock.Release() }()

	if err := runplan.RemoveConflictRecord(runplan.ConflictDir(geom.AnchorRoot)); err != nil {
		logger.Warn("weaver: stale plan-conflict record not removed", "err", err)
	}
	for _, name := range []string{outcomeFileName, summaryparser.FileName} {
		if err := os.Remove(filepath.Join(geom.RunDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return RunResult{}, fmt.Errorf("weaver: remove the stale %s: %w", name, err)
		}
	}

	st, err := LoadState(geom.RunDir, geom.ScratchDir)
	if err != nil {
		return stuck(stopReason(deps, &State{}, "the run state", err.Error())), nil
	}
	if st == nil {
		guid, err := newRunGUID()
		if err != nil {
			return RunResult{}, err
		}
		st = &State{RunGUID: guid}
	}
	if st.Batches == nil {
		st.Batches = map[int]*runplan.BatchState{}
	}

	resumed, err := resumeStrands(deps, st)
	if err != nil {
		return stuck(stopReason(deps, st, resumeLabel, err.Error())), nil
	}
	if resumed.Stuck != "" {
		return stuck(resumed.Stuck), nil
	}
	start := firstAttempt{batch: resumed.Batch, rung: resumed.Rung}
	if st.Stop != nil && !st.Stop.Reentered {
		plan, err := planparser.ParsePlan(geom.PlanDir)
		if err != nil {
			return stuck(stopReason(deps, st, reentryLabel, err.Error())), nil
		}
		re, err := reenter(deps, st, plan)
		if err != nil {
			return stuck(stopReason(deps, st, reentryLabel, err.Error())), nil
		}
		if re.Verify {
			return finishRun(deps, st)
		}
		if re.Batch != 0 {
			start = firstAttempt{batch: re.Batch, rung: re.Rung, reentry: true}
		}
	}

	resetMarkerRemoved := false
	for {
		plan, batch, finished, err := beginBatch(deps, st)
		var conflict *runplan.PlanConflictError
		switch {
		case errors.As(err, &conflict):
			if werr := runplan.WriteConflictRecord(runplan.ConflictDir(geom.AnchorRoot), conflict); werr != nil {
				logger.Warn("weaver: plan-conflict record not written", "err", werr)
			}
			return stuck(stopReason(deps, st, fmt.Sprintf("batch %02d", conflict.Batch), conflict.Error())), nil
		case err != nil:
			return stuck(stopReason(deps, st, "the batch begin", err.Error())), nil
		case finished:
			return finishRun(deps, st)
		}
		if !resetMarkerRemoved {
			marker := filepath.Join(geom.ScratchDir, resetPendingFileName)
			if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
				return stuck(stopReason(deps, st, "the reset marker", err.Error())), nil
			}
			resetMarkerRemoved = true
		}
		number, slug := runplan.BatchIdentity(batch)
		rung, reentryAttempt := 0, false
		if start.batch == number {
			rung, reentryAttempt = start.rung, start.reentry
		}
		start = firstAttempt{}
		end, err := runBatch(deps, st, plan, batch, rung, reentryAttempt)
		if err != nil {
			return stuck(stopReason(deps, st, fmt.Sprintf("batch %02d-%s", number, slug), err.Error())), nil
		}
		if !end.Done {
			return stuck(end.Reason), nil
		}
	}
}

// firstAttempt names the batch whose first attempt of a call starts on a told rung, and whether it is a re-entry's.
type firstAttempt struct {
	batch   int
	rung    int
	reentry bool
}

// stuck returns the stuck result for reason.
func stuck(reason string) RunResult {
	return RunResult{Outcome: resultStuck, StuckReason: reason}
}

// newRunGUID returns a random identifier for a run's first init.
func newRunGUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("weaver: mint the run guid: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// beginBatch begins the next batch under the state-mutation lease.
// It re-reads the plan and partitions the cards no batch has recorded done.
// It records the partition and re-resolves the plan for the first fresh batch.
// It records the batch's card set, card hashes and start commit once, and the records head on the run's first begin.
// It leaves the stop record alone, which a re-entry's Reentered marking reads, then saves.
// It reports finished when no card remains.
// A plan conflict is returned as a *runplan.PlanConflictError.
func beginBatch(deps RunDeps, st *State) (plan *planparser.Plan, batch batcher.Batch, finished bool, err error) {
	geom := deps.Geom
	lease, err := runplan.AcquireStateMutation(geom.ScratchDir)
	if err != nil {
		return nil, batcher.Batch{}, false, err
	}
	defer func() {
		if err := lease.Release(); err != nil {
			logger.Warn("weaver: release the state-mutation lease", "cause", err)
		}
	}()

	plan, err = planparser.ParsePlan(geom.PlanDir)
	if err != nil {
		return nil, batcher.Batch{}, false, err
	}
	if !plan.Approved {
		return nil, batcher.Batch{}, false, fmt.Errorf("weaver: plan %s is not approved; way forward: approve the plan through its review, then %s", geom.PlanDir, stepOrDefault(deps.ReentryStep))
	}
	if err := adoptPlan(deps, plan, st); err != nil {
		return nil, batcher.Batch{}, false, err
	}

	done, fresh, err := runplan.PendingPartition(plan, &st.State, deps.Batcher, batcher.DiskSizes(geom.WorktreeRoot))
	if err != nil {
		return nil, batcher.Batch{}, false, err
	}
	if len(fresh) == 0 {
		return plan, batcher.Batch{}, true, nil
	}
	number, _ := runplan.BatchIdentity(fresh[0])
	if existing := st.Batches[number]; existing != nil && !existing.Terminal && !slices.Equal(existing.Cards, runplan.BatchCardIDs(fresh[0])) {
		return nil, batcher.Batch{}, false, fmt.Errorf("weaver: batch %02d was begun over cards %s, but the plan's remaining cards now begin it with %s; way forward: %s",
			number, strings.Join(existing.Cards, ", "), strings.Join(runplan.BatchCardIDs(fresh[0]), ", "), reentryWayForward)
	}
	runplan.RecordBeginPartition(&st.State, done, fresh)

	batches, err := runplan.MapPartition(plan, st.Partition, partitionWayForward(deps.ReentryStep))
	if err != nil {
		return nil, batcher.Batch{}, false, err
	}
	conflictWayForward := func(string) string {
		return "the parent fixes the pending cards the findings name through plan review, then " + stepOrDefault(deps.ReentryStep)
	}
	if _, err := runplan.ResolveAtDispatch(geom.runPlan(), plan, batches, &st.State, number, conflictWayForward); err != nil {
		return nil, batcher.Batch{}, false, err
	}
	// The dispatch re-resolution may rewrite the plan on disk, so the batch is read from the plan as it now stands.
	if plan, err = plan.Reparse(); err != nil {
		return nil, batcher.Batch{}, false, err
	}
	if batches, err = runplan.MapPartition(plan, st.Partition, partitionWayForward(deps.ReentryStep)); err != nil {
		return nil, batcher.Batch{}, false, err
	}
	if batch, err = runplan.FindBatch(batches, number, "the partition was just recorded"); err != nil {
		return nil, batcher.Batch{}, false, err
	}

	if existing := st.Batches[number]; existing == nil || existing.Terminal {
		head, err := geom.git().HeadSHA(geom.WorktreeRoot)
		if err != nil {
			return nil, batcher.Batch{}, false, fmt.Errorf("weaver: read HEAD as batch %02d's start: %w", number, err)
		}
		hashes, err := runplan.BatchCardHashes(batch, geom.PlanDir)
		if err != nil {
			return nil, batcher.Batch{}, false, err
		}
		_, slug := runplan.BatchIdentity(batch)
		st.Batches[number] = &runplan.BatchState{
			Slug:       slug,
			Cards:      runplan.BatchCardIDs(batch),
			CardHashes: hashes,
			StartSHA:   head,
			Kind:       "strand",
			SpawnedAt:  deps.clock().Now().UTC().Format(time.RFC3339),
		}
	}
	if st.PlanRecordsCommit == "" {
		if st.PlanRecordsCommit, err = deps.RecordsHead(); err != nil {
			return nil, batcher.Batch{}, false, fmt.Errorf("weaver: read the records repository's head: %w", err)
		}
	}
	if err := saveRunState(geom, st, true); err != nil {
		return nil, batcher.Batch{}, false, err
	}
	return plan, batch, false, nil
}

// adoptPlan records the plan's fingerprint on the run's first begin.
// A plan that changed since, a card inserted or removed through plan review included, is adopted.
// A plan whose edit reaches a card whose batch is already done is refused.
func adoptPlan(deps RunDeps, plan *planparser.Plan, st *State) error {
	if st.PlanFingerprint == "" {
		return runplan.RestampBaseline(&st.State, deps.Geom.PlanDir, deps.Geom.RunDir)
	}
	current, err := runplan.Fingerprint(deps.Geom.PlanDir)
	if err != nil {
		return err
	}
	if current == st.PlanFingerprint {
		return nil
	}
	edited, err := runplan.EditedDoneCards(plan, &st.State)
	if err != nil {
		return err
	}
	if len(edited) > 0 {
		return fmt.Errorf("weaver: the plan edits card(s) %s whose work is already built; way forward: restore them, or start the run over with `lym weaver reset`, then %s", strings.Join(edited, ", "), stepOrDefault(deps.ReentryStep))
	}
	return runplan.RestampFingerprint(&st.State, deps.Geom.PlanDir, deps.Geom.RunDir)
}

// runBatch runs the attempts of one begun batch from startRung until it records done or the run must stop.
// Each attempt renders its prompt, starts a gated strand, waits on it, is judged and is ended through endAttempt.
// A failed attempt climbs to the next rung, and a failure on the last rung stops the row.
// A lost attempt is retried once on its rung, counted from the batch's latest re-entry attempt, and the next loss climbs.
// A NEEDS_PARENT report stops the row at once.
// An attempt that does not record done has its strand stopped after its outcome is recorded.
// A failed stop of that strand stops the row with the kind the attempt's outcome names.
// reentry marks the call's first attempt as a re-entry's.
func runBatch(deps RunDeps, st *State, plan *planparser.Plan, batch batcher.Batch, startRung int, reentry bool) (batchEnd, error) {
	geom := deps.Geom
	number, slug := runplan.BatchIdentity(batch)
	label := fmt.Sprintf("%02d-%s", number, slug)
	record := st.Batches[number]
	if record == nil {
		return batchEnd{}, fmt.Errorf("weaver: batch %s has no record", label)
	}
	batches, err := runplan.MapPartition(plan, st.Partition, partitionWayForward(deps.ReentryStep))
	if err != nil {
		return batchEnd{}, err
	}
	last := runplan.IsLastBatch(batches, batch)
	gate, err := geom.GateConfig()
	if err != nil {
		return batchEnd{}, fmt.Errorf("weaver: read the gate config: %w", err)
	}

	stop := func(kind string, rung int, cause string) (batchEnd, error) {
		st.Stop = &StopRecord{Kind: kind, Batch: number, Rung: rung}
		if err := saveRunState(geom, st, false); err != nil {
			cause += "; the stop record could not be saved: " + err.Error()
		}
		return batchEnd{Reason: stopReason(deps, st, fmt.Sprintf("batch %s (cards %s)", label, strings.Join(runplan.BatchCardIDs(batch), ", ")), cause)}, nil
	}

	rung := startRung
	firstOfCall := true
	for {
		if rung >= len(deps.Ladder) {
			return stop(stopKindBatch, len(deps.Ladder)-1, lastCause(st, number, "every ladder rung failed"))
		}
		prior := latestAttempt(st, number)
		attemptNumber := 1
		failureDigest, uncommitted := "", ""
		if prior != nil {
			attemptNumber = prior.Number + 1
			if prior.Outcome == outcomeFailed || prior.Outcome == outcomeLost || prior.Outcome == outcomeFault {
				failureDigest = fmt.Sprintf("Attempt %d, on rung %d, ended %s: %s", prior.Number, prior.Rung+1, prior.Outcome, prior.Cause)
				uncommitted = strings.Join(prior.Uncommitted, "\n")
			}
		}
		attempt := &AttemptState{
			Batch:      number,
			Number:     attemptNumber,
			Rung:       rung,
			ReportPath: AttemptReportPath(geom.ReportsDir, number, slug, attemptNumber),
			InFlight:   true,
			BeganAt:    deps.clock().Now().UTC().Format(time.RFC3339),
		}
		if reentry && firstOfCall {
			attempt.Reentry = true
			if st.Stop != nil {
				st.Stop.Reentered = true
			}
		}
		firstOfCall = false
		st.Attempts = append(st.Attempts, attempt)
		if err := saveRunState(geom, st, false); err != nil {
			return batchEnd{}, err
		}

		engine := deps.Runner.EngineName(deps.Ladder[rung])
		prompt, err := RenderBatchPrompt(BatchPromptInputs{
			StencilsDir:        geom.StencilsDir,
			Engine:             engine,
			PromptsDir:         geom.PromptsDir,
			Label:              label,
			Attempt:            attemptNumber,
			Cards:              batch.Cards,
			PlanDirDisplay:     planDirDisplay(geom),
			Gate:               gate,
			LastBatch:          last,
			RepoRoot:           geom.WorktreeRoot,
			DecisionRecordPath: geom.DecisionRecordPath,
			JournalPath:        JournalPath(geom.RunDir),
			ReportPath:         attempt.ReportPath,
			FailureDigest:      failureDigest,
			UncommittedPaths:   uncommitted,
			GateTiers:          gate.TiersValue(),
			RenameTools:        deps.RenameTools,
			ParentName:         geom.ParentName,
			NotePath:           frictionNotePath(deps, label, attemptNumber),
		})
		if err != nil {
			return batchEnd{}, err
		}
		gateSpec, err := batchGateSpec(geom, batch.Cards, last, deps.Gate)
		if err != nil {
			return batchEnd{}, err
		}
		spec := attemptSpec(attemptLaunch{Prompt: prompt, Engine: engine, ReportPath: attempt.ReportPath, Choice: deps.Ladder[rung], Label: label, Skills: deps.LanguageSkills, Timeout: deps.attemptTimeout()})

		var verdict attemptVerdict
		run, startErr := deps.Runner.StartGated(spec, gateSpec)
		if startErr != nil {
			verdict = attemptVerdict{Kind: verdictLost, Cause: fmt.Sprintf("the strand did not start: %v", startErr)}
		} else {
			attempt.StrandGUID = run.StrandGUID()
			attempt.StrandName = run.StrandName()
			if err := saveRunState(geom, st, false); err != nil {
				return batchEnd{}, errors.Join(err, stopAttempt(deps.Runner, attempt.StrandGUID))
			}
			result, waitErr := run.Wait()
			verdict = classifyAttempt(result, waitErr, attempt.ReportPath)
		}

		current, reparseErr := plan.Reparse()
		if reparseErr != nil {
			current = plan
		}
		plan = current
		out, stopErr, err := settleAttempt(settleInputs{Deps: deps, State: st, Plan: plan, Batch: batch, Label: label, Record: record, Attempt: attempt, Verdict: verdict})
		if err != nil {
			return batchEnd{}, err
		}
		if out.Outcome == outcomeDone {
			return batchEnd{Done: true}, nil
		}

		next, kind := nextAttemptRung(deps, st, number, attempt, out.Outcome)
		if stopErr != nil {
			if kind == "" {
				kind = stopKindBatch
			}
			cause := stopErr.Error()
			if out.Outcome == outcomeNeedsParent {
				cause = out.Cause + "; " + cause
			}
			return stop(kind, next, cause)
		}
		if kind != "" {
			return stop(kind, attempt.Rung, out.Cause)
		}
		rung = next
	}
}

// settleInputs is everything settleAttempt ends one attempt from.
type settleInputs struct {
	Deps    RunDeps
	State   *State
	Plan    *planparser.Plan
	Batch   batcher.Batch
	Label   string
	Record  *runplan.BatchState
	Attempt *AttemptState
	Verdict attemptVerdict
}

// settleAttempt ends one attempt through endAttempt.
// An attempt that does not record done, or whose end failed, then has its strand stopped.
// A clean end also clears the attempt's in-flight mark.
// The failed stop of a strand after a clean end is returned as stopErr, with the attempt still marked in flight.
// Any other fault, the end's own included, is returned as err.
func settleAttempt(in settleInputs) (out recordOutcome, stopErr, err error) {
	deps := in.Deps
	out, endErr := endAttempt(recordInputs{
		Geom:          deps.Geom,
		State:         in.State,
		Plan:          in.Plan,
		Batch:         in.Batch,
		Label:         in.Label,
		Record:        in.Record,
		Attempt:       in.Attempt,
		Verdict:       in.Verdict,
		ParentBranch:  deps.ParentBranch,
		CommitRecords: deps.CommitRecords,
		ReentryStep:   deps.ReentryStep,
	})
	if out.Outcome == outcomeDone && endErr == nil {
		return out, nil, nil
	}
	stopErr = stopAttempt(deps.Runner, in.Attempt.StrandGUID)
	if endErr != nil {
		return out, nil, errors.Join(endErr, stopErr)
	}
	if stopErr != nil {
		return out, stopErr, nil
	}
	in.Attempt.InFlight = false
	return out, nil, saveRunState(deps.Geom, in.State, false)
}

// nextAttemptRung judges what follows an attempt that ended with outcome without recording its batch done.
// A needs-parent attempt stops the row.
// A lost attempt retries on its own rung until the rung's lost allowance is spent, and any other ending climbs.
// A fault attempt retries on its own rung with no allowance, since the fault says nothing of the model.
// A climb past the last rung stops the row.
// It returns the rung the next attempt runs on, or the kind of the stop that ends the row.
func nextAttemptRung(deps RunDeps, st *State, number int, attempt *AttemptState, outcome string) (rung int, stopKind string) {
	switch outcome {
	case outcomeNeedsParent:
		return attempt.Rung, stopKindNeedsParent
	case outcomeFault:
		return attempt.Rung, ""
	case outcomeLost:
		if lostOnRung(st, number, attempt.Rung) < maxLostPerRung {
			return attempt.Rung, ""
		}
	}
	if attempt.Rung+1 >= len(deps.Ladder) {
		return attempt.Rung, stopKindBatch
	}
	return attempt.Rung + 1, ""
}

// doneBatches counts the batch records that are terminal and done.
func doneBatches(st *State) int {
	done := 0
	for _, record := range st.Batches {
		if record != nil && record.Terminal && record.Status == runplan.DigestStatusDone {
			done++
		}
	}
	return done
}

// latestAttempt returns the newest attempt recorded for batch, nil when there is none.
func latestAttempt(st *State, batch int) *AttemptState {
	for i := len(st.Attempts) - 1; i >= 0; i-- {
		if st.Attempts[i].Batch == batch {
			return st.Attempts[i]
		}
	}
	return nil
}

// lastCause returns the cause of batch's newest attempt, or fallback when it has none.
func lastCause(st *State, batch int, fallback string) string {
	if attempt := latestAttempt(st, batch); attempt != nil && attempt.Cause != "" {
		return attempt.Cause
	}
	return fallback
}

// lostOnRung counts the lost attempts of batch on rung, from the batch's latest re-entry attempt on.
// It counts from the batch's first attempt when none is marked.
// The count is read from the recorded attempts, so a crash and a new call never reset it.
func lostOnRung(st *State, batch, rung int) int {
	var attempts []*AttemptState
	for _, attempt := range st.Attempts {
		if attempt.Batch == batch {
			attempts = append(attempts, attempt)
		}
	}
	from := 0
	for i, attempt := range attempts {
		if attempt.Reentry {
			from = i
		}
	}
	lost := 0
	for _, attempt := range attempts[from:] {
		if attempt.Rung == rung && attempt.Outcome == outcomeLost {
			lost++
		}
	}
	return lost
}

// lostFixersOnRung counts the lost verify fixers on rung, from the latest re-entry fixer on.
// It counts from the first fixer when none is marked.
func lostFixersOnRung(st *State, rung int) int {
	from := 0
	for i, fixer := range st.Fixers {
		if fixer.Reentry {
			from = i
		}
	}
	lost := 0
	for _, fixer := range st.Fixers[from:] {
		if fixer.Rung == rung && fixer.Outcome == outcomeLost {
			lost++
		}
	}
	return lost
}

// planDirDisplay spells the plan directory relative to the panes' directory, as the prompts' card pointers carry it.
func planDirDisplay(geom Geometry) string {
	rel, err := filepath.Rel(geom.PaneCwd, geom.PlanDir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(geom.PlanDir)
	}
	return filepath.ToSlash(rel)
}

// frictionNotePath returns the friction note path of one attempt, empty when friction capture is off.
func frictionNotePath(deps RunDeps, label string, attempt int) string {
	if deps.FrictionDir == "" {
		return ""
	}
	return filepath.Join(deps.FrictionDir, fmt.Sprintf("weaver-%s-a%d.md", label, attempt))
}

// stopReason renders the one-line reason of a stop.
// It names the stopped batch with its cards, or the verify with its findings, and what stopped it.
// It then names the plan directory, the state file, the last report, the journal and the decision record.
// It ends with the step that re-enters.
// It writes no state.
func stopReason(deps RunDeps, st *State, label, cause string) string {
	geom := deps.Geom
	lastReport := "none"
	if len(st.Attempts) > 0 {
		lastReport = st.Attempts[len(st.Attempts)-1].ReportPath
	}
	cause = truncateCause(strings.Join(strings.Fields(cause), " "))
	return fmt.Sprintf("%s stopped: %s; plan directory %s; state file %s; last report %s; journal %s; decision record %s; way forward: fix what the stop names, then %s",
		label, cause, geom.PlanDir, filepath.Join(geom.RunDir, stateFileName), lastReport, JournalPath(geom.RunDir), geom.DecisionRecordPath, stepOrDefault(deps.ReentryStep))
}

// truncateCause bounds the leading text of cause and the way forward it ends with to stuckReasonBytes each.
// A way forward is cut only when it alone is overlong, since the reader acts on it.
func truncateCause(cause string) string {
	lead, wayForward := cause, ""
	if i := strings.LastIndex(cause, causeWayForwardMarker); i >= 0 {
		lead, wayForward = cause[:i], cause[i:]
	}
	return clipBytes(lead) + clipBytes(wayForward)
}

// clipBytes returns text cut to stuckReasonBytes on a rune boundary and marked with an ellipsis when it was cut.
func clipBytes(text string) string {
	if len(text) <= stuckReasonBytes {
		return text
	}
	cut := stuckReasonBytes
	for !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "..."
}

// finishRun runs the plan-level verify and its fixers once every batch is recorded.
// A failure goes to one verify fixer on the next rung, and the verify runs again before any next fixer.
// A pass writes summary.md and outcome.yaml, commits the run record and returns done.
// Every result it returns, done or stuck, carries the spans of the verify runs and fixers it started.
func finishRun(deps RunDeps, st *State) (result RunResult, err error) {
	geom := deps.Geom
	batchesDone := doneBatches(st)
	var spans []shuttleengine.GateSpan
	defer func() { result.VerifySpans = spans }()
	verifyStop := func(rung int, cause string) RunResult {
		st.Stop = &StopRecord{Kind: stopKindVerify, Rung: rung}
		if err := saveRunState(geom, st, false); err != nil {
			cause += "; the stop record could not be saved: " + err.Error()
		}
		return stuck(stopReason(deps, st, "the plan-level verify", cause))
	}

	var flaky []string
	for evaluation := 1; ; evaluation++ {
		out, err := runVerify(deps, st, evaluation, &spans)
		if err != nil {
			return stuck(stopReason(deps, st, "the plan-level verify", err.Error())), nil
		}
		if out.Passed {
			flaky = append(flaky, out.Flaky...)
			break
		}
		if out.Unjudged {
			return verifyStop(len(st.Fixers), out.Findings), nil
		}

		rung := nextFixerRung(st)
		reentryFixer := st.Stop != nil && st.Stop.Kind == stopKindVerify && !st.Stop.Reentered
		if reentryFixer {
			rung = len(deps.Ladder) - 1
		}
		if rung >= len(deps.Ladder) {
			return verifyStop(len(deps.Ladder)-1, out.Findings), nil
		}
		fixer := &FixerState{Number: len(st.Fixers) + 1, Rung: rung, Findings: out.Findings, Reentry: reentryFixer}
		fixer.ReplyPath = FixerReplyPath(geom.ReportsDir, fixer.Number)
		if reentryFixer {
			st.Stop.Reentered = true
		}
		st.Fixers = append(st.Fixers, fixer)
		if err := saveRunState(geom, st, false); err != nil {
			return stuck(stopReason(deps, st, "the plan-level verify", err.Error())), nil
		}
		stopCause, err := runFixer(deps, st, fixer, &spans)
		if err != nil {
			return stuck(stopReason(deps, st, "the plan-level verify", err.Error())), nil
		}
		if stopCause != "" {
			return verifyStop(fixer.Rung, stopCause), nil
		}
	}

	st.Stop = nil
	if err := saveRunState(geom, st, false); err != nil {
		return stuck(stopReason(deps, st, "the plan-level verify", err.Error())), nil
	}
	if err := writeSummary(deps, st, flaky); err != nil {
		return stuck(stopReason(deps, st, "the run summary", err.Error())), nil
	}
	if err := writeOutcome(OutcomePath(geom.RunDir), outcomeRecord{Outcome: resultDone, BatchesDone: batchesDone}); err != nil {
		return stuck(stopReason(deps, st, "the run outcome", err.Error())), nil
	}
	if err := deps.CommitRecords("weaver: run record"); err != nil {
		return stuck(stopReason(deps, st, "the run record commit", fmt.Sprintf("%v; the fault is transient", err))), nil
	}
	return RunResult{Outcome: resultDone, BatchesDone: batchesDone}, nil
}

// nextFixerRung returns the rung the next verify fixer runs on.
// The first fixer runs on the first rung.
// A lost fixer is retried on its rung until the rung's lost allowance is spent, and any other ending climbs.
func nextFixerRung(st *State) int {
	if len(st.Fixers) == 0 {
		return 0
	}
	last := st.Fixers[len(st.Fixers)-1]
	if last.Outcome == outcomeLost && lostFixersOnRung(st, last.Rung) < maxLostPerRung {
		return last.Rung
	}
	return last.Rung + 1
}

// runFixer spawns one verify fixer and waits for its end, then records the end on the fixer's state.
// A terminal result sets the record's status, since the wait never marks it, and the fixer's outcome.
// It returns a non-empty stop cause when the fixer's strand could not be stopped, and an error for any other fault.
// A fixer that spawned appends one rework span to spans, however its run then ends.
func runFixer(deps RunDeps, st *State, fixer *FixerState, spans *[]shuttleengine.GateSpan) (stopCause string, err error) {
	spec, err := fixerSpec(deps, st, fixer, fixer.Findings)
	if err != nil {
		return "", err
	}
	store := &fixerStore{st: st, number: fixer.Number}
	rec, _, err := deps.Fixer.SpawnOrAttach(store, spec)
	if err != nil {
		return "", err
	}
	start := deps.clock().Now()
	defer func() {
		*spans = append(*spans, shuttleengine.GateSpan{Kind: shuttleengine.GateSpanRework, Start: start, End: deps.clock().Now()})
	}()
	if err := saveRunState(deps.Geom, st, false); err != nil {
		return "", err
	}
	return awaitFixer(deps, st, fixer, store, rec)
}

// awaitFixer waits on the fixer strand rec names until it ends, then records the end on the fixer's state.
// It returns a non-empty stop cause when the fixer's strand could not be stopped, and an error for any other fault.
func awaitFixer(deps RunDeps, st *State, fixer *FixerState, store *fixerStore, rec *gatefixer.Record) (stopCause string, err error) {
	geom := deps.Geom
	timeout := time.Duration(deps.Config.FixerTimeoutMin) * time.Minute
	var result *gatefixer.Result
	for {
		if result, err = deps.Fixer.Await(rec, timeout, fixerWaitSlice); err != nil {
			return "", err
		}
		if result.Status != gatefixer.StatusRunning {
			break
		}
	}
	rec.Status = result.Status
	rec.DeadReason = result.DeadReason
	if err := store.Save(rec); err != nil {
		return "", err
	}
	fixer.Outcome = outcomeDone
	if result.Status != gatefixer.StatusDone {
		fixer.Outcome = outcomeLost
	}
	if err := saveRunState(geom, st, false); err != nil {
		return "", err
	}
	for _, warning := range result.Warnings {
		if strings.HasPrefix(warning, "stop strand ") {
			return (&StrandStopError{GUID: rec.StrandGUID, Err: errors.New(warning)}).Error(), nil
		}
	}
	return "", nil
}
