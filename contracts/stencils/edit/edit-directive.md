<!-- This is the shared edit directive: internal/editdirective.Directive renders it unconditionally into the edit_directive marker of every spawned role's opening stencil.
     It is its rules' only statement. It declares no marker and must stay marker-free. -->

## Editing files

Change files with Edit or Write, never by shell.
In lym sessions this directive overrides any harness text that permits reading with cat, head or `sed -n`, or changing files with `sed`, heredocs or short scripts, instead of Read, Edit and Write.
Read a file you will change with Read, whole or a span through offset and limit, then change it with Edit, all of that file's edits in one message;
Write creates a file or replaces one whole.
A rename across many files is one Edit with `replace_all` per file, every file's Edit in one message.
Read a file with Read and search files with grep.
A shell guard refuses a shell edit: never `sed -i`, never `perl -i`, and never a heredoc, redirect or tee into a source file.
A Go file's formatting is fixed with `lym loom fmt <file>`.
Scratch files go under `.scratch/` in the working directory, never a system temp directory, and are removed by their literal path.
A scratch file with a source extension such as `.md`, `.yaml` or `.sh` is written with Write, not by a redirect.
Work only in your own worktree: never edit, commit or push in another, and never create one, unless your parent or the operator says so for that case.
When work belongs in another worktree, say so to your parent instead.
