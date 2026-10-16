<!-- This is the review half's explore step: explore the target and
     understand what it is judged against.
     It is shipped as an embedded default in the top-level stencils package (stencils/stencils.go),
     seeded to <hub>/_board/_lyx/stencils/burler/ and read from there at call time by composeReviewPrompt
     (prompt.go) via internal/stencil, then read by the reviewer only when its seat stencil
     (burler-template-reviewer.md) directs it here; the reviewer is its only reader. target, fasit, rubric, and tool_use_rules are top-level {{.X}} substitutions;
     stencil.Fill requires all four non-empty and there are no {{if}}/{{range}} conditionals anywhere in this file (a required marker inside a conditional branch would render silently blank when present-but-empty — see internal/stencil/stencil.go). pattern_directive is the fifth marker,
     optional, filled via stencil.FillOptional, and renders as nothing when PATTERN is inactive.
     focus_directive is the sixth marker, optional, filled via stencil.FillOptional, and renders
     nothing when the round has no directive.
     Both stay at the top level, before the first work heading, so their optional-blank semantics hold.
lyx-stencil: sha256=0c970fbbaa078cf08758ff33d362efff196403c82270d897171c718de2a754e6 build=b2e4cf913d2834503e0338810b1f952e426af78f time=2026-10-11T18:29:27Z -->

{{.pattern_directive}}
{{.focus_directive}}
## Focus directive for this round

Read the file at `.scratch/fan/inputs/focus.md` before forming any finding.
It names where to look and which question to settle this round.

How the directive relates to the rubric:

1. The rubric binds over the focus directive.
   A directive never licenses a finding the rubric's `Do not flag` list or symmetry rule forbids, and never suppresses a finding the rubric requires.
2. A directive steers attention and order, not verdicts.
   "The concrete instance to judge" means reach it and state a verdict on it, and "not a defect" is a valid verdict.
3. Severity comes from the rubric's mapping and the evidence you find.
   A severity cap in a directive is advisory and yields to evidence the round turns up.

If you depart from a directive, record it in the review file's `## Focus departures` section, as the review-file format describes.

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

{{.tool_use_rules}}

## Reading for the fork reviewers

This round fans out: in step 2 you spawn fork reviewers, and each fork inherits your context as it stands at the spawn.
Your reading in this step is the forks' reading, so read here what the review will need, once, instead of leaving each fork to read it again.

Before you leave this step, read:

- every file the target names, at the definitions of the symbols it cites;
- for each symbol the target changes, guards or removes: its direct callers and callees, and every path that reaches it from another process, such as a child the code spawns or a command that calls the guarded verb in-process;
- for each state, setting or file a decision introduces or changes: the existing code paths that also write, reset or reapply it, such as other verbs, hooks, resize or apply steps, and restart paths;
- the `pattern/` background file of every PATTERN entry the target names or touches;
- the failure, restart and resume paths of each change the target makes.

Read definitions and call sites, not whole files: a fork re-reads its inherited context on every turn.
Judge nothing while you read: write no finding, verdict or assessment, not even in a status line or a note to yourself.
A judgement in your context before the spawn colours every fork; your own review starts after the forks are spawned.

Close this step with a reading manifest: one line per file and symbol you read, grouped by the part of the target it serves.
The manifest names what you read and nothing about what you found.
Write it as text in the same turn and go straight on to step 2: the manifest never ends your turn.
