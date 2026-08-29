# discussion-format / plan-format — classify review findings by kind (DRAFT)

> **Status: Someday, not yet designed in implementation detail.** This doc records the proposal and its motivating evidence; it is not a ready-to-build spec.

## Idea

lyx is building its own discussion and plan formats (pinned in the producers' own stencils, `contracts/stencils/loom/loom-template-discussion.md` and `contracts/stencils/loom/loom-template-plan.md`) and its own review rounds (`internal/burlerengine`, the Review Round Invariant in `CONSTRAINTS.md`).
Before those formats harden, they should carry a **finding-class** dimension on review findings, not just a severity marker.

## Where the idea came from

A 6-round discussion-review loop on the `pattern-into-lyx-consolidation` task produced blocking-finding counts of 6 → 5 → 4 → 3 → 3 → 3 — never converging.
Sorting the findings by kind explains it:

| Round | design/correctness | scope (missed call sites) | undecided fixture/step |
|---|---|---|---|
| r1 | 5 | 0 | 1 |
| r2 | 2 | 3 | 0 |
| r3 | 2 | 2 | 0 |
| r4 | 2 | 1 | 0 |
| r5 | 2 | 1 | 0 |
| r6 | 0 | 1 | 2 |

Design findings converged to zero.
Scope findings ("you missed these call sites") recurred in four of six rounds and never converged — because the orchestrator kept adding the newly-named files instead of fixing the cause, which was that hand-enumerating ~40 call sites from greps is not a reliable method.

## Why this matters specifically for lyx

The recurring findings were enumerations of `pattern.DirName` consumers — every one of which `go build ./...` reports exhaustively, instantly, and for free.
lyx is a Go project with a compiler, `go vet`, and a dense test suite already in the loop.
A discussion reviewer spending its budget on what the toolchain reports better is correct but economically wrong at that stage.

Each lyx stage has a different natural catchment, and the formats should encode that rather than asking every reviewer to look for everything:

- **Discussion** — is the design right? Is a decision missing, wrong, or resting on a false premise?
- **Plan** — is the work correctly batched, sequenced, and sized? Are the `verify:` commands right?
- **`go build` / `go test`** — does it compile, does it pass? Complete call-site enumeration lives here and nowhere else.
- **Code review (burler)** — does the implementation match the plan and the invariants?

The nuance worth keeping: an inventory is not worthless pre-plan, because the plan sizes batches from it.
But then the right behaviour is to raise it **once, as a design finding about method** — "this is hand-enumerated; delegate it to a mechanical sweep and say so" — never as N individual missing files across N rounds.

## Concrete proposal

1. **Discussion-Review's rubric** (`loom.md`'s [Discussion producer detail](loom.md#discussion-producer-detail--validation-checks-and-review-rubric) section, once a real `Bouncer` rubric exists to point at it) — define a finding-class vocabulary for discussion review: `design`, `scope`, `decision`, `consistency`.
   State that only `design` gates the round loop and only `design` is ever escalated to the operator; the rest auto-resolve.
2. **`Plan-Review`'s shipped rubric** (`contracts/stencils/loom/loom-rubric-plan-review.md`) — what remains open is layering the same finding-class dimension on top of the now-shipped rubric, with its own catchment unchanged: batching/sequencing/verify-command correctness gates; prose-level nits do not.
3. **Round-exit condition** — replace a flat round cap with "stop when a round returns zero gating-class findings," keeping the cap as a backstop.
   On the observed task this exits at r6 by rule rather than by operator override, and correctly refuses to exit at r2-r4.
4. **Per-class counts in whatever envelope lyx's review rounds emit** — so "same class, fourth round running" is visible.
   That is the signal to fix the approach, not patch the symptom again.
5. **The "what NOT to look for" instruction must be written symmetrically, into both sides, not just the reviewer.**
   Writing it into only the writer's own stencil (e.g. `loom-template-discussion.md` telling the Discussion agent not to enumerate cross-references) without also writing the matching instruction into the reviewer's prompt/rubric recreates the same non-convergent loop: a reviewer still operating under a default "flag every gap" mandate will flag the writer's now-correct omission as a missing gap.
   Conversely, instructing only the reviewer wastes the writer's own budget on enumeration nobody will use.
   Both sides must state the same boundary, from their own side: the writer's stencil says "do not enumerate X here, that belongs to <stage>"; the reviewer's rubric says "do not flag missing X here, that belongs to <stage>."
   For discussion review in a Go repo, that explicitly includes "complete call-site enumeration belongs to the compiler / a mechanical sweep, not this stage, on both sides."
   [loom.md](loom.md#discussion-review-rubric--what-to-also-flag-relocation-and-exclusion)'s relocation-and-exclusion rubric subsection is a concrete instance of this same principle.

## `scope` splits into two mechanical halves, neither an LLM lens

"Delegate scope to tooling" is not one delegation, it is two, and no single existing tool covers both:

- **Symbol-level references** (an identifier like `pattern.DirName` used as a Go symbol) — `go build ./...` catches these once the symbol is deleted; `quarry refs`/`assert-no-callers` (LSP-backed) can catch them *before* deletion, for plan sizing.
- **Bare string literals** (the same characters as a value, with no binding to any symbol — e.g. `"_pattern"` hardcoded in a YAML default, a doc-comment, or a test fixture) — neither `go build` nor `quarry refs` sees these, because there is nothing for either to resolve.
  `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals` is a narrow, hardcoded instance of catching this class (fixed token list, excludes `*_test.go`), not a general answer.

Both halves should stay strictly mechanical — a deterministic pre-gate or a `verify:` command, never a reviewer's own free-text search, and never an LLM-optional tool call.
[quarry-plan-symbol-fields.md](quarry-plan-symbol-fields.md) found direct evidence for why the "never an LLM-optional tool call" half of that matters: the [scout-vs-grep benchmark](https://github.com/Knatte18/quarry) benchmarked agents free to choose whether to use `lyx scout` (quarry's ancestor) against grep-only agents, and found no reliable win, plus a real trust-marker gap (`"resolution":"complete"` present on a response that was majority irrelevant cross-package noise) that specifically bites an agent exercising judgment about when to trust the tool.
A deterministic caller never has to make that judgment call.
This is why item 4's per-class counting and item 5's "what not to look for" framing matter more than trying to make `scope` itself smarter or more tool-equipped: the fix is removing the enumeration from reviewer judgment entirely, not improving the reviewer's access to a tool.

## The trap to design against

A class dimension will be read as a severity ladder, and reviewers will file real problems under a low-priority class to dodge the fix-everything default.
lyx's Review Round Invariant should state outright that **class governs who decides and when the loop stops, never whether a finding gets fixed** — everything still goes through the same decision tree.

## Related

A parallel proposal exists for millhouse's own `[GAP]`/`[NOTE]` review format (filed upstream, millhouse issue 788), observed independently.
This doc is lyx adopting the idea natively in its own formats, not inheriting millhouse's.

Observed on branch `pattern-into-lyx-consolidation`; the round-by-round review files are under that branch's `_mill/reviews/`.
