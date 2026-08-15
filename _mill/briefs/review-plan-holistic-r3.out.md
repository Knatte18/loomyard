MILL_REVIEW_BEGIN
# Review: Shed: outer phase-FSM skeleton — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-08-15
```

Reviewed the overview, all six batch files, `_mill/discussion.md` in full, `manifest/designs/shed.md`, `CONSTRAINTS.md`, `docs/overview.md`, `docs/reference/status-schema.md`, `manifest/roadmap.md`, and the cited `internal/state`, `internal/lock`, `internal/treadleengine`, and `internal/loomengine/coherence.go` source.

Verified: the Batch Index DAG (00-overview.md) has no cycle, every `depends-on` batch number and every `file:` target exists, and step numbering across all 31 cards is sequential with no gaps. `## All Files Touched` is an exact union of every card's `Creates:`/`Edits:` targets (Move-source and Deletes paths correctly excluded). Every card carries Context/Edits/Creates/Deletes/Moves/Requirements/Commit; all `Moves:` are bare `none`, so no rename mechanic is required and none is missing.

Every `### Decision:` in the overview's Shared Decisions and every named decision in `_mill/discussion.md`'s Decisions section is faithfully threaded through the batch cards I checked: the two-value `Outcome` contract and its correction in `shed.md:27` (batch 1 card 2, batch 6 card 25), the told-never-derived path contract and the `StatusLockPath` field (batch 1 card 3, batch 5), the single-commit-point merge persist with the `found==false` guard (batch 2 cards 8–9), the cancellation-as-pause routing keyed on `ctx.Err()` rather than an error sentinel (batch 2 card 9, batch 3 card 15), the already-done short-circuit's exact position and idempotent `Result` (batch 2 card 9, batch 3 card 17), the exact bounce-budget boundary and its zero-means-default rule (batch 2 cards 9/12), the field-ownership split and external-writer lock contract (batch 4 cards 18–19), and the Shed Producer-Seam Invariant's allowlist, including its transitivity claim, which I confirmed against the real import lists of `internal/lock` (no internal imports) and `internal/state` (`internal/fsx`, `internal/lock` only) — batch 5's `CONSTRAINTS.md` text is accurate.

Cross-checked `internal/state.ReadJSONStrict`/`UpdateJSON`/`internal/lock`'s actual behavior (blocking vs. non-blocking acquire, `MkdirAll` timing, `DisallowUnknownFields`) against every locking/persistence scenario in batches 2–4; all match the real primitives. Checked `manifest/designs/loom.md`'s existing anchors into `shed.md` (`#producer-contract-vs-producer-definition`, `#engine-adapters--a-thin-shared-seam-not-one-per-producer`) — neither heading is renamed by batch 6's cards 25–29, so those external links stay intact.

No BLOCKING or NIT-worthy issues found.

## Verdict

APPROVE
Plan is internally consistent, fully traces to the discussion's decisions, and its test/doc batches are correctly scoped and ordered.
MILL_REVIEW_END
