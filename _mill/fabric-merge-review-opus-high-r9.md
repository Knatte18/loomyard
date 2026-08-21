# fabric merge surface — independent review (round 9, tag `opus-high-r9`)

Reviewer: fresh clean-room pass, Opus/high.
Worktree: `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-round4`, branch `fabric-merge-crucible-round4`.
Scope: the merge lifecycle quintet (`MergeIn`/`Merge`/`MergeContinue`/`MergeAbort`/`MergeInProgress`) plus `MergeStageResolved`, `internal/gitrepo/merge.go`, and the `lyx fabric merge*` CLI surface.

## Status

IN PROGRESS — findings section below is appended to as the pass proceeds.

## What was tested

### Hermetic gates (baseline, before any edit)

- `go build ./...` — OK.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — OK.
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok.
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — all ok (35.6s / 3.0s / 1.7s).

### Sabotage re-proofs of round 8's own new mechanisms

Sabotage was performed in an ISOLATED COPY of the tree (`scratchpad/r9/sabotage`, `tar`-cloned without `.git`), so no production or test file in this worktree was touched during Job 1.

| # | Sabotage | Test(s) that must fail | Result |
|---|---|---|---|
| S1 | `recheckMergePreconditionsUnderLock`: delete the foreign-state re-check | `TestMergeIn_ForeignStateAppearingWhileWaitingForLock_Refuses`, `TestMerge_…` | FAIL (both) — guard holds |
| S2 | `recheckMergePreconditionsUnderLock`: delete the dirtiness re-check | `TestMergeIn_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt`, `TestMerge_…` | FAIL (both) — guard holds |
| S3 | `recheckMergePreconditionsUnderLock`: delete the record re-check | `TestMergeIn_RecordAppearingWhileWaitingForLock_…`, `TestMerge_…` | FAIL (both) — guard holds |
| S4 | `MergeAbort`: move lock acquisition back below the record read + conclude-landed guard | `TestMergeAbort_ConcludeLandingWhileWaitingForLock_RefusesInsteadOfResetting` | FAIL — guard holds |
| S5 | `errConflictsWithRecord`: drop the `merge-stage` step from the runtime conflict message | `TestErrConflictsWithRecord_ReservedKeysAreAlwaysTheHelperOwnValues` | FAIL — guard holds |
| S6 | `MergeStageResolved`: delete the foreign-state refusal | `TestMergeStageResolved_ForeignMergeStateRefusesWithoutStaging` | FAIL — guard holds |
| S7 | `MergeIn`: delete the WEFT-side start re-read under the lock | `TestMergeIn_StartsAreReReadUnderLock` | FAIL on `WeftStart` — guard holds |
| S8 | CLI merge-stage: `uniquePreservingOrder(args)` → `args` | `TestRunCLI_MergeStageEchoesEachPathOnce` | FAIL — guard holds |
| S9 | `MergeContinue`: move lock acquisition back below the record read + guards | `TestMergeContinue_RecordRetiredWhileWaitingForLock_ReportsNoMergeInProgress` | FAIL — guard holds |

No round-8 test survived sabotage of the mechanism it claims to guard.

### Live substrate (real bare warp/weft pair, `lyx fabric clone`, dev binary redeployed from this source)

Hub recipe: `GIT_CONFIG_GLOBAL` with `[init] defaultBranch = main` before the first `git init`; bare `warp.git` + `weft.git`; seeded warp `main`; `lyx fabric clone <weft-bare> <warp-bare>` from an empty parent; `lyx fabric add feat` for the source pair.

- **S1 live — happy path.** conflicting divergence on BOTH sides (`shared.txt` warp, `_lyx/raddle-note.md` weft) → `merge-in feat` returns `conflicts:["_lyx/raddle-note.md","shared.txt"]`, `partial:false`, two `merge_staged` entries. Plain `git add _lyx/raddle-note.md` in the visible worktree refuses (`beyond a symbolic link`) exactly as the help claims. `merge-stage` + `merge --continue` → `committed:true`, both sides carry a two-parent merge commit with the recorded source as parent 2.
- **S2 live — partial staging.** Stage only the warp path, then `merge --continue` → `merge preconditions failed: unresolved conflicts remain`. See finding F1.
- **S3 live — `merge --abort`.** Restores both HEADs exactly, clean status both sides, record gone, `merge --abort`/`--continue` afterwards both answer `no merge in progress`.
- **S4 live — `merge` over a conflicting source.** Self-aborts: `ErrMergeInRequired`, `partial:true`, two `merge_staged` + two `worktree_reset` entries, both HEADs restored byte-exact, no `MERGE_HEAD` on either side, record gone. See finding F2 for the message.
- **S5 live — guard set.** warp-only branch → `source branch is not fabric-managed`; nonexistent branch → `source branch is not fabric-managed; source branch not found` (see F3); detached weft HEAD → `checkout is not on a branch`; dirty warp → `worktree dirty`.
- **S6 live — the post-fetch not-synced re-decision (round 6's fix).** Side clone pushes to `origin/main`; local `main` gains its own commit and is never fetched, so pre-fetch `rev-list --left-right --count HEAD...@{u}` = `1 0` (guard-stage sees "ahead", passes). `lyx fabric merge feat` refuses with `branch not synced to upstream`; post-call count is `1 1`, worktree clean, no record. The re-decision still holds.
- **S7 live — sibling refusals mid-merge.** `pull`, `checkout`, `remove` all return `a merge is in progress; run MergeContinue or MergeAbort first`; a second `merge-in` returns `merge already in progress; worktree dirty`.
- **S8 live — hand-landed conclude adoption.** Resolve + `merge-stage` both sides, then plain `git commit --no-edit` in warp only (record's `warp_committed` still empty). `merge --continue` adopts the hand-landed warp commit (`merge_committed` naming it), concludes weft, reports `committed:true`, deletes the record.
- **S9 live — unmappable weft conflict.** A conflicting weft file at the weft ROOT (`rootnote.txt`, outside every wired name) → `merge produced conflicts outside the fabric-managed tree; operator intervention required`, `partial:true`, both sides reset byte-exact, record gone. The offending path IS visible to the operator: `level=WARN … weft_conflicts=[rootnote.txt]` is emitted at default verbosity, not only under `-vv`.

## Findings

(appended provisionally as spotted)
