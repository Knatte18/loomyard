// readoptstop.go holds the failed stop of a resume whose gone pair can never be re-adopted.
// It defines the closed set of permanent answers and the way forward each row and answer calls for.
// It also writes the status and posts the notice that record the stop.

package battencli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// failedStopCommitPrefix heads the commit message of the status commit a failed stop makes.
const failedStopCommitPrefix = "batten"

// readoptPermanent reports whether err is an answer to re-making the pair that no re-run can change:
// the pair is not live; a remote leftover stands;
// or a records worktree directory, a checked-out branch or a task path blocks the re-adoption.
// Every other answer is transient.
func readoptPermanent(err error) bool {
	var leftover *fabricengine.ErrRemoteLeftover
	var notPair *fabricengine.ErrRecordsWorktreeNotPair
	var checkedOut *fabricengine.ErrAdoptLocalCheckedOut
	var taken *fabricengine.ErrTaskPathTaken
	return errors.Is(err, fabricengine.ErrPairNotLive) ||
		errors.As(err, &leftover) ||
		errors.As(err, &notPair) ||
		errors.As(err, &checkedOut) ||
		errors.As(err, &taken)
}

// childStatusRels returns the tree-relative paths of the child run's status file under each run directory segment.
// A run may carry the slug's own segment or the legacy self segment.
func childStatusRels(slug string) ([]string, error) {
	var rels []string
	for _, runID := range []string{slug, shedrun.SelfRunID} {
		_, statusRel, err := shedrun.TreeRunFileRels(runID)
		if err != nil {
			return nil, err
		}
		rels = append(rels, statusRel)
	}
	return rels, nil
}

// readoptRecordRels returns the paths a tag read must carry:
// the child run's status under each run directory segment, then the origin record.
func readoptRecordRels(slug string) ([]string, error) {
	rels, err := childStatusRels(slug)
	if err != nil {
		return nil, err
	}
	return append(rels, fabricengine.OriginRecordRel()), nil
}

// failReadopt records the permanent failure addErr as the stop of a run whose status is status.
// A running status is written failed with the failure and the way forward of the row, committed and pushed,
// and the run's parent is told once.
// A status in any other state, a failed one included, keeps its state: no write and no notice.
// A way forward that cannot be worked out refuses the pass as transient, leaving the status untouched.
// A status write that fails and a run lock held elsewhere do the same.
// It returns the error, so the pass exits non-zero.
func (c *battenCLI) failReadopt(status shedengine.Status, addErr error) error {
	rerun := "lym batten run " + c.slug
	wayForward, err := readoptWayForward(c, status.CurrentProducer, addErr)
	if err != nil {
		return fmt.Errorf("battencli: re-making the pair of %q failed (%w) and its way forward could not be worked out: %v; fix the cause it names, then re-run %s", c.slug, addErr, err, rerun)
	}
	failure := fmt.Sprintf("battencli: re-making the pair of %q failed: %s", c.slug, readoptCause(addErr))
	stop := errors.New(failure + "; way forward: " + wayForward)
	if status.State != shedengine.StateRunning {
		return stop
	}

	result, err := shedengine.WriteFailedStop(shedengine.FailedStopRequest{
		StatusPath:     c.shedPaths.StatusPath,
		LockPath:       c.shedPaths.LockPath,
		StatusLockPath: c.shedPaths.StatusLockPath,
		Error:          failure,
		WayForward:     wayForward,
	})
	if err != nil {
		return fmt.Errorf("%s; the stop could not be recorded in the status: %w; re-run %s", failure, err, rerun)
	}
	if result.Busy {
		return fmt.Errorf("%s; another batten run holds the run lock, so the stop was not recorded; re-run %s", failure, rerun)
	}
	if !result.Written {
		return stop
	}
	if c.readopt.CommitFailed != nil {
		if err := c.readopt.CommitFailed(c.slug, status.CurrentProducer); err != nil {
			logger.Warn("battencli: the failed stop was written but not committed or pushed", "slug", c.slug, "cause", err)
		}
	}
	if c.readopt.Notice != nil {
		line := fmt.Sprintf("batten: the run %q cannot re-adopt its pair and stopped failed; way forward: %s", c.slug, wayForward)
		if err := c.readopt.Notice(c.slug, line); err != nil {
			logger.Warn("battencli: the failed stop notice was not posted", "slug", c.slug, "cause", err)
		}
	}
	return stop
}

// readoptCause words what addErr reports for the stop's error text.
// A checked-out refusal is worded without its own way forward.
// That way forward names an `--adopt-local` re-run that a batten run never makes.
// The stop carries the way forward of the row instead.
func readoptCause(addErr error) string {
	var checkedOut *fabricengine.ErrAdoptLocalCheckedOut
	if !errors.As(addErr, &checkedOut) {
		return addErr.Error()
	}
	if checkedOut.Records {
		return fmt.Sprintf("records branch %q is checked out in the records worktree %s", checkedOut.Branch, checkedOut.Path)
	}
	return fmt.Sprintf("branch %q is checked out in %s", checkedOut.Branch, checkedOut.Path)
}

// readoptWayForward words the way forward of the permanent answer addErr for a run whose status reads producer.
// Before Land the answers that leave nothing to restore settle through a cancel that removes the pair;
// at Land the landed work is restored first, since a cancel would release a landed entry.
// A failed tag or file read is returned, and the pass refuses as transient.
func readoptWayForward(c *battenCLI, producer string, addErr error) (string, error) {
	atLand := producer == battenrecipe.NameLand
	rerun := "lym batten run " + c.slug
	cancel := fmt.Sprintf("run `lym batten cancel %s --remove` from the prime, which removes the pair, archives and deletes the task branch, and clears the run's record and claim", c.slug)

	var notLive *fabricengine.ErrNotLive
	var leftover *fabricengine.ErrRemoteLeftover
	var notPair *fabricengine.ErrRecordsWorktreeNotPair
	var checkedOut *fabricengine.ErrAdoptLocalCheckedOut
	var taken *fabricengine.ErrTaskPathTaken
	switch {
	case errors.As(addErr, &notLive):
		if atLand {
			return landRestoreWayForward(c, notLive)
		}
		return cancel, nil
	case errors.As(addErr, &leftover):
		if atLand {
			return leftoverArchiveWayForward(c, leftover)
		}
		return cancel, nil
	case errors.As(addErr, &notPair):
		recordsRoot, err := c.readopt.RecordsRoot()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("the directory %s stands where the pair's records worktree goes without being one; save anything worth keeping from it, then run `git -C %s worktree remove --force %s` when `git -C %s worktree list` shows it registered, else delete the directory; then run `%s`", notPair.Path, recordsRoot, notPair.Path, recordsRoot, rerun), nil
	case errors.As(addErr, &checkedOut):
		if checkedOut.Records {
			return fmt.Sprintf("the records branch %s is checked out in the records worktree %s; switch that worktree off it with `git -C %s switch --detach`; then run `%s`", checkedOut.Branch, checkedOut.Path, checkedOut.Path, rerun), nil
		}
		return fmt.Sprintf("the task branch %s is checked out in %s; run `lym fabric checkout <other branch>` in %s to move it off the branch; then run `%s`", checkedOut.Branch, checkedOut.Path, checkedOut.Path, rerun), nil
	case errors.As(addErr, &taken):
		return fmt.Sprintf("the path %s stands where the task worktree goes; save anything worth keeping from it, then delete it; then run `%s`", taken.Path, rerun), nil
	}
	return "", fmt.Errorf("no way forward is worded for %w", addErr)
}

// leftoverArchiveWayForward words the way forward of a records branch left on origin, for a run at Land:
// archive the leftover tip under a tag, delete the branch on origin, then re-run.
func leftoverArchiveWayForward(c *battenCLI, leftover *fabricengine.ErrRemoteLeftover) (string, error) {
	recordsRoot, err := c.readopt.RecordsRoot()
	if err != nil {
		return "", err
	}
	tip := leftover.RemoteTip
	if len(tip) < fabricengine.ArchiveTagSHALen {
		return "", fmt.Errorf("the leftover tip %q of %s is too short to name an archive tag", tip, leftover.Branch)
	}
	tag := fabricengine.ArchiveTagName(c.slug, tip)
	tags, err := c.readopt.Tags(c.slug)
	if err != nil {
		return "", err
	}
	tagStands := false
	for _, existing := range tags {
		tagStands = tagStands || (existing.Name == tag && existing.Commit == tip)
	}
	steps := []string{fmt.Sprintf("`git -C %s fetch origin %s`", recordsRoot, tip)}
	if !tagStands {
		steps = append(steps, fmt.Sprintf("`git -C %s tag %s %s`", recordsRoot, tag, tip))
	}
	steps = append(steps,
		fmt.Sprintf("`git -C %s push origin %s`", recordsRoot, tag),
		fmt.Sprintf("`git -C %s push --force-with-lease=refs/heads/%s:%s origin --delete %s`", recordsRoot, leftover.Branch, tip, leftover.Branch),
	)
	upstream, err := c.readopt.Upstream(leftover.Branch)
	if err != nil {
		return "", err
	}
	if upstream {
		steps = append(steps, fmt.Sprintf("`git -C %s branch --unset-upstream %s`", recordsRoot, leftover.Branch))
	}
	steps = append(steps, fmt.Sprintf("`lym batten run %s`", c.slug))
	return "archive the leftover records branch on origin and delete it there: " + strings.Join(steps, ", then "), nil
}

// landRestoreWayForward returns the instruction that restores a landed run whose branch pair is not live.
// The records branch is restored from the newest finished archive tag, and the task branch at its parent branch.
// Each branch is restored only when it is the one that is gone.
// The instruction also names running the resume right after the restore.
// A cleanup in between would archive and delete the restored records branch, which has no worktree.
// When no tag holds a finished run, the instruction says no copy of the records is left,
// and the landing must be finished by hand.
func landRestoreWayForward(c *battenCLI, notLive *fabricengine.ErrNotLive) (string, error) {
	recordsRoot, err := c.readopt.RecordsRoot()
	if err != nil {
		return "", err
	}
	taskBranch, err := c.readopt.TaskBranch(c.slug)
	if err != nil {
		return "", err
	}
	recordsBranch := fabricengine.RecordsBranchName(taskBranch)

	var steps []string
	var parentRecord []byte
	if notLive.Records {
		tags, err := c.readopt.Tags(c.slug)
		if err != nil {
			return "", err
		}
		best, err := newestFinishedTag(c.slug, tags)
		if err != nil {
			return "", err
		}
		if best == nil {
			return "no copy of the run's records is left on origin or in the hub, so the pair cannot be restored; finish the landing by hand, the case of the board note `landed-run-left-paused`", nil
		}
		if best.OriginOnly {
			steps = append(steps, fmt.Sprintf("`git -C %s fetch origin tag %s`", recordsRoot, best.Name))
		}
		steps = append(steps, fmt.Sprintf("`git -C %s branch %s %s`", recordsRoot, recordsBranch, best.Name))
		parentRecord = best.Files[fabricengine.OriginRecordRel()]
	}

	gone, err := c.readopt.TaskBranchGone(c.slug)
	if err != nil {
		return "", err
	}
	if gone {
		if !notLive.Records {
			content, found, err := c.readopt.BranchFile(recordsBranch, fabricengine.OriginRecordRel())
			if err != nil {
				return "", err
			}
			if !found {
				return "", fmt.Errorf("the origin record of the records branch %s, which names the parent of the task branch, is absent", recordsBranch)
			}
			parentRecord = content
		}
		var origin fabricengine.Origin
		if err := json.Unmarshal(parentRecord, &origin); err != nil || origin.ParentBranch == "" {
			return "", fmt.Errorf("the origin record of %s names no parent branch for the task branch (%v)", c.slug, err)
		}
		steps = append(steps, fmt.Sprintf("`git -C %s branch %s %s`", c.location.WorktreePath(), taskBranch, origin.ParentBranch))
	}

	if len(steps) == 0 {
		return fmt.Sprintf("the branches of the pair are live again; run `lym batten run %s`", c.slug), nil
	}
	return fmt.Sprintf("restore what the landing needs: %s; then run `lym batten run %s` right away, since a `lym fabric cleanup --apply` in between would archive and delete a restored records branch, which has no worktree, and the restore would have to be run again from the tag", strings.Join(steps, ", then "), c.slug), nil
}

// newestFinishedTag returns the tag with the latest committer time whose child run status reads done.
// It returns nil when no tag does, and the status is read under either run directory segment.
// A tag whose status is absent or does not decode is skipped.
func newestFinishedTag(slug string, tags []fabricengine.RecordsTag) (*fabricengine.RecordsTag, error) {
	rels, err := childStatusRels(slug)
	if err != nil {
		return nil, err
	}
	var best *fabricengine.RecordsTag
	for i := range tags {
		if !childRunDone(tags[i], rels) {
			continue
		}
		if best == nil || tags[i].CommitterTime.After(best.CommitterTime) {
			best = &tags[i]
		}
	}
	return best, nil
}

// childRunDone reports whether the child run status held by tag under any of rels decodes with state done.
func childRunDone(tag fabricengine.RecordsTag, rels []string) bool {
	for _, rel := range rels {
		data, present := tag.Files[rel]
		if !present {
			continue
		}
		var status shedengine.Status
		if json.Unmarshal(data, &status) == nil && status.State == shedengine.StateDone {
			return true
		}
	}
	return false
}
