# Batch: docs

```yaml
task: 'preflight: split into two Shed rows -- a generic one, and loom''s own'
batch: 'docs'
number: 5
cards: 5
verify: go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli
depends-on: [4]
```

## Batch Scope

This batch carries every documentation change the Documentation Lifecycle requires for the split: the module design doc's producer table and its row renumbering, the repo overview's module tree and precondition-layer paragraph, the cross-cutting invariant whose tier-3 bullet names a now-deleted symbol, the on-disk status contract's check-4 section, and the roadmap item this task completes.
It runs last and depends on batch 4 because several of these are written against the post-deletion tree — `CONSTRAINTS.md`'s tier-3 bullet and `loom-status-spec.md`'s validation checklist both describe behaviour that only exists once the composite is gone.

Every file here is prose.
No Go source is touched, so the batch's own risk is drift rather than breakage; the `verify:` chain is run unchanged for uniformity and because `internal/lyxcwd`'s fabric-vocabulary guard also walks markdown under `internal/`, which is where a careless doc edit could still fail a test.

Batch-local decision: the row counts in `internal/loomshed/loomshed_test.go`, `docs/overview.md`'s module-tree line and `internal/loomcli/wiring_test.go` are **not** touched here.
Two of them ("the thirteen rows", "loom's own 13-row producer list") were already written against the post-task count and become correct rather than needing an edit — batch 3 and batch 4 confirmed them in place.
The one that was doubly wrong, the smoke suite's "eight of its thirteen rows", was fixed by card 27 where the file was already open.

## Cards

### Card 29: loom's producer table gains a row

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/loomengine/seed.go`
  - `contracts/specs/loom-status-spec.md`
  - `manifest/designs/hardener.md`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Insert `Loom-Preflight` as row 2 of the producer table and renumber the existing rows 2 through 13 to 3 through 14.
  The new row's cells: `Kind` simple, `Type` mechanical, `Input` loom's own status file pointing at `contracts/specs/loom-status-spec.md`'s check-4 validation checklist, `Output` "pass/fail — no artifact, a gate signal only" (matching row 1's own Output cell, per the thin-Output carve-out the section below the table already names).
  Table cells stay on one line each, per this repo's markdown convention.
  Rewrite the paragraph immediately below the table.
  It currently says `Preflight` is built as `internal/loomengine.Preflight` and "validates the four preconditions", enumerating all four including status-file coherence.
  It must now describe the two-row shape: row 1 is the generic, product-agnostic gate built as `internal/preflightshed`'s general producer over `internal/preflight.Check`, validating worktree geometry, worktree-pair cleanliness, and fabric readiness and sync; row 2 is loom's own, built as `internal/loomengine.CheckSeed` over told paths, validating that loom's status file is a coherent fresh seed.
  Say that row 1 is reusable verbatim by a second product's list and that this is what makes `manifest/designs/hardener.md`'s "its own Preflight" possible; keep the existing sentence about `Shed` bouncing or escalating on stuck.
  Then fix every other row-number reference in the file, which is a closed set of four: the Raddle paragraph's "see rows 12–13 above" and the build-order paragraph's "`Publish` and `Finalize` (rows 12–13)" both become 13–14; the producer-authoring paragraph's "`Discussion-Validate` (row 3)" becomes row 4; and the Plan-Sweep detail section's "`Plan-Sweep` (row 5)" becomes row 6.
  Finally, update the module-decomposition table's `Preflight` row, which names `internal/loomengine` as the new Go package and describes it as validating all four preconditions: it becomes two entries or one two-part entry naming `internal/preflightshed` for the generic tier-1/tier-2 rows and `internal/loomengine` for the seed check, both marked Done.
  Use semantic line breaks throughout — one sentence per line, breaking inside long sentences only at an internal independent-clause boundary, never at a fixed column.
- **Commit:** `docs(loom): add Loom-Preflight to the producer table and renumber the rows`

### Card 30: the repo overview gains the new package

- **Context:**
  - `internal/preflightshed/doc.go`
  - `internal/preflight/doc.go`
  - `internal/landingshed/doc.go`
  - `CLAUDE.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one line to the module tree for `internal/preflightshed`, placed beside its two neighbours — the `internal/landingshed` line and the `internal/preflight` line — describing it as the general `Preflight` `ShedProducer` over `internal/preflight`'s tier-1/tier-2 checks, shared by reference across producer lists.
  Match the surrounding lines' box-drawing prefix, column alignment and one-line-per-entry style exactly; this block is an aligned ASCII tree and a mis-padded entry is visible.
  Extend the precondition-and-geometry paragraph further down, which today names `internal/preflight` as the tier-1/tier-2 precondition layer and `internal/hubgeom`/`internal/standalonegeom` as its two `Geometry` constructors: add `internal/preflightshed` as the producer-shaped wrapper that lets a `Shed` producer list name that layer as a row, and keep the existing cross-reference to the Told-Geometry Invariant.
  Confirm rather than edit the module-tree line for `internal/loomshed`, which reads "loom's own 13-row producer list" — that count is wrong today and correct after this task, so it needs no change.
  Use semantic line breaks in any prose you add.
- **Commit:** `docs(overview): add internal/preflightshed to the module tree and precondition layer`

### Card 31: retarget the Told-Geometry tier-3 bullet

- **Context:**
  - `internal/loomengine/seed.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/preflightshed/doc.go`
  - `internal/preflight/doc.go`
  - `CLAUDE.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The Told-Geometry Invariant's three-resolution-tiers bullet names a symbol this task deleted.
  Its tier-3 entry currently reads "`loomengine.Preflight` — tiers 1+2 plus the orchestrator's own status seed".
  Retarget it to name `loomengine.CheckSeed`, and state the structural change alongside the rename: tier 3 is now a **separate producer row** from tier 2 rather than a function composing it, so tier 3 validates the orchestrator's own status seed alone and does not re-run tiers 1 and 2.
  Change nothing else in that bullet — tiers 1 and 2 keep their exact current wording.
  Do not add `internal/preflightshed` to either the machine-enforced list or the review-obligation list further down that invariant: it deliberately resolves geometry through `preflight.Check`, so it is a tier-2 resolver rather than a told package, and the invariant's own membership predicate ("takes its absolute paths from its caller and has no direct production import of `internal/lyxcwd`") excludes it.
  Claiming membership would be false, and the package's own doc comment states its position instead.
  Do not add a new invariant section — this task introduces no new cross-cutting invariant, only a change to an existing one's tier list.
  This file states rules only, no rationale and no narrative; keep the edit in that register and on semantic line breaks.
- **Commit:** `docs(constraints): retarget the Told-Geometry tier-3 bullet at loomengine.CheckSeed`

### Card 32: the status contract's check-4 section

- **Context:**
  - `internal/loomengine/seed.go`
  - `internal/loomengine/coherence.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/loomshed/seed.go`
  - `internal/shedengine/run.go`
  - `CLAUDE.md`
- **Edits:**
  - `contracts/specs/loom-status-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three statements in this spec are now false, and all three rewordings are pinned rather than left open.
  (1) The paragraph stating that "loom's `Preflight` **requires the file to exist** and fails loud if it is missing — the file's existence *is* the handoff signal, consistent with `Preflight`'s other precondition checks (clean worktree, fabric ready, no half-finished prior run)".
  Its parenthetical is wrong twice over: those other checks belong to the generic row, and it is `Shed`'s own step-1 read gate, not loom's row, that fails loud on a missing file.
  Rewrite it so the file's existence is still the handoff signal but `Shed`'s read gate is what enforces it, and loom's own row requires the file to be a *coherent fresh seed* — a different and narrower claim.
  (2) The validation-checklist bullet reading "`shed.current_producer` must equal `\"Preflight\"` — the only way check 4 is ever reached".
  It becomes the row-2 name, `"Loom-Preflight"`, with the reason restated: that is what `Shed` persists before calling the row, since `Run` writes the next row's name into `current_producer` and appends the finished row's history entry before making the call.
  (3) The fresh-start bullet, which tolerates only entries naming `"Preflight"`.
  It gains the second tolerated name: entries naming either `"Preflight"` or `"Loom-Preflight"` are tolerated, and any third producer is a half-finished failure.
  Keep its existing rationale about `Run` appending a history entry before persisting the blocked state, extended to cover a stuck at either row.
  Leave the *seed* shape untouched wherever it appears — both the prose sentence and the schema block still say a fresh seed carries `current_producer: "Preflight"`, which is exactly what `internal/loomshed/seed.go` still writes and is correct.
  That divergence between the seed contract and the row-2 contract is real and deliberate; add one sentence to the checklist section saying so outright, so a reader hitting both claims does not read one as a typo for the other.
  Use semantic line breaks.
- **Commit:** `docs(spec): retarget loom-status-spec's check-4 section at the two-row shape`

### Card 33: close the roadmap item

- **Context:**
  - `manifest/designs/loom.md`
  - `internal/preflightshed/doc.go`
  - `internal/loomengine/seed.go`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move the Planned item "preflight: split into two Shed rows — a generic one, and loom's own" out of the "Loom infrastructure cleanup" subsection and into `## Done`, inserted at the top of that list where the newest completed items sit, keeping the file's `1.` numbering convention.
  Rewrite it to past tense in the shape the surrounding Done entries use: a short paragraph naming what shipped — the generic row now lives in `internal/preflightshed` as a told-name `ShedProducer` over `internal/preflight.Check`, reusable verbatim by a second product's list; loom's own half is `internal/loomengine.CheckSeed` over told paths, driven as a second row named `Loom-Preflight`; the `check3BlocksSeed` short-circuit is gone because `Shed`'s own sequencing already provides what it hand-rolled; and seed-check coverage moved from Tier 2 to Tier 1 — followed by a "See" line pointing at the two package docs.
  Do **not** carry the item's "13 rows to 14" wording forward.
  That count was right for the design table and wrong for the code, which went from twelve rows to thirteen; state the reconciled numbers or omit counts entirely, but do not repeat the wrong one.
  Do not carry forward the sentence listing the "13-row" references to move, either — that list is stale by the time this lands.
  If the subsection is left with no items after the removal, remove the now-empty `### Loom infrastructure cleanup` heading too rather than leaving a bare heading.
  Use semantic line breaks.
- **Commit:** `docs(roadmap): move the preflight split to Done`

## Batch Tests

`verify:` runs the same three-command chain as every other batch, and none of the five cards can move it — this batch touches no Go source at all.
It is run rather than set to `null` for two reasons.

First, `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary` walks every `internal/**/*.md` file as well as production Go, applying the same bare-token and host-phrase rules; none of this batch's five files live under `internal/`, so it cannot fire here, but running it costs nothing and proves that rather than assuming it.

Second, this batch is the task's last, so its verify is the final full-tree gate before handoff — Tier 1, Tier 2, and the smoke-tag compile all green over the completed change.
A batch that skipped verification would leave that final confirmation to `pipeline.done_gate` alone, which runs only the two `go test` invocations and not the smoke vet.

The batch's own correctness is a review obligation rather than a machine check: the producer table's renumbering, the four other row-number references in the same file, and the three pinned rewordings in the status spec are all prose consistency, and no test in this repo asserts a design doc against the code it describes.
Each card names its target text verbatim for exactly that reason.
