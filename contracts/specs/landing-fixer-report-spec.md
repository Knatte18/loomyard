# Landing fixer report spec — what the merge fixer writes when a turn ends

> **Status: Contract — pinned.** This doc pins the merge fixer's report format: what the fixer session writes at the end of a turn, and what Parent-Align and Finalize read from it.

## What it is

The merge fixer is the one session Parent-Align and Finalize spawn to fix a merge or test failure inside their row.
It ends each turn by writing one report, a markdown file at the path its prompt names, and the row reads that report to decide whether the run goes on or halts.

## Format

The first line is the verdict, exactly `verdict: fixed` or `verdict: escalate`.
Sections follow, each opened by its own `## ` heading, in this order:

- `## Parent changes` — one bullet per parent change the fixer judged against the decision record, each carrying the fixer's finding on that change.
  The section is empty in `finalize` mode, where the fixer judges no parent change.
- `## Fixes` — one bullet per commit the fixer made, naming the commit and what it fixed.
- `## Conflict` — written for `escalate` only: the parent change and the decision of the record they conflict over, in prose.

A bullet is a line opening with `- `.
An empty section is its heading with no bullet under it.

## Verdicts

- `fixed` — the fixer made every change the failure needed, and the row goes on to re-check.
- `escalate` — a parent change conflicts with a decision of the record, which the fixer does not settle; the row halts to the arbiter first on a loom or darn run and to the orch on a pawl run with the `## Conflict` text.
  An `escalate` report with no `## Conflict` section is invalid.

## Sole parser

`internal/landingshed` is the sole parser of the format, through `ParseFixerReport`; consumers read only the `FixerReport` it returns.
A missing report is an error wrapping `fs.ErrNotExist`, so a caller tells a missing report from an unparseable one.
A first line that is no known verdict, and an `escalate` report without its conflict, are errors naming the line and this spec.
