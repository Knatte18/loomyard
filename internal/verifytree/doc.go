// Package verifytree is the one function every plan-verify site calls.
//
// Verify checks that the worktree is clean, skips a run when the verified-tree record already names HEAD's tree and the same command, and otherwise runs the command and records the pass.
// The record is written only by Verify and only after a pass, so a crash before that write leaves a mismatch and the next call runs again.
// While a command runs, Verify keeps a running marker that loom status reads through ReadMarker.
//
// Every site of one worktree shares one directory, Dir(anchorRoot), so a pass at one site lets the next site skip.
// The package imports no resolver: the worktree and the directory are told through Paths.
package verifytree
