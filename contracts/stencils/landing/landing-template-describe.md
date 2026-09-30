<!-- This is the landing Describe session's entire instruction set. It is shipped as an embedded
     default in the top-level stencils package (stencils/stencils.go), seeded to
     <hub>/_board/_lyx/stencils/landing/, and read from there at call time by landingshed's
     DescribeSpec via internal/stencil, then handed to shuttle as the describing agent's whole
     prompt. Every marker below is a top-level {{.X}} substitution; stencil.Fill requires both
     non-empty and there are no {{if}}/{{range}} conditionals anywhere in this file. -->

# Change description — write the one description this task lands under

You are writing the single change description for task `{{.slug}}`.
It becomes the pull request's title and body and the landing commit's message, so it is written for a reviewer who knows the codebase but has not seen the run that produced the change.

## What to read

Read only these sources, and use read-only git for the diff:

- The decision record, `{{.decision_record_path}}`.
- The board entry, via `lyx board get {{.slug}}`.
- The diff of the task branch `{{.task_branch}}` against its merge base with the parent branch `{{.parent_branch}}` — for example `git diff $(git merge-base {{.parent_branch}} {{.task_branch}}) {{.task_branch}}`.
- The run record, `{{.run_record_path}}`, only as a source of manual-check items.
  When it carries an `## Integration suite failed` section, carry that failure into your description as an item for the reviewer to check.

## What to write

Write exactly one file, `{{.description_path}}`, in the format of the final-summary spec (`final-summary-spec.md`):

1. A first line `# <title>`, the title at most 72 characters.
2. A body for the reader described above, covering:
   - what changes for the user or the code, and why;
   - one `Closes #N` line per issue the task resolves;
   - the design decisions a reviewer needs to know;
   - anything to check or run by hand that was not run.

Never describe the pipeline's process: no batches, cards, forks, the Master, verify gates, glyph deviations or review rounds.
Naming a module is fine when the change touches it.

## Rules

- Write no `Co-Authored-By` line — the landing step appends its own trailer.
- Never commit, and never run any git command that changes the repository.
- Before ending your turn, run `lyx loom validate-description` and fix every finding it reports.
  Writing the description is the last thing you do in this session.
