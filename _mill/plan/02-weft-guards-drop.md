# Batch: weft-guards-drop

```yaml
task: "Add a local-only file category to weft"
batch: "weft-guards-drop"
number: 2
cards: 6
verify: go build ./cmd/lyx && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [1]
```

## Batch Scope

Batch 1 stopped the weft participating in a merge;
this batch strips its power to *block* one, and stops a merge abort resetting it.
Four guards drop their weft arm — `pairDirtyReason`, `detachedHeadReason`, `syncedToUpstreamReason`, and `resolveMergeSources`' refusal arm — and `resetMergeSides` resets the warp checkout alone.
`resolveMergeSources` keeps its weft SHA *read*, because `mergeState.WeftSource` still needs a value;
what it loses is the power to append `mergeReasonNotFabricManaged` or `mergeReasonSourceNotFound` on the weft's behalf.
Two shipped tests assert today's diverged-weft refusal and are inverted here rather than deleted.

Batch-local decision beyond `## Shared Decisions`: two unexported signatures change.
`resolveMergeSources` drops its `l *lyxcwd.Location` parameter, which becomes dead once `weftBranchExists` is no longer called — and that in turn makes `Merge`'s own `lyxcwd.ResolveWorktree` call dead, so it goes too.
`resetMergeSides` drops its `weftSHA` parameter for the same reason, which ripples to its four call sites and to `export_test.go`'s test shim.
The `mergeReasonNotFabricManaged` constant itself stays: `mergevocab_test.go` pins its string, and an unreferenced package-level constant is legal Go.

## Cards

### Card 8: pairDirtyReason and detachedHeadReason evaluate the warp side only

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/mergeerrors.go`
- **Edits:**
  - `internal/fabricengine/mergeguards.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/mergeguards.go`:
  (a) narrow `pairDirtyReason` to the warp side — delete the `worktreeDirty(scopeTracked, f.weftPath)` call and its `weftDirty` variable, and return `mergeReasonWorktreeDirty` on `warpDirty` alone;
  (b) narrow `detachedHeadReason` the same way — delete the `f.weft.HeadDetached()` call and its `weftDetached` variable, returning `mergeReasonDetachedHead` on `warpDetached` alone.
  Rewrite both doc comments: each currently promises that both sides are evaluated unconditionally so the aggregated reason never discloses which side failed, and that promise is replaced by the reason this batch exists — a non-participant's state cannot affect a warp-only merge's correctness, so checking it could only refuse a merge that would have been right.
  Keep both function names, both signatures, and both returned reason constants exactly as written.
- **Commit:** `refactor(fabricengine): dirty and detached guards evaluate the warp side only`

### Card 9: syncedToUpstreamReason evaluates the warp side only

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergeerrors.go`
- **Edits:**
  - `internal/fabricengine/mergeguards.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/mergeguards.go`, narrow `syncedToUpstreamReason` to the warp side: delete the `sideNotSyncedToUpstream(f.weft, f.weftPath)` call and its `weftNotSynced` variable, returning `mergeReasonNotSynced` on `warpNotSynced` alone.
  Keep `sideNotSyncedToUpstream` itself and `upstreamSHAAt` intact — both keep warp-side callers.
  Rewrite `syncedToUpstreamReason`'s doc comment: keep its pre-fetch-fast-path explanation, which is unchanged and still load-bearing, and replace the both-sides-evaluated paragraph with the reason the weft arm is gone.
  State the decisive case explicitly, because it is what makes this card load-bearing rather than tidying: the per-transition status push warns and continues on a rejected push, which makes a locally-diverged weft a routine, expected state, so a retained weft arm here would refuse every subsequent landing with `mergeReasonNotSynced`.
  Do not touch `sideNotSyncedToUpstream`'s own body or doc comment.
- **Commit:** `refactor(fabricengine): the not-synced guard evaluates the warp side only`

### Card 10: resolveMergeSources keeps the weft SHA read and loses its refusal

- **Context:**
  - `internal/fabricengine/mergeerrors.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/mergevocab_test.go`
- **Edits:**
  - `internal/fabricengine/mergeguards.go`
  - `internal/fabricengine/merge.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/mergeguards.go`, change `resolveMergeSources`' signature from `resolveMergeSources(f *Fabric, l *lyxcwd.Location, source string) (mergeSources, []string)` to `resolveMergeSources(f *Fabric, source string) (mergeSources, []string)`.
  Inside it, keep the weft `Fetch()`, the two `f.weft.ResolveSHA` calls, and the `pickMergeSourceSHA` call that produces `weftSHA` — `mergeState.WeftSource` still records it.
  Delete the `weftManaged` computation (`weftBranchExists(l, weftBranch) || weftRemoteErr == nil`), the `mergeReasonNotFabricManaged` append, and the `weftManaged && !weftFound` append of `mergeReasonSourceNotFound`.
  Discard `weftFound` via the blank identifier so the warp arm's `mergeReasonSourceNotFound` append remains the function's only reason.
  Rewrite the doc comment: the long asymmetric-gating explanation describes behaviour this card removes, so replace it with a statement that the warp arm alone can refuse, that an unresolvable weft counterpart now leaves `WeftSource` empty and merges anyway, and that `mergeReasonNotFabricManaged` is no longer reachable from this function while its constant is retained for `mergevocab_test.go`.
  Remove the now-unused `lyxcwd` import from `mergeguards.go` if no other declaration in that file references it.
  In `internal/fabricengine/merge.go`, update both call sites to `resolveMergeSources(f, source)`;
  in `Fabric.Merge`, additionally delete the now-unused `l` variable and its `lyxcwd.ResolveWorktree(f.warpPath)` call together with the error branch that follows it, keeping `Fabric.MergeIn`'s `l` (still consumed by `resolveMergeGeometry`) and keeping `merge.go`'s `lyxcwd` import.
- **Commit:** `refactor(fabricengine): the weft counterpart no longer refuses a merge`

### Card 11: resetMergeSides resets the warp checkout only

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/mergestate.go`
- **Edits:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/export_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/destroy.go`, change `resetMergeSides`' signature from `(f *Fabric) resetMergeSides(rec *Mutations, warpSHA, weftSHA string) error` to `(f *Fabric) resetMergeSides(rec *Mutations, warpSHA string) error`, delete the whole `weftReq` `pathRequest` literal and its `resetHardTo(rec, weftReq, f.weft, weftSHA)` call, and return the warp `resetHardTo` call's error directly.
  Rewrite its doc comment: it currently explains the two-sided reset, the first-side-failure-aborts ordering, and why the weft request declares `ownedWeftCheckout`;
  replace those with the `abort-does-not-reset-weft` reasoning — the weft was never a merge participant so an abort has nothing to restore there, and with the weft advancing per transition a reset would discard already-pushed status history and leave the local weft behind its own origin, breaking the next push too.
  Delete the `ownedWeftCheckout` constructor and its doc comment as well: `resetMergeSides`' `weftReq` is its sole caller across the whole tree, and `export_test.go` does not re-export it, so it is dead once that request is gone.
  Correct the sentence at `internal/fabricengine/destroy.go:520` that describes `ownedWarpCheckout` and `ownedWeftCheckout` sharing one repo-agnostic predicate, so it names the surviving constructor alone.
  Update all four call sites to pass `st.WarpStart` alone: `internal/fabricengine/merge.go`'s `MergeIn` unmappable-conflict path, `merge.go`'s `Merge` conflict self-abort, `merge.go`'s `selfAbortMergeAttempt`, and `internal/fabricengine/mergelifecycle.go`'s `MergeAbort`.
  In `internal/fabricengine/export_test.go`, narrow `ResetMergeSidesForTest` to `ResetMergeSidesForTest(f *Fabric, rec *Mutations, warpSHA string) error` and update its body.
  `mergeState.WeftStart` stays recorded and stays filled — it simply stops being a reset target.
- **Commit:** `refactor(fabricengine): a merge abort resets the warp checkout only`

### Card 12: invert the shipped diverged-weft refusal tests

- **Context:**
  - `internal/fabricengine/mergeguards.go`
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/merge_target_integration_test.go`
  - `internal/fabricengine/mergestate_integration_test.go`
  - `internal/fabricengine/mergecrucible_integration_test.go`
  - `internal/fabricengine/mergein_recovery_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/merge_target_integration_test.go`, rewrite `TestMerge_UnfetchedDivergedWeftRefuses` and `TestMerge_FetchedDivergedWeftRefuses` so each asserts that the merge now proceeds rather than refusing.
  Rename both to names stating the new property (for example `TestMerge_UnfetchedDivergedWeftDoesNotRefuse` and `TestMerge_FetchedDivergedWeftDoesNotRefuse`), keep each fixture's divergence setup and its precondition assertions unchanged, and replace the `assertMergeRefusedAsNotSynced` call with assertions that the returned error is nil, that `MergeResult.Committed` is true, that the target warp HEAD moved off `warpBefore`, and that the target weft HEAD still equals `weftBefore`.
  Rewrite both doc comments to state what the inverted test now pins: a weft diverged from its own upstream can no longer refuse a merge the warp alone completes, in either the pre-lock-guard or the post-fetch-sync layer.
  Keep `assertMergeRefusedAsNotSynced` and the warp-side `TestMerge_UnfetchedDivergedTargetRefuses`/`TestMerge_FetchedDivergedTargetRefuses` tests exactly as written — warp-side dirty, detached and not-synced still refuse, and those tests are the proof.
  In `internal/fabricengine/mergestate_integration_test.go`, update `wantWorktreeResetEntries` to expect exactly one `KindWorktreeReset` entry carrying the warp pre-merge SHA, drop its `wantWeftSHA` parameter, and update every caller and every surrounding assertion that reads the weft entry — including `TestMergeState_ResetMergeSides_PrimePairBothSidesAdmitted`, whose name and doc comment must be corrected to describe a warp-only reset.
  Update every `fabricengine.ResetMergeSidesForTest` call in that file to the narrowed signature.
  Extending beyond the plan's original scope (added during implementation, since the full-package `verify:` surfaces these too): `internal/fabricengine/mergecrucible_integration_test.go`'s `TestMergeCrucible_DetachedHeadRefused`'s `WeftDetached` table case pinned `detachedHeadReason`'s now-removed weft arm — invert it in place to assert `MergeIn` proceeds (no error, both HEADs move as an ordinary merge would) while leaving the `WarpDetached` case untouched.
  `internal/fabricengine/mergein_recovery_integration_test.go`'s `TestMergeIn_NotFabricManaged_NothingMutated` pinned `resolveMergeSources`' now-removed `mergeReasonNotFabricManaged` arm — invert it to assert `MergeIn` no longer refuses a source with no weft counterpart (the fixture's same-SHA `feature` branch make this the degenerate `AlreadyUpToDate` case, so nothing mutates either, keeping the original nothing-mutated assertions valid), renaming to state the new property.
  That same file's `TestMergeIn_DirtyPair_ByteIdenticalErrorEitherSide` pinned `pairDirtyReason`'s now-removed weft arm on its weft-dirty half — split it into the warp-dirty case (unchanged assertion: refuses with `"worktree dirty"`) and a weft-dirty case rewritten to assert `MergeIn` no longer refuses, renaming both to state what each half now pins rather than keeping the single byte-identical-either-side name that no longer holds.
- **Commit:** `test(fabricengine): a diverged weft no longer refuses a merge`

### Card 13: integration coverage for the dropped weft guards and the one-sided abort

- **Context:**
  - `internal/fabricengine/mergeguards.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/merge_target_integration_test.go`
  - `internal/fabricengine/mergein_integration_test.go`
  - `internal/fabricengine/mergein_recovery_integration_test.go`
  - `internal/fabricengine/mergeweftlocal_integration_test.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:**
  - `internal/fabricengine/weftguards_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/weftguards_integration_test.go` in `package fabricengine_test`, opening with the `//go:build integration` constraint and a file-header comment.
  Reuse the package's existing fixture helpers rather than adding new ones.
  Cover four scenarios, one test function each:
  (1) a weft worktree carrying uncommitted tracked changes no longer refuses a `Merge` the warp alone can complete, while a dirty *warp* worktree still refuses with `mergeReasonWorktreeDirty`;
  (2) a weft checkout on a detached HEAD no longer refuses, while a detached warp HEAD still refuses with `mergeReasonDetachedHead`;
  (3) a source branch whose weft counterpart exists on neither the local weft repo nor `origin` merges successfully, leaving `mergeState.WeftSource` empty and appending neither `mergeReasonNotFabricManaged` nor `mergeReasonSourceNotFound` — and, separately, that a source resolvable on no side still reports `mergeReasonSourceNotFound` from the warp arm;
  (4) `MergeAbort` against an attempt whose weft gained its own commits during the attempt window resets the warp to `WarpStart` and leaves the weft HEAD exactly where the abort found it, with those commits intact.
  In `internal/fabricengine/doc.go`, amend the merge-surface guard narrative around `internal/fabricengine/doc.go:1018-1046` so the detached-HEAD precondition, the guard aggregation description and the abort description all describe the warp side alone, and add one sentence recording that the weft has lost its power to block a merge as well as its participation in one.
- **Commit:** `test(fabricengine): pin the dropped weft guards and the warp-only abort`

## Batch Tests

`verify:` runs `go build ./cmd/lyx`, then the untagged tier over `./internal/fabricengine/...`, then the `integration` tier over the same package.

- The `integration` tier is chained as its own ` && ` invocation because cards 12 and 13 edit and create `//go:build integration` files;
  without the tag those files never compile and the batch's whole regression surface would be silently skipped.
- Card 12's inverted tests are this batch's primary proof: they were green asserting the opposite behaviour before it, so a batch that left the guards in place fails them.
- `mergevocab_test.go` is untagged and covered by the first invocation;
  it is the reason card 10 keeps the `mergeReasonNotFabricManaged` constant rather than deleting it.
- The scope stays one package: nothing outside `internal/fabricengine` is edited, and `pipeline.done_gate` runs the repo-wide sweep at the end of the run.
