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
// A marker written before it had a state reads as running.
//
// Every site of one worktree shares one directory, Dir(anchorRoot), so a pass at one site lets the next site skip.
// The package imports no resolver: the worktree and the directory are told through Paths.
package verifytree
