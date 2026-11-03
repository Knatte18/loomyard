<!-- This is the Claude engine's copy of the assist-fan engine block: how an assistant reviewer on Claude runs its lens subagents inside its one turn.
     burler-assist-cluster-fan pulls it in with {{template "engine-assist-fan"}}; internal/stencilstore reads it for the assistant's engine. It declares no marker. -->
Run the subagents with the Agent tool, `subagent_type: "general-purpose"`, one call per lens, all in a SINGLE message, as soon as your exploration is done.
A subagent starts with none of your context, so its prompt names, after the boilerplate, the target's files and the fasit's files by path.
The Agent tool returns every subagent's final message in the same turn: never end your turn while a subagent runs.
