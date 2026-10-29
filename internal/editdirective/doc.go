// doc.go carries the package godoc for editdirective: what the directive says, who renders it and why the package derives no path.

// Package editdirective renders the edit directive every spawned role's opening stencil carries, so every role is told the same way to change files.
//
// # What it says
//
// The directive states how a role changes files: with Edit or Write, or by script with `sed -i`, `perl -i`, `awk` or a shell heredoc or redirect wherever a script is simpler.
// A Go file's formatting goes through `lym loom fmt`, and scratch files go under `.scratch/`.
// It also keeps a role in its own worktree: no edit, commit, push or new worktree elsewhere unless its parent or the operator says so for that case.
// It names the tools a role edits with and the worktree it edits in, never what the role may write.
// It has no inactive case: the text renders in a repo without a `PATTERN.md` as in one with it.
//
// # Who renders it
//
// Each spawning module's renderer calls Directive once per prompt and passes the text as the value of the MarkerName marker its opening stencil carries.
// The marker is required, so a renderer that forgets it fails at composition rather than shipping a prompt without the rule.
//
// # Imports
//
// The package imports only the standard library, `stencil` and `stencilstore`.
// It derives no path: the stencil is read at call time from the told stencils directory through `stencilstore.Read`, never from embedded bytes.
package editdirective
