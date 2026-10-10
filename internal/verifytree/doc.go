// Package verifytree is the one function every plan-verify site calls.
//
// Verify checks that the worktree is clean, skips a run when the verified-tree record holds an entry of the same command naming HEAD's tree, and otherwise runs the command and records the pass.
// The record holds one entry per command, each with the command, HEAD's tree, HEAD's commit and the time of its pass.
// A pass replaces its own command's entry and drops every other command's entry naming a different tree, except the entry of the base command the caller names on Site.
// LatestPass returns the entry of one command, which a caller uses as a diff base.
// A record in an older format, or a malformed one, reads as no record.
// The record is written only by Verify and only after a pass, so a crash before that write leaves a mismatch and the next call runs again.
// A command that outlives the timeout is killed and returns a failed result with TimedOut set and no record;
// a caller's own cancel is a returned error and no record.
// While a command runs, Verify keeps a running marker that loom status reads through ReadMarker.
//
// Given a gate-slot pool, Verify takes a slot after the dirty and skip checks and before it spawns.
// The marker is written in the waiting state with its wait start, then rewritten as running with a fresh start time once the slot is held, and the timeout counts only from that moment.
// The command runs with the lease's environment, so GOFLAGS carries the slot's `-p` cap and a nested gate run inherits the held slot, and the slot is released whatever the outcome.
// A cancelled wait is a returned error with no record.
// A nil pool runs unslotted with the parent's environment, the form of a standalone run and of a unit test.
// Either way the command's environment drops the strand-name variable (agentname.StrandNameEnv), so a test that drives `lyx fabric` never meets the sandboxed-role guard.
// Bound: a test is code the fork writes, which could clear the variable itself.
// A verify command never sees gateslot.PrebuiltLyxEnv, slotted or not: Verify strips an inherited value, so the command's own `go test` builds lyx once per test binary.
// A marker written before it had a state reads as running.
//
// Each run that spawns its command writes its own log, `verify-<n>.log` in the verify directory with n one above the highest existing number, and names it on Result.Log, so a later verify never overwrites an earlier run's evidence.
// After each such run the directory keeps the newest verifyLogKeep logs plus the one the Publish failure record names, and removes the rest.
//
// The Publish failure record is the one never-tracked file Publish leaves when its plan verify or `publish_verify` fails.
// It names the failing verify as a FailureKind, the failing tests, the failing run's own log, HEAD and the merge-in commit Publish made.
// Publish writes it through WritePublishFailure, the round gate reads it through ReadPublishFailure and checks its log path with IsLogPath, and a later passing Publish removes it through RemovePublishFailure, which leaves the log to pruning.
// The record sits in the verify directory beside the logs.
//
// Every site of one worktree shares one directory, Dir(anchorRoot), so a pass at one site lets the next site skip.
// The package imports no resolver: the worktree and the directory are told through Paths.
package verifytree
