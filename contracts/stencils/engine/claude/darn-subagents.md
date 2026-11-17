<!-- This is the Claude engine's copy of the darn-subagents engine block:
     how the darn writer hands lookups and per-package work to fresh subagents, as an item of its Step 4 list.
     A Step 4 list pulls it in after its item number with {{template "engine-darn-subagents"}}.
     internal/stencilstore reads it for the launch's engine.
     It declares one required marker, {{.subagent_model}}, the model each fresh subagent runs on. -->
Hand work that needs no overview of the change to fresh subagents through the Agent tool, each with `subagent_type: "general-purpose"` and `model: "{{.subagent_model}}"`: a batch of lookups, and per-package work whose edits you have already decided, one subagent per package, each owning its own files.
   Launch every subagent of a batch together in one message.
   Give each Agent call the `effort`, and each prompt the skills, that the subagent passage of your system prompt names.
