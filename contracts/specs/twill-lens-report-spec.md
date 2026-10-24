# Lens report spec — what a lens review's writer saves of each lens fork's report

> **Status: Contract — pinned.** This doc pins the format of a lens report: the file a lens review's writer saves one lens fork's final message in, so an audit of the lenses reads every report from the run records.

## What it is

A writer that reviews its work through lenses spawns one fork per lens, and each fork returns its report as its final message.
Once the last fork has returned, and before it judges any finding, the writer saves each fork's report in a lens report of its own.
Every lens fan does so: the Plan-Write and Discussion-Write writers' twill fans, Burler's reviewer fan and Burler's fixer self-check fan.

## Where it is

A lens report sits beside the record the writer records its lens review in, at the path its prompt names: the record's path with its extension replaced by `.lens-<lens>.md`, where `<lens>` is the lens's name.
The record is the plan directory's `lens-review.md` for Plan-Write, the support log for Discussion-Write, the round's review file for Burler's reviewer and the round's fixer-report for Burler's fixer.
`internal/lensreport` declares the name rule.

## Format

The file opens with a fenced `yaml` block of two keys:

```yaml
lens: <lens>
outcome: reported
```

- `lens` is the lens's name, as the prompt's lens list gives it.
- `outcome` is exactly `reported` when the fork returned a final message, or `no report` when it returned none.

A blank line follows the block, and then the fork's final message, verbatim: never edited, merged with another lens's report, summarized or reformatted.
A `no report` file ends at the block.

## Readers

No Go code parses a lens report.
Its readers are audits and reviews of the lenses, which read it from the run records.
