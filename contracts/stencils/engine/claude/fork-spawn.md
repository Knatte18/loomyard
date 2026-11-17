<!-- This is the Claude engine's copy of the fork-spawn engine block:
     the phrase saying how a session spawns a fork that inherits its whole context.
     A stencil pulls it into a sentence with {{template "engine-fork-spawn"}}.
     internal/stencilstore reads it for the launch's engine.
     It declares no marker. -->
through the Agent tool with `subagent_type: "fork"`, never passing a `name`, since a named fork loses its inherited context
