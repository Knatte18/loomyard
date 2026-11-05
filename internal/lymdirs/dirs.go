// dirs.go declares the lym directory-name tokens: the two worktree-level directories, the Bolt checkout directory, the beam directory under a worktree's .lym and the pause directory under the board-level .lym.
// internal/lymdirs is the sole declarer of each worktree-level literal and of the Bolt checkout's, enforced by TestEnforcement_GeometryLiterals in internal/lymcwd/enforcement_test.go.

package lymdirs

// LymDirName is the durable, fabric-synced, git-tracked lym directory within a worktree.
// internal/lymdirs is the sole declarer of this literal, enforced by TestEnforcement_GeometryLiterals in internal/lymcwd/enforcement_test.go.
const LymDirName = "_lym"

// DotLymDirName is the ephemeral, machine-bound, never-git-tracked sibling of LymDirName.
// Every never-tracked file lives under it at the mirrored subpath of the _lym content it relates to.
// internal/lymdirs is the sole declarer of this literal, enforced by TestEnforcement_GeometryLiterals in internal/lymcwd/enforcement_test.go.
const DotLymDirName = ".lym"

// BoltDirName is the name of the Bolt checkout inside a hub.
// internal/lymdirs is the sole declarer of this literal, enforced by TestEnforcement_GeometryLiterals in internal/lymcwd/enforcement_test.go.
const BoltDirName = "_bolt"

// PauseDirName is the directory under the hub's board-level DotLymDirName that holds the pause record and its lock.
const PauseDirName = "pause"

// HibernateDirName is the directory under the hub's board-level DotLymDirName that holds the hibernate request, its lock and the whole text of a notice too long for one creel line.
const HibernateDirName = "hibernate"

// CreelDirName is the directory under the hub's board-level DotLymDirName that holds creel's journal and the daemon lock.
const CreelDirName = "creel"

// FailedPushDirName is the directory under the hub's board-level DotLymDirName that holds one failed-push record per repository side and branch.
const FailedPushDirName = "failed-push"

// RunProcDirName is the directory under the hub's board-level DotLymDirName that holds the owner record of the processes a run starts.
const RunProcDirName = "runproc"

// AdoptDirName is the directory under the hub's board-level DotLymDirName that holds the adopt check's record, output and lock beside the log of the test suite an adopt starts.
const AdoptDirName = "adopt"

// FrontTaskDirName is the directory under the hub's board-level DotLymDirName that holds the front-task record, its lock and the output of the actions it runs.
const FrontTaskDirName = "fronttask"

// BeamDirName is the directory under a worktree's DotLymDirName that holds dobby's lock, state record and markers.
const BeamDirName = "beam"

// CreelFlagDirName is the directory under CreelDirName that holds one flag file per strand holding unread mail.
const CreelFlagDirName = "flags"

// HeddleDirName is the directory under a worktree's DotLymDirName that holds heddle's render lock.
const HeddleDirName = "heddle"

// LocksDirName is the directory under a checkout's DotLymDirName that holds the lock files guarding the checkout's tracked files.
const LocksDirName = "locks"
