<!-- This is the Claude engine's copy of the foreground-timeout engine block:
     the phrase that gives a long foreground wait command a timeout of an hour.
     A stencil pulls it into a sentence with {{template "engine-foreground-timeout"}}.
     internal/stencilstore reads it for the launch's engine.
     It declares no marker. -->
with the Bash tool's `timeout` at 3600000
