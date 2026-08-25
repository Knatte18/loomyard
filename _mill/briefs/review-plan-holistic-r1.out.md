MILL_REVIEW_BEGIN
# Review: loom: Plan-Review producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-25
```

## Findings

### [BLOCKING:decision] `Plan-Revalidate` row (discussion's round-5 BLOCKING decision) is dropped
**Location:** 00-overview.md Shared Decisions ("fifteen recipe rows"); batch 3 card 9 (`contracts/recipes/loom-recipe.yaml`, `loomshed.go`)
**Issue:** `_mill/discussion.md`'s Q&A log records an explicit `[round 5, BLOCKING]` decision: add a sixteenth row `Plan-Revalidate` (`engine: PlanValidate`, reusing the already-registered engine), with `Plan-Bouncer.on_done` repointed from `Batchifier` to `Plan-Revalidate`, `Plan-Revalidate.on_stuck: Plan-Write`, `on_done: Batchifier`.
Its own "Decision:" subsection ("A `Plan-Revalidate` row re-runs the mechanical checks after the segment") gives the failure mode this closes: `Plan-Burler` is `fix-scope: overlay` and can rewrite `_lyx/plan` with a format regression; the judge is rubric-forbidden from checking format (`Do not flag` item 1); `Batchifier` never parses the plan; and `Webster` carries no `on_stuck` at all — so an unrecoverable block is the only outcome.
The plan never mentions `Plan-Revalidate` anywhere (grepped across all four batch files and the overview — zero hits), and Card 9's recipe/`shape_test.go` changes wire `Plan-Bouncer.on_done: Batchifier` directly — exactly the option the discussion's own decision rejected ("Rejected: leaving `Plan-Bouncer.on_done: Batchifier` and accepting the gap").
This single omission is also why the plan's row-count arithmetic (fourteen→fifteen, table "kept at fourteen entries") diverges from the discussion's own (fourteen→sixteen rows, table moves to fifteen because `Plan-Revalidate` is a genuine new table row, unlike the Bouncer/Burler pair which the table collapses by design) — every downstream doc/test count in the plan is internally consistent with dropping the row, but the row's disposition itself is never stated or argued against anywhere in the plan.
**Fix:** Either add the `Plan-Revalidate` row (new `NamePlanRevalidate` constant, recipe row reusing `PlanValidate`, `Plan-Bouncer.on_done` repointed, and the corresponding row-count updates across `loomrecipe`'s fixture/coverage/shape/recipe/sequence tests and `designs/loom.md`'s table-count paragraph, per discussion.md's own wiring), or, if the plan author has a reasoned basis to override this decision, state that disposition explicitly in a Shared Decision rather than silently reproducing the rejected alternative.

## Verdict

REQUEST_CHANGES
Plan silently drops discussion.md's round-5 BLOCKING `Plan-Revalidate` decision, reproducing the exact rejected wiring.
MILL_REVIEW_END
