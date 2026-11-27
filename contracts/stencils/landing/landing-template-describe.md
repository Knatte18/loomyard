<!-- This is the landing Describe session's entire instruction set.
     It is shipped as an embedded default in the top-level stencils package (stencils/stencils.go),
     seeded to the Bolt checkout's `_lym/stencils/landing/`.
     internal/seatengine reads it from there at call time.
     It fills the template from the lead's value map, which landingshed's DescribeTable builds.
     It then hands the result to shuttle as the describing agent's whole prompt.
     Every marker below is a top-level {{.X}} substitution, and stencil.FillWith requires each to be non-empty.
     internal/parentdirective renders parent_directive, and internal/editdirective renders edit_directive unconditionally.
     There are no {{if}}/{{range}} conditionals in this file. -->

# Change description — write the one description this task lands under

{{.parent_directive}}
{{.edit_directive}}

You are writing the single change description for task `{{.slug}}`.
It becomes the pull request's title and body and the landing commit's message, so it is written for a reviewer who knows the codebase but has not seen the run that produced the change.
The change may have been fixed after an earlier description was written, so describe the diff as it stands now.

## What to read

Read only these sources, and use read-only git for the diff:

- The decision record, `{{.decision_record_path}}`.
  Its `## Goal` is the task's scope as its `Added after Discussion` entries amend it.
  An entry between `<!-- lym:carry-over … -->` marker lines in `## Open risks` lists the findings a review left open.
  Each finding a Plan-Review or Code-Review entry lists becomes one item to check by hand in your description, worded as the finding and where it is, never as a review round.
  A Discussion-Review entry is not carried into the description.
- The board entry, via `lym board get '{"slug":"{{.slug}}"}'`.
- The diff of the task branch `{{.task_branch}}` against its merge base with the parent branch `{{.parent_branch}}`, read as `git diff {{.merge_base}} {{.task_branch}}`.
- The run records, one per plan generation with the oldest first and the live one last, only as a source of manual-check items; `(none)` means the run has none:
{{.run_record_paths}}
  A manual-check item from an older generation still counts when the live change does not make it obsolete.
  When a record carries an `## Integration suite failed` section, carry that failure into your description as an item for the reviewer to check.
  When a record carries an `## Integration suite triage` section, carry it into your description as an item for the reviewer to check too, naming the flaky and pre-existing identities it lists.
  Name each verify command failure the section lists by its first run's log, as a failure of the verify step rather than a flaky test.
  When a record carries an `## Integration suite fix` section, carry it into your description as an item for the reviewer to check, naming the fix commits and the identities they cleared.

## The regression check

The regression check below has already run.
lym appends it after your description in the pull request and the landing commit, so write no regression section of your own.
Never list as an item to check or run by hand a regression compare this check settled: drop such an item that an earlier description or a run record names, or rewrite it from the check when it asks for more than the check did.

{{.drift_verdict}}

## What to write

Write exactly one file, `{{.description_path}}`, in the format of the final-summary spec (`loom-final-summary-spec.md`):

1. A first line `# <title>`, the title at most 72 characters.
2. A body for the reader described above, covering:
   - what changes for the user or the code, and why;
   - one `Closes #N` line per issue the task resolves;
   - the design decisions a reviewer needs to know;
   - anything to check or run by hand that was not run;
   - last, the `## Merge danger` section the spec defines, its door and blast radius called from the diff as it stands now.

Never describe the pipeline's process: no batches, cards, forks, the Master, verify gates, glyph deviations or review rounds.
Naming a module is fine when the change touches it.

The reader knows this repository and nothing of the tooling that wrote the change.
Name nothing of that tooling that the repository itself does not hold: not LoomYard or lym, the board or a board entry, the task, the Sketch or the decision record, the parent or an orch, the scoper, a gate, a tier or a perch.
Never point at the scope by its numbering, as "item 3 of the task" does, and never report what the parent recorded or ruled: state the requirement or the decision itself.
`lym loom validate-description` names each line that matches one of the hub's forbidden terms; rewrite such a line in the repository's own words.

A claim that a result agrees with a reference, or that the change improves one, carries before and after figures from the current code, taken from the commits or the regression check.
Leave out a claim that has none.

{{template "seat-directive-assistant-chair"}}

## Your reviewer

Your one assistant reviews your description once you have ended your turn on a text that passes the check: it looks for the tooling's terms, for claims the diff does not bear out, and for deliverables the scope says the description states.
You never wait for it yourself.
Once your description passes `lym loom validate-description` and you end your turn, lym holds the turn end until the reviewer's end marker appears or the review's time runs out.
When the reviewer reports findings, they reach you once, as a message naming a file that holds its report.
Weigh each finding against the diff and the decision record, fix the description where a finding holds, run `lym loom validate-description` again, and end your turn.
When its end marker reads anything other than `done`, or its report approves, your description stands as you wrote it.

## Rules

- Write no `Co-Authored-By` line and no credit lines — the landing step appends the configured credit block itself.
- Never commit, and never run any git command that changes the repository.
- Before ending your turn, run `lym loom validate-description` and fix every finding it reports.
  A clean `validate-description` run is the last thing you do in this session.
