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
// What lives under it is a cache a fresh machine rebuilds or starts without; an absent cache reads as empty.
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

// BoardServeDirName is the directory under the hub's board-level `.lym`.
// It holds the board server's state, lock, launcher page and log.
// It holds the server's bearer token, so it is never tracked.
const BoardServeDirName = "board-serve"

// SlackDirName is the directory under the hub's board-level `.lym` that holds the Slack setup file and channel map.
// It also holds the inbox, outbound record, relay state, relay lock and relay log.
const SlackDirName = "slack"

// FailedPushDirName is the directory under the hub's board-level `.lym` that holds the failed-push records.
// It holds one record per repository side and branch.
const FailedPushDirName = "failed-push"

// RunProcDirName is the directory under the hub's board-level `.lym` that holds the owner record of a run's processes.
const RunProcDirName = "runproc"

// AdoptDirName is the directory under the hub's board-level `.lym` for the adopt.
// It holds each subject's adopt check files and fix scratch, and no record, lock or worker output.
// It also holds the log of the test suite that an adopt starts.
const AdoptDirName = "adopt"

// ClaudeAccountsDirName is the directory under the hub's board-level `.lym`.
// It holds the Claude account file and usage records.
const ClaudeAccountsDirName = "claude"

// ClaudeAccountsFileName is the name of the Claude account file under that directory.
// It maps each worktree to its account and holds the pick counters.
const ClaudeAccountsFileName = "accounts.json"

// ClaudeUsageDirName is the directory under the Claude accounts directory that holds one usage record per account.
const ClaudeUsageDirName = "usage"

// FrontTaskDirName is the directory under the hub's board-level `.lym` that holds the front-task record and its lock.
// It also holds the output of the actions it runs.
const FrontTaskDirName = "fronttask"

// DriftJudgeDirName is the directory under the hub's board-level `.lym` that holds the task checkout snapshots.
// Each task's snapshot of its checkouts is taken before the drift judge runs.
const DriftJudgeDirName = "drift-judge"

// HandOverDirName is the directory under the hub's board-level `.lym` that holds this machine's hand-over marks.
// It holds one mark per slug.
// A pair removal does not take it, since it sits under the hub's `.lym` and never in a pair.
const HandOverDirName = "hand-over"

// PusherDirName is the directory under the hub's board-level `.lym` that holds the pusher locks.
// It holds one lock per push target, which the target's one background pusher holds while it runs.
const PusherDirName = "pusher"

// PushLedgerDirName is the directory under the hub's board-level `.lym` that holds the ledger of owed pushes.
// It holds one entry per push target, the ledger lock and the time the hub last had nothing owed.
const PushLedgerDirName = "push-ledger"

// IgnoreCheckDirName is the directory under the hub's board-level `.lym` that holds scratch repositories.
// Ignore rules are checked in them, each in its own, removed after use.
const IgnoreCheckDirName = "ignore-check"

// OrphanDirName is the directory under the hub's board-level `.lym` that holds the dobby's record of orphan pairs.
// The record keeps when each orphan was first noticed, so a dobby restart neither resets nor skips its grace.
const OrphanDirName = "orphan"

// BeamDirName is the directory under a worktree's `.lym` that holds dobby's lock, state record and markers.
const BeamDirName = "beam"

// CreelFlagDirName is the directory under the `creel` directory that holds one flag file per strand with unread mail.
const CreelFlagDirName = "flags"

// ParentReviewDirName is the directory under a worktree's `.lym` that holds a run's parent-review asks.
// It holds the ask record and the parent orch's report for each answered ask.
const ParentReviewDirName = "parent-review"

// ImplementerDirName is the directory under a worktree's `.lym` that holds what both implementers share.
// It holds the plan-conflict record and the last-routed conflict.
const ImplementerDirName = "implementer"

// HeddleDirName is the directory under a worktree's `.lym` that holds heddle's render lock.
const HeddleDirName = "heddle"

// ReviewSnapshotDirName is the directory under a worktree's `.lym` that holds the review target snapshots.
// It holds one copy of the overlay target files per snapshot id.
// It is a never-tracked cache, and an absent one reads as empty.
const ReviewSnapshotDirName = "review-snapshot"

// UsageDirName is the directory under a worktree's `_lym` that holds a run's usage records.
// It holds one file per agent session, plus the run's identity and its landed diff size.
const UsageDirName = "usage"

// RaddleDirName is the directory under a worktree's `_lym` that holds a raddle run's files:
// the proposals, the approval, the run's state, the staged store and the part records.
// internal/lymdirs is the only place `raddle` is named under `_lym`.
const RaddleDirName = "raddle"

// WeaverDirName is the directory under a worktree's `_lym` and `.lym` that holds Weaver's run:
// the durable state, reports, journal and outcome under `_lym`, and the prompts and locks under `.lym`.
const WeaverDirName = "weaver"

// LocksDirName is the directory under a checkout's `.lym` that holds the lock files for the checkout's tracked files.
const LocksDirName = "locks"

// HardenSandboxDirName is the directory under a worktree's `.lym` that holds a run's throwaway sandbox hubs.
// It is a never-tracked cache that goes with the worktree, and an absent one reads as empty.
const HardenSandboxDirName = "harden-sandbox"

// HardenDirName is the directory under a worktree's `_lym` that holds a Hardener campaign's archived rounds.
// It also holds the campaign's final bouncer ledger.
const HardenDirName = "harden"
