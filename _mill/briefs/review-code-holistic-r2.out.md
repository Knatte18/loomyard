MILL_REVIEW_BEGIN
# Review: fabric: clone-does-everything + subpath-in-weft + init dissolution — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-01
```

## Findings

### [BLOCKING] Fabric.Commit still reads the wired name-set from the per-pair weft base, not the repo-wide BoardDir
**Location:** `internal/fabricengine/commit.go:122` (`wiredNames, err := WiredNames(f.weftPath)`)
**Issue:** Batch 3 stops materializing a per-worktree `fabric.yaml` (only `<BoardDir>/_lyx/config/fabric.yaml` exists now), and batch 2 card 7 migrated six analogous name-set-reading call sites (`checkJunctionHealth`, `Reconcile`, `junctionRepointedDetail`, `PairInSync`, `Topology.Checkout`, `Topology.Remove`) to `RepoWiredNames`/`hubgeometry.BoardDir`. `Fabric.Commit`'s classify-and-dispatch step was not migrated: `f.weftPath` is the per-pair weft worktree root (set via `New(warpPath, weftPath)`, e.g. `weft_verbs.go`'s `fabricengine.New(l.WorktreeRoot, l.WeftWorktree())`), so for any pair created under the new clone-does-everything flow, `WiredNames(f.weftPath)` will fail with "not initialized here; run \"lyx fabric reconcile\"" every time `Fabric.Commit` is called. Currently masked because (a) no production caller invokes `Fabric.Commit` today (board's `Sync` uses `CommitWeftAt`'s wildcard-stage path instead) and (b) every integration test for it (`commit_integration_test.go`'s `seedFabricConfig`) seeds a per-pair `fabric.yaml` directly, hiding the gap. `doc.go` documents `Fabric.Commit` extensively as shipped, exported public API, so this is a real latent break in the module's own contract, not dead code that can be ignored.
**Fix:** Change `commit.go:122` to resolve the name-set the same way the six migrated sites do (e.g. resolve a `*hubgeometry.Layout` for `f.warpPath` and call `RepoWiredNames(l)`, or thread `hubgeometry.BoardDir(hub)` into `Fabric`), and update `commit_integration_test.go`'s `seedFabricConfig` to seed the repo-wide base instead of/in addition to the per-pair one.

### [BLOCKING] docs/overview.md's fabric module bullet omits the new `unwire` verb
**Location:** `docs/overview.md:172`
**Issue:** The fabric module bullet's "CLI surface is `lyx fabric clone|add|list|remove|checkout|pairs|reconcile|prune|cleanup|status|commit|push|pull|sync`" still lists the pre-batch-6, 14-verb surface; batch 6 registered a 15th verb, `unwire` (confirmed live in `internal/fabriccli/fabric.go`, `cmd/lyx/helptree_test.go`, and `SANDBOX-FABRIC-SUITE.md`'s F0). Card 33 edited this exact file/section (removing the `init` bullet, sweeping the `lyx init` cross-reference) but did not update this verb list, leaving the module's own doc.md table stale — a direct miss against CLAUDE.md's "a task ... changing observable CLI behavior ... must update `docs/overview.md`, if the module table ... changes" rule.
**Fix:** Append `|unwire` to the CLI-surface list in `docs/overview.md:172`.

### [NIT] Stale `lyx init --undo` reference in gitignore.go doc comment
**Location:** `internal/gitignore/gitignore.go:105`
**Issue:** `Remove`'s doc comment still says "It exists so that `lyx init --undo` can revert only the entries it originally added via Ensure" — `lyx init --undo` was deleted in batch 6 and replaced by `lyx fabric unwire`. This file was not in card 28's sweep list, so it was missed. Same category as the three prior-round non-blocking stale-reference findings.
**Fix:** Reword to name `lyx fabric unwire` instead of the deleted `lyx init --undo`.

### [NIT] Stale-removal test's "Add wires nothing" premise is invalidated by batch 5's eager wiring
**Location:** `internal/fabricengine/reconcile_stale_removal_test.go:74-98` (`TestReconcile_AddsMissingRemovesStaleNoOpsCorrect`)
**Issue:** The test's comment/setup assumes "Add itself wires nothing, so `_pattern` is genuinely missing on disk, matching the add-missing shape." Since `newFabricFixture` seeds the repo-wide `fabric.yaml` before `topology.Add` runs, and batch 5 (card 20) makes `Add` eagerly wire `_lyx`/`_pattern` via `RepoWiredNames`, `_pattern` is already wired by `Add` itself before the test's own `WireJunctions(hostLayout, slug, []string{"_lyx", "_extra"})` call — so the "missing `_pattern` junction not added by Reconcile" assertion passes without Reconcile's add-missing branch ever actually adding it. Add-missing behavior is still genuinely exercised elsewhere (`TestReconcile_ConvergesAllWorktreesToRepoWidePathspec`'s pathspec-widening case), so this is a test-quality/comment-accuracy gap, not a coverage hole.
**Fix:** Either seed the fixture's repo-wide config with only `_lyx` before `Add`, or drop the stale "Add itself wires nothing" claim from the comment and rely on the widening test for add-missing proof.

## Verdict

REQUEST_CHANGES
Fabric.Commit's unmigrated per-pair name-set read and a stale fabric verb list in docs/overview.md must be fixed.
MILL_REVIEW_END
