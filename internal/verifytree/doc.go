// Package verifytree is the one function every plan-verify site calls.
//
// Verify checks that the worktree is clean, skips a run when the verified-tree record holds an entry of the same command naming HEAD's tree, and otherwise runs the command and records the pass.
// The record holds one entry per command, each with the command, HEAD's tree, HEAD's commit and the time of its pass.
// A pass replaces its own command's entry and drops every other command's entry naming a different tree, except the entry of the base command the caller names on Site.
// LatestPass returns the entry of one command, which a caller uses as a diff base.
// A record in an older format, or a malformed one, reads as no record.
// The record is written only by Verify and only after a pass, so a crash before that write leaves a mismatch and the next call runs again.
// While a command runs, Verify keeps a running marker that loom status reads through ReadMarker.
//
// Every site of one worktree shares one directory, Dir(anchorRoot), so a pass at one site lets the next site skip.
// The package imports no resolver: the worktree and the directory are told through Paths.
package verifytree
