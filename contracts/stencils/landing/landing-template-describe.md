<!-- This is the landing Describe session's entire instruction set. It is shipped as an embedded
     default in the top-level stencils package (stencils/stencils.go), seeded to
     the Bolt checkout's `_lym/stencils/landing/`, and read from there at call time by internal/seatengine,
     which fills it from the lead's value map that landingshed's DescribeTable builds, then hands it to shuttle as the describing agent's whole
     prompt. Every marker below is a top-level {{.X}} substitution; stencil.FillWith requires every
     marker non-empty, parent_directive (rendered by internal/parentdirective) and edit_directive (rendered unconditionally by internal/editdirective) included, and there are no {{if}}/{{range}} conditionals anywhere in this file. -->

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
   - anything to check or run by hand that was not run.

Never describe the pipeline's process: no batches, cards, forks, the Master, verify gates, glyph deviations or review rounds.
Naming a module is fine when the change touches it.

## Rules

- Write no `Co-Authored-By` line and no credit lines — the landing step appends the configured credit block itself.
- Never commit, and never run any git command that changes the repository.
- Before ending your turn, run `lym loom validate-description` and fix every finding it reports.
  A clean `validate-description` run is the last thing you do in this session.
