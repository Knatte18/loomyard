I have completed my verification. The plan is well-grounded against the source: the muxengine seam helpers (`withOpLock`, `requireSessionLocked`, `loadOrInitStateLocked`, `sendKeysLiteralArg`, `launchStrandLocked`), the `AddSpec`/`Strand` shapes, `hubgeometry.Layout` accessors, `configreg.Modules()`, `muxengine.New`/`StatusResult` locations, and the registration/helptree/drift/sandbox guards all match the cards' claims. The batch DAG is acyclic with accurate `depends-on`, global card numbering is sequential 1–27 with no gaps, all `Moves:` are `none` (no rename mechanic needed), and every `### Decision:` is faithfully implemented.

MILL_REVIEW_BEGIN
# Review: Build internal/shuttle: one LLM agent via a swappable engine — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-05
```

## Findings

### [NIT] Card 15 Context omits muxengine/strand.go
**Location:** Batch 4 (run-loop), Card 15
**Issue:** The card builds a `muxengine.AddSpec{...}` literal but lists only `muxengine/state.go` in Context; `AddSpec` is defined in `strand.go` (which card 14 does include). The requirement spells out the exact field set, so this is hygiene, not a hard blocker.
**Fix:** Add `internal/muxengine/strand.go` to card 15's `Context:`.

### [NIT] Batch 5 cards 20/21 have an unavoidable transient red — call it out
**Location:** Batch 5 (cli-and-registration), Cards 20 & 21
**Issue:** `sandbox_coverage_test.go` asserts both directions (registered⊆covered AND covered⊆registered), so registering `shuttle` (card 20) without its suite tag OR adding the tag (card 21) before registration each fails one direction — reordering cannot make both per-card commits green; only the batch-end verify is green.
**Fix:** Note in the batch that the sandbox guard passes only once both cards land (batch-level verify); no reorder resolves it.

## Verdict

APPROVE
Thorough, source-accurate plan; valid DAG, faithful decisions, only minor context/note nits.
MILL_REVIEW_END
