MILL_REVIEW_BEGIN
# Review: fabric: one ownership-and-dirtiness gate for all destruction (slice 12) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-10
```

## Findings

### [BLOCKING:scope] Card 26's Edits list omits the two files it requires editing
**Location:** batch 5 (branch-callsites), card 26 (`internal/fabricengine/weftwiring.go`)
**Issue:** Card 26 gives `removeWeftWorktree` a new `branchPrefix` parameter ("has no config in scope today; add a parameter for the branch prefix... and pass it from both call sites"). Its two callers, `Remove` (`remove.go:102`) and `rollbackAdd` (`add.go:229`), are free functions' only callers and both are `*Topology` methods that must be edited to pass `t.cfg.BranchPrefix` at the call site — verified via grep, exactly these two call sites exist and neither file appears in the card's `Context:` or `Edits:` (only `internal/fabricengine/weftwiring.go` is listed).
**Fix:** Add `internal/fabricengine/remove.go` and `internal/fabricengine/add.go` to card 26's `Edits:` list (they are already touched by other cards in batches 3/5, but this card needs its own declared license and instructions to update these two call sites or the signature change leaves the package non-compiling).

### [NIT:consistency] Card 31's roadmap.md edit under-specifies how to split a combined 4-slice bullet
**Location:** batch 6 (guard-and-docs), card 31 (`manifest/roadmap.md`)
**Issue:** `roadmap.md`'s single Planned bullet currently narrates all of slices 12-15 together (rationale, ordering, why-12-goes-first). "Move slice 12 to completed in whatever form that file already uses for a landed item, leaving the slice 13 and slice 14 dependency statements intact" doesn't say whether to split the bullet into a new Done entry plus a trimmed Planned entry, or how much of the shared rationale text moves versus stays — a real editorial judgment call with more than one defensible outcome.
**Fix:** State explicitly that the existing combined bullet is split: a new short Done-style bullet for slice 12 (mirroring the concise register of existing Done entries), and the Planned bullet edited to describe slices 13-15 only, keeping the "13/14 depend on 12" statements.

## Verdict

REQUEST_CHANGES
Card 26 leaves two required call-site edits (remove.go, add.go) outside its declared Edits scope.
MILL_REVIEW_END
