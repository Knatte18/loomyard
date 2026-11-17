<!-- This is the Codex engine's copy of the darn-subagents engine block:
     how the darn writer hands lookups and per-package work to fresh subagents, as an item of its Step 4 list.
     A Step 4 list pulls it in after its item number with {{template "engine-darn-subagents"}}.
     internal/stencilstore reads it for the launch's engine.
     It declares no marker: each fresh subagent runs on the session's own model. -->
Hand work that needs no overview of the change to fresh subagents, each spawned with `spawn_agent` and `fork_turns: "none"`: a batch of lookups, and per-package work whose edits you have already decided, one subagent per package, each owning its own files.
   Spawn every subagent of a batch together, then wait for every one with `wait_agent`.
   Give each `spawn_agent` call the `reasoning_effort`, and each prompt the skills, that the subagent passage of your developer instructions names, and leave its `model` unset.
