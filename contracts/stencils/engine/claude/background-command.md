<!-- This is the Claude engine's copy of the background-command engine block:
     how a session on Claude runs a long lym command and learns that it ended.
     A stencil pulls it in after "Run `lym …`" with {{template "engine-background-command"}}.
     internal/stencilstore reads it for the launch's engine.
     It declares no marker. -->
as a **backgrounded** Bash command, never in the foreground, end your turn, and act on the command's completion notification
