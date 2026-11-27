<!-- This is the review half's explore step: explore the target and understand what it is judged against.
     It ships as an embedded default in contracts/stencils/stencils.go.
     It is seeded to the Bolt checkout's `_lym/stencils/burler/`.
     `composeReviewPrompt` (internal/burlerengine/prompt.go) reads it from there at call time through internal/stencil.
     Only the reviewer reads it, and only when its seat stencil (burler-template-reviewer.md) directs it here.
     target, fasit, rubric, and tool_use_rules are top-level {{.X}} substitutions.
     The fill requires all four non-empty.
     The file holds no {{if}}/{{range}} conditional:
     a required marker inside a conditional branch would render silently blank when present-but-empty.
     See internal/stencil/stencil.go.
     pattern_directive is the fifth marker, optional, and renders as nothing when PATTERN is inactive.
     focus_directive is the sixth marker, optional, and renders nothing when the round has no directive.
     Both stay at the top level, before the first work heading, so their optional-blank semantics hold.
     fork_reading is the seventh marker, optional, and renders nothing in a solo round;
     it sits directly after tool_use_rules on the same line, so an empty value adds no byte. -->

{{.pattern_directive}}
{{.focus_directive}}
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
Each finding also carries one of the four classes — `design`, `scope`, `decision`, `consistency` — and the rubric says what `design` means for this target.

## Tool-use rules — how you gather evidence

Read what your focus needs, or the whole target when no focus is set.

{{.tool_use_rules}}{{.fork_reading}}
