<!-- This is burler round instruction 3 of 3: job B — fix every finding,
     the write-surface/git discipline for the fix, the fixer-report, and the never-push/never-touch-`_lyx` rule.
     It is shipped as an embedded default in the top-level stencils package (stencils/stencils.go),
     seeded to <hub>/_board/_lyx/stencils/burler/ and read from there at call time by composePrompt
     (prompt.go) via internal/stencil, then read by the agent only when the round orchestrator
     (burler-template-round-orchestrator.md) directs it here, after instructions 1 and 2.
     Every marker below is a top-level {{.X}} substitution;
     stencil.Fill requires all three non-empty and there are no {{if}}/{{range}} conditionals anywhere in this file (a required marker inside a conditional branch would render silently blank when present-but-empty — see internal/stencil/stencil.go). -->

## Fix-everything rule (BLOCKING — do not skip low-severity findings)

Every finding you record in the review gets fixed in job B — all severities, including LOW and NIT.
Severity affects how a finding is reported, not whether it gets fixed.
The only legitimate reason to leave a finding unfixed is something you genuinely cannot do alone this round (an operator decision on a real tradeoff,
or a capability you do not have);
even then you must say so explicitly, with the specific reason, in the fixer-report's deferred section.
Never leave a finding unfixed just because it looked small — small findings are usually the cheapest to fix, not a reason to skip them.

## Fix-scope rules — your write surface and git discipline for job B

{{.fix_scope_rules}}

## Fixer-report rule (write this to `{{.fixer_report_path}}`)

Write `{{.fixer_report_path}}` unconditionally, every round — even when the verdict was APPROVED and nothing needed fixing, in which case state "nothing fixed".
Include what you changed and a deferred-with-reason section (empty if nothing was deferred).
The round is not done until BOTH `{{.review_path}}` and `{{.fixer_report_path}}` exist on disk — a skipped fixer-report leaves the round looking unfinished.

## Never push, never touch `_lyx`

You must never push,
and you must never run any git command against a `_lyx` path — those are owned by the loop that spawned you, not by this round.
If your fix-scope rules above name commit-per-fix on the repo, stay inside the working tree;
nothing here ever authorizes an `_lyx` commit.
