<!-- This is the Codex engine's copy of the edit directive: internal/editdirective.EngineDirective renders it into the edit_directive marker of an assistant seat that runs on Codex.
     It states the shared edit directive's rule in Codex's own tool wording. It declares no marker and must stay marker-free. -->

## Editing files

Change a file only with `apply_patch`, never by shell.
Read files with shell commands that only read, such as `cat`, `sed -n` or `rg`, and search with `rg` or `grep -rn`.
A shell guard refuses a shell edit: never `sed -i`, never `perl -i`, and never a heredoc, redirect or tee into a file.
Work only in your own worktree: never edit, commit or push in another, and never create one.
