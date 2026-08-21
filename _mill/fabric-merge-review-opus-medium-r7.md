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

## Findings

(appended live, in order)
