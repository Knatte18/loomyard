// resume.go holds what a new call of Weaver's row does before it begins a batch.
// It finds the strands a crash left running and judges where the open batch stands.
// After a stop it re-enters the stopped batch, the needs-parent step or the plan-level verify.

package weaverengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// reentryWayForward is the way forward of a plan whose card set no longer matches the partition a stopped run recorded.
const reentryWayForward = "put the plan's card set back as the recorded partition holds it, or run lym weaver reset to start the run over from the plan it started with"

// resumeLabel names a resume in a stop reason.
const resumeLabel = "the resume of the run's strands"

// reentryLabel names a re-entry in a stop reason.
const reentryLabel = "the re-entry"

// reentrySite labels the batch gate runs of a re-entry in the running marker.
const reentrySite = "weaver re-entry"

// resumeResult is what resumeStrands judged about the run a crash interrupted.
type resumeResult struct {
	// Batch is the number of the begun batch with no terminal record that continues, zero when none does.
	Batch int
	// Rung is the ladder rung that batch's next attempt starts on.
	Rung int
	// Stuck is the stop reason of a resume that must stop the row before it starts anything, empty when it goes on.
	Stuck string
}

// reentry is what reenter decided about the stop a call re-enters.
type reentry struct {
	// Batch is the stopped batch whose next attempt runs first, zero when the run goes on from its next begin.
	Batch int
	// Rung is the ladder rung that attempt starts on.
	Rung int
	// Verify is set when no batch is left to run, so the plan-level verify is next.
	Verify bool
}

// resumeStrands settles the strands an earlier call left behind and judges where the open batch stands.
// It stops the strand of every attempt and fixer whose outcome state records.
// It then probes each attempt recorded in flight by its report path.
// It records the result of a run it finds, and counts a run it does not find as a machinery loss.
// It attaches a fixer whose record is live, and probes a fixer recorded without one.
// A failed stop or a failed probe is returned as a stuck reason, with nothing started.
// Last it judges the open batch from its last attempt, unless a recorded stop is still to be re-entered.
// The attempt count behind that judgement is read from the recorded attempts, so a crash never resets it.
func resumeStrands(deps RunDeps, st *State) (resumeResult, error) {
	stuckWith := func(cause string) resumeResult {
		return resumeResult{Stuck: stopReason(deps, st, resumeLabel, cause)}
	}

	cause, err := stopRecordedStrands(deps, st)
	if err != nil || cause != "" {
		return stuckWith(cause), err
	}
	if cause, err = resumeAttempts(deps, st); err != nil || cause != "" {
		return stuckWith(cause), err
	}
	if cause, err = resumeFixers(deps, st); err != nil || cause != "" {
		return stuckWith(cause), err
	}
	return judgeOpenBatch(deps, st)
}

// stopRecordedStrands stops the strand of every attempt with a recorded outcome and an uncleared in-flight mark.
// It also stops the strand of every fixer whose outcome is recorded.
// It returns the failed stop's text as a stop cause.
func stopRecordedStrands(deps RunDeps, st *State) (stopCause string, err error) {
	changed := false
	for _, attempt := range st.Attempts {
		if attempt.Outcome == "" || !attempt.InFlight {
			continue
		}
		if err := stopAttempt(deps.Runner, attempt.StrandGUID); err != nil {
			return err.Error(), nil
		}
		attempt.InFlight = false
		changed = true
	}
	for _, fixer := range st.Fixers {
		if fixer.Outcome == "" || fixer.Record == nil {
			continue
		}
		if err := stopAttempt(deps.Runner, fixer.Record.StrandGUID); err != nil {
			return err.Error(), nil
		}
	}
	if !changed {
		return "", nil
	}
	return "", saveRunState(deps.Geom, st, false)
}

// resumeAttempts resumes every attempt recorded in flight with no outcome, one after another.
func resumeAttempts(deps RunDeps, st *State) (stopCause string, err error) {
	var open []*AttemptState
	for _, attempt := range st.Attempts {
		if attempt.InFlight && attempt.Outcome == "" {
			open = append(open, attempt)
		}
	}
	if len(open) == 0 {
		return "", nil
	}
	plan, err := planparser.ParsePlan(deps.Geom.PlanDir)
	if err != nil {
		return "", err
	}
	for _, attempt := range open {
		if cause, err := resumeAttempt(deps, st, plan, attempt); err != nil || cause != "" {
			return cause, err
		}
		if reparsed, reparseErr := plan.Reparse(); reparseErr == nil {
			plan = reparsed
		}
	}
	return "", nil
}

// resumeAttempt finds the run of one in-flight attempt by its report path and ends the attempt on the run's result.
// A found run is a handle nobody waited on: its strand is recorded and sent the decisions it lacks before the wait.
// A run that is not found carries no gate verdict and counts as a machinery loss, never as done.
func resumeAttempt(deps RunDeps, st *State, plan *planparser.Plan, attempt *AttemptState) (stopCause string, err error) {
	geom := deps.Geom
	if attempt.Rung < 0 || attempt.Rung >= len(deps.Ladder) {
		return "", fmt.Errorf("weaver: attempt %d of batch %02d names rung %d, but the ladder holds %d rung(s)", attempt.Number, attempt.Batch, attempt.Rung+1, len(deps.Ladder))
	}
	batches, err := runplan.MapPartition(plan, st.Partition, partitionWayForward(deps.ReentryStep))
	if err != nil {
		return "", err
	}
	batch, err := runplan.FindBatch(batches, attempt.Batch, "the partition the run recorded holds it")
	if err != nil {
		return "", err
	}
	record := st.Batches[attempt.Batch]
	if record == nil {
		return "", fmt.Errorf("weaver: attempt %d is recorded for batch %02d, which has no record", attempt.Number, attempt.Batch)
	}
	_, slug := runplan.BatchIdentity(batch)
	label := fmt.Sprintf("%02d-%s", attempt.Batch, slug)

	gateSpec, err := batchGateSpec(geom, batch.Cards, runplan.IsLastBatch(batches, batch), deps.Gate)
	if err != nil {
		return "", err
	}
	spec := attemptSpec(attemptLaunch{ReportPath: attempt.ReportPath, Choice: deps.Ladder[attempt.Rung], Label: label, Skills: deps.LanguageSkills, Timeout: deps.attemptTimeout()})
	run, found, err := deps.Runner.ProbeGated(spec, gateSpec)
	if err != nil {
		return fmt.Sprintf("probing for the run of attempt %d of batch %s failed: %v; way forward: follow the way forward the probe's error names", attempt.Number, label, err), nil
	}

	var verdict attemptVerdict
	if !found {
		verdict = attemptVerdict{Kind: verdictLost, Cause: "no live run holds the attempt's report path, so its strand was lost"}
	} else {
		attempt.StrandGUID = run.StrandGUID()
		attempt.StrandName = run.StrandName()
		if attempt.StrandName == "" {
			attempt.StrandName = strandNameOf(deps.Reed, attempt.StrandGUID)
		}
		if err := deliverUndelivered(deps, attempt.StrandName, &attempt.Delivered); err != nil {
			return "", err
		}
		if err := saveRunState(geom, st, false); err != nil {
			return "", err
		}
		result, waitErr := run.Wait()
		verdict = classifyAttempt(result, waitErr, attempt.ReportPath)
	}

	_, stopErr, err := settleAttempt(settleInputs{Deps: deps, State: st, Plan: plan, Batch: batch, Label: label, Record: record, Attempt: attempt, Verdict: verdict})
	if err != nil {
		return "", err
	}
	if stopErr != nil {
		return stopErr.Error(), nil
	}
	return "", nil
}

// deliverUndelivered sends strandName the decision entries whose headings delivered lacks, and records the headings sent.
// It sends nothing when no delivery seam was told or the strand has no name.
func deliverUndelivered(deps RunDeps, strandName string, delivered *[]string) error {
	if deps.Deliver == nil || strandName == "" {
		return nil
	}
	sent, err := deps.Deliver(strandName, *delivered)
	if err != nil {
		return fmt.Errorf("weaver: deliver the recorded decisions to strand %s: %w", strandName, err)
	}
	*delivered = mergeDelivered(*delivered, sent)
	return nil
}

// resumeFixers resumes every verify fixer recorded with no outcome.
// A fixer whose record is terminal takes its outcome from the record.
// A fixer whose record is live is attached to, sent the decisions it lacks and awaited.
// A fixer recorded without a record is probed by its reply path, and a run found is stopped.
// A fixer with no live strand is lost, and the verify runs again before any new fixer.
func resumeFixers(deps RunDeps, st *State) (stopCause string, err error) {
	for _, fixer := range st.Fixers {
		if fixer.Outcome != "" {
			continue
		}
		switch {
		case fixer.Record != nil && fixer.Record.Status != "":
			fixer.Outcome = outcomeLost
			if fixer.Record.Status == gatefixer.StatusDone {
				fixer.Outcome = outcomeDone
			}
		case fixer.Record != nil:
			cause, err := attachFixer(deps, st, fixer)
			if err != nil || cause != "" {
				return cause, err
			}
		default:
			cause, err := probeUnrecordedFixer(deps, fixer)
			if err != nil || cause != "" {
				return cause, err
			}
		}
		if err := saveRunState(deps.Geom, st, false); err != nil {
			return "", err
		}
	}
	return "", nil
}

// attachFixer attaches to the live record of fixer, sends its strand the decisions it lacks and awaits it.
func attachFixer(deps RunDeps, st *State, fixer *FixerState) (stopCause string, err error) {
	spec, err := fixerSpec(deps, st, fixer, fixer.Findings)
	if err != nil {
		return "", err
	}
	store := &fixerStore{st: st, number: fixer.Number}
	rec, _, err := deps.Fixer.SpawnOrAttach(store, spec)
	if err != nil {
		return "", err
	}
	if fixer.StrandName == "" {
		fixer.StrandName = strandNameOf(deps.Reed, rec.StrandGUID)
	}
	if err := deliverUndelivered(deps, fixer.StrandName, &fixer.Delivered); err != nil {
		return "", err
	}
	if err := saveRunState(deps.Geom, st, false); err != nil {
		return "", err
	}
	return awaitFixer(deps, st, fixer, store, rec)
}

// strandNameOf returns the name reed lists for the strand guid names, empty when reed lists none.
func strandNameOf(reed shuttleengine.ReedOps, guid string) string {
	status, err := reed.Status()
	if err != nil {
		return ""
	}
	for _, strand := range status.Strands {
		if strand.GUID == guid {
			return strand.Name
		}
	}
	return ""
}

// probeUnrecordedFixer looks for the run of a fixer whose spawn never recorded its strand, by the fixer's reply path.
// A run it finds is stopped through its handle, and the fixer counts as a machinery loss either way.
func probeUnrecordedFixer(deps RunDeps, fixer *FixerState) (stopCause string, err error) {
	if fixer.Rung < 0 || fixer.Rung >= len(deps.Ladder) {
		return "", fmt.Errorf("weaver: fixer %d names rung %d, but the ladder holds %d rung(s)", fixer.Number, fixer.Rung+1, len(deps.Ladder))
	}
	reply, err := filepath.Abs(fixer.ReplyPath)
	if err != nil {
		return "", fmt.Errorf("weaver: resolve fixer %d's reply path: %w", fixer.Number, err)
	}
	run, found, err := deps.Runner.ProbeGated(shuttleengine.Spec{OutputFiles: []string{reply}, ModelChoice: deps.Ladder[fixer.Rung]}, nil)
	if err != nil {
		return fmt.Sprintf("probing for the run of verify fixer %d failed: %v; way forward: follow the way forward the probe's error names", fixer.Number, err), nil
	}
	if found {
		if err := run.Stop(); err != nil {
			return (&StrandStopError{GUID: run.StrandGUID(), Err: err}).Error(), nil
		}
	}
	fixer.Outcome = outcomeLost
	return "", nil
}

// judgeOpenBatch judges the begun batch with no terminal record from its last attempt.
// It judges nothing while a recorded stop awaits its re-entry, since the re-entry decides where the batch starts.
// A failed attempt starts the next one on the next rung.
// A lost one retries on its own rung until the rung's lost allowance is spent.
// A fault one retries on its own rung, since the fault says nothing of the model.
// A failure on the last rung, and a needs-parent attempt, stop the row and record the stop.
func judgeOpenBatch(deps RunDeps, st *State) (resumeResult, error) {
	if st.Stop != nil && !st.Stop.Reentered {
		return resumeResult{}, nil
	}
	number, record := openBatch(st)
	if record == nil {
		return resumeResult{}, nil
	}
	last := latestAttempt(st, number)
	if last == nil {
		return resumeResult{Batch: number}, nil
	}
	rung, kind := nextAttemptRung(deps, st, number, last, last.Outcome)
	if kind == "" {
		return resumeResult{Batch: number, Rung: rung}, nil
	}
	st.Stop = &StopRecord{Kind: kind, Batch: number, Rung: last.Rung}
	if err := saveRunState(deps.Geom, st, false); err != nil {
		return resumeResult{}, err
	}
	label := fmt.Sprintf("batch %02d-%s (cards %s)", number, record.Slug, strings.Join(record.Cards, ", "))
	return resumeResult{Stuck: stopReason(deps, st, label, last.Cause)}, nil
}

// openBatch returns the number and record of the lowest-numbered batch with no terminal record.
// The record is nil when no batch is open.
func openBatch(st *State) (int, *runplan.BatchState) {
	number := 0
	var open *runplan.BatchState
	for n, record := range st.Batches {
		if record == nil || record.Terminal {
			continue
		}
		if open == nil || n < number {
			number, open = n, record
		}
	}
	return number, open
}

// reenter decides where a call that finds a recorded stop starts again.
// It maps the recorded partition onto the plan and restamps the plan baseline,
// so an edit to a card's text, the stopped batch's included, carries into the next attempt.
// A plan that lost a recorded card or gained an unrecorded one is refused with reentryWayForward.
// A verify stop returns the verify as next, and a needs-parent stop returns the asking batch and rung.
// A batch stop is checked at HEAD.
// A stopped batch whose post-batch checks and gates pass is recorded done with no spawn.
// A row with gate entries of its own never takes that path.
// Any other stopped batch gets one attempt on the last rung.
func reenter(deps RunDeps, st *State, plan *planparser.Plan) (reentry, error) {
	geom := deps.Geom
	batches, err := runplan.MapPartition(plan, st.Partition, reentryWayForward)
	if err != nil {
		return reentry{}, err
	}
	if err := runplan.RestampBaseline(&st.State, geom.PlanDir, geom.RunDir); err != nil {
		return reentry{}, err
	}

	stop := st.Stop
	switch stop.Kind {
	case stopKindVerify:
		return reentry{Verify: true}, saveRunState(geom, st, false)
	case stopKindNeedsParent:
		return reentry{Batch: stop.Batch, Rung: stop.Rung}, saveRunState(geom, st, false)
	}

	batch, err := runplan.FindBatch(batches, stop.Batch, "the partition the run recorded holds it")
	if err != nil {
		return reentry{}, err
	}
	record := st.Batches[stop.Batch]
	if record == nil {
		return reentry{}, fmt.Errorf("weaver: the stop names batch %02d, which has no record", stop.Batch)
	}
	if record.CardHashes, err = runplan.BatchCardHashes(batch, geom.PlanDir); err != nil {
		return reentry{}, err
	}
	if err := saveRunState(geom, st, false); err != nil {
		return reentry{}, err
	}

	deviations, reasons, err := checkStoppedBatch(deps, st, plan, record)
	if err != nil {
		return reentry{}, err
	}
	if len(reasons) == 0 && len(deps.Gate) == 0 {
		passed, err := stoppedBatchGatesPass(deps, batch, runplan.IsLastBatch(batches, batch))
		if err != nil {
			return reentry{}, err
		}
		if passed {
			return reentry{}, recordDoneWithoutSpawn(deps, st, record, deviations)
		}
	}
	return reentry{Batch: stop.Batch, Rung: len(deps.Ladder) - 1}, nil
}

// checkStoppedBatch runs the clean-tree check and the shared post-batch checks over a stopped batch's work at HEAD.
// The checks read from the batch's recorded start commit, with no renames.
// It checks no report's head_sha, since a repair committed after the stop moves HEAD past the last report.
// It returns the scope guard's deviations and the reasons the batch does not pass, none when it does.
func checkStoppedBatch(deps RunDeps, st *State, plan *planparser.Plan, batch *runplan.BatchState) (deviations, reasons []string, err error) {
	geom := deps.Geom
	batches, err := runplan.MapPartition(plan, st.Partition, reentryWayForward)
	if err != nil {
		return nil, nil, err
	}
	index := slices.IndexFunc(batches, func(b batcher.Batch) bool { return slices.Equal(runplan.BatchCardIDs(b), batch.Cards) })
	if index < 0 {
		return nil, nil, fmt.Errorf("weaver: no batch of the recorded partition holds cards %s", strings.Join(batch.Cards, ", "))
	}
	stopped := batches[index]
	number, slug := runplan.BatchIdentity(stopped)
	label := fmt.Sprintf("%02d-%s", number, slug)

	dirty, err := checkCleanTree(geom.git(), geom.WorktreeRoot)
	if err != nil {
		return nil, nil, err
	}
	if len(dirty) > 0 {
		return nil, []string{fmt.Sprintf("the worktree holds uncommitted paths: %s; commit or remove them", strings.Join(dirty, ", "))}, nil
	}
	head, err := geom.git().HeadSHA(geom.WorktreeRoot)
	if err != nil {
		return nil, nil, err
	}

	deviations, warnings, err := runplan.PostBatchChecks(runplan.PostBatchInputs{
		Plan:                   plan,
		State:                  &st.State,
		Batch:                  batch,
		Geom:                   geom.runPlan(),
		Batches:                batches,
		Number:                 number,
		Cards:                  stopped.Cards,
		DecisionRecordPath:     geom.DecisionRecordPath,
		StartSHA:               batch.StartSHA,
		HeadSHA:                head,
		Label:                  label,
		HandBackRemovedTargets: true,
		HandBackWayForward:     "the next attempt of this batch is told this finding: restore the removed symbol or report its rename",
		RecordWarning: func(id, class, detail string) (string, bool) {
			return recordBatchWarning(batch, id, class, detail)
		},
	})
	for _, warning := range warnings {
		logger.Warn("weaver: post-batch warning", "batch", label, "warning", warning)
	}
	if err != nil {
		if errors.Is(err, runplan.ErrCardNotDone) || errors.Is(err, runplan.ErrLaterTargetRemoved) {
			return nil, []string{err.Error()}, nil
		}
		return nil, nil, err
	}
	return deviations, nil, nil
}

// stoppedBatchGatesPass runs at HEAD the calls batchGateCalls returns for batch, each through the verify seam.
// It reports whether every one passed, and stops at the first that does not.
func stoppedBatchGatesPass(deps RunDeps, batch batcher.Batch, last bool) (bool, error) {
	calls, err := batchGateCalls(deps.Geom, batch.Cards, last)
	if err != nil {
		return false, err
	}
	verify := deps.verify()
	for _, call := range calls {
		res, err := verify(verifytree.Site{Label: reentrySite, Attempt: 1, Hold: deps.Geom.VerifyHold}, call)
		if err != nil {
			return false, fmt.Errorf("weaver: re-entry gate %q: %w", call, err)
		}
		if res.Status != verifytree.StatusPassed && res.Status != verifytree.StatusSkipped {
			return false, nil
		}
	}
	return true, nil
}

// recordDoneWithoutSpawn takes a stopped batch's record terminal done without an attempt.
// It records no distilled digest, since that needs a report.
// The digest it writes carries the deviations, the batch's departures and its head.
// The card trail is the head, as endAttempt records it.
// It sets the batch's end commit from HEAD, clears the stop record, saves the state and commits the run records.
func recordDoneWithoutSpawn(deps RunDeps, st *State, batch *runplan.BatchState, deviations []string) error {
	geom := deps.Geom
	number := 0
	for n, record := range st.Batches {
		if record == batch {
			number = n
		}
	}
	label := fmt.Sprintf("%02d-%s", number, batch.Slug)

	head, err := geom.git().HeadSHA(geom.WorktreeRoot)
	if err != nil {
		return fmt.Errorf("weaver: read HEAD to record batch %s done: %w", label, err)
	}
	departures, err := runplan.BatchDepartures(geom.runPlan(), batch.StartSHA, head)
	if err != nil {
		return err
	}
	batch.Digest = &runplan.Digest{Batch: label, Status: runplan.DigestStatusDone, HeadSHA: head, Deviations: deviations, Departures: departures}
	batch.CardSHAs = []string{head}
	batch.Terminal = true
	batch.Status = runplan.DigestStatusDone
	if st.BatchEnds == nil {
		st.BatchEnds = map[int]string{}
	}
	st.BatchEnds[number] = head
	st.LastEnd = head
	st.Stop = nil
	if err := saveRunState(geom, st, false); err != nil {
		return err
	}
	return commitRecords(deps.CommitRecords, label, deps.ReentryStep)
}
