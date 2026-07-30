# Batch: slice-3-design-doc-completion

```yaml
task: 'fabric: warp-side commit lock + push coalescing'
batch: slice-3-design-doc-completion
number: 5
cards: 1
verify: null
depends-on: [3, 4]
```

## Batch Scope

Marks slice 3 DONE in the campaign design doc and resolves its two recorded open questions inline, the way slices 1-2 recorded theirs (Documentation Lifecycle: a planned slice completing updates its design doc). This batch depends on BOTH batch 3 (Fabric.Commit lock + async wiring) and batch 4 (board delegation) because the slice is not complete until board delegates too. It is docs-only — no runnable surface — so `verify: null`.

## Cards

### Card 12: Mark slice 3 DONE and resolve its two open questions

- **Context:**
  - `_mill/discussion.md`
  - `docs/overview.md`
  - `manifest/roadmap.md`
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `manifest/designs/fabric-unified-view.md`, `## Build order` list item 3 ("**Warp-side commit lock + push coalescing ...**"), prepend a `DONE — ` marker and a one-line pointer to the shipped behavior, matching the `**DONE — ...**` style of items 1 and 2 (which cite the landing task and point at `internal/fabricengine/doc.go` for shipped behavior). Cite: the combined write lock taken for any committing `Fabric.Commit`; the `fabricengine.CoalescePush` primitive board now delegates to; the rebase-free async push (`gitrepo.PushRebaseFree`) and elimination of the host-root `.gitrepo-push.lock`.
  - In `## Open questions (for whoever builds this)`, convert the two slice-3 bullets to the resolved `**DONE — ...**` shape used by the other resolved bullets in that section: (1) "**Warp-side lock shape (slice 3)**" → resolved as a single combined fabric-level lock (`.weft/weft.write.lock`) taken by every committing `Fabric.Commit` call, no lock-ordering — cite discussion `### combined-commit-lock`. (2) "**Where the coalescing loop lives (slice 3)**" → resolved as a new `fabricengine.CoalescePush` closure-parameterized primitive (NOT a generalization of `gitrepo.PushCoalesced`, since board's loop coalesces the commit step which is above gitrepo's scope, and gitrepo is geometry-blind) that board's `Sync` delegates to — cite discussion `### coalescing-loop-in-fabricengine-via-closures`.
  - Do NOT move `manifest/roadmap.md` — this is a planned slice completing, tracked in the design doc, not a roadmap-level item (per the project's roadmap-only-for-planned-item-add/complete convention and CONSTRAINTS' Documentation Lifecycle).
  - Confirm `docs/overview.md`'s module table and execution-stack section need no change (no new module is registered; the coalescing primitive is an internal addition to `fabricengine`) and leave it unedited — this confirmation is the card's due-diligence, not an edit.
- **Commit:** `docs(fabric): mark slice 3 done and resolve its open questions in the design doc`

## Batch Tests

`verify: null` — this batch edits only `manifest/designs/fabric-unified-view.md`, a design document with no runnable surface. Correctness is a review obligation (accuracy of the DONE marker and the two resolved open-question bullets against the shipped code). The repo-wide `done_gate` (`go test ./...`) and the module-wide overview `verify` (`go build ./...`) still run at the mill-go pipeline boundaries and cover any accidental non-doc change.
