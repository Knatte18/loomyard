MILL_REVIEW_BEGIN
# Review: fabric: fold snapshot-tracking into the Warp-SHA trailer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (model ID claude-sonnet-5, per this session's system prompt)
reviewed_file: plan/
date: 2026-07-31
```

## Findings

### [BLOCKING] Card 12's per-branch-scoping case needs a fixture not in its Context
**Location:** Batch 3 (snapshot-reader) / Card 12
**Issue:** "Per-branch scoping" instructs recording a tag then running "a coordinated Checkout to another [branch]" and asserting the miss. `checkout.go`'s `Topology.Checkout(l *hubgeometry.Layout, branch string)` is the only "coordinated Checkout" in the codebase, and it needs a full `*hubgeometry.Layout` — built, per `checkout_index_refresh_test.go` (an existing, near-identical worked example: record correspondence, coordinated-switch, assert a miss), via `newFabricFixture` (`lyxtest.CopyPairedLocal` + `fabricengine.NewTopology` + `WireJunctions`) in the external `fabricengine_test` package. That is a materially heavier harness than the `newPlainWarpRepo`/`lyxtest.CopyWeft`/`newFabric` pattern Card 12 otherwise directs the implementer to reuse "rather than building a parallel harness," and neither `checkout_index_refresh_test.go`/`reconcile_stale_registration_test.go` (source of `newFabricFixture`) nor the fixture-shift itself is named anywhere in Card 12.
**Fix:** Either add `checkout_index_refresh_test.go`/`reconcile_stale_registration_test.go` to Card 12's Context and say to mirror that fixture, or clarify that a raw `git checkout <branch>` inside the existing lightweight weft-only fixture (bypassing `Topology.Checkout` entirely — `SnapshotWarpSHA` only cares about weft's current branch) satisfies the assertion, matching the file's simple-harness idiom.

### [NIT] Card 19's unborn-weft fixture has no named construction pattern
**Location:** Batch 4 (empty-commit-rule) / Card 19
**Issue:** The "unborn weft HEAD plus tags" case needs a weft checkout with zero commits. Every other fixture-construction instruction in this batch points at an existing helper or file to mirror (e.g. "read `weftgit_pathspec_integration_test.go` first," "`weftgit_unborn_warp_test.go` shows how that fixture is built" for the *warp*-unborn case). No equivalent pointer exists for the weft-unborn case, even though `newUnbornWarpRepo` in the already-listed `weftgit_unborn_warp_test.go` is the obvious template to mirror for weft.
**Fix:** Name the helper to add (e.g. `newUnbornWeftRepo`, mirroring `newUnbornWarpRepo`) or explicitly say to build a fresh `git init` weft directory in place of `lyxtest.CopyWeft`.

## Verdict

REQUEST_CHANGES
Card 12's per-branch-scoping test needs a heavier fixture than its Context supports; everything else checked out cleanly against source.
MILL_REVIEW_END
