<!-- This is the darn writer's Step 4 list for a run whose verify gate has no cold fixer:
     the writer and its subagents run the gate themselves,
     bar the measurement calls of an entry whose deliverable is gate measurements,
     which the writer asks its parent for first.
     internal/darnengine's DarnTable reads it for the writer's engine,
     fills the {{.subagent_model}} marker its Claude engine block declares from loom.subagent_model,
     then fills the result whole into darn-template-write's make_change marker. -->

1. Edit the repository to make the change.
   Follow the existing patterns, and the PATTERN entries in this prompt.
   Each commit is one change, with its own reads, edits and gate run.
2. {{template "engine-darn-subagents"}}
   A fresh subagent sees none of this session, so each prompt names the files the subagent owns, the edits to make, and the gate rule from {{template "engine-standing-instructions"}} with the timeout it names, for the subagent to run on its own package.
   Keep in this session the work that needs the overview: the design, a change that spans packages, the commits and the change description.
   Keep here too the measurement calls of an entry whose deliverable is gate measurements, under item 3's rule for them.
3. Test with `lym gate test` after the last edit of each commit; it computes its scope from your changes, and when it answers `done: false` with exit 75 the run goes on, so run the same command again; never run a raw test command or a module-wide run.
   A gate line saying a tier ran no test of a project means the project skipped its own tests, and the gate's pass covers none of them.
   When your change needs those tests and the hub's `gate.refuse_raw` does not list `dotnet test`, run them by hand as that line's way forward names, and check that the run's summary line reports them: this is the one raw test command you run.
   When `gate.refuse_raw` lists it, report the skipped tests to your parent, or with no parent name them in the change description.
   Learn how an external tool such as tmux behaves from a test you write in the repository, in the tier the hub runs that tool's tests in,
   and run that one test through `lym gate test --tier <that tier>` with its package and, after `--`, a filter that selects it;
   then check that the gate's report shows the test ran, not skipped.
   Never learn it from processes you start in the shell, and never start a background or detached process from it:
   such a process outlives your command and runs outside every memory cap lym sets.
   The measurement calls of an entry whose deliverable is itself gate measurements, such as test times per package,
   fall outside these rules.
   Before any of them, ask your parent with `lym creel post --parent --expect-reply "<text>"`,
   naming every call you plan and their count, then end your turn to wait.
   Run only the calls your parent's answer allows, within the limits it sets, one at a time in the foreground.
   Run them in this session: no subagent runs them.
   The gate run after the last edit of each commit still follows for the entry's code or committed results.
   With no parent, or when the post is refused and no strand answers,
   run only the calls an `Added after Discussion` entry of the record allows,
   within its limits, one at a time in the foreground.
   With no such entry, on a task a Code-Review round has run on,
   a `Decision for <slug>:` board note, `<slug>` being this task's, that says the scope is closed
   and files the measurements for a new task settles the point:
   run none of them, do not stop, make what the record asks without them,
   and name the note in the change description's design decisions.
   With no such entry or note, run none of them and stop.
   Write the change description, your one output file, as a stop report in the final-summary spec's form,
   naming the measurements the entry asks for and their scope, never as gate or tier calls,
   and as the way forward `lym loom decision add` with an entry naming the calls it allows and their limits,
   then `lym loom resume`, after which the resumed writer runs them under that entry;
   on a task a Code-Review round has run on, name instead a board note titled `Decision for <slug>: <call>`
   that says the scope is closed and files the measurements for a new task, then `lym loom resume`.
   Then create the stop marker Step 3 names, and end your turn with no further commit.
4. Update, in the same commits as the code, the docs the change makes stale: the package doc of each package whose behavior changes, `doc.go` for a Go package;
   the repository's overview doc when a module table or execution stack it shows changes;
   and, when the repository has a `PATTERN.md`, its entry for a cross-cutting invariant the change adds or changes.
   A figure a doc or comment quotes as measured on a case the change alters is measured again from the current code, or dropped.
5. Apply each bullet of the record's `## PATTERN changes` section to `PATTERN.md` in your own commits.
6. Commit on the task branch after each coherent step, as soon as its gate passes, in small commits that each leave the tree green.
   Never hold the change uncommitted until the end:
   a stall, a dead session or a reboot then loses at most one step, and your parent sees your progress in the branch's history.
   Never push: the landing steps that follow push the branch.
