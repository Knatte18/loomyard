# Batch: fabric-mutation-trace

```yaml
task: 'shed: the LLM driver as a generic stepper and mender'
batch: fabric-mutation-trace
number: 2
cards: 2
verify: go build ./... && go test ./internal/fabricengine/ -run 'TestMutations|TestRefDetail' && go test -tags integration ./internal/fabricengine/ -run 'TestPushAnchored_PushesAndRecordsBranchPush|TestPushWarpRebaseFreeAt_PushesAndRecordsBranchPush'
depends-on: [1]
```

## Batch Scope

Makes fabric's in-memory mutation record durable: every `Mutations.Append`/`AppendRef` also writes one `Info` record to the trace sink, and every `branch_created`/`branch_pushed` entry gains a `detail` naming side, repository and (for pushes) remote, so the driver can later prove which branch a failed step created and where.
Batch 5's skill text relies on both the `"fabric: mutation"` record and the `refDetail` grammar (overview decisions `trace-record-message-vocabulary` and `ref-detail-format`).
One batch because both cards live in `internal/fabricengine` and the detail change is only observable through the record the first card makes durable.

## Cards

### Card 3: log every appended mutation at Info

- **Context:**
  - `internal/logger/logger.go`
  - `internal/logger/sink.go`
- **Edits:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/mutation_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/fabricengine/mutation.go`, import `internal/logger` and, in both `(*Mutations).Append` and `(*Mutations).AppendRef`, after appending the entry, call `logger.Info("fabric: mutation", "kind", string(kind), "target", <the entry's recorded Target>, "detail", detail)`.
    `Append` logs the hub-relative `Target` it recorded (the result of `hubRelativeTarget`), not the raw argument, so the trace matches the envelope's `mutations` array.
    The nil-receiver early return stays first, so a nil `*Mutations` records and logs nothing.
    `Extend` logs nothing; add one sentence to its doc comment saying its entries were already logged when first appended.
  - Extend the `Mutations` type doc comment with one sentence: each appended entry is also written to the durable trace sink, so a killed process's trace keeps every mutation up to the last completed one.
  - Tests in `internal/fabricengine/mutation_test.go`, arming the sink per the overview's `trace-tests-use-sink-override` decision and reading the single `trace-*.log` file:
    `TestMutations_AppendLogsOneRecord` (one `Append` → exactly one line containing `msg="fabric: mutation"`, `kind=`, `target=`, `detail=` with the recorded values);
    `TestMutations_AppendRefLogsOneRecord` (same for `AppendRef`);
    `TestMutations_ExtendLogsNothing` (after arming, `Extend` onto a fresh recorder adds no `fabric: mutation` line — build the `other` record before arming the sink, or count lines before and after);
    `TestMutations_NilReceiverLogsNothing` (a nil `*Mutations`'s `Append`/`AppendRef` add no line).
  - These tests spawn nothing (Test Tier Purity).
- **Commit:** `feat(fabricengine): write every recorded mutation to the durable trace`

### Card 4: name side, repository and remote on branch entries

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/mutation_test.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/pushanchored.go`
  - `internal/fabricengine/spawn.go`
  - `internal/fabricengine/pushanchored_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/fabricengine/mutation.go`, add unexported `refDetail(side, repo, remote string) string` returning `"side=" + side + " repo=" + repo`, plus `" remote=" + remote` when `remote` is non-empty.
    `repo` is made absolute with `filepath.Abs` when it is not already (keep the input on error) and converted with `filepath.ToSlash`.
    Update the doc comments of `KindBranchCreated` and `KindBranchPushed` to state their `detail` grammar and that `side` is `warp` or `weft`.
  - Grep `internal/fabricengine` for `AppendRef(KindBranchCreated` and `AppendRef(KindBranchPushed` before editing and make every hit pass a `refDetail` detail; at planning time the hits are:
    `internal/fabricengine/add.go`'s `KindBranchCreated` after `createGitWorktree` (`refDetail("warp", l.WorktreePath(), "")`, since `git worktree add -b` ran in that checkout) and its `KindBranchPushed` after the `push -u origin warpBranch` call (`refDetail("warp", l.WorktreePath(), "origin")`);
    `internal/fabricengine/weftwiring.go`'s `createWeftWorktree` (`refDetail("weft", weftRepoRoot, "")`) and `pushWeftBranch` (`refDetail("weft", weftPath, "origin")`);
    `internal/fabricengine/checkout.go`'s weft `switch -c` site (`refDetail("weft", weftWorktree, "")`);
    and `recordPushIfAdvanced` in `internal/fabricengine/weftgit.go`.
    `KindBranchDeleted`/`KindRemoteBranchDeleted` sites in `internal/fabricengine/destroy.go` are not changed.
  - Change `recordPushIfAdvanced`'s signature to `recordPushIfAdvanced(rec *Mutations, repo *gitrepo.Repo, side, repoPath string, hasUnpushedBefore bool, hasUnpushedErr error)`.
    After resolving `branch`, read the branch's configured remote read-only with `gitexec.Run([]string{"config", "--get", "branch." + branch + ".remote"}, repoPath)`, trimmed; on error the remote is `""` (a failure to observe is not a failure to push, as the doc comment already says for the other samples).
    Record `refDetail(side, repoPath, remote)`.
    Update every caller: `coalesce.go` (`"warp", warpPath` and `"weft", weftPath`), `pushanchored.go` (`"weft", target`), `spawn.go`'s `PushWarpAt` and `PushWarpRebaseFreeAt` (`"warp", warpPath`), and `weftgit.go`'s `PushWeft` (`"weft", f.weftPath`).
  - Add `TestRefDetail` to `internal/fabricengine/mutation_test.go`: table over (create with empty remote, push with remote, relative repo made absolute), asserting the exact strings.
  - In `internal/fabricengine/pushanchored_integration_test.go`'s `TestPushAnchored_PushesAndRecordsBranchPush`, additionally assert the found `KindBranchPushed` entry's `Detail` starts with `"side=weft repo="` and contains `" remote="`.
- **Commit:** `feat(fabricengine): record side, repository and remote on branch entries`

## Batch Tests

`go build ./...` catches any missed `recordPushIfAdvanced` caller.
The untagged `TestMutations*`/`TestRefDetail` run `mutation_test.go` (cards 3 and 4).
The two tagged push tests run the real push sites through `recordPushIfAdvanced` against a hubforge pair; `TestPushAnchored_PushesAndRecordsBranchPush` carries the new detail assertion, `TestPushWarpRebaseFreeAt_PushesAndRecordsBranchPush` proves the warp path still records.
The `add.go`/`weftwiring.go`/`checkout.go` detail edits are covered by the done gate's full integration run.
