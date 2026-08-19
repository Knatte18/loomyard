# Batch: sibling-verb guards and vocabulary assertions

```yaml
task: 'fabric: merge-conflict primitive'
batch: sibling-verb guards and vocabulary assertions
number: 5
cards: 3
verify: go test ./internal/fabricengine/ ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration -run 'Merge|Commit|Pull|Checkout|Remove|Cleanup' ./internal/fabricengine/
depends-on: [4]
```

## Batch Scope

Closes the resolution window against corruption: every sibling mutating verb whose write would corrupt or be corrupted by a live merge refuses with the single typed, side-free `ErrMergeInProgress` while a merge record exists — `Commit` (which also refuses foreign git merge state, pre-empting git's raw "cannot do a partial commit during a merge"), `Pull`, `Checkout`, and `Remove` for the pair itself.
`Cleanup` needs no new guard — it deletes only orphaned weft branches and already skips live pairs — so card 14 pins that existing behaviour rather than adding one.
`PushWeft`, the push half of `sync`, and every read-only verb stay deliberately unguarded.
Also lands the explicit vocabulary assertion the enforcement walk cannot provide (it permits side tokens inside the owner set).
Nothing downstream consumes new interfaces;
batch 6 depends on this batch only for ordering.

## Cards

### Card 13: merge-in-progress refusals on sibling mutating verbs

- **Context:**
  - `internal/fabricengine/mergeerrors.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/cleanup.go`
- **Edits:**
  - `internal/fabricengine/mergestate.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/remove.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `mergestate.go` (the record code's natural home):
  `mergeBlocksMutation(warpPath, weftPath string) (bool, error)` — constructs the pair's state probe from explicit paths (`newPaired` internally) and reports whether the merge record exists;
  an unreadable weft gitdir (e.g. a half-torn-down pair) reports `false` rather than blocking, since no recorded merge can exist without a readable weft gitdir.

  Wire the refusals, each returning `&ErrMergeInProgress{}` before any mutation:
  - `Fabric.Commit` (`commit.go`): at the top of `Commit`, before classification and before the lock — refuse when the record exists **and equally** when `foreignMergeStatePresent()` reports git-level merge state with no record (the human's plain-git half-merge case), so the caller never receives git's raw "cannot do a partial commit during a merge".
  - `Fabric.Pull` (`pull.go`): after the `SkipGit` early return, before the weft pull — a pull hard-resets and re-anchors, which would discard the in-progress merge.
  - `Topology.Checkout` (`checkout.go`): before the dirty-weft pre-flight — probe via `mergeBlocksMutation(l.WorktreePath(), WeftWorktree(l))`;
    a coordinated branch switch out of a half-merged pair is refused.
  - `Topology.Remove` (`remove.go`): before any teardown for the named slug — probe the pair's paths;
    `force` does not override (force answers dirtiness only, the gate's own rule).
  - `Topology.Cleanup` (`cleanup.go`): **no new guard, and `cleanup.go` is not edited.**
    `Cleanup` deletes orphaned weft *branches*, never worktrees, and a mid-merge pair is by definition live — a warp worktree sits on the paired warp branch, so `cleanup.go`'s `liveWarpBranches[warpBranch]` arm `continue`s before the branch ever becomes a `CleanupBranchEntry`;
    a checked-out weft branch is additionally protected unconditionally further down.
    `CleanupBranchEntry` also has no field that could carry a refusal message (`Branch`/`Deleted`/`Protected`/`Error`, with `Error` documented as deletion-failure-only), so the "protection entry carrying `ErrMergeInProgress`'s text" this card previously described was unimplementable.
    Card 14 asserts the existing behaviour instead of a new guard.

  Record-only checks for `Pull`/`Checkout`/`Remove`;
  the foreign-state half applies to `Commit` alone, per the lock decision's consequence table.
  Deliberately untouched: `PushWeft`, the push half of `sync`, `Status`, `Diff`, `List`, `pairs`, and every other read-only verb.
- **Commit:** `feat(fabricengine): refuse sibling mutations while a merge is in progress`

### Card 14: sibling-disposition integration tests

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/mergein_integration_test.go`
  - `internal/fabricengine/export_test.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergesiblings_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `//go:build integration`, `package fabricengine_test`, reusing `newMergePairFixture`;
  test names containing `Merge`.
  Establish a live recorded merge by driving a real conflicted `MergeIn`, then assert the full disposition table:
  - `Fabric.Commit` → `*ErrMergeInProgress`, nothing mutated (both HEADs and worktree states unchanged, empty mutation record on the returned result).
  - `Fabric.Commit` during **foreign** git merge state with no record (plain-git conflicted merge in the warp checkout, no `MergeIn` involved) → the same typed refusal, never git's raw message.
  - `Fabric.Pull` → typed refusal, nothing mutated.
  - `Topology.Checkout` on the pair → typed refusal, branches unchanged on both sides.
  - `Topology.Remove` of the pair (with and without `force`) → typed refusal, worktrees intact.
  - `Topology.Cleanup` (both dry-run and `apply`, with and without `force`) → succeeds, and the mid-merge pair's weft branch is never deleted and never appears as a deleted entry: it is a live pair, so `cleanup.go` skips it before entry construction.
    Assert the pair's worktrees and both branches are intact afterwards, and that the merge record still exists — pinning that no new guard is needed here.
  - `PushWeft` succeeds unchanged during the live record (pushing already-committed history is unaffected).
  - Read-only verbs succeed during the live record: `Fabric.Status`, `Fabric.Diff` (against a pre-merge SHA), `Topology.List`, `Topology.Status` (pairs) — exactly what an operator inspecting a stuck merge needs.
  - After `MergeAbort`, every guarded verb works again (spot-check `Commit`).
- **Commit:** `test(fabricengine): sibling-verb dispositions during a live merge`

### Card 15: explicit side-free vocabulary assertion for the merge surface

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/lyxcwd/enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergevocab_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Untagged Tier 1 test file, `package fabricengine` (in-package, to reach the unexported closed reason set), test names containing `Merge`.
  The repo's `TestEnforcement_FabricVocabulary` walk deliberately permits warp/weft tokens inside the owner set, so it can never catch a leak on this surface — this test is the explicit assertion the discussion's Testing section requires:
  - Reflect over `MergeResult` and `MergeOptions`: every exported field name and every JSON tag contains no `warp`/`weft` token and no fabric-sense `host` phrase (case-insensitive).
  - Instantiate every named merge error (`MergeGuardError` with the full closed reason set, `ErrMergeInRequired{Source: "x"}`, `ErrForeignMergeState`, `ErrNoMergeInProgress`, `ErrMergeIncomplete`, `ErrUnmergeableState`, `ErrMergeInProgress`) and assert each `Error()` output is side-free by the same token check, and that `ErrMergeInRequired`'s message does not contain its `Source` value.
  - Assert every member of the closed guard-reason set is side-free, path-free (no `/` or `\`), and matches the pinned literal list verbatim — so adding a member without updating this test fails loudly, per the guards decision's same-commit rule.
- **Commit:** `test(fabricengine): side-free vocabulary assertions for the merge surface`

## Batch Tests

`verify` widens the integration `-run` pattern to `Merge|Commit|Pull|Checkout|Remove|Cleanup` because this batch edits five existing verbs — their existing regression suites (`commit_*`, `pull_*`, `checkout_*`, `remove_*`, `cleanup_*` test files) must still pass alongside the new disposition tests.
The untagged run picks up `mergevocab_test.go` and the `cmd/lyx` guard walk.
