// doc.go carries the package godoc for editdirective:
// what the directive says, who renders it and why the package derives no path.

// Package editdirective renders the edit directive every spawned role's opening stencil carries,
// so every role is told the same way to change files.
//
// # What it says
//
// The directive states how a role changes files.
// It reads a file with Read, searches files with grep, and changes a file with Edit.
// All of one file's edits go in one message.
// It writes a whole file with Write, and it never edits by shell, which the shell guard refuses.
// It states that in lym sessions it overrides any harness text that permits changing files by shell.
// A Go file's formatting goes through `lym loom fmt`.
// Scratch files go under `.scratch/`, and those with a source extension are written with Write.
// `.scratch` is a link to a directory kept for the task alone,
// so a build there resolves a relative reference from the link's target;
// the directive has a project built there name the working tree's files by absolute path.
// It also keeps a role in its own worktree:
// no edit, commit, push or new worktree elsewhere unless its parent or the operator says so for that case.
// It names the tools a role edits with and the worktree it edits in, never what the role may write.
// It has no inactive case: the text renders in a repo without a `PATTERN.md` as in one with it.
//
// # One copy per engine
//
// Each engine states the directive in its own engine block, in its own tool wording,
// and each copy carries every rule above.
// No shared copy exists: a prompt carries the copy of the engine it launches on.
//
// # Who renders it
//
// Each spawning module's renderer calls Directive once per prompt, with the engine its launch resolves to.
// It passes the text as the value of the MarkerName marker its opening stencil carries.
// The marker is required, so a renderer that forgets it fails at composition.
// No prompt ships without the rule.
//
// # Imports
//
// The package imports only the standard library, `stencil` and `stencilstore`.
// It derives no path:
// the engine's copy is read at call time from the told stencils directory through `stencilstore.Read`.
// It is never read from embedded bytes.
package editdirective
