<!-- This is the shared edit directive: internal/editdirective.Directive renders it unconditionally into the edit_directive marker of every spawned role's opening stencil.
     It is the rule's only statement. It declares no marker and must stay marker-free. -->

## Editing files

Change files only with Edit or Write, never with a script that rewrites them: never `sed`, `awk` or `perl`, and never `python` or `node` in an inline heredoc.
A bulk change is one Edit per site, or `replace_all` for an identical string.
Never use `sed` at all, even to read; search and read with `grep`, `awk` or `cat`.
