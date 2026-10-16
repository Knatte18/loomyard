<!-- This is the fixer's instruction: fix every finding, the write-surface/git discipline for the fix, the fixer-report, and the never-push/never-touch-`_lyx` rule.
     It is shipped as an embedded default in the top-level stencils package (stencils/stencils.go),
     seeded to <hub>/_board/_lyx/stencils/burler/ and read from there at call time by composeFixPrompt
     (prompt.go) via internal/stencil, then read by the fixer only when the fixer stencil
     (burler-template-fixer.md) directs it here.
     The friction_directive marker is optional and renders as nothing when the round has no note.
     The self_check marker is optional too, and renders as nothing when the round names no fixer fan;
     it sits directly before the Fixer-report heading with no line of its own, so an empty value adds no byte.
     Every other marker below is a top-level {{.X}} substitution;
     stencil.Fill requires them all non-empty and there are no {{if}}/{{range}} conditionals anywhere in this file (a required marker inside a conditional branch would render silently blank when present-but-empty — see internal/stencil/stencil.go). -->

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
A fix adds a test only when the finding is a coverage gap; for any other finding it corrects the existing test that asserted the wrong thing, per `PATTERN-test-economy`.

## Fix-scope rules — your write surface and git discipline

{{.fix_scope_rules}}

{{.self_check}}## Fixer-report rule (write this to `{{.fixer_report_path}}`)

Write `{{.fixer_report_path}}` unconditionally, every round — even when nothing needed fixing, in which case state "nothing fixed".
Include what you changed, a `Disputed` section (empty if you disputed nothing) and a deferred-with-reason section (empty if nothing was deferred).
The review at `{{.review_path}}` is the reviewer's file: you never write it.
The round is not done until `{{.fixer_report_path}}` exists on disk — a skipped fixer-report leaves the round looking unfinished.

## Never push, never touch `_lyx`

You must never push,
and you must never run any git command against a `_lyx` path — those are owned by the loop that spawned you, not by this round.
If your fix-scope rules above name commit-per-fix on the repo, stay inside the working tree;
nothing here ever authorizes an `_lyx` commit.
