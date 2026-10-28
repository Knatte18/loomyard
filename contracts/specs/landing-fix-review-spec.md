# Landing fix review spec — what the fix review writes

> **Status: Contract — pinned.** This doc pins the fix review's report format: what the fix review session writes when it ends, and what Finalize reads from it.

## What it is

The fix review is the one solo session Finalize spawns when the merge fixer's change is over the caps and a review follows Finalize's reroute.
It reads only the fixer's commits and ends by writing one report, a markdown file at the path its prompt names.
An approval lets Finalize land; a rejection sends the run back along Finalize's reroute edge.

## Format

The first line is the verdict, exactly `verdict: approved` or `verdict: rejected`.
One section follows, opened by the heading `## Findings`: one bullet per finding, each naming the commit or file and what is wrong with it.
A bullet is a line opening with `- `.

## Verdicts

- `approved` — the change fixes the failure and keeps every test's meaning; the `## Findings` section may be empty.
- `rejected` — the change weakens or dodges a test, or is wrong in itself; the `## Findings` section holds at least one bullet.
  A `rejected` report with no finding is invalid.

## Sole parser

`internal/landingshed` is the sole parser of the format, through `ParseFixReviewReport`; consumers read only the `FixReviewReport` it returns.
A missing report is an error wrapping `fs.ErrNotExist`.
A first line that is no known verdict, and a `rejected` report without a finding, are errors naming the line and this spec.
