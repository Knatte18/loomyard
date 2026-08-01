MILL_REVIEW_BEGIN
# Review: fabric: audit and migrate all remaining direct git mutations onto Fabric — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-01
```

## Findings

No findings. The implementation matches all four batches and their shared decisions precisely.

Verified end-to-end:

- Batch 1: `warpforward.go` defines exactly the four one-line delegations (`CheckoutDetached`/`RestoreBranch`/`CurrentBranch`/`ResetHard`), no `Warp`/`Weft` token in any signature, doc comments cover the required content. `fabric.go`'s package and struct docs are reconciled with the new carve-out rule, preserving the existing Commit/SyncWeft/RevertWithWeft/Pull description rather than replacing it. `fabric-unified-view.md`'s Scope-boundary section carries the required one-line addendum, forward-pointing to CONSTRAINTS.md. `warpforward_integration_test.go` is `//go:build integration`, package `fabricengine_test`, reuses `newFabricFixture`/`currentBranchOf`, and covers all four required cases (round-trip, invalid-ref error, discard-both-commit-and-worktree-change, detached-HEAD error) under `TestFabricWarp*` names matching the batch verify's `-run` filter.
- Batch 2: `WarpBisector` defined in `integration.go` with the exact three-method signature; `bisect`/`checkoutAndVerify`/`BisectAndEscalate` retyped off `*gitrepo.Repo`; the `gitrepo` import removed from `integration.go` (confirmed no other use). `runlevel.go` adds `RunDeps.Bisector`, constructs `fabricengine.New(deps.Layout.WorktreeRoot, deps.Layout.WeftWorktree())` inline when nil, swaps the `gitrepo`/`fabricengine` imports correctly. `runlevel_test.go`'s `newRunFixture` injects `Bisector: gitrepo.New(worktree)`; `integration_test.go`'s nil-bisector doc-comment updated to "WarpBisector" wording, no import added (matches the card's "only edit" constraint).
- Batch 3: `WarpResetter` defined in `chain.go` with exactly one method; `RestartChain` retyped to drop the `worktree string` param and call `resetter.ResetHard(startSHA)`. `gitquery.go`'s `ResetHard` deleted, package doc trimmed, `HeadSHA`/`ChangedFiles`/`Dirty` and the `gitexec` import intact. `gitquery_test.go`'s `TestResetHard` removed, doc comment updated, shared helpers (`newScratchRepo`/`mustGit`/`commitFile`) intact. `spawn.go` adds `SpawnDeps.Resetter`, constructs `fabricengine.New(...)` inline at the `RestartChain` call site with `return nil, err` on failure, matching `*SpawnResult, error`'s two-return shape. `chain_test.go`/`spawn_test.go` inject real `gitrepo.New(worktree)` handles at every relevant call/fixture site, preserving all existing assertions.
- Batch 4: `rawgitmutation_test.go` is untagged (Tier 1), scans exactly `internal/websterengine`/`internal/builderengine`, bans `gitrepo.New(`/`gitexec.RunGit(` as raw substrings, allowlists exactly `gitwrap.go`/`gitquery.go` with the documented reasons, has the vacuous-scan floor (4), and skips cleanly with no go toolchain. `tierpurity_test.go`'s `allowedSpawners` gained the required `cmd/lyx/rawgitmutation_test.go` entry. CONSTRAINTS.md's Fabric Git Invariant already reflects the closed gap and the new machine-check bullet.
- Cross-batch grep confirms zero raw `gitrepo.New(`/`gitexec.RunGit(` occurrences in websterengine/builderengine production (non-`_test.go`) source outside the two allowlisted files — the batch-4 guard will pass against the tree left by batches 2/3.
- No out-of-plan files found; every file in the manifest maps to a batch's Context/Edits/Creates list. No duplicated helper/interface logic between the two independently-defined `WarpBisector`/`WarpResetter` seams — this duplication is the plan's own explicit, rationale-backed decision (consumer-side interface per package), not an oversight.

## Verdict

APPROVE
All four batches align with the plan, shared decisions, and CONSTRAINTS.md; no blocking or nit issues found.
MILL_REVIEW_END
