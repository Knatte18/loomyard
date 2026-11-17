<!-- This is the Codex engine's copy of the subagent-lookups engine block:
     how a writer sends its independent lookups to fresh subagents in one batch.
     A stencil pulls it in on lines of their own, after the lookups are listed, with {{template "engine-subagent-lookups"}}.
     internal/stencilstore reads it for the launch's engine.
     It declares no marker: each fresh subagent runs on the session's own model. -->
Send the independent ones as one batch to fresh subagents, each spawned with `spawn_agent` and `fork_turns: "none"`, all spawned together, then wait for every one with `wait_agent`.
Give each `spawn_agent` call the `reasoning_effort` that the subagent passage of your developer instructions names, and leave its `model` unset.
