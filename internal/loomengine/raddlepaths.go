// raddlepaths.go declares the paths of a raddle run's own files under the worktree's anchor.
// They are the proposals, the approval record, the run's state and its lock, the staged store and the part records.

package loomengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// The names of the raddle run's files within RaddleDir.
// The constants are unexported, so only package loomengine uses them.
const (
	raddleProposalFileName         = "proposal.yaml"
	raddleColdProposalFileName     = "cold-proposal.yaml"
	raddleReturnedProposalFileName = "returned-proposal.yaml"
	raddleApprovalFileName         = "approval.json"
	raddleStateFileName            = "state.json"
	raddleStateLockSuffix          = ".lock"
	raddleStageDirName             = "stage"
	raddleRecordsDirName           = "records"
	raddlePartRecordPrefix         = "decision-"
	raddlePartRecordSuffix         = ".md"
)

// RaddleDirRel returns the worktree-anchor-relative path of a raddle run's directory.
// It joins lymdirs.LymDirName and lymdirs.RaddleDirName.
// It keeps callers that build a fabric commit pathspec from naming a directory segment lymdirs owns.
func RaddleDirRel() string {
	return filepath.Join(lymdirs.LymDirName, lymdirs.RaddleDirName)
}

// RaddleDir returns the path of a raddle run's directory in the worktree at l.
// The path is rooted at `l.AnchorPath()`.
func RaddleDir(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), RaddleDirRel())
}

// RaddleProposal returns the path of the proposal the raddle agent writes, in the worktree at l.
func RaddleProposal(l *lymcwd.Location) string {
	return filepath.Join(RaddleDir(l), raddleProposalFileName)
}

// RaddleColdProposal returns the path of the cold proposal, the first proposal of the run, stored once, in the worktree at l.
func RaddleColdProposal(l *lymcwd.Location) string {
	return filepath.Join(RaddleDir(l), raddleColdProposalFileName)
}

// RaddleReturnedProposal returns the path of the proposal a return set aside for the next session, in the worktree at l.
func RaddleReturnedProposal(l *lymcwd.Location) string {
	return filepath.Join(RaddleDir(l), raddleReturnedProposalFileName)
}

// RaddleApproval returns the path of the approval record the approval verb writes, in the worktree at l.
func RaddleApproval(l *lymcwd.Location) string {
	return filepath.Join(RaddleDir(l), raddleApprovalFileName)
}

// RaddleStatePath returns the path of the raddle run's state record, in the worktree at l.
func RaddleStatePath(l *lymcwd.Location) string {
	return filepath.Join(RaddleDir(l), raddleStateFileName)
}

// RaddleStateLockPath returns the path of the lock that guards the run's state record.
// The lock sits in the never-tracked lock directory, mirroring the state file's path.
func RaddleStateLockPath(l *lymcwd.Location) string {
	return lock.TrackedFileLockPath(RaddleStatePath(l), raddleStateLockSuffix)
}

// RaddleStageDir returns the path of the directory holding the staged raddle store, in the worktree at l.
func RaddleStageDir(l *lymcwd.Location) string {
	return filepath.Join(RaddleDir(l), raddleStageDirName)
}

// RaddlePartRecord returns the path of the decision record of the part slug, in the worktree at l.
func RaddlePartRecord(l *lymcwd.Location, slug string) string {
	return filepath.Join(RaddleDir(l), raddleRecordsDirName, raddlePartRecordPrefix+slug+raddlePartRecordSuffix)
}
