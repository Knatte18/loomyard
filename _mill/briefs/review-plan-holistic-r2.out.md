MILL_REVIEW_BEGIN
# Review: fabric: clone doesn't commit written module configs — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic)
reviewed_file: plan/
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Card 4 test 5's hubforge.NewHub fallback reads a record that does not exist
**Location:** batch 3 / card 4, requirement 5 (mutation record shape)
**Issue:** The fallback text says "drive the hub through `hubforge.NewHub` and read the record it produced instead," but `hubforge.Hub` (hub.go) exposes no `Mutations`/`Mutated()` accessor — `NewHub` discards `CloneAndWire`'s returned `res.Mutations` entirely, keeping only `Path`/`Anchor`/`Location`/`Topology`/`WarpBare`/`WeftBare`/`WeftBase`/`Container`. An implementer who takes the documented fallback has no way to read the record.
**Fix:** Either drop the fallback clause and require the direct `CloneAndWire`-against-bare-repos route only, or add an accessor exposing the record on `Hub`/`NewHub` (which would need its own Edits entry on `internal/hubforge/hub.go` in this card, currently `Edits: none`).

### [BLOCKING:scope] Card 1 references `fabricengine.OriginRecordRel` outside its Context list
**Location:** batch 1 / card 1
**Issue:** Requirements ask the godoc comment to describe `ConfigFileRel`'s shape as "the same shape `fabricengine.OriginRecordRel` returns," but `OriginRecordRel` is declared in `internal/fabricengine/origin.go`, which is not in card 1's `Context:` (`internal/lyxdirs/dirs.go`, `internal/fabricengine/commitweftpaths.go`, `CONSTRAINTS.md`). Per the Context-completeness rule, a named function must have its declaring file in Context or Edits.
**Fix:** Add `internal/fabricengine/origin.go` to card 1's `Context:` list (low-impact — the card already states the fact needed, "anchor-relative," inline, but the file should still be listed).

## Verdict
REQUEST_CHANGES
Card 4's mutation-record fallback is unimplementable as written; card 1 has a minor Context-completeness gap.
MILL_REVIEW_END
