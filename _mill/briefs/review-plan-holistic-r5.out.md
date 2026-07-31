MILL_REVIEW_BEGIN
# Review: fabric: fold snapshot-tracking into the Warp-SHA trailer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-07-31
```

## Findings

### [BLOCKING] Card 20 Context omits snapshot.go, whose function it asserts against
**Location:** batch 04-empty-commit-rule.md / Card 20 (Pin the correspondence overwrite)
**Issue:** Requirements assert `SnapshotWarpSHA` returns the dangling SHA raw with a nil error, but `internal/fabricengine/snapshot.go` (where `SnapshotWarpSHA` is defined, per batch 3 Card 11) is absent from both Context and Edits — every sibling card (11-14, 16, 18, 19, 21, 22, 23) that touches this function lists snapshot.go, only Card 20 omits it.
**Fix:** Add `internal/fabricengine/snapshot.go` to Card 20's Context list.

### [NIT] Batches 3-4 verify scope omits cmd/lyx despite editing fabricengine test files
**Location:** batches 03-snapshot-reader.md and 04-empty-commit-rule.md (Batch Tests sections)
**Issue:** Batch 1's own rationale is that `cmd/lyx`'s module-wide tierpurity/hermetic-env guards "can fire on a test-file edit made in internal/gitrepo," so batch 1/2 include `./cmd/lyx/...` in verify; batches 3 and 4 add/edit fabricengine test files (index_test.go, snapshot_integration_test.go, commit_integration_test.go) under the identical risk but their verify commands never run `./cmd/lyx/...` — a mistake there is only caught at batch 5.
**Fix:** Add `./cmd/lyx/...` to batches 3 and 4's verify commands, or note explicitly why the identical batch-1 rationale doesn't apply here.

### [NIT] Card 12's negative guidance cites a file outside Context and outside the plan manifest
**Location:** batch 03-snapshot-reader.md / Card 12 (Integration coverage for the reader)
**Issue:** The "do not use Topology.Checkout" guidance names `newFabricFixture`/`reconcile_stale_registration_test.go` (package fabricengine_test), a file neither in Card 12's Context nor in this review's file manifest at all — harmless since the explanation is self-contained (no positive instruction requires opening it), but technically a Context-completeness gap.
**Fix:** Either add the file to Context or rephrase the guidance without naming the specific unreachable file/fixture.

## Verdict

REQUEST_CHANGES
One card-level context-completeness gap (Card 20) blocks; two NITs are minor batch-verify/context polish.
MILL_REVIEW_END
