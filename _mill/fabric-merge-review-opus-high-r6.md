# fabric merge surface — independent review (round 6, tag `opus-high-r6`)

Reviewer: Opus, high effort. Clean-room: findings below were formed before reading any prior-round `_mill/fabric-merge-review-*` material.
Worktree: `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-round4`, branch `fabric-merge-crucible-round4`.

## What was tested

Appended as each command/scenario returned.

### Baseline gates (before any edit)

- `go build ./...` — clean.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — clean.
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok (fabricengine 0.355s, fabriccli 0.004s, gitrepo 0.005s, cmd/lyx 0.963s).
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — all ok (fabricengine 31.4s, fabriccli 2.6s, gitrepo 1.6s).

### Sabotage sweep — round 5's and round 4's own mechanisms (each applied alone, suite re-run, then reverted)

Harness: apply one production-source edit, run `go test -tags integration -count=1 ./internal/fabricengine/...`, `git checkout --` the file.
"GREEN" = the whole integration suite still passed with the mechanism removed, i.e. no test can detect that clause's loss.

| # | Sabotage | Result |
|---|---|---|
| S1 | `MergeIn`: delete the under-lock `weftStart` re-read (keep warp's) | **GREEN — uncovered** |
| S2 | `MergeIn`: delete the post-lock record re-check | red (`TestMergeIn_RecordAppearingWhileWaitingForLock_...`) |
| S3 | `Merge`: delete the post-lock record re-check | red (`TestMerge_RecordAppearingWhileWaitingForLock_...`) |
| S4 | `MergeAbort`: move record load + conclude-landed guard back ahead of the lock | red (`TestMergeAbort_ConcludeLandingWhileWaitingForLock_...`) |
| S5 | `MergeContinue`: move record load back ahead of the lock | red (`TestMergeContinue_RecordRetiredWhileWaitingForLock_...`) |
| S6 | `sideConcludeAlreadyLanded`: drop the live-`MERGE_HEAD` refusal | **GREEN — uncovered** |
| S7 | `sideConcludeAlreadyLanded`: drop the `squash` refusal | **GREEN — uncovered** (the squash test passes on `len(parents) < 2` instead) |
| S8 | `sideConcludeAlreadyLanded`: drop the `sourceSHA == ""` refusal | GREEN, but behaviourally a no-op (no real parent SHA equals `""`), so nothing to cover |
| S9 | `sideConcludeAlreadyLanded`: drop `parents[0] != start` | red (`TestMergeContinue_MergeOfSourceOntoWrongBase_...`) |
| S10 | `sideConcludeAlreadyLanded`: drop the source-membership loop | red (`TestMergeContinue_MergeOfWrongSourceOntoStart_...`) |
| S11 | `sideConcludeAlreadyLanded`: drop the `head == start` early return | GREEN, but behaviourally a no-op (`parents[0] != start` refuses the same states) |
| S12 | `sideConcludeMayHaveLanded`: drop the `committed != ""` clause | **GREEN — uncovered** |
| S13 | `sideConcludeMayHaveLanded`: drop the HEAD-moved clause | red (2 tests) |
| S14 | `sideConcludeMayHaveLanded`: drop the outcome filter | red (`TestMergeIn_OneSideFastForwardsOtherConflicts_...`) |
| S15 | `detachedHeadReason`: drop the refusal | red (`TestMergeCrucible_DetachedHeadRefused`) |
| S16 | `resolveMergeSources`: drop the fabric-managed refusal | red (`TestMergeIn_NotFabricManaged_NothingMutated`) |

### Non-`-z` git-output-parsing audit (round 5 F1's family)

Every `--name-only` / `--porcelain` / `ls-files` site reachable from the merge surface:
`gitrepo/merge.go:155` (`ConflictedFiles`, has `-z`), `fabricengine/dirtiness.go:57` (`status --porcelain`, emptiness-only — quoting cannot affect the boolean),
`gitrepo/gitrepo.go:148` (`ls-files --cached`, emptiness-only).
`status.go:178`, `pull.go:435`, `warpprobe.go:128`, `weftgit.go:163`, `worktreelist.go:28` are outside the merge surface.
**No second instance of the F1 class exists in the merge surface.**

### Live driving (real hub, `.dev-bin/lyx`, no launcher)

Lab hub built by hand at `<scratch>/lab/h1`: `GIT_CONFIG_GLOBAL` with `init.defaultBranch = main`, `git init --bare` warp + weft (weft empty — the documented bootstrap shape), seeded warp `main`, `lyx fabric clone <weft-bare> <warp-bare>` from an empty work dir. Prime pair `warp`/`warp-weft` on `main`/`main-weft`.

- `lyx fabric add task1` → pair created, `task1`/`task1-weft`.
- Divergence on BOTH sides (warp `README.md` via plain git, weft `_lyx/README.md` via `lyx fabric commit`), on the pair and on the prime, so the merge is a real merge.
- `lyx fabric merge-in task1` from the prime → exit 1, `ok:false`, `conflicts:["README.md","_lyx/README.md"]` — one flat, worktree-relative, side-free list; `mutations` carries two `merge_staged` entries; `partial:false`. Correct.
- Conflict markers on both sides carry the **SHA**, never a branch: warp `>>>>>>> eef5fd82…`, weft `>>>>>>> b91a3da4…`. No `-weft` leak.
- Sibling refusals while the record is live: `commit`, `pull`, `push`, `sync`, `checkout task1`, `remove task1` all returned the single fixed `a merge is in progress; run MergeContinue or MergeAbort first`, `mutations: []`.
- `lyx fabric merge --continue -m …` with conflicts still unresolved → `merge preconditions failed: unresolved conflicts remain`, nothing mutated.
- Resolved both sides, `git add` each, `lyx fabric merge --continue -m "merged task1 into main"` → `ok:true`, `committed:true`, `already_up_to_date:false`, one `merge_committed` per side; both checkouts clean, no merge in progress, both logs carry the named conclude commit.

## Findings

(in progress)
