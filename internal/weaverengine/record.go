// record.go ends every batch attempt the same way.
// It records the attempt's outcome, journals its report, saves the run state and commits Weaver's records.
// A recordable attempt first passes the shared post-batch checks, and a pass takes its batch terminal done.

package weaverengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// The outcomes an attempt record takes once it ends.
const (
	outcomeDone        = "done"
	outcomeFailed      = "failed"
	outcomeLost        = "lost"
	outcomeNeedsParent = "needs_parent"
	// outcomeFault marks an attempt whose end Weaver could not judge, since a check or a write of its own failed.
	outcomeFault = "fault"
)

// defaultReentryStep is the step that re-enters the run when the told one is empty.
const defaultReentryStep = "re-step the row"

// minAbbreviatedSHA is the shortest hex head_sha that resolves as an abbreviation.
const minAbbreviatedSHA = 4

// fullSHALength is the length of a full commit id in hex digits.
const fullSHALength = 40

// recordInputs is everything endAttempt records one ended attempt from.
type recordInputs struct {
	// Geom is the told geometry the worktree, the run directories and the state are read from.
	Geom Geometry
	// State is the run state, which endAttempt updates and saves.
	State *State
	// Plan is the parsed plan the post-batch checks read.
	Plan *planparser.Plan
	// Batch is the batch the attempt builds, and Label names it as NN-<slug>.
	Batch batcher.Batch
	Label string
	// Record is the batch's own record in State.Batches.
	Record *runplan.BatchState
	// Attempt is the ended attempt's record in State.Attempts.
	Attempt *AttemptState
	// Verdict is how the attempt ended, and carries its parsed report when one parsed.
	Verdict attemptVerdict
	// ParentBranch names the branch a clean parent merge-in comes from; nil accepts no merge.
	ParentBranch websterengine.ParentBranchFunc
	// CommitRecords commits Weaver's run directory and the plan directory.
	CommitRecords func(msg string) error
	// ReentryStep is the step that re-enters the run, and an empty one means defaultReentryStep.
	ReentryStep string
}

// recordOutcome is what endAttempt recorded about one attempt.
type recordOutcome struct {
	// Outcome is the attempt's recorded outcome, one of the outcome constants.
	Outcome string
	// Cause says why a failed, lost or fault attempt ended so.
	Cause string
	// Uncommitted holds the tracked paths a failed attempt left uncommitted.
	Uncommitted []string
}

// endAttempt ends one attempt, whatever its verdict.
// A failed, lost or needs-parent attempt keeps its journal entry and its commit, as a done one does.
// A recordable verdict first passes the clean-tree check, the head check and the shared post-batch checks.
// A pass takes the batch's record terminal done and clears the run's stop record.
// Any other finding fails the attempt.
// Every verdict then records the attempt's outcome and cause and the batch's end commit.
// It appends the report's journal when a report parsed, saves the state and commits.
// The journal append precedes the state save, so a crash between them re-appends nothing.
// A check that cannot run, a failed save or a failed commit is returned as an error.
// The outcome is returned beside it as far as it was recorded.
func endAttempt(in recordInputs) (recordOutcome, error) {
	var checkErr error
	var out recordOutcome
	switch in.Verdict.Kind {
	case verdictRecordable:
		out, checkErr = checkRecordable(in)
	case verdictFailed:
		out = recordOutcome{Outcome: outcomeFailed, Cause: in.Verdict.Cause}
	case verdictLost:
		out = recordOutcome{Outcome: outcomeLost, Cause: in.Verdict.Cause}
	case verdictNeedsParent:
		out = recordOutcome{Outcome: outcomeNeedsParent, Cause: in.Verdict.Need}
	default:
		return recordOutcome{}, fmt.Errorf("weaver: attempt %d of batch %s ended with unknown verdict %q", in.Attempt.Number, in.Label, in.Verdict.Kind)
	}
	if checkErr != nil {
		out = recordOutcome{Outcome: outcomeFault, Cause: checkErr.Error()}
	}

	head, err := in.Geom.git().HeadSHA(in.Geom.WorktreeRoot)
	if err != nil {
		return out, errors.Join(checkErr, fmt.Errorf("weaver: read HEAD to record the end of batch %s: %w", in.Label, err))
	}
	number, _ := runplan.BatchIdentity(in.Batch)
	in.Attempt.Outcome = out.Outcome
	in.Attempt.Cause = out.Cause
	in.Attempt.Uncommitted = out.Uncommitted
	if out.Outcome == outcomeDone {
		in.Attempt.InFlight = false
	}
	if in.State.BatchEnds == nil {
		in.State.BatchEnds = map[int]string{}
	}
	in.State.BatchEnds[number] = head
	in.State.LastEnd = head

	if report := in.Verdict.Report; report != nil {
		if err := AppendJournal(JournalPath(in.Geom.RunDir), JournalHeading(in.Label, in.Attempt.Number), report.Journal); err != nil {
			return out, errors.Join(checkErr, err)
		}
	}
	if err := saveRunState(in.Geom, in.State, false); err != nil {
		return out, errors.Join(checkErr, err)
	}
	if err := commitRecords(in.CommitRecords, in.Label, in.ReentryStep); err != nil {
		return out, errors.Join(checkErr, err)
	}
	return out, checkErr
}

// checkRecordable runs the checks a recordable attempt owes before its batch is recorded done.
// A finding about the attempt's work is returned as a failed outcome, and a check that cannot run as an error.
func checkRecordable(in recordInputs) (recordOutcome, error) {
	git := in.Geom.git()
	report := in.Verdict.Report

	dirty, err := checkCleanTree(git, in.Geom.WorktreeRoot)
	if err != nil {
		return recordOutcome{}, err
	}
	if len(dirty) > 0 {
		cause := fmt.Sprintf("the worktree holds uncommitted paths the attempt left: %s; commit or remove them", strings.Join(dirty, ", "))
		return recordOutcome{Outcome: outcomeFailed, Cause: cause, Uncommitted: dirty}, nil
	}

	head, problem, err := reconcileReportHead(git, in.Geom.WorktreeRoot, report.HeadSHA, in.ParentBranch)
	if err != nil {
		return recordOutcome{}, err
	}
	if problem != "" {
		return recordOutcome{Outcome: outcomeFailed, Cause: problem}, nil
	}

	batches, err := runplan.MapPartition(in.Plan, in.State.Partition, partitionWayForward(in.ReentryStep))
	if err != nil {
		return recordOutcome{}, err
	}
	number, _ := runplan.BatchIdentity(in.Batch)
	deviations, warnings, err := runplan.PostBatchChecks(runplan.PostBatchInputs{
		Plan:                   in.Plan,
		State:                  &in.State.State,
		Batch:                  in.Record,
		Geom:                   in.Geom.runPlan(),
		Batches:                batches,
		Number:                 number,
		Cards:                  in.Batch.Cards,
		StartSHA:               in.Record.StartSHA,
		HeadSHA:                head,
		Label:                  in.Label,
		Renames:                report.Renames,
		HandBackRemovedTargets: true,
		HandBackWayForward:     "the next attempt of this batch is told this finding: restore the removed symbol or report its rename",
		RecordWarning: func(id, class, detail string) (string, bool) {
			return recordBatchWarning(in.Record, id, class, detail)
		},
	})
	for _, warning := range warnings {
		logger.Warn("weaver: post-batch warning", "batch", in.Label, "warning", warning)
	}
	if err != nil {
		if errors.Is(err, runplan.ErrCardNotDone) || errors.Is(err, runplan.ErrLaterTargetRemoved) {
			return recordOutcome{Outcome: outcomeFailed, Cause: err.Error()}, nil
		}
		return recordOutcome{}, err
	}

	departures, err := runplan.BatchDepartures(in.Geom.runPlan(), in.Record.StartSHA, head)
	if err != nil {
		return recordOutcome{}, err
	}
	digest := runplan.Distill(report)
	digest.Batch = in.Label
	digest.Deviations = deviations
	digest.Departures = departures
	in.Record.Digest = &digest
	in.Record.CardSHAs = []string{head}
	in.Record.Terminal = true
	in.Record.Status = digest.Status
	in.State.Stop = nil
	return recordOutcome{Outcome: outcomeDone}, nil
}

// reconcileReportHead resolves a report's head_sha and cross-checks it against the worktree's HEAD.
// It returns the full SHA the batch is recorded at, or a problem naming why the head cannot be accepted.
// HEAD equal to the head is accepted.
// Otherwise HEAD's first-parent chain is walked down to the head, and each commit on it must be a clean parent merge.
// A merge is clean when Git.MergeRejection accepts it against the told parent branch.
// A git failure is returned as an error.
func reconcileReportHead(git websterengine.Git, worktree, reportHead string, parentBranch websterengine.ParentBranchFunc) (head, problem string, err error) {
	head = reportHead
	if len(reportHead) >= minAbbreviatedSHA && len(reportHead) < fullSHALength && isHex(reportHead) {
		commits, err := git.CommitsNamedBy(worktree, strings.ToLower(reportHead))
		if err != nil {
			return "", "", err
		}
		if len(commits) != 1 {
			return "", fmt.Sprintf("the report's head_sha %q names %d commits; it must be the output of `git rev-parse HEAD`", reportHead, len(commits)), nil
		}
		head = commits[0]
	}

	current, err := git.HeadSHA(worktree)
	if err != nil {
		return "", "", err
	}
	for walked := current; walked != head; {
		parents, err := git.CommitParents(worktree, walked)
		if err != nil {
			return "", "", fmt.Errorf("weaver: walk the first-parent chain from %s: %w", current, err)
		}
		if len(parents) < 2 {
			return "", fmt.Sprintf("the report's head_sha %s is not the worktree's HEAD %s, and only clean parent merges may sit between them", head, current), nil
		}
		if reason := git.MergeRejection(worktree, walked, parents, parentBranch); reason != "" {
			return "", fmt.Sprintf("commit %s above the report's head_sha %s is not a clean merge of the parent branch: %s", walked, head, reason), nil
		}
		walked = parents[0]
	}
	return head, "", nil
}

// isHex reports whether s is non-empty and holds only hex digits of either case.
func isHex(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return s != ""
}

// partitionWayForward is the way forward of a recorded partition that no longer matches the plan.
func partitionWayForward(reentryStep string) string {
	return "restore the plan the run recorded, or start the run over with `lym weaver reset`, then " + stepOrDefault(reentryStep)
}

// stepOrDefault returns step, or defaultReentryStep when step is empty.
func stepOrDefault(step string) string {
	if step == "" {
		return defaultReentryStep
	}
	return step
}

// recordBatchWarning records a later card's drift finding once on bs, keyed by id.
// It returns the warning's text and whether it was newly recorded.
func recordBatchWarning(bs *runplan.BatchState, id, class, detail string) (string, bool) {
	if slices.ContainsFunc(bs.AuditWarnings, func(w runplan.AuditWarning) bool { return w.Identity == id }) {
		return "", false
	}
	bs.AuditWarnings = append(bs.AuditWarnings, runplan.AuditWarning{Identity: id, Class: class, Detail: detail})
	return detail, true
}

// checkCleanTree returns the worktree's uncommitted paths through Git.DirtyPaths, empty for a clean tree.
func checkCleanTree(git websterengine.Git, worktree string) ([]string, error) {
	paths, err := git.DirtyPaths(worktree)
	if err != nil {
		return nil, fmt.Errorf("weaver: read the worktree's uncommitted paths: %w", err)
	}
	return paths, nil
}

// saveRunState is Weaver's one save of its state from Run.
// With leaseHeld unset it first acquires the state-mutation lease over the scratch directory.
// With it set the caller already holds the lease, and a second acquire in the same process would block forever.
// Under the lease it loads the state file.
// It merges each attempt's and fixer's delivered headings as loaded with those st holds.
// It then writes the result and leaves st holding the merged lists.
// The decision verb's process records deliveries into the same file, so a save never drops a heading it recorded.
func saveRunState(geom Geometry, st *State, leaseHeld bool) error {
	if !leaseHeld {
		lease, err := runplan.AcquireStateMutation(geom.ScratchDir)
		if err != nil {
			return fmt.Errorf("weaver: save the run state: %w", err)
		}
		defer func() {
			if err := lease.Release(); err != nil {
				logger.Warn("weaver: release the state-mutation lease", "cause", err)
			}
		}()
	}
	loaded, err := LoadState(geom.RunDir, geom.ScratchDir)
	if err != nil {
		return err
	}
	if loaded != nil {
		for _, attempt := range st.Attempts {
			for _, before := range loaded.Attempts {
				if before.Batch == attempt.Batch && before.Number == attempt.Number {
					attempt.Delivered = mergeDelivered(before.Delivered, attempt.Delivered)
				}
			}
		}
		for _, fixer := range st.Fixers {
			for _, before := range loaded.Fixers {
				if before.Number == fixer.Number {
					fixer.Delivered = mergeDelivered(before.Delivered, fixer.Delivered)
				}
			}
		}
	}
	return SaveState(geom.RunDir, geom.ScratchDir, st)
}

// mergeDelivered returns the headings in loaded followed by those in held that loaded lacks.
func mergeDelivered(loaded, held []string) []string {
	merged := slices.Clone(loaded)
	for _, heading := range held {
		if !slices.Contains(merged, heading) {
			merged = append(merged, heading)
		}
	}
	return merged
}

// RecordDelivery adds headings to the Delivered list of the attempt or fixer whose strand is strandName.
// It runs under the state-mutation lease, so it never races a batch begin or a save of the run's own process.
// An empty strand name, a name the state does not record and an absent state file change nothing.
func RecordDelivery(anchorRoot, strandName string, headings []string) error {
	if strandName == "" {
		return nil
	}
	runDir, scratchDir := Dir(anchorRoot), ScratchDir(anchorRoot)
	lease, err := runplan.AcquireStateMutation(scratchDir)
	if err != nil {
		return fmt.Errorf("weaver: record a delivery: %w", err)
	}
	defer func() {
		if err := lease.Release(); err != nil {
			logger.Warn("weaver: release the state-mutation lease", "cause", err)
		}
	}()

	st, err := LoadState(runDir, scratchDir)
	if err != nil || st == nil {
		return err
	}
	changed := false
	for _, attempt := range st.Attempts {
		if attempt.StrandName == strandName {
			changed = addDelivered(&attempt.Delivered, headings) || changed
		}
	}
	for _, fixer := range st.Fixers {
		if fixer.StrandName == strandName {
			changed = addDelivered(&fixer.Delivered, headings) || changed
		}
	}
	if !changed {
		return nil
	}
	return SaveState(runDir, scratchDir, st)
}

// addDelivered appends the headings *delivered lacks and reports whether it added any.
func addDelivered(delivered *[]string, headings []string) bool {
	merged := mergeDelivered(*delivered, headings)
	if len(merged) == len(*delivered) {
		return false
	}
	*delivered = merged
	return true
}

// commitRecords commits Weaver's run directory and the plan directory through the told commit seam.
// The commit message is `weaver: batch <label>`.
// A failure is returned with its way forward: the fault is transient, so re-step the row.
func commitRecords(commit func(msg string) error, label, reentryStep string) error {
	if commit == nil {
		return errors.New("weaver: no commit seam was told; way forward: the CLI layer must set RunDeps.CommitRecords")
	}
	if err := commit("weaver: batch " + label); err != nil {
		return fmt.Errorf("weaver: commit the run records of batch %s: %w; way forward: the fault is transient, %s", label, err, stepOrDefault(reentryStep))
	}
	return nil
}
