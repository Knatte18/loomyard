<!-- This is the Bouncer's escalation brief: what a one-shot fork of the run's parent session reads when a review segment cannot settle a question itself.
     Go renders it into the Bouncer run directory (round-<N>-escalation.md) and the parent notice points at it.
     Every marker below is a top-level {{.X}} substitution, and there are no {{if}}/{{range}} conditionals anywhere in this file.
     Go interpolates only paths, round, cause, segment and slug; the wording lives here and nowhere else. -->

# Escalation — review segment `{{.segment}}`, round {{.round}}

You are a one-shot fork of the parent session of a loom run.
The run's review segment `{{.segment}}` for task `{{.slug}}` has stopped at round {{.round}} and is `awaiting` a decision.
The cause is `{{.cause}}`:

- `circling`: the judge found no progress on recurring gating findings across the rounds.
- `budget`: the review budget is spent without convergence.

## Materials

Read these before you decide anything:

1. The round's review at `{{.review_path}}`.
2. The round's ledger at `{{.ledger_path}}`.
3. The judge's verdict at `{{.verdict_path}}`.
4. The decision record at `{{.decision_record_path}}`, when it exists.

The run's worktree is `{{.worktree}}`.

## What to do

Settle the question only when you can do it on the materials and the task's own record.
When you can:

1. Record each design call you make with `lyx loom decision add {{.slug}} --by parent --title … --decision … --rationale …`, run in `{{.worktree}}`.
2. Record your decision on the segment with exactly one of:
   - `lyx loom circling accept {{.slug}}` ends the segment unconverged on this round.
     The segment passes with its open findings, recorded as unconverged.
   - `lyx loom circling continue {{.slug}}` runs one more round.
3. Resume the run with `lyx loom start` in `{{.worktree}}`.

When you cannot settle the question, record nothing and report back to your parent session, which asks the operator.
The run stays `awaiting`.
