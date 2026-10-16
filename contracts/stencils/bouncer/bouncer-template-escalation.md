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

1. The arbiter attempt's `envelope.json`, when your parent session names one: read it first, since the arbiter already judged the review materials by its own rule and its `question` is what you answer.
2. The round's review at `{{.review_path}}`.
3. The round's ledger at `{{.ledger_path}}`.
4. The judge's verdict at `{{.verdict_path}}`.
5. The decision record at `{{.decision_record_path}}`, when it exists.
6. The support log `support-log.md` in the decision record's directory, read when the definition below needs its `## Interview`.

The run's worktree is `{{.worktree}}`.

## What to do

Settle the question only when a recorded operator decision answers the envelope's question directly.
A recorded operator decision is one of:

- A passage of the task's board entry that the entry marks as an operator decision with a date, as `Operator decision (<date>)` does.
- A board note whose body states that it records an operator decision.
- A decision-record entry added with `--by operator` that is already in the committed decision record when the escalation arrives.
- A Discussion answer that the support log's `## Interview` shows the operator gave.

These never count: an `Idea` or agent-suggestion passage, an operator view not marked as a decision, a decision-record entry added with `--by arbiter` or `--by parent`, a writer's auto-pick, and an `## Auto-mode assumptions` entry.
`--by` is an unverified label, since only `--by arbiter` is restricted, so a `--by operator` entry counts only when it was in the committed decision record before the escalation.
Never settle a question whose answer widens the task's scope, and never accept over an open design-wrong finding unless a recorded operator decision covers that finding.
At cause `budget`, decide nothing: report back to your parent session which input could not be read.

When a recorded operator decision answers the question:

1. Record the operator decision you applied, citing where it is recorded, with `lyx loom decision add {{.slug}} --by parent --title … --decision … --rationale …`, run in `{{.worktree}}`, when the decision record exists; never record with `--by operator`.
   Without a decision record, name the applied decision and its source in your report back to your parent session instead.
2. {{.decide_options}}
3. Resume the run with `lyx loom resume` in `{{.worktree}}`.

Otherwise record nothing and report back to your parent session, which asks the operator.
The run stays `awaiting`.
