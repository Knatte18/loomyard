# fabric merge — orchestrator pre-count (ROUND AGENTS MUST NOT READ THIS)

This file matches the `fabric-merge-review-*` filename pattern the per-round prompt declares off-limits.
It is the orchestrator's private ground truth, counted **before** round 1 was spawned, at `HEAD = 9115020a` (tree identical to `a2bf44e2` for every merge-surface file).

A round total **below** a number here is the truncation signal.
A total **above** it is the correct direction, and the round correcting the orchestrator is the round working.
Every count below names what its method cannot see.

## Baseline gates the orchestrator ran itself, before round 1 (all green)

| Gate | Command | Result |
|---|---|---|
| build | `go build ./...` | green |
| vet | `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` | green |
| hermetic | `go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` | green |
| integration (tag `integration`) | `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` | green — fabricengine 25.2s, fabriccli 2.1s, gitrepo 1.3s |

There are **zero** `//go:build smoke` files anywhere under `internal/fabricengine`, `internal/fabriccli`, `internal/gitrepo`.
The only `smoke` tag in the whole `cmd/lyx` tree is in `tierpurity_test.go`.
fabric's live tier is the `integration` tag, not `smoke` — a round reporting "ran the smoke tier" for fabric has run nothing.

## Count 1 — merge-surface entry points

- Exported `*Fabric` methods, whole package: **18**.
- Of those, merge-surface: **5** — `MergeIn`, `Merge`, `MergeContinue`, `MergeAbort`, `MergeInProgress`.
- Exported `*Topology` methods: **8** — `Add`, `Checkout`, `Cleanup`, `List`, `Prune`, `Reconcile`, `Remove`, `Status`.

*Blind spot:* the pattern is `^func (f \*Fabric) [A-Z]` / `^func (t \*Topology) [A-Z]`.
It cannot see a merge entry point reached through a package-level function (`fabricengine.Open`, `CoalescePushBothAt`, `PushWarpAt`), nor anything routed via the `*ForTest` export seam in `export_test.go`.

## Count 2 — merge-state record write/delete sites (non-test)

- `saveMergeState` call sites outside `mergestate.go`: **8** — `merge.go` 144, 153, 165, 344, 353, 365; `mergelifecycle.go` 43, 60.
- `deleteMergeState` call sites outside `mergestate.go`: **7** — `merge.go` 194, 220, 380, 404, 477; `mergelifecycle.go` 140, 178.
- `resetMergeSides` call sites: **4** — `merge.go` 191, 377, 474; `mergelifecycle.go` 175. Definition lives in `destroy.go:1196`, not in any `merge*.go` file.

*Blind spot:* a literal-identifier grep. It cannot see a save/delete reached through a helper that wraps it, and it counts the definition-adjacent doc-comment mentions in `mergestate.go` only because they were explicitly excluded by path — a round counting "mentions of `deleteMergeState`" rather than "call sites" will legitimately report a larger number (there are doc-comment mentions in `mergestate.go`, `mergelifecycle.go` and `merge.go` headers).

## Count 3 — post-record error returns that leave the record on disk

Manual read of every `return MergeResult{}, ...` that executes **after** the first successful `saveMergeState` and does **not** delete the record first.

| Function | Line numbers | Count |
|---|---|---|
| `MergeIn` | 154, 165, 177, 183, 191, 194, 206, 210, 214, 218, 220 | 11 |
| `Merge` | 353, 365, 377, 380, 389, 393, 397, 401, 404 | 9 |
| `MergeContinue` | 125(via `concludeMergeSides`), 131, 135, 137, 140 | 5 |
| `MergeAbort` | 175, 178 | 2 |
| **Total** | | **27** |

Of these, the subset where the record survives **after both sides already landed their conclude commit** — the shape where a subsequent `MergeAbort` would hard-reset away work that was actually committed:
`MergeIn` 210, 214, 218, 220; `Merge` 393, 397, 401, 404; `MergeContinue` 131, 135, 137, 140 — **12**.

Two of the 27 are **documented as deliberate**: `merge.go` 191 / 377 (reset-failure retention, `selfAbortMergeAttempt`'s own doc comment) and the `concludeMergeSides` failure path returning `*ErrMergeIncomplete` (`merge.go` 206 / 389).

*Blind spots, and they are large here:*
- This is a hand-read of two files, not a grep. It has no reproducible enumeration command behind it, so it is the number most likely to be wrong.
- It counts a `deleteMergeState` failure itself (194, 220, 380, 404, 140, 178) as "leaves the record" — arguably tautological; a round that excludes those will report **21** and be defensible.
- It does not model whether a leftover record is actually *harmful* at each site; several are the correct recovery state.
- It ignores `MergeStart` failing after git already wrote `MERGE_HEAD` but before `saveMergeState` returned.

Whichever total a round reports, the thing to check is that it **decomposes** the number, not that it matches.

## Count 4 — the closed vocabularies

- `mergeReason*` guard-reason constants in `mergeerrors.go`: **7** — `AlreadyInProgress`, `UnresolvedConflicts`, `NoMergeInProgress`, `WorktreeDirty`, `NotSynced`, `SourceNotFound`, `NotFabricManaged`.
- Exported typed errors in `mergeerrors.go`: **7** — `MergeGuardError`, `ErrMergeInRequired`, `ErrForeignMergeState`, `ErrNoMergeInProgress`, `ErrMergeIncomplete`, `ErrUnmergeableState`, `ErrMergeInProgress`.
- `mergeOutcome*` on-disk outcome strings in `mergestate.go`: **4** — `staged`, `conflicted`, `fast_forwarded`, `up_to_date`, mapping `gitrepo.MergeOutcome`'s **4** members.
- Note: `mergeReasonNoMergeInProgress` is **declared but never used** as a guard reason — the no-merge case returns the typed `*ErrNoMergeInProgress` instead. That is a real dangling-constant observation, not a count artifact. Do not volunteer it to a round; see whether one finds it.

*Blind spot:* `grep -c "^\tmergeReason"` counts declarations in the one const block only. A reason string constructed inline elsewhere, or a member added in a second block, is invisible to it.

## Count 5 — git invocations in the new gitrepo layer

`runChecked` call sites in `internal/gitrepo/merge.go`: **8** — `merge --squash`, `merge --no-commit`, `diff --cached --quiet`, `commit -m`, `commit --no-edit`, `diff --name-only --diff-filter=U`, `rev-parse --verify --quiet MERGE_HEAD`, `merge --ff-only`.

*Blind spot:* `ResolveSHA` uses go-git, not `runChecked`, so it is correctly outside this count but is still a new git-touching path. The gitrepo Client Boundary Invariant's pinned CLI list is the thing to check this against, and that list lives in `CONSTRAINTS.md`, not in this file.

## Count 6 — sibling mutating verbs and their merge disposition

The spec (plan card 13, `_mill/plan/05-sibling-guards-vocabulary.md` at `3b800bc8`) names exactly **4** verbs to guard and one explicitly not to:

| Verb | Spec disposition | Shipped |
|---|---|---|
| `Fabric.Commit` | record **and** foreign-state refusal | `commit.go` 123 + 130 — both arms present |
| `Fabric.Pull` | record-only, after the `SkipGit` early return | `pull.go` 221 |
| `Topology.Checkout` | record-only, before the dirty-weft pre-flight | `checkout.go` 48–53 |
| `Topology.Remove` | record-only, `force` does not override | `remove.go` 65–70 |
| `Topology.Cleanup` | **no guard, file not edited** | asserted by card 14 |
| `PushWeft`, push half of `sync`, all read-only verbs | deliberately unguarded | — |

So: **4** guarded verbs, **5** `ErrMergeInProgress` return sites (Commit has two).

*Blind spot, and this is the interesting one:* the spec's list is itself a claim, not a proof. `Fabric` alone exports 18 methods and `Topology` 8; the spec reasoned about a subset. Verbs the spec never adjudicates at all include `PullWeft`, `ResetHard`, `CheckoutDetached`, `RestoreBranch`, `RecordCorrespondence`, `RebuildIndex`, `Topology.Add`, `Topology.Prune`, `Topology.Reconcile`, and `Destroy`/`Coalesce`/`Bolt` at package level. A round that enumerates every mutating entry point and adjudicates each against a live merge record will exceed 4, and exceeding it is the correct direction.

## Deliberately not written down here

The orchestrator's own hypotheses about where the defects are.
They stay out of the round's prompt and out of this file's shared surface so a round's findings remain its own.
