MILL_REVIEW_BEGIN
# Review: fabric: clone-does-everything + subpath-in-weft + init dissolution — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewer_self_id: claude-sonnet-4.5 (Sonnet 5 per system context)
reviewed_file: plan/
date: 2026-08-01
```

## Findings

### [BLOCKING] Batch 4 is missing a dependency on batch 2
**Location:** `00-overview.md` Batch Index (`batch: 4`, `depends-on: [1, 3]`); `04-clone-does-everything.md`
**Issue:** Batch 3 stops `configsync.ReconcileAll` from materializing a per-pair `fabric.yaml` (card 11), and batch 4's clone flow only writes `fabric.yaml` at the repo-wide `BoardDir` (card 16), never at the per-pair weft base. But `checkJunctionHealth`/`Reconcile`'s inline load (reconcile.go:161/341), `PairInSync` (drift.go:92), `Topology.Checkout` (checkout.go:156), and `Topology.Remove` (remove.go:104) still read fabric.yaml from the per-pair weft base until batch 2 (card 7) migrates them to `RepoWiredNames`/`BoardDir`. If a DAG-driven scheduler lands batch 4 before batch 2 (both are only gated on batch 1/3, with no edge between them), any hub created by the new `lyx fabric clone` has junction-health/checkout/remove/pairs broken package-wide — exactly the regression batch 2's card 7 itself warns about ("otherwise `lyx fabric checkout` hard-fails and rolls back on every worktree"; "would permanently report 'junction check unavailable' and block loom preflight"). This is undetected by either batch's own `verify:` because batch 4's new tests never run `reconcile`/`checkout`/`remove` post-clone, and batch 2's/pre-existing fixtures still hand-seed the per-pair location directly via `lyxtest.SeedConfig`, bypassing the real clone-driven materialization gap.
**Fix:** Add `2` to batch 4's `depends-on` (mirroring batch 5, which already depends on `[1, 2]` for the analogous reason), or explicitly justify in batch 4's scope why the intermediate broken state is acceptable.

### [BLOCKING] Card 23's Context omits fabric.go, which its Requirements reference
**Location:** Batch 6, Card 23 (`06-init-dissolution-and-unwire.md`)
**Issue:** Card 23's Requirements call `New(l.WorktreeRoot, l.WeftWorktree())`, `EnvSyncOptions()`, and `ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName})` — all defined in `internal/fabricengine/fabric.go` (`New` at fabric.go:57, `EnvSyncOptions` at fabric.go:95, `ScopedPathspec` at fabric.go:105, plus the `Fabric`/`SyncOptions` types) — but `fabric.go` is not listed in Card 23's `Context:` (which lists `initengine/undo.go`, `junction.go`, `weftwiring.go`, `weftgit.go`, `reconcile.go`, `gitignore.go`, `hubgeometry.go`). Per the Context completeness rule, the implementer may only read files in `Context:`. `undo.go` (which is in Context) does show call-site usage of these three symbols with matching arguments, which mitigates but does not fully substitute for the actual declarations (return types, `SyncOptions` field shape).
**Fix:** Add `internal/fabricengine/fabric.go` to Card 23's `Context:` list.

### [NIT] Stale-removal in Reconcile leaves Action stale when only a junction was removed
**Location:** Batch 2, Card 9 (`02-reconcile-declarative-convergence.md`)
**Issue:** When a pair is otherwise `ReconcileActionAlreadyHealthy` but has a stale on-disk junction absent from the repo-wide `pathspec`, Card 9 only appends the removal outcome to `pr.Detail`, never updates `pr.Action`. The JSON `action` field would still read `"already_healthy"` even though a junction was actually deleted — misleading for any consumer keying off `Action` rather than parsing `Detail` text.
**Fix:** Either add a `ReconcileActionStaleRemoved` (or similar) action value for this outcome, or explicitly note in the card that `Action` intentionally stays unchanged and only `Detail` reflects the removal.

### [NIT] Card 30's Context omits anchor.go for the `.fabric-anchor` filename
**Location:** Batch 6, Card 30 (`06-init-dissolution-and-unwire.md`)
**Issue:** Card 30 asserts `<BoardDir>/.fabric-anchor` still exists after `Unwire`, but `hubgeometry.FabricAnchorName` (the constant naming that file, added in batch 1's `anchor.go`) is not in Card 30's `Context:` (`unwire.go`, `reconcile.go`, `junction.go`, `hubgeometry.go`, `lyxtest.go`). An implementer following Context literally could hardcode the string `.fabric-anchor` instead of using the exported constant, inconsistent with Card 4's own anchor tests.
**Fix:** Add `internal/hubgeometry/anchor.go` to Card 30's `Context:`.

## Verdict

REQUEST_CHANGES
Two BLOCKING gaps (batch-4/batch-2 ordering, Card 23 Context omission) plus two NITs; otherwise exceptionally well-grounded.
MILL_REVIEW_END
