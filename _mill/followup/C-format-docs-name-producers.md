```yaml
slug: format-docs-name-producers
title: "format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate"
depends_on: ["plan-format-drop-v3-suffix"]
brief: |
  Rewrite discussion-format.md and the renamed plan-format.md to name their producers and contracts explicitly in producer-model terms, add the new Discussion-Review-Gate producer covering discussion-format.md's checks 1–2, and scoped-edit loom.md's producer table rows 2–7 to name the artifacts that actually exist and to list the new gate.
```

# format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate

## Why

`discussion-format.md` and the renamed `plan-format.md` are the two pinned contracts the flat producer model points at — every Input/Output cell in `loom.md`'s producer table is supposed to be a pointer into one of these two files, never a restated copy.
Both files still describe themselves in pre-producer terms, so the pointer rule they are meant to anchor currently has nothing coherent to point at.

**`loom-table-names-real-artifacts`.**
`loom.md`'s producer table currently names two artifacts that exist nowhere in the pinned contracts: `discussion.md` and `plan.md`.
The real artifacts are `_lyx/discussion/decision-record.md` (not `discussion.md`) and the `_lyx/plan/` directory (not `plan.md`),
and the two-file access boundary becomes part of the `Plan-Write` Input pointer.
A producer table whose Input/Output pointers name nonexistent files defeats the pointer rule it is meant to demonstrate.
This was not left open for this task to re-derive — the real paths are already pinned by `discussion-format.md` and by `loom.md:188`'s own statement that the Planner writes `_lyx/plan/NN-<card>.md` per card plus `00-overview.md` as the done-sentinel.

## What needs to happen

1. Rewrite `discussion-format.md` and the renamed `plan-format.md` to name their producers and contracts explicitly in producer-model terms.
2. Add the `Discussion-Review-Gate` producer, covering checks 1–2 of `discussion-format.md:80–82`.
   See the dedicated subsection below for its full rationale.
3. Scoped-edit `loom.md`'s table rows 2–7 to name the real artifacts — `_lyx/discussion/decision-record.md` and `_lyx/plan/` — and insert `Discussion-Review-Gate` into the producer list.
4. Fix `discussion-format.md:1`'s own title, which still reads "the `discussion.md` ↔ Plan contract" — the same nonexistent artifact the `loom.md` table named.
5. Restate `discussion-format.md:14` in producer-model terms.
   It currently grounds the two-file split in "Builder's 'distilled digest, never raw prose' rule (see `builder-contract.md`'s digest contract)" — a live contract justified by a retired design doc.
   The rule itself is sound and stays;
only its attribution is rewritten.
6. Rewrite `plan-format.md:5`'s "Coexistence, not replacement" section, which asserts the format does not retire v2.
   That claim is false once task A deletes v2, so the renamed file would otherwise carry the claim forward about itself.

### The `Discussion-Review-Gate` producer

**`discussion-review-gate-exists`.**
The `Discussion` side is not inherently asymmetric — scope a `Discussion-Review-Gate` mechanical producer, mirroring `Plan-Review-Gate`.
It runs checks 1–2 of `discussion-format.md:80–82`: both files exist under `_lyx/discussion/`,
and `decision-record.md` has all seven required sections present (Goal, Scope, Decisions, Constraints, Auto-mode assumptions, Open risks, Acceptance criteria).
Both are per-run, artifact-observable properties of exactly the kind `Plan-Review-Gate` already hard-fails on,
and both are already written down — this task names them as a producer, it does not design anything new.

**Check 3 is not a gate check.**
`discussion-format.md:83`'s claim — that the Plan producer's declared input set never names `support-log.md` — is a property of the producer *definition*, not of any run's artifacts.
There is nothing per-run for a gate to evaluate.
It becomes a build-time test assertion over the producer definition instead: a static property caught once and forever by a compile- or test-time guard, rather than re-evaluated on every run.
This task's body states this explicitly, in so many words, so nobody re-files it later as a missing gate check.

**`discussion-stays-two-files-with-current-names`.**
`_lyx/discussion/` stays a two-file directory: `decision-record.md` (the Plan producer's sole input) and `support-log.md` (read only by the Discussion-review gate).
`decision-record.md` is **not** renamed.
`discussion-format.md:16` states the filenames are self-describing on purpose, `decision-record.md` pairs with `support-log.md`, and the file holds seven sections, so naming it after one of its own sections would mislead.
Rejected alternatives: `decisions.md` and `decision.md` — both terser, both lose the sibling parallelism, and both would force a code sweep across `DiscussionDecisionRecord` in `internal/loomengine/config.go`, `discussionpath_test.go`, `discussion_test.go`, and `prompttemplate.go`, for no contract gain.
Note: the sourcing discussion cites this sentence as `discussion-format.md:15`,
but the self-describing-filenames sentence is actually at `:16` — `:15` is the preceding sentence about the filesystem boundary.
This task uses `:16`;
the off-by-one from the discussion is not propagated into this body.

**The symmetric "what NOT to look for" rule.**
Per `review-finding-classification.md` item 5, a "what NOT to look for" instruction must be written symmetrically into both the producer's own format-contract and the reviewing producer's rubric.
Writing it into only one side recreates the non-convergent review loop that doc exists to prevent.
This task honours that rule when writing the `Discussion-Review-Gate`'s rubric: whatever the gate is told not to look for must also be written into `discussion-format.md` itself, not only into the gate's own instructions.

## Scope

This task owns `loom.md`'s producer table rows 2–7 only — the artifact-name fixes and the `Discussion-Review-Gate` insertion.
Task E owns everything else in `loom.md` and runs after both this task and task F.

## Sequencing

`depends_on: plan-format-drop-v3-suffix` — this task edits the renamed file.

Task E depends on this task, so that E writes `loom.md`'s finished table state rather than guessing at it.

## Acceptance

Docs-only;
this task has no test surface of its own.

The `Discussion-Review-Gate`'s checks are specified here, not implemented — implementation lands with `Shed`.
Check 3's build-time assertion is likewise specified rather than written, since the producer definition it would assert over does not exist yet.

Every relative markdown link and anchor introduced or touched by this task must resolve.
