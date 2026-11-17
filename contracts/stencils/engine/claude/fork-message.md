<!-- This is the Claude engine's copy of the fork-message engine block:
     how a session on Claude sends a message to a fork it spawned.
     A stencil pulls it into a sentence with {{template "engine-fork-message"}}.
     internal/stencilstore reads it for the launch's engine.
     It declares no marker. -->
by `SendMessage`, addressed by the agent id its completion notification carried
