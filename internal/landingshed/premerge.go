// premerge.go implements the pre-merge step both producers run ahead of their clean-tree check:
// it clears the merge-in the row itself left parked, so the merge that follows starts afresh.

package landingshed

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// mergeInVerb is the verb a parked record carries when a merge-in of the parent branch stopped part-way.
const mergeInVerb = "merge-in"

// preMerge holds the three told seams of the pre-merge step.
// A nil mergeState skips the whole step, so a producer built by a struct literal runs no probe.
type preMerge struct {
	mergeState          func() (fabricengine.MidMergeState, error)
	abortMerge          func() error
	stopConflictSession func() (string, error)
}

// newPreMerge copies the pre-merge seams out of deps.
func newPreMerge(deps Deps) preMerge {
	return preMerge{
		mergeState:          deps.MergeState,
		abortMerge:          deps.AbortMerge,
		stopConflictSession: deps.StopConflictSession,
	}
}

// clear probes the pair's merge state and discards the row's own parked merge-in, if any.
//
// It returns a Stuck reason when the state cannot be cleared, an error when the probe or the strand table cannot be read, and ("", nil) when the pair is free for a fresh merge-in.
// A parked merge-in of parentBranch is stopped over: every live conflict session of the run is stopped first, then the merge is aborted.
// A failed stop leaves the merge untouched.
// Any other parked merge, and git merge state fabric did not start, is never stopped over or touched.
func (m preMerge) clear(producer, parentBranch string) (string, error) {
	if m.mergeState == nil {
		return "", nil
	}
	state, err := m.mergeState()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: probe merge state: %w", producer, err)
	}
	const mergeWayForward = `conclude it with "lyx fabric merge --continue" or discard it with "lyx fabric merge --abort", then resume`
	switch state.Kind {
	case fabricengine.MidMergeNone:
		return "", nil
	case fabricengine.MidMergeParked:
		if state.Verb != mergeInVerb || state.Source != parentBranch {
			return fmt.Sprintf("a fabric merge (%s of %s) is in progress in the task worktree; way forward: %s", state.Verb, state.Source, mergeWayForward), nil
		}
	default:
		return "a git merge, cherry-pick or squash that fabric did not start is in progress in the task worktree; way forward: conclude or abort it with git, then resume", nil
	}

	if m.stopConflictSession != nil {
		guid, err := m.stopConflictSession()
		if err != nil {
			if guid == "" {
				return "", fmt.Errorf("landingshed: %s: stop the conflict session: %w", producer, err)
			}
			return fmt.Sprintf("a conflict session of this run (strand %s) is still live and could not be stopped: %v; way forward: run \"lyx reed remove %s\", then \"lyx loom resume\"", guid, err, guid), nil
		}
	}
	if m.abortMerge != nil {
		if err := m.abortMerge(); err != nil {
			return fmt.Sprintf("the parked merge-in of parent branch %q could not be aborted: %v; way forward: %s", parentBranch, err, mergeWayForward), nil
		}
	}
	logger.Info("landingshed: aborted the parked merge-in before merging afresh", "producer", producer, "source", state.Source, "source_sha", state.SourceSHA, "start_sha", state.StartSHA)
	return "", nil
}
