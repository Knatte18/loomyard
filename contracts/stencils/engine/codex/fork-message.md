<!-- This is the Codex engine's copy of the fork-message engine block:
     how a session on Codex sends a message to a fork it spawned.
     A stencil pulls it into a sentence with {{template "engine-fork-message"}}.
     internal/stencilstore reads it for the launch's engine.
     It declares no marker. -->
with `send_message`, addressed to the task name that `spawn_agent` returned for that fork
