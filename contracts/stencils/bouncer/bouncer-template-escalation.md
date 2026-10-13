<!-- This is the Bouncer's escalation brief: what a one-shot fork of the run's parent session reads when a review segment cannot settle a question itself.
     Go renders it into the Bouncer run directory (round-<N>-escalation.md) and the parent notice points at it.
     Every marker below is a top-level {{.X}} substitution, and there are no {{if}}/{{range}} conditionals anywhere in this file.
     Go interpolates only paths, round, cause, segment and slug, plus the decide fragment it picks by the hard cap; the wording lives here and nowhere else. -->

# Escalation — review segment `{{.segment}}`, round {{.round}}

You are a one-shot fork of the parent session of a loom run.
The run's review segment `{{.segment}}` for task `{{.slug}}` has stopped at round {{.round}} and is `awaiting` a decision.
The cause is `{{.cause}}`:

- `circling`: the judge found no progress on recurring gating findings, or a design-wrong finding returned after a continue.
- `budget`: the soft-cap rule could not read its inputs: the round's review, its ledger or the previous round's ledger.
- `hard-cap`: the round is at or past the perch's hard cap with a design-wrong finding still open, or a continue was recorded there.

What a ledger entry's `design-wrong` and `mechanical` tags mean is defined in `{{.judge_stencil_path}}`.

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

1. Record each design call you make with `lyx loom decision add {{.slug}} --by parent --title … --decision … --rationale …`, run in `{{.worktree}}`, when the decision record exists.
   Without one, name each design call in your report back to your parent session instead.
2. {{.decide_options}}
3. Resume the run with `lyx loom resume` in `{{.worktree}}`.

When you cannot settle the question, record nothing and report back to your parent session, which asks the operator.
The run stays `awaiting`.
