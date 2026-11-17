<!-- This is the darn writer's Step 4 list for a run whose verify gate has no cold fixer:
     the writer and its subagents run the gate themselves.
     internal/darnengine's DarnTable reads it for the writer's engine,
     fills the {{.subagent_model}} marker its Claude engine block declares from loom.yaml's subagent_model,
     then fills the result whole into darn-template-write's make_change marker. -->

1. Edit the repository to make the change.
   Follow the existing patterns, and the PATTERN entries in this prompt.
   Each commit is one change, with its own reads, edits and gate run.
2. {{template "engine-darn-subagents"}}
   A fresh subagent sees none of this session, so each prompt names the files the subagent owns, the edits to make, and the gate rule from {{template "engine-standing-instructions"}} with the timeout it names, for the subagent to run on its own package.
   Keep in this session the work that needs the overview: the design, a change that spans packages, the commits and the change description.
3. Test with `lym gate test` after the last edit of each commit; it computes its scope from your changes, and when it answers `done: false` with exit 75 the run goes on, so run the same command again; never run a raw test command or a module-wide run.
   A gate line saying a tier ran no test of a project means the project skipped its own tests, and the gate's pass covers none of them.
   When your change needs those tests and the hub's `refuse_raw` does not list `dotnet test`, run them by hand as that line's way forward names, and check that the run's summary line reports them: this is the one raw test command you run.
   When `refuse_raw` lists it, report the skipped tests to your parent, or with no parent name them in the change description.
4. Update, in the same commits as the code, the docs the change makes stale: the package doc of each package whose behavior changes, `doc.go` for a Go package;
   the repository's overview doc when a module table or execution stack it shows changes;
   and, when the repository has a `PATTERN.md`, its entry for a cross-cutting invariant the change adds or changes.
5. Apply each bullet of the record's `## PATTERN changes` section to `PATTERN.md` in your own commits.
6. Commit on the task branch in small, coherent commits, each leaving the tree green.
   Never push: the landing steps that follow push the branch.
