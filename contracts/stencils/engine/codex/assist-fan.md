<!-- This is the Codex engine's copy of the assist-fan engine block:
     how an assistant reviewer on Codex runs its lens subagents inside its one turn.
     burler-assist-cluster-fan pulls it in with {{template "engine-assist-fan"}}.
     internal/stencilstore reads it for the assistant's engine.
     It declares no marker. -->
Spawn the subagents with `spawn_agent`, one per lens, each with `fork_turns: "all"` so it inherits everything you have read, all in one go, as soon as your exploration is done.
Then wait for every subagent with `wait` inside this same turn, and keep waiting until the last one has returned: never end your turn while a subagent runs, since a turn you end leaves your report unwritten.
