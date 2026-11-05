// dirs.go declares the lym directory-name tokens: the two worktree-level directories, the Bolt checkout directory, the beam directory under a worktree's .lym, the pause directory under the board-level .lym and the Slack directory under it.
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

// SlackDirName is the directory under the hub's board-level DotLymDirName that holds the hub's Slack setup file, presence, cursor, outbound record, relay state, relay lock and relay log.
const SlackDirName = "slack"

// FailedPushDirName is the directory under the hub's board-level DotLymDirName that holds one failed-push record per repository side and branch.
const FailedPushDirName = "failed-push"

// RunProcDirName is the directory under the hub's board-level DotLymDirName that holds the owner record of the processes a run starts.
const RunProcDirName = "runproc"

// AdoptDirName is the directory under the hub's board-level DotLymDirName that holds the adopt check's record, output and lock beside the log of the test suite an adopt starts.
const AdoptDirName = "adopt"

// FrontTaskDirName is the directory under the hub's board-level DotLymDirName that holds the front-task record, its lock and the output of the actions it runs.
const FrontTaskDirName = "fronttask"

// DriftJudgeDirName is the directory under the hub's board-level DotLymDirName that holds one snapshot of a task's checkouts per task, taken before the drift judge runs.
const DriftJudgeDirName = "drift-judge"

// BeamDirName is the directory under a worktree's DotLymDirName that holds dobby's lock, state record and markers.
const BeamDirName = "beam"

// CreelFlagDirName is the directory under CreelDirName that holds one flag file per strand holding unread mail.
const CreelFlagDirName = "flags"

// HeddleDirName is the directory under a worktree's DotLymDirName that holds heddle's render lock.
const HeddleDirName = "heddle"

// UsageDirName is the directory under a worktree's LymDirName that holds a run's usage records: one file per agent session, the run's identity and its landed diff size.
const UsageDirName = "usage"

// RaddleDirName is the directory under a worktree's LymDirName that holds a raddle run's files: the proposals, the approval, the run's state, the staged store and the part records.
// internal/lymdirs is the only place `raddle` is named under LymDirName.
const RaddleDirName = "raddle"

// LocksDirName is the directory under a checkout's DotLymDirName that holds the lock files guarding the checkout's tracked files.
const LocksDirName = "locks"
