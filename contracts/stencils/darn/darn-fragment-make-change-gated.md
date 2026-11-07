<!-- This is the darn writer's Step 4 list for a run whose verify gate has a cold fixer:
     the writer and its subagents run no gate, and the writer acts only on what the fixer leaves.
     internal/darnengine's DarnTable fills its one marker, {{.subagent_model}}, from loom.yaml's subagent_model,
     then fills the result whole into darn-template-write's make_change marker. -->

1. Edit the repository to make the change.
   Follow the existing patterns, and the PATTERN entries in this prompt.
   Each commit is one change, with its own reads and edits.
2. Hand work that needs no overview of the change to fresh subagents through the Agent tool, each with `subagent_type: "general-purpose"` and `model: "{{.subagent_model}}"`: a batch of lookups, and per-package work whose edits you have already decided, one subagent per package, each owning its own files.
   Launch every subagent of a batch together in one message.
   Give each Agent call the `effort`, and each prompt the skills, that the subagent passage of your system prompt names.
   A fresh subagent sees none of this session, so each prompt names the files the subagent owns and the edits to make, and tells the subagent to run no `lym gate test`.
   Keep in this session the work that needs the overview: the design, a change that spans packages, the commits and the change description.
3. Run no `lym gate test` yourself.
   Once you end your turn, the row's verify gate tests your committed tree, and a cold fixer clears its failures before you hear of them.
   You are re-prompted only with what the fixer leaves: act on that alone.
4. Update, in the same commits as the code, the docs the change makes stale: the package doc of each package whose behavior changes, `doc.go` for a Go package;
   the repository's overview doc when a module table or execution stack it shows changes;
   and, when the repository has a `PATTERN.md`, its entry for a cross-cutting invariant the change adds or changes.
5. Apply each bullet of the record's `## PATTERN changes` section to `PATTERN.md` in your own commits.
6. Commit on the task branch in small, coherent commits.
   Never push: the landing steps that follow push the branch.
