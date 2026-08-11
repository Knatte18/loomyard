# Batch: constructive-recording

```yaml
task: 'fabric: accumulate the result envelope from mutations, not control flow (slice 14)'
batch: 'constructive-recording'
number: 5
cards: 5
verify: go test ./internal/fabricengine/ ./internal/fabriccli/ && go test -tags integration -run TestMutationRecord ./internal/fabricengine/
depends-on: [4]
```

## Batch Scope

Batch 4 gave the destruction gate provably total coverage;
this batch hand-records the constructive mutations no chokepoint covers — links created, branches created and pushed, commits landed, exclude files rewritten, worktrees switched, repos advanced.
It is one batch because every card records through the same recorder into the same enum, and splitting by kind would leave a half-populated record that no assertion could meaningfully pin.

A constructive record is appended at the **success site**, after the act observably happened, never at the attempt.
Two rules the implementer must not soften: a `Target` that is a filesystem path goes through `Append` (which converts), and a `Target` that is a git ref goes through `AppendRef` (which does not).

After this batch the engine's record is complete.
Batch 6 emits it, batch 7 asserts it against the filesystem.

## Cards

### Card 17: `link_created` and `file_written`

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/gitexclude.go`
- **Edits:**
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Record `KindLinkCreated` at every `fslink.CreateDirLink` success site, with the **link** path as the target (never the link's own target path):

  - `createPortal` — `internal/fabricengine/portals.go`; add a leading `rec *Mutations` parameter and thread it from its callers.
  - `seedLyxJunction`'s two creation sites and `adoptDotLyxContent`'s own — `internal/fabricengine/junction.go`; `seedLyxJunction` already carries `rec` from batch 4, and `adoptDotLyxContent` gains a leading `rec *Mutations` parameter threaded from it.
  - `wireBoardLink`'s two creation sites — `internal/fabricengine/junction.go`; already carries `rec` from batch 4.

  A junction **re-point** therefore records as `link_removed` (from the gate, batch 4) followed by `link_created` (here) — two entries for what physically happens.
  There is deliberately no `link_repointed` kind, and none may be added: the new target does not exist at `repointLink`'s recording site, so its `Detail` would be unfillable.

  Record `KindFileWritten` when a `.git/info/exclude` rewrite actually changed the file — `mutateGitExclude` in `internal/fabricengine/gitexclude.go` already returns a `changed bool` that says so, and its callers are where the record goes:

  - `seedGitExclude` and `unseedGitExclude` — `internal/fabricengine/junction.go`
  - `seedWeftArtifactExcludes` — `internal/fabricengine/weftgit.go`

  Each gains a leading `rec *Mutations` parameter, threaded from its callers, and records the resolved exclude-file path with `Append` only when `changed` is true.
  A rewrite that changed nothing records nothing, per the record-only-on-observed-effect rule.
  `mutateGitExclude` and `writeFileAtomically` themselves stay unparameterised — they are the mechanism, and their callers are the ones that know the act was a fabric mutation.

  Do not change any behaviour, ordering, or error text.
- **Commit:** `feat(fabricengine): record link_created and file_written at their success sites`

### Card 18: `worktree_created`, `branch_created` and `branch_pushed` on the add path

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/weftwiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/fabricengine/add.go`:

  - The warp worktree's own creation is already recorded by the gate (`createGitWorktree` mints it). Immediately after that call succeeds, additionally record `rec.AppendRef(KindBranchCreated, warpBranch, "")` — the `-b warpBranch` argument means the same call created a branch, and a branch is a ref, not a path.
  - After the `git push -u origin <warpBranch>` call succeeds, record `rec.AppendRef(KindBranchPushed, warpBranch, "")`. Record only on a nil error and a zero exit code, matching the gate's own rule.

  In `internal/fabricengine/weftwiring.go`:

  - `createWeftWorktree` gains a leading `rec *Mutations` parameter and records `KindWorktreeCreated` with the created weft worktree path on success, plus `rec.AppendRef(KindBranchCreated, branch, "")` when the call created the branch rather than checking out an existing one. This site does **not** route through the gate — `createGitWorktree` is the gate's minter for the *warp* side, and the Fabric Destruction Chokepoint Invariant governs destruction, not creation — so the record here is hand-written by design, not an oversight. Say so in a comment.
  - `pushWeftBranch` gains a leading `rec *Mutations` parameter and records `rec.AppendRef(KindBranchPushed, branch, "")` on success.

  Thread the new parameters from the callers of both functions (`internal/fabricengine/add.go`), passing the verb's own recorder.
  Do not change any behaviour, ordering, or error text.
- **Commit:** `feat(fabricengine): record worktree, branch and push mutations on the add path`

### Card 19: `commit_created` and `branch_pushed` on the commit and push paths

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/fabric.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/spawn.go`
  - `internal/fabricengine/coalesce.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `(*Fabric).Commit` — `internal/fabricengine/commit.go` — record from the already-populated `CommitResult` fields rather than from control flow, immediately after `commitBothSides` returns and **before** the `PartialCommitError` branch returns, so a partial commit records the side that landed:

  - when the warp side committed, `rec.Append(KindCommitCreated, f.warpPath, <the warp SHA the result carries>)`
  - when the weft side committed, `rec.Append(KindCommitCreated, f.weftPath, result.WeftSHA)`

  The `Target` is the worktree the commit landed in (a path, so `Append`) and the `Detail` is the SHA.
  A `PartialCommitError` is an ordinary error like any other — it is **not** a second trigger for `partial`, and it needs no special case here beyond making sure the record is appended on the path that returns it.

  **The three push entry points need an explicit resolution and an explicit success predicate, because neither is in scope at the call site today.** `(*Fabric).PushWeft` is `return f.weft.PushCoalesced()`, `PushWarpAt` is `return gitrepo.New(warpPath).PushCoalesced()`, and `CoalescePushBothAt` pushes through `pushRebaseFreeLogged` — none of them names a branch, and `PushCoalesced` returns a bare `error` that is `nil` both when a push landed and when there was nothing to push.
  Worse, `pushRebaseFreeLogged` maps `gitrepo.ErrPushRejected` to a warning and a `nil` return (commits left unpushed, per fabric's contract), so a naive "record on nil error" would append `branch_pushed` for a push the remote **rejected** — the exact lie of commission this slice exists to kill.

  Use the two `gitrepo` accessors that already exist, and nothing else:

  - **Resolution:** `(*gitrepo.Repo).CurrentBranch()` gives the ref name to pass to `AppendRef`.
  - **Success predicate:** `(*gitrepo.Repo).HasUnpushed()` sampled **before** and **after** the push call. Record `branch_pushed` only on a `true` → `false` transition. That is a real observation of a state change, matching the record-only-on-observed-effect Shared Decision, and it is the same before/after shape `CoalescePushBothAt`'s own loop already uses with `headOrEmpty`. Nothing-to-push (`false` before) and rejected (`true` still after) both correctly record nothing.

  Apply it at:

  - `(*Fabric).PushWeft` — `internal/fabricengine/weftgit.go`, against `f.weft`
  - `PushWarpAt` — `internal/fabricengine/spawn.go`, against the `gitrepo.New(warpPath)` handle it already builds
  - `CoalescePushBothAt` — `internal/fabricengine/coalesce.go`, per side inside the existing `step` closure, beside the `beforeWarp`/`beforeWeft` sampling that is already there. It can therefore record up to **two** `branch_pushed` entries, one per side that genuinely advanced, and none for a side whose path is empty or whose HEAD is unborn.

  If either accessor errors, record nothing for that side and do **not** propagate the error — a failure to *observe* is not a failure to push, and turning an observation error into a verb failure would change behaviour this card is forbidden to change.
  Do not change any behaviour, ordering, or error text.
- **Commit:** `feat(fabricengine): record commit_created and branch_pushed on the commit and push paths`

### Card 20: `worktree_switched` and `repo_advanced`

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/weftgit.go`
- **Edits:**
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/fabricengine/checkout.go`, record `KindWorktreeSwitched` after each `git switch` succeeds, with the switched **worktree root** as the `Target` (a path, so `Append`) and the branch switched to as the `Detail`:

  - `(*Topology).Checkout`'s warp-side `git switch <branch>` in the prime warp worktree
  - `switchOrForkWeft`'s `git switch <weftBranch>` in the weft worktree
  - `switchOrForkWeft`'s `git switch -c <weftBranch> <parentWeftBranch>`, which additionally records `rec.AppendRef(KindBranchCreated, weftBranch, "")` because `-c` created it

  `switchOrForkWeft` gains a leading `rec *Mutations` parameter threaded from `Checkout`.
  `rollbackSwitch`'s own two `git switch` calls also record `KindWorktreeSwitched` — a rollback switch is a real mutation of the working tree and the record must carry it, in order, which is the whole point of `Checkout`'s both-sides-rollback case.
  Record only on a zero exit code and a nil error;
  `rollbackSwitch` discards those values today (`_, _, _, _ = gitexec.RunGit(...)`), so capture the exit code and error there in order to apply the rule, without otherwise changing the function's best-effort posture.

  In `internal/fabricengine/pull.go`, record `KindRepoAdvanced`:

  - after the weft fast-forward pull succeeds (the `f.PullWeft(opts)` call inside `(*Fabric).Pull`), `rec.Append(KindRepoAdvanced, f.weftPath, "")`. This is load-bearing: `Pull` sets `result.WeftPulled = true` and can then return a `*PartialPullError` with `Stage: "unpushed-check"` or `Stage: "fetch"` having created no commit and executed no gate primitive — without this entry the record would be empty and `partial` would be `false` while the weft worktree had genuinely been advanced. That is this slice's own defect class inside the very verb that produced defect 1.
  - after each warp advance succeeds, `rec.Append(KindRepoAdvanced, f.warpPath, <the new tip SHA when it is in hand, otherwise empty>)`. The `ResetHard` underneath is separately recorded by the gate as `worktree_reset`; both entries are correct and both belong — one names the primitive, the other names the effect.

  `PartialPullError.Stage` surfaces as the `Detail` of the relevant mutation where a mutation exists for it, never as a top-level envelope field — the envelope's key set stays fixed across verbs.
  Do not change any behaviour, ordering, or error text.
- **Commit:** `feat(fabricengine): record worktree_switched and repo_advanced`

### Card 21: engine-level mutated-then-errored assertions

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/remove_guard_integration_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mutation_record_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabricengine/mutation_record_integration_test.go` behind the `integration` build tag, in `package fabricengine_test` — the **external** test package, matching `internal/fabricengine/remove_guard_integration_test.go`, whose fixture builder `newFabricFixture` is declared there and is unreachable from the internal package.
  Everything this card asserts is exported (`RefusalOf`, `Mutated()`, `Topology.Remove`, `Topology.Add`), so the external package costs nothing.
  The package's `TestMain` already calls `lyxtest.HermeticGitEnv()`, per the Hermetic Git Test Environment Invariant.

  Every test function name starts with `TestMutationRecord` so the batch's `-run TestMutationRecord` verify selects exactly this file's tests and nothing else.

  Assert the two headline mutated-then-errored paths at the engine boundary, reading the record through `res.Mutated().Entries()`:

  - **`Remove` refusing on a dirty warp worktree after `removePortal`/`removeLaunchers` already ran.** `internal/fabricengine/remove.go` runs the portal and launcher removals *before* its dirty pre-flight, so a correctly-refusing `Remove` has already destroyed both. Assert the returned record names both deletions, that the returned error is non-nil, and — separately — that `RefusalOf(err)` reports `false`: that pre-flight returns a bare `fmt.Errorf("worktree has uncommitted changes; use --force")`, never a `*destructiveRefusal`, so no `refusal` object exists on this path. Assert its **absence**, not its contents. This slice does not convert that pre-flight into a gate refusal.
  - **`Add` failing partway and running `rollbackAdd`.** Assert the record carries the creations *and* the rollback's own destructions, in execution order — a `worktree_created` before the `worktree_removed` that undoes it, not merely both present.

  Assert order explicitly (compare the `Kind` sequence), not just set membership: array order is the only thing carrying ordering in this vocabulary, so an assertion that ignores it asserts half the contract.

  Do not add cases already owned by batch 7's matrix cross-check (`Prune`/`Cleanup` partial removal, `Checkout` both-sides rollback, `Pull` on a dirty warp) — those are asserted against the manifest diff there, which is strictly stronger than a hand-written expectation here.
- **Commit:** `test(fabricengine): assert the record survives Remove's refusal and Add's rollback`

## Batch Tests

`verify: go test ./internal/fabricengine/ ./internal/fabriccli/ && go test -tags integration -run TestMutationRecord ./internal/fabricengine/` runs the two packages' untagged suites — which must stay green, since no behaviour changes here — and then the one new tagged file, selected by the `TestMutationRecord` name prefix card 21 mandates.
The tagged run is deliberately name-scoped rather than the whole `integration` suite for this package: the full tagged suite is minutes long and this batch's own new surface is one file.
The whole tagged suite runs at the done gate (`pipeline.done_gate` is already `go test ./... && go test -tags integration ./...` for this hub), which is what catches a regression this scope cannot see.
