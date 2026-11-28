<!-- This is the darn writer's Step 4 list for a run whose verify gate has a cold fixer:
     the writer and its subagents run no gate, except the writer's single measuring run of one test class,
     and the writer acts only on what the fixer leaves.
     The writer asks its parent for the single run of the test a task's reported failure names, before it diagnoses it,
     and for the measurement calls of an entry whose deliverable is gate measurements, before it runs any.
     internal/darnengine's DarnTable reads it for the writer's engine,
     fills the {{.subagent_model}} marker its Claude engine block declares from loom.subagent_model,
     fills the {{.measure_call}} marker with the measuring call the hub's gate config can run,
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
   There are two exceptions.
   One is a value the change needs that only a test run shows, such as a golden test's output.
   The other is a fact the change needs about how an external tool such as tmux behaves:
   learn it from a test you write in the repository, in the tier the hub runs that tool's tests in.
   For either, run that one test class once through `{{.measure_call}}`,
   with the args by which the tier's runner selects one class.
   The call names one package or project and selects one class, so it never starts a whole tier.
   The args reach every tier the call runs, so they must suit each of them.
   A test that call does not reach, one of a tier the key does not run or one a `//lym:only` marker holds for another key,
   runs through `lym gate test --tier <its tier> <its package> -- <the args by which its tier's runner selects that one test>`,
   with no `--gate`, which holds no test back.
   Never learn how a tool behaves from processes you start in the shell, and never start a background or detached process from it:
   such a process outlives your command and runs outside every memory cap lym sets.
   Never run a raw test command for it: a raw run can exit 0 with no output when no test ran, and only the gate's report says so.
   When that report says a project ran no test, the property that makes it run is the one its project file's `IsTestProject` condition names.
   When the tier's command holds `{args}`, run the same gate call once more with `-p:<property>=true` among the args after `--`,
   and never the raw command the report's line suggests.
   When that run still runs no test, or the tier's command holds no `{args}`, name the tests to your parent.
   The measurement calls of an entry whose deliverable is itself gate measurements, such as test times per package,
   fall outside these rules.
   Before any of them, ask your parent with `lym creel post --parent --expect-reply "<text>"`,
   naming every call you plan and their count, then end your turn to wait.
   Run only the calls your parent's answer allows, within the limits it sets, one at a time in the foreground.
   Run them in this session: no subagent runs them.
   The verify gate's test of your committed tree is none of these calls and still runs.
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
   Once you end your turn, the row's verify gate tests your committed tree, and a cold fixer clears its failures before you hear of them.
   You are re-prompted only with what the fixer leaves: act on that alone.
4. Update, in the same commits as the code, the docs the change makes stale: the package doc of each package whose behavior changes, `doc.go` for a Go package;
   the repository's overview doc when a module table or execution stack it shows changes;
   and, when the repository has a `PATTERN.md`, its entry for a cross-cutting invariant the change adds or changes.
5. Apply each bullet of the record's `## PATTERN changes` section to `PATTERN.md` in your own commits.
6. Commit on the task branch after each coherent step, as soon as its edits are made, in small commits.
   Never hold the change uncommitted until the end:
   a stall, a dead session or a reboot then loses at most one step, and your parent sees your progress in the branch's history.
   Never push: the landing steps that follow push the branch.
