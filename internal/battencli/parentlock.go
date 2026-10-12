// parentlock.go wires the seams of Run-Shed's parent lock backstop: the lock record of the child's run, its release, the child's agent states and the moved-parent read.

package battencli

import (
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
)

// childParentLock builds the parent lock seams of the task pair for slug, over the lock store under boardDir.
// bound is landing's idle bound for the lock.
// The task pair is located at call time, so a pair that is not there yet fails the seam and not the wiring.
func childParentLock(slug string, locateTask func() (*lyxcwd.Location, error), boardDir string, bound time.Duration) *battenshed.ChildParentLock {
	store := &parentlock.Store{Dir: parentlock.Dir(boardDir)}
	return &battenshed.ChildParentLock{
		Read: func() (parentlock.Record, bool, error) {
			taskLocation, err := locateTask()
			if err != nil {
				return parentlock.Record{}, false, err
			}
			return runLockRecord(store, slug, taskLocation.WorktreePath())
		},
		Release: func(record parentlock.Record) (bool, error) {
			return store.Release(record.Branch, record.Owner)
		},
		Agents: func() ([]parentlock.AgentState, error) {
			taskLocation, err := locateTask()
			if err != nil {
				return nil, err
			}
			return readAgentStates(taskLocation)
		},
		SelfPID:   os.Getpid(),
		IdleBound: bound,
		ParentMoved: func() (bool, error) {
			taskLocation, err := locateTask()
			if err != nil {
				return false, err
			}
			fab, err := fabricengine.Open(taskLocation)
			if err != nil {
				return false, err
			}
			return parentMovedSinceMergeIn(landingshed.MergeRecordPath(loomengine.LoomScratchDir(taskLocation)), fab.ReadParentTips, fab.HeadSHA, fab.HoldsWork)
		},
		GateRow: loomshed.NamePRGate,
	}
}

// lockIdleBound returns landing's idle bound for the parent lock of the hub of prime.
// A landing config that cannot be read logs a Warn and leaves the zero config, whose bound is the default.
func lockIdleBound(prime *lyxcwd.Location) time.Duration {
	cfg, err := landingshed.LoadConfig(fabricengine.ConfigDirs(prime), "landing")
	if err != nil {
		logger.Warn("battencli: landing config unreadable; the parent lock's idle bound falls back to its default", "error", err)
	}
	return cfg.IdleRelease()
}

// runLockRecord returns the lock record in store whose owner is the run for slug in the pair at pairPath, and false when none is.
// A record that cannot be read is skipped: it names no run.
func runLockRecord(store *parentlock.Store, slug, pairPath string) (parentlock.Record, bool, error) {
	entries, err := store.List()
	if err != nil {
		return parentlock.Record{}, false, err
	}
	for _, entry := range entries {
		if entry.Err == "" && entry.Record.Owner.Slug == slug && entry.Record.Owner.Pair == pairPath {
			return entry.Record, true, nil
		}
	}
	return parentlock.Record{}, false, nil
}

// readAgentStates returns the agent states of the live runs of the task pair.
func readAgentStates(taskLocation *lyxcwd.Location) ([]parentlock.AgentState, error) {
	anchor := taskLocation.AnchorPath()
	cfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(taskLocation), "shuttle")
	if err != nil {
		return nil, err
	}
	waits, err := shuttleengine.ReadAgentWaits(cfg, anchor, claudeengine.NewFromConfig(cfg), time.Now())
	if err != nil {
		return nil, err
	}
	states := make([]parentlock.AgentState, 0, len(waits))
	for _, wait := range waits {
		states = append(states, parentlock.AgentState{Name: wait.StrandName, Busy: wait.Busy, Waiting: wait.Waiting})
	}
	return states, nil
}

// parentMovedSinceMergeIn reports whether the parent branch the merge record at recordPath names has an upstream tip other than the one Merge-In merged that does not hold the task's work.
// tips fetches first when asked.
// An absent record, a parent with no upstream and an upstream still at the merged tip each read as not moved.
func parentMovedSinceMergeIn(
	recordPath string,
	tips func(branch string, fetch bool) (fabricengine.ParentTips, error),
	taskHead func() (string, error),
	holdsWork func(base, source string) (bool, error),
) (bool, error) {
	record, found, err := landingshed.ReadMergeRecord(recordPath)
	if err != nil || !found {
		return false, err
	}
	parent, err := tips(record.ParentBranch, true)
	if err != nil {
		return false, err
	}
	if parent.Relation == fabricengine.ParentNoUpstream || parent.Upstream == "" || parent.Upstream == record.MergedTip {
		return false, nil
	}
	head, err := taskHead()
	if err != nil {
		return false, err
	}
	holds, err := holdsWork(parent.Upstream, head)
	if err != nil {
		return false, err
	}
	return !holds, nil
}
