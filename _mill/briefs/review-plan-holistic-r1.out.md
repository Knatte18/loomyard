I have read the full plan and all referenced source files. Here is my review.

MILL_REVIEW_BEGIN
# Review: Board fixes from sandbox run — payload keys, help, rerender — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-28
```

## Findings

### [BLOCKING] Card 1 rename breaks test files it does not edit
**Location:** Batch 1, Card 1 (also Cards 2/3/5)
**Issue:** Card 1 renames exported `Store.SetPhase`/`Board.SetPhase` and the `set-phase` command but its Edits omit `store_test.go` and `cli_test.go`; those packages reference the old names (`store_test.go` `TestSetPhase`/`TestMergeTasks` call `s.SetPhase`; `cli_test.go` `TestCLIContract.TestCLISetPhase`/`TestCLIGetTask` and `TestCLIErrorAndEdgeCases` use `set-phase`/`id_or_slug`/`phase`), so Card 1's commit fails to compile/pass and no card is explicitly assigned to convert these existing cases (only "add new cases").
**Fix:** Add `store_test.go` and `cli_test.go` to Card 1 Edits and require converting every existing `SetPhase`/`set-phase`/`id_or_slug`/`phase` reference (incl. `TestSetPhaseNil`, `TestCLISetPhase`, `TestCLIGetTask`) in the same commit.

### [BLOCKING] Folding `group` into store allowlist invalidates task.go unit tests
**Location:** Batch 1, Card 4
**Issue:** Card 4 removes the `group` checks from `NewTask`/`ApplyPatch`, but `task_test.go` calls `NewTask`/`ApplyPatch` directly and asserts they error on `group`; once the check moves to the store allowlist those functions no longer reject `group` at all, so those subtests fail (no error returned), not just on message. `store_test.go` `TestUpsertTaskGroupKeyError` also asserts the exact old message string.
**Fix:** Card 4 must specify deleting/relocating the `NewTask`/`ApplyPatch` group-rejection subtests to the store layer and updating `TestUpsertTaskGroupKeyError`'s exact-message assertion — not merely "update if it asserts the old message."

### [BLOCKING] Manifest breaks existing single-pass orphan-removal test
**Location:** Batch 3, Card 9
**Issue:** Existing `TestRenderToDisk` pre-creates `proposal-ghost.md` and asserts a single `RenderToDisk` removes it via glob. Under the manifest, a first render with no prior manifest removes nothing (seeds only), so the ghost survives and the test fails. Card 9's "existing orphaned-proposal behavior still holds" is misleading; it only holds across two renders.
**Fix:** Card 9 must explicitly restructure the hand-placed-ghost test to a two-render (or pre-seeded-manifest) form, since the manifest only cleans files it previously recorded.

### [NIT] Store.MergeTasks signature change not reflected in Card 5 Edits
**Location:** Batch 1, Card 5
**Issue:** If `Store.MergeTasks`'s `setPhase *[2]any` parameter changes, `store_test.go` `TestMergeTasks` (`s.MergeTasks(..., &[2]any{"c", phase})`) breaks, but `store_test.go` is not in Card 5's Edits.
**Fix:** State whether `Store.MergeTasks` keeps `*[2]any`; if it changes, add `store_test.go` to Card 5 and name the call site.

## Verdict

REQUEST_CHANGES
Existing board tests break under the rename/allowlist/manifest changes; reassign their conversion explicitly.
MILL_REVIEW_END
