# `fabric merge` — fixer report (round `opus-medium-r1`)

Companion to `_mill/fabric-merge-review-opus-medium-r1.md`.
Every finding recorded in that review is fixed here unless the deferred section below says
otherwise, with a per-finding commit on branch `fabric-merge-crucible-hardening`. Nothing is pushed.

Gate commands used throughout (tags named explicitly, per the campaign rule):

```sh
go build ./...
go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...
go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...   # tag: none (hermetic)
go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...   # tag: integration
./deploy-dev   # re-run before EVERY live re-verification
```

All new regression tests live in `internal/fabricengine/mergecrucible_integration_test.go`
(`//go:build integration`), following the existing `*_integration_test.go` pattern and the hubforge
Fabric-Fixture invariant. They reuse `newMergePairFixture` (`mergein_integration_test.go`) and
`openFreshFabric` (`mergein_recovery_integration_test.go`) rather than growing a parallel fixture
set.

---

## F2 — BLOCKING — refuse a merge while either checkout has a detached HEAD

**Commit** `a20dc2ca`.

**Implemented**

- `internal/gitrepo/merge.go` — new `(*Repo).HeadDetached() (bool, error)`. `CurrentBranch()` could
  not serve: it collapses detachment into an error indistinguishable from a genuine read failure.
  Like `ResolveSHA` it is a go-git read of on-disk state, so it stays off the gitrepo Client
  Boundary Invariant's pinned CLI list, and its godoc says so.
- `internal/fabricengine/mergeerrors.go` — new closed-set member
  `mergeReasonDetachedHead = "checkout is not on a branch"` (side-free, path-free, order-free).
- `internal/fabricengine/mergeguards.go` — new `detachedHeadReason(f)`, evaluating **both** sides
  unconditionally before combining, exactly as `pairDirtyReason` does, so the aggregated reason
  never reveals which side was detached.
- `internal/fabricengine/merge.go` — wired into both `MergeIn`'s and `Merge`'s guard aggregation,
  immediately after the dirty guard, still inside the strictly read-only guard stage.
- `internal/gitrepo/doc.go` — `HeadDetached` added to the admitted merge-surface list.
- `internal/fabricengine/doc.go` — new "Both checkouts must be on a branch" paragraph in
  "# The merge surface", naming why the asymmetry (orphan on one side, final-and-unabortable on the
  other) makes this a precondition and not a curiosity, and pointing at
  `internal/websterengine/integration.go` as the production driver of `CheckoutDetached`.
- `internal/fabricengine/mergevocab_test.go` — pinned closed-set list extended in the same commit,
  per the guards decision's same-commit rule.

**Deliberately NOT changed**: `MergeContinue`/`MergeAbort` are left unguarded against detachment.
They are the recovery half of the quartet; refusing there would strand a pair that got detached
mid-resolution, which is the opposite of what the guard is for.

**Test** `TestMergeCrucible_DetachedHeadRefused` — table-driven over `WarpDetached` / `WeftDetached`,
asserting the sole guard reason, both HEADs unchanged, and `MergeInProgress() == false` (a guard
refusal must write no record).

**False-green proof**: replaced the `reasons = append(reasons, detachedReasons...)` wiring in
`merge.go` with a discard (`_, err = detachedHeadReason(f)`), re-ran the test — both subtests failed
at the intended assertion:

```
mergecrucible_integration_test.go:67: MergeIn(feature) error = <nil> (<nil>); want *fabricengine.MergeGuardError
```

Restored the file from a byte-copy backup, confirmed the diff was empty, re-ran green.

**Gates**: `go build` / `go vet` green; hermetic `go test` green (fabricengine, fabriccli, gitrepo,
cmd/lyx); `-tags integration` green (fabricengine 23.7 s, fabriccli 2.2 s, gitrepo 1.5 s).

**Live re-verification** (after `./deploy-dev`, fresh hub `v_detach`):

```
$ git checkout --detach HEAD && lyx fabric merge-in task1
{"error":"fabricengine: merge preconditions failed: checkout is not on a branch",
 "mutations":[],"ok":false,"partial":false}
warp HEAD unchanged (4083d46 == main)
$ git checkout main && lyx fabric merge-in task1
{"already_up_to_date":false,"committed":true, …}     # still works on a branch
```

---

## F1 — BLOCKING — `MergeContinue` must refuse an attempt that never reached both sides

**Commit** `51426758`.

**Implemented**

- `internal/fabricengine/mergeerrors.go` — new closed-set member
  `mergeReasonAttemptIncomplete = "merge attempt did not reach both sides"`.
- `internal/fabricengine/mergelifecycle.go` — new `mergeAttemptIncompleteReason(st)`, true when
  either recorded outcome is empty (the shape a crash before the first `MergeStart`, or between the
  two, leaves behind — `merge.go` persists a side's outcome only once that side's `MergeStart` has
  returned). `MergeContinue` now aggregates it with `mergeReasonUnresolvedConflicts` into a single
  `*MergeGuardError`, so which precondition failed never discloses evaluation order, and refuses
  **before** the write lock and before anything lands.
- `concludeMergeSides`' godoc now states the precondition explicitly: every caller must have
  established both recorded outcomes are non-empty. `MergeIn`/`Merge` satisfy it by construction;
  `MergeContinue` enforces it.
- `internal/fabricengine/doc.go` — new "Not every crash is continuable, and the record says which"
  paragraph, which also supplies the qualification the existing crash-recovery paragraph needed.
- `internal/fabricengine/mergevocab_test.go` — pinned closed-set list extended, same commit.

**Why refuse rather than resume the un-started side**: teaching `MergeContinue` to run the missing
`MergeStart` itself would let a *new* conflict appear during a continue, which the verb's contract
does not admit, and would make `--continue` a mutation entry point rather than a conclude. Refusal
plus `MergeAbort` covers the whole window with no new failure mode, and `MergeAbort` always works
because it restores from the recorded pre-merge SHAs rather than from how far the attempt got.

**Test** `TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides` — reconstructs the crash
state byte-for-byte (real `git merge --no-commit` on the warp side, record saved with an empty weft
outcome via `SaveMergeStateForTest`), resumes through a **fresh** `Fabric` handle, and asserts the
sole guard reason, both HEADs unchanged, and then that `MergeAbort` still recovers the same record
and clears it. The fixture commits a divergent commit on each target branch first, so the
reconstructed attempt stages rather than fast-forwards — the staged shape is the crash window under
test, and the first draft of this test was wrong precisely because it fast-forwarded.

**False-green proof**: replaced the aggregation line with a discard
(`_ = mergeAttemptIncompleteReason(st)`), re-ran — the test failed at the intended assertion with
the original defect verbatim:

```
MergeContinue on an attempt that never reached both sides
  error = fabricengine: merge conclude did not finish; run MergeContinue again
  (*fabricengine.ErrMergeIncomplete); want *fabricengine.MergeGuardError
```

Restored from backup, `diff -q` reported the files identical, re-ran green.

**Gates**: `go build` / `go vet` green; hermetic `go test` green; `-tags integration` green
(fabricengine 40.1 s, fabriccli 3.0 s, gitrepo 1.9 s).

**Live re-verification** (after `./deploy-dev`, fresh hub `v_f1`, crash state reconstructed by hand):

```
$ lyx fabric merge --continue
{"error":"fabricengine: merge preconditions failed: merge attempt did not reach both sides",
 "mutations":[],"ok":false,"partial":false}
warp HEAD 210f4b3 — unchanged, nothing landed
$ lyx fabric merge --abort
{"committed":false,"mutations":[worktree_reset ×2],"ok":true,"partial":false}
warp 210f4b3, weft 640016e, worktrees clean, record gone
```
