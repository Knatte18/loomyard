// pairparts.go holds the read-only reads of what is left of a pair: its local parts, and whether its task branch is on origin.

package fabricengine

import (
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// PairParts reports which parts of a pair are present, read locally with no network call.
type PairParts struct {
	// TaskWorktree reports a registered linked worktree at the pair's location.
	TaskWorktree bool
	// RecordsWorktree reports a directory at the pair's records worktree path.
	RecordsWorktree bool
	// RecordsBranch reports the local sibling branch.
	RecordsBranch bool
	// TaskBranch reports the local task branch.
	TaskBranch bool
	// Launchers reports the pair's launcher directory.
	Launchers bool
}

// Any reports whether any part of the pair is present.
func (p PairParts) Any() bool {
	return p.TaskWorktree || p.RecordsWorktree || p.RecordsBranch || p.TaskBranch || p.Launchers
}

// PairParts reads which parts of slug's pair are present locally, by the same reading Remove makes of what is left.
func (t *Topology) PairParts(l *lymcwd.Location, slug string) (PairParts, error) {
	state, err := t.inspectPair(l, slug)
	if err != nil {
		return PairParts{}, err
	}
	return PairParts{
		TaskWorktree:    state.taskWorktree,
		RecordsWorktree: state.siblingWorktree,
		RecordsBranch:   state.siblingBranch,
		TaskBranch:      state.taskBranch,
		Launchers:       state.launchers,
	}, nil
}

// TaskBranchOnOrigin reports whether slug's task branch exists on the task repository's origin.
// A repository with no origin remote reads false, and a failed lookup on a configured origin is returned.
func (t *Topology) TaskBranchOnOrigin(l *lymcwd.Location, slug string) (bool, error) {
	if _, err := gitrepo.New(l.WorktreePath()).RemoteURL(originRemoteName); err != nil {
		return false, nil
	}
	tip, err := remoteHeadTip(l.WorktreePath(), t.cfg.BranchPrefix+slug)
	if err != nil {
		return false, err
	}
	return tip != "", nil
}
