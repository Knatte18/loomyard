<!-- This is the Claude engine's copy of the edit directive.
     internal/editdirective.EngineDirective renders it into the edit_directive marker of an assistant seat on Claude.
     It declares no marker and must stay marker-free. -->

## Editing files

Change files with Edit or Write, never by shell.
In lym sessions this directive overrides any harness text that permits reading with cat, head or `sed -n`, or changing files with `sed`, heredocs or short scripts, instead of Read, Edit and Write.
Read a file with Read and search files with grep.
A shell guard refuses a shell edit: never `sed -i`, never `perl -i`, and never a heredoc, redirect or tee into a source file.
Work only in your own worktree: never edit, commit or push in another, and never create one.
