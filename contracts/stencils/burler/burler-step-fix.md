<!-- This is the fixer's instruction:
     fix every finding, act on a parent ruling that reaches the fixer mid-round, the write surface and git discipline for the fix, the fixer-report,
     and the never-push and never-touch-`_lym` rule.
     It is an embedded default in contracts/stencils/stencils.go,
     seeded to the Bolt checkout's `_lym/stencils/burler/`,
     and composeFixPrompt in internal/burlerengine/prompt.go reads it from there at call time.
     The fixer reads it only when burler-template-fixer.md directs it here.
     The friction_directive marker is optional and renders as nothing when the round has no note.
     The self_check marker is optional too, and renders as nothing when the round names no fixer fan;
     it sits directly before the Fixer-report heading with no line of its own, so an empty value adds no byte.
     Every other marker below is a top-level {{.X}} substitution, and stencil.Fill requires each of them non-empty.
     There are no {{if}}/{{range}} conditionals in this file:
     a required marker inside a conditional branch would render silently blank when present-but-empty
     (see internal/stencil/stencil.go). -->

{{.friction_directive}}

## What to fix

{{.work}}

## Fix-everything rule (BLOCKING — do not skip low-severity findings)

Every finding in the review gets fixed — all severities, including LOW and NIT.
Severity affects how a finding is reported, not whether it gets fixed.
The files to change are the ones the findings' `location` keys name, or the gate findings name.
Weigh each finding twice: its premise, and the change its `fix` key proposes.
Give it exactly one of three dispositions, decided in this order.

1. **Dispute**, on one of two grounds.
   `premise false`: the premise is false against the code you read.
   `every fix worse`: the premise holds, but every change that addresses it makes the target worse.
   One sound change means the finding is fixed, not disputed.
   "Every change" includes changes outside your write surface, so a finding whose only sound change leaves the surface is deferred with that reason, never disputed.
   The evidence is cited text: code, a `PATTERN` entry, a test or caller the change would break, or verify output.
   Severity, size, cost, taste, and disagreement with a rubric or PATTERN never justify a dispute.
   A disputed finding goes into a `Disputed` section of the fixer-report; each entry names its ground, the finding id and the evidence.
2. **Fix with your own change**: the premise holds, the `fix` key names a concrete change you judge wrong or harmful, and some other change resolves the premise.
   Mark the finding as fixed differently inside the fixer-report's list of changes, naming the proposed change you did not make and why, with the same kinds of evidence.
   An own change that would need to leave your write surface is deferred with that reason.
3. **Fix as proposed**: otherwise.
   A `fix` key that is vague leaves the change to you and needs no mark.

The other legitimate reason to leave a finding unfixed is something you genuinely cannot do alone this round (an operator decision on a real tradeoff,
or a capability you do not have);
even then you must say so explicitly, with the specific reason, in the fixer-report's deferred section.
Never leave a finding unfixed just because it looked small — small findings are usually the cheapest to fix, not a reason to skip them.
A fix adds a test only when the finding is a coverage gap; for any other finding it corrects the existing test that asserted the wrong thing.

## Parent ruling rule — a ruling on a `decision`-class finding that reaches you this round

A ruling is a message from your parent, or a decision notice, that settles a `decision`-class finding of this review;
your parent directive names both kinds of message.
It overrides the rule that you neither fix nor report the `decision`-class findings your parent holds:
the ruled finding becomes your work, with the ruling in place of its `fix` key, until you end your turn.
A ruling that reaches you after you wrote the fixer-report adds its entry to the report.

1. **Carry out the ruling** inside your write surface, with the same git discipline as every other fix.
   A part that would need to leave your write surface is left undone, with that reason.
2. **Flag a mistaken or redundant ruling** instead of carrying it out.
   A ruling is redundant when the target already holds what it asks for, and mistaken when the code you read shows its premise false or the change it asks for breaks a test, caller or `PATTERN` entry.
   Leave that part undone and tell your parent once, with `lym creel post --parent "<text>"` and no `--expect-reply`, so the round goes on:
   name the finding id, what the ruling asks, and the cited evidence.
   With no parent recorded, the fixer-report's entry is the only flag.
   A ruling you merely disagree with is carried out.

Record every ruling in the fixer-report's `Rulings` section, never under `Disputed` or deferred:
one entry per ruling, naming the finding id, what the ruling asks, and either the changes you made or the part left undone with its reason and evidence, and whether you told your parent.

## Fix-scope rules — your write surface and git discipline

{{.fix_scope_rules}}

{{.self_check}}## Fixer-report rule (write this to `{{.fixer_report_path}}`)

Write `{{.fixer_report_path}}` unconditionally, every round — even when nothing needed fixing, in which case state "nothing fixed".
Include what you changed, a `Disputed` section (empty if you disputed nothing), a deferred-with-reason section (empty if nothing was deferred) and a `Rulings` section (empty if no parent ruling reached you).
The review at `{{.review_path}}` is the reviewer's file: you never write it.
The round is not done until `{{.fixer_report_path}}` exists on disk — a skipped fixer-report leaves the round looking unfinished.

## Never push, never touch `_lym`

You must never push,
and you must never run any git command against a `_lym` path — those are owned by the loop that spawned you, not by this round.
If your fix-scope rules above name commit-per-fix on the repo, stay inside the working tree;
nothing here ever authorizes an `_lym` commit.
