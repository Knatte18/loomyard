// dirs.go declares the names of lym's directories.
// They are the two worktree-level directories and the Bolt checkout;
// the subdirectories under a worktree's `.lym` and under a hub's `.lym` are declared here too.
// internal/lymdirs is the sole declarer of the worktree-level and Bolt literals;
// TestEnforcement_GeometryLiterals in internal/lymcwd/enforcement_test.go enforces it.

package lymdirs

// LymDirName is the durable, fabric-synced, git-tracked lym directory within a worktree.
const LymDirName = "_lym"

// DotLymDirName is the ephemeral, machine-bound directory in a worktree that git never tracks.
// Every never-tracked file lives under it, at the subpath that mirrors the `_lym` content it relates to.
const DotLymDirName = ".lym"

// BoltDirName is the name of the Bolt checkout inside a hub.
const BoltDirName = "_bolt"

// PauseDirName is the directory under the hub's board-level `.lym` that holds the pause record and its lock.
const PauseDirName = "pause"

// HibernateDirName is the directory under the hub's board-level `.lym` that holds the hibernate request and its lock.
// It also holds the whole text of a notice too long for one creel line.
const HibernateDirName = "hibernate"

// CreelDirName is the directory under the hub's board-level `.lym` that holds creel's journal and the daemon lock.
const CreelDirName = "creel"

// SlackDirName is the directory under the hub's board-level `.lym` that holds the Slack setup file and channel map.
// It also holds the inbox, outbound record, relay state, relay lock and relay log.
const SlackDirName = "slack"

// FailedPushDirName is the directory under the hub's board-level `.lym` that holds the failed-push records.
// It holds one record per repository side and branch.
const FailedPushDirName = "failed-push"

// RunProcDirName is the directory under the hub's board-level `.lym` that holds the owner record of a run's processes.
const RunProcDirName = "runproc"

// AdoptDirName is the directory under the hub's board-level `.lym` for the adopt worker.
// It holds the adopt record, its lock and the worker's output.
// It sits beside the adopt check's record, output and lock.
// It also holds the log of the test suite that an adopt starts.
const AdoptDirName = "adopt"

// FrontTaskDirName is the directory under the hub's board-level `.lym` that holds the front-task record and its lock.
// It also holds the output of the actions it runs.
const FrontTaskDirName = "fronttask"

// DriftJudgeDirName is the directory under the hub's board-level `.lym` that holds the task checkout snapshots.
// Each task's snapshot of its checkouts is taken before the drift judge runs.
const DriftJudgeDirName = "drift-judge"

// TestCoverDirName is the directory under the hub's board-level `.lym` that holds the per-package coverage maps.
// Every worktree of the hub on the machine shares them.
const TestCoverDirName = "testcover"

// TestMapExitFileName is the file under TestCoverDirName that a `lym gate testmap --upkeep` pass writes as it exits.
// It holds the call's process ID, the HEAD it planned at, the queued work it left and its plan and mismatch counts.
const TestMapExitFileName = "testmap-exit.json"

// BeamDirName is the directory under a worktree's `.lym` that holds dobby's lock, state record and markers.
const BeamDirName = "beam"

// CreelFlagDirName is the directory under the `creel` directory that holds one flag file per strand with unread mail.
const CreelFlagDirName = "flags"

// ParentReviewDirName is the directory under a worktree's `.lym` that holds a run's parent-review asks.
// It holds the ask record and the parent orch's report for each answered ask.
const ParentReviewDirName = "parent-review"

// HeddleDirName is the directory under a worktree's `.lym` that holds heddle's render lock.
const HeddleDirName = "heddle"

// UsageDirName is the directory under a worktree's `_lym` that holds a run's usage records.
// It holds one file per agent session, plus the run's identity and its landed diff size.
const UsageDirName = "usage"

// RaddleDirName is the directory under a worktree's `_lym` that holds a raddle run's files:
// the proposals, the approval, the run's state, the staged store and the part records.
// internal/lymdirs is the only place `raddle` is named under `_lym`.
const RaddleDirName = "raddle"

// LocksDirName is the directory under a checkout's `.lym` that holds the lock files for the checkout's tracked files.
const LocksDirName = "locks"
