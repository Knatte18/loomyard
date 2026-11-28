<!-- This is the darn writer's Step 4 list for a run whose verify gate has a cold fixer:
     the writer and its subagents run no gate, and the writer acts only on what the fixer leaves.
     The writer asks its parent for the single run of the test a task's reported failure names, before it diagnoses it.
     internal/darnengine's DarnTable reads it for the writer's engine,
     fills the {{.subagent_model}} marker its Claude engine block declares from loom.yaml's subagent_model,
     then fills the result whole into darn-template-write's make_change marker. -->

1. When the task starts from a reported test failure, ask your parent before you diagnose it or make any edit:
   send `lym creel post --parent --expect-reply "<text>"`, then end your turn to wait.
   The text names the test exactly and the tier it belongs to,
   and asks the parent to run that test once on your base through `lym gate test`.
   When the parent reports that the run's report shows the named test ran and passed,
   the failure no longer reproduces on your base: the entry is one that landed work overtook, and Step 3 settles it.
   When the report does not show the named test ran, because it was skipped for another tier or matched no test,
   diagnose the failure as usual.
   Edit the repository to make the change.
   Follow the existing patterns, and the PATTERN entries in this prompt.
   Each commit is one change, with its own reads and edits.
2. {{template "engine-darn-subagents"}}
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
