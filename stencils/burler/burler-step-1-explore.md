<!-- This is burler round instruction 1 of 3: explore the target and
     understand what it is judged against.
     It is shipped as an embedded default in the top-level stencils package (stencils/stencils.go),
     seeded to <hub>/_board/_lyx/stencils/burler/ and read from there at call time by composePrompt
     (prompt.go) via internal/stencil, then read by the agent only when the round orchestrator
     (burler-template-round-orchestrator.md) directs it here. target, fasit, rubric, and tool_use_rules are top-level {{.X}} substitutions;
     stencil.Fill requires all four non-empty and there are no {{if}}/{{range}} conditionals anywhere in this file (a required marker inside a conditional branch would render silently blank when present-but-empty — see internal/stencil/stencil.go). pattern_directive is the fifth marker,
     and the one optional one: it is filled via stencil.FillOptional and renders as nothing when PATTERN is inactive.
     It stays at the top level, before the first work heading, so its optional-blank semantics hold. -->

{{.pattern_directive}}
## What to review (the target)

{{.target}}

## What to judge it against (the fasit)

{{.fasit}}

The fasit is the source of truth the target is judged AGAINST.
A review that ignores the fasit degenerates into a pure internal-consistency check — always read it and hold the target to it.

## Rubric

{{.rubric}}

The rubric tells you what counts as BLOCKING, MEDIUM, LOW, or NIT for THIS target.
It maps its own criteria onto that fixed four-value severity vocabulary;
it never introduces a new severity name, and neither do you.

## Tool-use rules — how you gather evidence in job A

{{.tool_use_rules}}
