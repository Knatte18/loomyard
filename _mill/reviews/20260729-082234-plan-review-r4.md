MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] Batch 2 Cards 5/6 are in forward-reference order
**Location:** 02-board-weft-topology.md, Card 5 vs Card 6
**Issue:** Card 5 rewrites `CloneHub` to call `ensureBoardWorktree(weftPath, hostBranch, hubgeometry.BoardDir(hubPath))`, explicitly noting the function comes "from Card 6's new `boardweft.go`" — but Card 6 (which creates `boardweft.go`/`ensureBoardWorktree`) is numbered and sequenced *after* Card 5. Applied in card-number order, Card 5's commit leaves `internal/fabricengine/clone.go` referencing a function that does not exist yet, so the package does not compile between Card 5's commit and Card 6's. Verified against the live `clone.go`/`weftwiring.go`: no such helper exists pre-plan, and Card 6 is the sole card that creates it.
**Fix:** Reorder so the card creating `boardweft.go`/`ensureBoardWorktree` (current Card 6) lands before the card that wires `CloneHub` to call it (current Card 5) — swap their card numbers/positions, or merge them into a single card.

### [NIT] Batch 3: render.go signature change and board.go's call-site fix are split across three cards
**Location:** 03-board-dual-store-facade.md, Card 12 → Card 14 (Card 13 interposed)
**Issue:** Card 12 changes `RenderToDisk`/`Render` to a 4-arg `(tasks, notes []Task, out Outputs)` shape but only edits `render.go`/`layer.go`; `board.go`'s existing call site (`RenderToDisk(b.boardPath, store.Tasks(), b.out)`, 3 args) isn't fixed until Card 14, with the unrelated Card 13 (sync.go) landing in between — widening the batch's internal non-compiling window further than necessary. The batch's own Scope text already argues for keeping render/board changes in one batch rather than splitting across batches, which this doesn't contradict, but the sequencing could still be tightened.
**Fix:** Move Card 14 (or at least board.go's `RenderToDisk` call-site update) to immediately follow Card 12, ahead of Card 13.

### [NIT] Batch 6 Scope contradicts Card 32 on fabric-unified-view.md
**Location:** 06-board-weft-docs.md, Batch Scope vs Card 32
**Issue:** The Batch Scope states manifest/designs/fabric-unified-view.md's "After `board: move storage to weft:main`" passage "is NOT edited by this batch, despite appearing plausible... as a docs-batch target" — but Card 32 explicitly requires editing that same passage (line 94, the inline "board-weft-storage removes `board-url`..." wording) to stop referencing the deleted design doc. Verified: line 94 is inside that exact passage.
**Fix:** Reword the Batch Scope sentence to clarify only the passage's *sequencing/dependency claim* stays unedited, not the dead doc-name reference Card 32 sweeps on the same line.

### [NIT] Card 33 misses a second stale wiki reference in SANDBOX-FABRIC-SUITE.md
**Location:** 06-board-weft-docs.md, Card 33
**Issue:** Card 33 deletes the "Board-URL fallback" pre-condition bullet and updates F1's Watch text, but Pre-conditions item 4 still reads "...the operator seeds it once, alongside initializing the weft wiki" — a dangling reference to the wiki-based board provisioning workflow this task removes entirely.
**Fix:** Add this line to Card 33's edit list, dropping the "alongside initializing the weft wiki" clause.

## Verdict

REQUEST_CHANGES
Batch 2's Card 5/6 ordering leaves fabricengine uncompilable mid-batch; the rest are minor doc/sequencing polish.
MILL_REVIEW_END
