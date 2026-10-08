// reset.go decides, read-only, what `lyx webster reset` may do.
// PlanReset resolves the commit a reset moves the task branch to and the paths the run itself wrote, and refuses with a way forward before fabric is ever called.
// Webster runs no mutating git: every probe here is a read, and the reset itself belongs to fabric.

package websterengine

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ResetTarget names the commit a reset moves the task branch to.
// No raw SHA is accepted: the target is always one the run recorded.
type ResetTarget string

const (
	// ResetToStart is the run's start commit.
	ResetToStart ResetTarget = "start"
	// ResetToPreFix is the HEAD the verify gate started its fixes from.
	ResetToPreFix ResetTarget = "pre-fix"
	// ResetToReportHead is the head_sha of the report the named in-flight batch's fork wrote, taking --batch.
	ResetToReportHead ResetTarget = "report-head"
	// ResetToLastBatchHead is the last batch head the run recorded.
	ResetToLastBatchHead ResetTarget = "last-batch-head"
	// ResetToBatchStart is the start commit the named batch recorded, taking --batch.
	ResetToBatchStart ResetTarget = "batch-start"
)

// ResetTargets lists every ResetTarget in the order the verb names them.
var ResetTargets = []ResetTarget{ResetToStart, ResetToPreFix, ResetToReportHead, ResetToLastBatchHead, ResetToBatchStart}

// ResetDeps is what PlanReset reads.
type ResetDeps struct {
	// Geom is the told geometry.
	Geom Geometry
	// State is the loaded state.json, nil when the run has none.
	State *State
	// Engine audits the run's sessions for the writes the dirt check exempts.
	Engine shuttleengine.Engine
	// Reed answers whether a recovery strand is live; nil leaves the live-strand refusal of report-head unchecked.
	Reed shuttleengine.ReedOps
	// ParentBranch names the branch the run merges its parent in from; nil or an error leaves the parent-branch refusal unchecked.
	ParentBranch ParentBranchFunc
	// Branch returns the branch checked out in the task worktree as fabric reads it, and errors on a detached HEAD or a branch that is not the pair's own.
	Branch func() (string, error)
	// Standalone is set when the run has no task pair: git's keep form performs the move and guards the uncommitted changes, so the foreign-dirty-path refusal is skipped.
	Standalone bool
}

// ResetPlan is what a reset may do.
type ResetPlan struct {
	// Target is the target the plan resolved.
	Target ResetTarget
	// SHA is the commit the reset moves the task branch to.
	SHA string
	// OwnPaths are the worktree-relative, slash-separated tracked paths the run wrote successfully, exempt from the dirt check.
	OwnPaths []string
	// ArchiveOnly marks a reset to start that moves no branch and only archives the run record, because Reason keeps the start from being moved to.
	// SHA and OwnPaths are empty then.
	ArchiveOnly bool
	// Reason says why the start cannot be moved to, set with ArchiveOnly.
	Reason string
}

// errStartUnresolvable marks a recorded start that cannot be resolved to one commit, which a reset to start answers by archiving without moving.
var errStartUnresolvable = errors.New("the recorded start cannot be resolved")

// archiveOnlyPlan is the plan of a reset to start that archives the run record and moves nothing, because reason keeps the start from being moved to.
func archiveOnlyPlan(reason string) ResetPlan {
	return ResetPlan{Target: ResetToStart, ArchiveOnly: true, Reason: reason}
}

// noStartToMoveTo returns why the recorded starts name no commit to move to, or "" when they do:
// no batch recorded a start, or a recorded start is missing from the repository.
func noStartToMoveTo(geom Geometry, bases evidenceBases) string {
	if len(bases.Starts) == 0 {
		return "no batch recorded a start commit"
	}
	var missing []string
	for _, start := range bases.Starts {
		if !geom.git().SHAExists(geom.WorktreeRoot, start) {
			missing = append(missing, start)
		}
	}
	if len(missing) > 0 {
		return fmt.Sprintf("recorded start commit(s) %s are not in this repository", strings.Join(missing, ", "))
	}
	return ""
}

// wayForwardSteps renders an ordered step list: `way forward: 1) <a>; 2) <b>` for two or more steps, `way forward: <a>` for one, and "" for none.
// It is the one renderer of an ordered step list, so each step is a distinct action and none restates another.
func wayForwardSteps(steps ...string) string {
	switch len(steps) {
	case 0:
		return ""
	case 1:
		return "way forward: " + steps[0]
	}
	numbered := make([]string, len(steps))
	for i, s := range steps {
		numbered[i] = fmt.Sprintf("%d) %s", i+1, s)
	}
	return "way forward: " + strings.Join(numbered, "; ")
}

// resetRefusal builds a refusal of the reset to target, ending in its way forward.
func resetRefusal(to ResetTarget, reason, wayForward string) error {
	return fmt.Errorf("webster: reset --to %s refused: %s; %s", to, reason, wayForward)
}

// PlanReset resolves to against the run's record and refuses, read-only, when a reset would be unsafe.
// batch is the --batch number, zero when none: the report-head and batch-start targets require it and every other target refuses it.
// The refusals run in this order: the batch pairing, run lock held (except for report-head, which Master runs inside its run), no state, merge in progress, a checked-out branch that is not the task branch,
// no recorded target, a recorded commit missing from the repository, a target that is not an ancestor of HEAD, and a dirty tracked path outside the run's own writes.
// A reset to start skips the recorded-target, missing-commit and ancestor refusals, which guard a branch move:
// when no start is recorded, a recorded start is missing, the starts share no oldest commit and no common ancestor, or the resolved start is not an ancestor of HEAD, it returns an archive-only plan with the reason.
// It plans only a run-recorded commit that is an ancestor of HEAD, exempts only paths the run's own transcripts record a successful write to, and never reads a force flag.
func PlanReset(deps ResetDeps, to ResetTarget, batch int) (ResetPlan, error) {
	geom := deps.Geom
	if err := refuseBatchPairing(to, batch); err != nil {
		return ResetPlan{}, err
	}
	rerun := fmt.Sprintf("re-run `%s`", resetVerb(to, batch))

	active, err := RunActive(geom.ScratchDir)
	if err != nil {
		return ResetPlan{}, err
	}
	if active && to != ResetToReportHead {
		return ResetPlan{}, fmt.Errorf("%w: %q (run.lock held); %s", ErrRunBusy, geom.ScratchDir,
			wayForwardSteps("wait for it to finish, or check `lyx webster status`"))
	}
	if deps.State == nil {
		return ResetPlan{}, resetRefusal(to, "the run has no state.json", wayForwardSteps("run `lyx webster run` first"))
	}
	if err := refuseMidMerge(geom.git(), geom.WorktreeRoot); err != nil {
		return ResetPlan{}, err
	}
	if err := refuseForeignBranch(deps, to, rerun); err != nil {
		return ResetPlan{}, err
	}

	bases, err := runEvidenceBases(geom, deps.State)
	if err != nil {
		return ResetPlan{}, err
	}
	switch to {
	case ResetToStart:
		// A start that cannot be moved to archives without moving, so it has no recorded-target refusal.
	case ResetToPreFix:
		if deps.State.PreFixHead == "" {
			return ResetPlan{}, resetRefusal(to, "the verify gate recorded no pre-fix head", wayForwardSteps("run `lyx webster run`"))
		}
	case ResetToLastBatchHead:
		if !slices.ContainsFunc(slices.Collect(maps.Values(deps.State.Batches)), recordedTerminalHead) {
			return ResetPlan{}, resetRefusal(to, "no batch recorded a head commit", wayForwardSteps(resetToStartStep))
		}
	case ResetToBatchStart:
		if err := refuseBatchStart(deps.State, batch); err != nil {
			return ResetPlan{}, err
		}
	case ResetToReportHead:
		if err := refuseReportHeadBatch(deps, batch); err != nil {
			return ResetPlan{}, err
		}
	default:
		names := make([]string, len(ResetTargets))
		for i, target := range ResetTargets {
			names[i] = string(target)
		}
		return ResetPlan{}, fmt.Errorf("webster: reset target %q is not one of %s", to, strings.Join(names, ", "))
	}

	if to == ResetToStart {
		if reason := noStartToMoveTo(geom, bases); reason != "" {
			return archiveOnlyPlan(reason), nil
		}
	} else {
		missing := bases.Missing
		if to == ResetToPreFix {
			missing = nil
			if !geom.git().SHAExists(geom.WorktreeRoot, deps.State.PreFixHead) {
				missing = []string{deps.State.PreFixHead}
			}
		}
		if len(missing) > 0 {
			return ResetPlan{}, fmt.Errorf("webster: reset --to %s refused: %s", to, missingCommitsClause(missing))
		}
	}

	sha, err := resolveResetSHA(geom, deps.State, bases, to, batch)
	if errors.Is(err, errStartUnresolvable) {
		return archiveOnlyPlan(err.Error()), nil
	}
	if err != nil {
		return ResetPlan{}, err
	}
	head, err := geom.git().HeadSHA(geom.WorktreeRoot)
	if err != nil {
		return ResetPlan{}, err
	}
	reachable, err := geom.git().IsAncestor(geom.WorktreeRoot, sha, head)
	if err != nil {
		return ResetPlan{}, err
	}
	if !reachable {
		reason := fmt.Sprintf("the recorded commit %s is not an ancestor of HEAD %s, so the branch was rewritten under the run", sha, head)
		if to == ResetToStart {
			return archiveOnlyPlan(reason), nil
		}
		return ResetPlan{}, resetRefusal(to, reason, wayForwardSteps(resetToStartStep))
	}

	own, err := ownTrackedPaths(deps)
	if err != nil {
		return ResetPlan{}, err
	}
	if deps.Standalone {
		return ResetPlan{Target: to, SHA: sha, OwnPaths: own}, nil
	}
	dirty, err := dirtyTrackedPaths(geom.WorktreeRoot)
	if err != nil {
		return ResetPlan{}, err
	}
	var foreign []string
	for _, p := range dirty {
		if !slices.Contains(own, p) {
			foreign = append(foreign, p)
		}
	}
	if len(foreign) > 0 {
		return ResetPlan{}, resetRefusal(to, "tracked changes outside the run's own writes would be lost: "+strings.Join(foreign, ", "),
			wayForwardSteps("commit them on the task branch, or restore each with `git checkout -- <path>`", rerun))
	}
	return ResetPlan{Target: to, SHA: sha, OwnPaths: own}, nil
}

// refuseForeignBranch refuses when the checked-out branch is detached, the parent branch, or one the caller reports is not the pair's own.
func refuseForeignBranch(deps ResetDeps, to ResetTarget, rerun string) error {
	steps := wayForwardSteps("check out the task worktree's own branch with `git switch <task-branch>`", rerun)
	if deps.Branch == nil {
		return resetRefusal(to, "the checked-out branch is unknown", steps)
	}
	branch, err := deps.Branch()
	if err != nil {
		return resetRefusal(to, fmt.Sprintf("the worktree is not on the task branch (%v)", err), steps)
	}
	if deps.ParentBranch != nil {
		parent, perr := deps.ParentBranch()
		if perr == nil && parent != "" && parent == branch {
			return resetRefusal(to, fmt.Sprintf("the worktree is on the parent branch %q, not the task branch", branch), steps)
		}
	}
	return nil
}

// resetToStartStep is the way-forward step that restarts the run, the fallback of every target that cannot move the branch.
const resetToStartStep = "run `" + stepResetToStart + "`"

// resetVerb spells the reset verb for to, with --batch when the target takes one.
func resetVerb(to ResetTarget, batch int) string {
	if batch > 0 {
		return fmt.Sprintf("lyx webster reset --to %s --batch %02d", to, batch)
	}
	return "lyx webster reset --to " + string(to)
}

// refuseBatchPairing refuses a --batch value that does not fit the target: report-head and batch-start need one, every other known target takes none.
// An unknown target passes, since PlanReset refuses it by name later.
func refuseBatchPairing(to ResetTarget, batch int) error {
	switch to {
	case ResetToReportHead, ResetToBatchStart:
		if batch <= 0 {
			return resetRefusal(to, "this target needs the batch to resolve against", wayForwardSteps(fmt.Sprintf("re-run `lyx webster reset --to %s --batch NN`", to)))
		}
	case ResetToStart, ResetToPreFix, ResetToLastBatchHead:
		if batch != 0 {
			return resetRefusal(to, fmt.Sprintf("--batch %d does not apply to this target", batch), wayForwardSteps(fmt.Sprintf("re-run `lyx webster reset --to %s` without --batch", to)))
		}
	}
	return nil
}

// recordedTerminalHead reports whether bs is a terminal batch that recorded a head commit.
func recordedTerminalHead(bs *BatchState) bool {
	return bs != nil && bs.Terminal && bs.Digest != nil && bs.Digest.HeadSHA != ""
}

// recordedBatchesAfter returns the numbers of the batches after batch in the recorded partition's order that satisfy keep, in that order.
// A state with no partition, or one that does not hold batch, orders by batch number.
func recordedBatchesAfter(st *State, batch int, keep func(*BatchState) bool) []int {
	var after []int
	index := slices.IndexFunc(st.Partition, func(pb PartitionBatch) bool { return cardNumberInt(pb.Cards[0]) == batch })
	if index >= 0 {
		for _, pb := range st.Partition[index+1:] {
			after = append(after, cardNumberInt(pb.Cards[0]))
		}
	} else {
		for number := range st.Batches {
			if number > batch {
				after = append(after, number)
			}
		}
		sort.Ints(after)
	}
	return slices.DeleteFunc(after, func(number int) bool {
		bs := st.Batches[number]
		return bs == nil || !keep(bs)
	})
}

// refuseBatchStart refuses a batch-start reset when batch recorded no start commit or a later batch recorded one, which the reset would discard.
func refuseBatchStart(st *State, batch int) error {
	to := ResetToBatchStart
	if bs := st.Batches[batch]; bs == nil || bs.StartSHA == "" {
		return resetRefusal(to, fmt.Sprintf("batch %d recorded no start commit", batch), wayForwardSteps(resetToStartStep))
	}
	if later := recordedBatchesAfter(st, batch, func(bs *BatchState) bool { return bs.StartSHA != "" }); len(later) > 0 {
		return resetRefusal(to, fmt.Sprintf("batch %d is not the run's last begun batch, so its start would discard the commits of batch %d", batch, later[0]), wayForwardSteps(resetToStartStep))
	}
	return nil
}

// refuseReportHeadBatch refuses a report-head reset unless batch is begun, not terminal, the last begun batch of the partition and without a live recovery strand.
func refuseReportHeadBatch(deps ResetDeps, batch int) error {
	to := ResetToReportHead
	st := deps.State
	bs := st.Batches[batch]
	if bs == nil {
		return resetRefusal(to, fmt.Sprintf("batch %d is not begun", batch), wayForwardSteps("check the batch number with `lyx webster status`"))
	}
	if bs.Terminal {
		return resetRefusal(to, fmt.Sprintf("batch %d already reached a terminal record (%s)", batch, bs.Status), wayForwardSteps("run `lyx webster reset --to last-batch-head`"))
	}
	if later := recordedBatchesAfter(st, batch, func(*BatchState) bool { return true }); len(later) > 0 {
		return resetRefusal(to, fmt.Sprintf("batch %d was begun after batch %d, so its report head is not the run's tip", later[0], batch), wayForwardSteps(resetToStartStep))
	}
	if deps.Reed != nil && bs.Kind == "recovery" && bs.StrandGUID != "" {
		live, err := StrandLive(deps.Reed, bs.StrandGUID)
		if err != nil {
			return err
		}
		if live {
			return resetRefusal(to, fmt.Sprintf("a recovery strand of batch %d is live", batch),
				wayForwardSteps(fmt.Sprintf("wait for its `lyx webster recover-batch %02d` to finish, then re-run `%s`", batch, resetVerb(to, batch))))
		}
	}
	return nil
}

// reportHeadSHA returns the head_sha of batch's report on disk, refusing when it is unreadable, missing from the repository or does not descend from the batch's start.
func reportHeadSHA(geom Geometry, st *State, batch int) (string, error) {
	to := ResetToReportHead
	bs := st.Batches[batch]
	report, err := ParseReport(filepath.Join(geom.ReportsDir, ReportFileName(batch, bs.Slug)))
	if err != nil {
		return "", resetRefusal(to, fmt.Sprintf("batch %d has no readable report (%v)", batch, err),
			wayForwardSteps(fmt.Sprintf("wait for the fork's report, or run `lyx webster recover-batch %02d`", batch)))
	}
	git := geom.git()
	if !git.SHAExists(geom.WorktreeRoot, report.HeadSHA) {
		return "", fmt.Errorf("webster: reset --to %s refused: %s", to, missingCommitsClause([]string{report.HeadSHA}))
	}
	descends := report.HeadSHA == bs.StartSHA
	if !descends {
		if descends, err = git.IsAncestor(geom.WorktreeRoot, bs.StartSHA, report.HeadSHA); err != nil {
			return "", err
		}
	}
	if !descends {
		return "", resetRefusal(to, fmt.Sprintf("the report head %s does not descend from batch %d's start %s", report.HeadSHA, batch, bs.StartSHA), wayForwardSteps(resetToStartStep))
	}
	return report.HeadSHA, nil
}

// resolveResetSHA returns the commit to reset to.
// The start target is the oldest recorded start; when starts are recorded but none is the oldest of all, it is their octopus merge-base.
// Each other target is the one commit its name says, read from the state or, for report-head, from the batch's report under the geometry's reports directory.
func resolveResetSHA(geom Geometry, st *State, bases evidenceBases, to ResetTarget, batch int) (string, error) {
	switch to {
	case ResetToPreFix:
		return st.PreFixHead, nil
	case ResetToBatchStart:
		return st.Batches[batch].StartSHA, nil
	case ResetToReportHead:
		return reportHeadSHA(geom, st, batch)
	case ResetToLastBatchHead:
		if bases.Last == "" {
			return "", resetRefusal(to, "the recorded batch heads share no single latest commit", wayForwardSteps(resetToStartStep))
		}
		return bases.Last, nil
	}
	if bases.Start != "" {
		return bases.Start, nil
	}
	base, err := octopusMergeBase(geom.WorktreeRoot, bases.Starts)
	if err != nil {
		return "", fmt.Errorf("%w: the recorded start commits %s share no single oldest commit and no common ancestor (%v)", errStartUnresolvable, strings.Join(bases.Starts, ", "), err)
	}
	return base, nil
}

// ownTrackedPaths returns the sorted, deduplicated worktree-relative tracked paths the run wrote successfully.
// A failed write is not evidence: the exemption is by path, not content.
func ownTrackedPaths(deps ResetDeps) ([]string, error) {
	if deps.Engine == nil {
		return nil, nil
	}
	writes, err := loadRunWrites(deps.Engine, deps.State, deps.Geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}
	var own []string
	for _, ev := range slices.Concat(writes.Master, writes.Forks) {
		if !ev.Succeeded {
			continue
		}
		rel, ok, err := trackedRel(deps.Geom.git(), deps.Geom.WorktreeRoot, ev.Path)
		if err != nil {
			return nil, err
		}
		if ok && !slices.Contains(own, rel) {
			own = append(own, rel)
		}
	}
	sort.Strings(own)
	return own, nil
}
