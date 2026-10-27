// dirs.go declares the lym directory-name tokens: the two worktree-level directories, the beam directory under a worktree's .lym and the pause directory under the board-level .lym.
// internal/lymdirs is the sole declarer of either worktree-level literal, enforced by TestEnforcement_GeometryLiterals in internal/lymcwd/enforcement_test.go.

package lymdirs

// LymDirName is the durable, fabric-synced, git-tracked lym directory within a worktree.
// internal/lymdirs is the sole declarer of this literal, enforced by TestEnforcement_GeometryLiterals in internal/lymcwd/enforcement_test.go.
const LymDirName = "_lym"

// DotLymDirName is the ephemeral, machine-bound, never-git-tracked sibling of LymDirName.
// Every never-tracked file lives under it at the mirrored subpath of the _lym content it relates to.
// internal/lymdirs is the sole declarer of this literal, enforced by TestEnforcement_GeometryLiterals in internal/lymcwd/enforcement_test.go.
const DotLymDirName = ".lym"

// PauseDirName is the directory under the hub's board-level DotLymDirName that holds the pause record and its lock.
const PauseDirName = "pause"

// HibernateDirName is the directory under the hub's board-level DotLymDirName that holds the hibernate request, its lock and the daemon's log.
const HibernateDirName = "hibernate"

// CreelDirName is the directory under the hub's board-level DotLymDirName that holds creel's journal and the daemon lock.
const CreelDirName = "creel"

// FailedPushDirName is the directory under the hub's board-level DotLymDirName that holds one failed-push record per repository side and branch.
const FailedPushDirName = "failed-push"

// RunProcDirName is the directory under the hub's board-level DotLymDirName that holds the owner record of the processes a run starts.
const RunProcDirName = "runproc"

// AdoptDirName is the directory under the hub's board-level DotLymDirName that holds the log of the test suite an adopt starts.
const AdoptDirName = "adopt"

// BeamDirName is the directory under a worktree's DotLymDirName that holds dobby's lock, state record and markers.
const BeamDirName = "beam"
