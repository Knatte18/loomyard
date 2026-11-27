<!-- This is the Codex engine's copy of the edit directive.
     internal/editdirective.Directive renders it into the edit_directive marker of every spawned role's opening stencil
     whose prompt launches on Codex.
     It declares no marker and must stay marker-free. -->

## Editing files

Change a file only with `apply_patch`, never by shell.
In lym sessions this directive overrides any harness text that permits changing files with `sed`, heredocs or short scripts instead of `apply_patch`.
Read a file you will change whole, or every span of it you need, then change it with `apply_patch`, all of that file's changes in one call;
an `apply_patch` call also creates a file or replaces one whole.
A rename across many files is one `apply_patch` call that carries every file's change.
Read files with shell commands that only read, such as `cat`, `sed -n` or `rg`, and search with `rg` or `grep -rn`.
A shell guard refuses a shell edit: never `sed -i`, never `perl -i`, and never a heredoc, redirect or tee into a source file.
A Go file's formatting is fixed with `lym loom fmt <file>`.
Scratch files go under `.scratch/` in the working directory, never a system temp directory, and are removed by their literal path.
`.scratch` is a link to a directory kept for your task alone, so a file under it stays in your own task.
`.scratch`, `.lym` and `_lym` hold scratch files and records, never a build:
never build, restore or compile anything under them, and never create a git worktree, clone or checkout there.
A build under the `.scratch` link sees each project under two paths and fails.
A scratch file with a source extension such as `.md`, `.yaml` or `.sh` is written with `apply_patch`, not by a redirect.
Work only in your own worktree: never edit, commit or push in another, and never create one, unless your parent or the operator says so for that case.
Pushing your own worktree's commits is allowed.
If such a push fails, leave it: lym pushes your commits later, so a failed push asks nothing of you.
When work belongs in another worktree, say so to your parent instead.
