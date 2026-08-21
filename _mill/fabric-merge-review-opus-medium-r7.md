# fabric merge surface — independent review (round 7, `opus-medium-r7`)

Clean-room round 7 of the fabric merge-surface crucible campaign.
Findings below were formed without reading any prior `_mill/fabric-merge-review-*` material.

## Scope reviewed

- `internal/fabricengine/merge.go`, `mergelifecycle.go`, `mergeerrors.go`, `mergeguards.go`, `mergestate.go`, `mergestage.go`, `mergepaths.go`
- `internal/gitrepo/merge.go`
- `internal/fabriccli/merge_verbs.go`, `envelope.go`, `weft_verbs.go`
- `internal/fabricengine/doc.go` "# The merge surface" (lines 846–1069)
- All `merge*_test.go` / `merge*_integration_test.go` siblings

## What was tested

### Baseline gates (all green, before any edit)

- `go build ./...` — OK
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — OK
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — OK (exit 0)
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — OK
  (fabricengine 31.8s, fabriccli 2.6s, gitrepo 1.7s)
- `./deploy-dev` — dev binary at `.dev-bin/lyx`

### Live substrate — real bare warp/weft pair, hub driven by the dev binary

Hub built by hand under the scratch directory (never inside the worktree), `GIT_CONFIG_GLOBAL` with
`[init] defaultBranch = main` exported before the first `git init`:
`git init --bare warp.git weft.git` → seed+push warp `main` → `lyx fabric clone <weft-bare> <warp-bare>` →
`lyx fabric add task-a`.

1. **Two-sided conflicting MergeIn.** Divergent `src/app.txt` (warp side) and `_lyx/raddle/notes.md`
   (weft side, written through the junction and landed with `lyx fabric push`) on both `main` and
   `task-a`. `lyx fabric merge-in task-a` reported
   `{"conflicts":["_lyx/raddle/notes.md","src/app.txt"], ... "ok":false,"partial":false}` with two
   `merge_staged` mutations. Record on disk carried both starts, both resolved sources, and
   `conflicted` on both outcomes. Live `MERGE_HEAD` on both sides matched the recorded sources exactly.
   → unified conflict list, record shape, and `merge_staged` recording all correct.
2. **MergeAbort from the two-sided conflicted state.** Both HEADs restored to the recorded starts,
   both `MERGE_HEAD`s gone, both `git status --porcelain` empty, record deleted, two `worktree_reset`
   mutations. A second `--abort` and a `--continue` both returned `fabricengine: no merge in progress`
   (not foreign state). A fresh `merge-in task-a` afterwards reproduced the same conflicts rather than
   being refused as foreign — so the abort really does clear git-level merge state, not just fabric's.
3. **Operator-route conflict resolution (see F1).** Resolved BOTH conflicted paths through the single
   visible worktree, then staged as an operator would: `git add src/app.txt` succeeded;
   `git add _lyx/raddle/notes.md` failed with
   `fatal: pathspec '_lyx/raddle/notes.md' is beyond a symbolic link` (exit 128). `merge --continue`
   then refused with `unresolved conflicts remain`. There is no CLI surface for `MergeStageResolved`.
4. **MergeContinue after staging the weft side with raw `git -C <weft> add`.** Concluded with
   `committed:true` and two `merge_committed` mutations. `rev-list --parents -n1 HEAD` on both sides
   showed exactly two parents in exactly the recorded (start, source) order — the shape
   `sideConcludeAlreadyLanded` demands. Record deleted, resolved content correct on both sides.

### Sabotage sweep

Harness: apply exactly one source mutation, `go build ./...` (a build break is discarded, not counted
as proof), then `go test -tags integration -count=1 ./internal/fabricengine/... ./internal/fabriccli/...
./internal/gitrepo/...`, then restore. A PASS after sabotage means the mechanism is unguarded.

Round 6's own new mechanisms (the named re-examination target):

| # | sabotage | result |
|---|---|---|
| S-a | `len(parents) != 2` → `len(parents) < 2` | caught — `TestMergeContinue_OctopusMergeCarryingTheSource_IsNeverAdopted` |
| S-b | drop `parents[0] != start` | caught — `TestMergeContinue_MergeOfSourceOntoWrongBase_IsNeverAdopted` |
| S-c | drop `parents[1] != sourceSHA` | caught — `TestMergeContinue_MergeOfWrongSourceOntoStart_IsNeverAdopted` |
| S-f | drop the squash refusal in adoption | caught — `TestMergeContinue_SquashRecordCarryingATwoParentMerge_IsNeverAdopted` |
| S-g | drop the live-`MERGE_HEAD` refusal in adoption | caught — `TestMergeContinue_SecondMergeStartedOverALandedConclude_LeavesNoLiveMergeHead` |
| S-h | drop `MergeStageResolved`'s foreign-state guard (F7) | caught — `TestMergeStageResolved_ForeignMergeStateRefusesWithoutStaging` |
| S-i | drop `finalizeMergeResult`'s `Conflicts` nil-safety (F8) | caught — `TestMergeCrucible_ConflictsIsEmptyNeverNil` (4 subtests) |
| S-e | `filepath.ToSlash` → blanket `strings.ReplaceAll(…, "\\", "/")` (F6) | caught — `…/single_anchor_segment_containing_a_backslash_is_not_split` |
| **S-d** | **DELETE `filepath.ToSlash` entirely (identity)** | **PASS — UNGUARDED → F2** |

Each of S-a/S-b/S-c was caught by a *different* test, so round 6's three parentage clauses are
independently pinned and each test fails for its own reason. Round 6's work is sound except S-d.

Wider surface:

| # | sabotage | result |
|---|---|---|
| S-1 | `MergeStart` drop the `--ff` pin | caught — `TestMergeStart_HostileMergeFFConfig` |
| S-2b | `MergeStart` drop the `mergeHeadPresent` classification arm | caught — 2 tests, both packages |
| S-3 | `ConflictedFiles` drop `-z` | caught — 7 tests |
| **S-4** | **`StageResolved` `add -A --` → `add --`** | **PASS — UNGUARDED → F3** |
| S-5 | `MergeFFOnly` → `reset --hard` | caught — `TestMergeFFOnly_FailsLoudlyOnDivergedPair` |
| S-6 | `detachedHeadReason` drop the weft half | caught — `…/WeftDetached` |
| S-7 | `pairDirtyReason` drop the weft half | caught — 2 tests |
| **S-8** | **`foreignMergeStatePresent` drop BOTH weft probes** | **PASS — UNGUARDED → F4** |
| **S-10** | **`foreignMergeStatePresent` drop the warp conflicted-index probe** | **PASS — UNGUARDED → F4** |
| **S-11** | **`foreignMergeStatePresent` drop the warp `MERGE_HEAD` probe** | **PASS — UNGUARDED → F4** |
| **S-12** | **`syncedToUpstreamReason` drop the weft half** | **PASS — UNGUARDED → F5** |
| **S-13** | **`sideNotSyncedToUpstream` drop the behind-passes clause** | **PASS — UNGUARDED → F5** |
| S-14 | `syncSideBeforeMerge` drop the whole FF advance | caught — 2 tests |
| S-9 | `sideConcludeMayHaveLanded` drop the HEAD-moved clause | caught — 2 tests |

### Live: is `Merge`'s not-synced guard reachable at all? (→ F5)

`git add`-probe first (`git version 2.53.0`): a delete/modify conflict resolved by deletion stages
with **plain** `git add -- f.txt`, exit 0, index clean afterwards — so `StageResolved`'s godoc
rationale for `-A` is false on any modern git (→ F3).

Then, on the live hub: created pairs `target` and `task-b`; both sides of `target` carry a real
`@{u}` (`origin/target`, `origin/target-weft`). A separate clone pushed a commit to `origin/target`;
`target` then made a local commit **without fetching**. State at call time:

```
target HEAD:          47828cdf…      (local commit)
target origin/target: 81b823c7…      (stale, pre-fetch)
real remote tip:      c815ec1a…
```

`lyx fabric merge task-b` returned:

```
{"already_up_to_date":false,"committed":true,"mutations":[…],"ok":true,"partial":false}   exit=0
```

and afterwards `git rev-list --left-right --count 'HEAD...@{u}'` = **`3	1`** — genuinely diverged,
with `origin/target` now advanced to `c815ec1a…` because this very call fetched it. The guard that
exists to refuse exactly this never fired.

## Findings
