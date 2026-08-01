# Batch: builder-resethard-migrate

```yaml
task: 'fabric: audit and migrate all remaining direct git mutations onto Fabric'
batch: builder-resethard-migrate
number: 3
cards: 4
verify: go test -tags integration -run 'TestRestartChain|TestSpawnBatch|TestHeadSHA|TestChangedFiles|TestDirty|TestChainMembers|TestChainEndFor' ./internal/builderengine/
depends-on: [1]
```

## Batch Scope

Migrate `internal/builderengine`'s chain-rollback reset off the raw `gitexec.RunGit([]string{"reset","--hard",sha}, worktree)` call and onto a `*fabricengine.Fabric` handle, via a narrow consumer-side interface `WarpResetter`. `builderengine.ResetHard` (the raw `gitexec` wrapper in `gitquery.go`) is deleted; `RestartChain` takes a `WarpResetter` instead of a `worktree` string and calls `resetter.ResetHard(startSHA)`; production constructs a real `*Fabric` inline in `SpawnBatch`; tests inject a `*gitrepo.Repo` over their existing scratch worktree. This also introduces `ErrInvalidSHA` validation on the reset target (the old raw call had none) — a benign behavior change, since `RestartChain`'s reset target is always a previously-recorded real commit SHA (`ChainStartSHAs[chainEnd]`). Touches only `internal/builderengine`. Depends on batch 1 for `Fabric.ResetHard`.

## Cards

### Card 6: Define WarpResetter and retype RestartChain off the worktree string

- **Context:**
  - `internal/fabricengine/warpforward.go`
  - `internal/gitrepo/reset.go`
- **Edits:**
  - `internal/builderengine/chain.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/builderengine/chain.go`, define an exported interface `WarpResetter` with exactly one method: `ResetHard(sha string) error`. Doc-comment it as the warp-only hard-reset surface `RestartChain` drives, structurally satisfied by both `*gitrepo.Repo` (tests) and `*fabricengine.Fabric` (production).
  - Change `RestartChain`'s signature from `RestartChain(worktree string, st *State, plan *Plan, chainEnd int, reportsDir string) error` to `RestartChain(resetter WarpResetter, st *State, plan *Plan, chainEnd int, reportsDir string) error` — drop the `worktree string` parameter entirely (it was used ONLY to feed the reset call; every other step uses `st`/`plan`/`chainEnd`/`reportsDir`).
  - Replace the reset call `if err := ResetHard(worktree, startSHA); err != nil {` with `if err := resetter.ResetHard(startSHA); err != nil {`. The surrounding recorded-anchor check, member-report deletion, `st.Batches` reset, and `st.CurrentBatch = 0` are unchanged.
  - Update `RestartChain`'s doc comment (currently "resets worktree's host repo to it via ResetHard") to describe the reset going through the injected `WarpResetter` rather than a worktree path. Keep the "recorded state.json SHA is the ONLY reset target" invariant wording.
  - Do not touch `ChainMembers` or `ChainEndFor`.
- **Commit:** `refactor(builderengine): retype RestartChain onto a WarpResetter interface`

### Card 7: Delete builderengine.ResetHard and its direct test

- **Context:**
  - `internal/builderengine/chain.go`
- **Edits:**
  - `internal/builderengine/gitquery.go`
  - `internal/builderengine/gitquery_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Delete the `ResetHard(worktree, sha string) error` function from `internal/builderengine/gitquery.go` (the raw `gitexec.RunGit([]string{"reset","--hard",sha}, worktree)` wrapper). It has no remaining caller after Card 6 (its only consumer was `chain.go`'s `RestartChain`, now migrated). Keep `HeadSHA`, `ChangedFiles`, `Dirty` and the `gitexec` import (still used by all three).
  - Update `gitquery.go`'s file-leading package doc comment to drop the `ResetHard (the chain-rollback act ...)` clause, leaving the description of the three read-only query helpers intact.
  - In `internal/builderengine/gitquery_test.go`, delete `TestResetHard` (it directly exercised the now-deleted function). Update the file-leading doc comment ("exercises HeadSHA, ChangedFiles, Dirty, and ResetHard") to drop the `ResetHard` mention. Leave `newScratchRepo`, `mustGit`, `commitFile`, and the other three tests intact — those helpers are shared with `chain_test.go`/`spawn_test.go`. `gitquery_test.go` is `//go:build integration`.
  - Deleting these makes builderengine's production source contain `gitexec.RunGit(` only in `gitquery.go`'s three read-only helpers — exactly what the batch-4 regression guard allowlists.
- **Commit:** `refactor(builderengine): delete the raw-gitexec ResetHard now that RestartChain uses Fabric`

### Card 8: Construct the Fabric handle inline in SpawnBatch via a nil-defaulted SpawnDeps seam

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/buildercli/weft.go`
  - `internal/fabricengine/fabric.go`
  - `internal/builderengine/chain.go`
- **Edits:**
  - `internal/builderengine/spawn.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add one field to the `SpawnDeps` struct: `Resetter WarpResetter`. Doc-comment it as the chain-restart reset seam — nil (the production default) makes `SpawnBatch` construct a real `*fabricengine.Fabric` inline via `fabricengine.New(deps.Layout.WorktreeRoot, deps.Layout.WeftWorktree())`; a test injects a `*gitrepo.Repo` fake so the restart path never requires a paired weft fixture.
  - At the `RestartChain` call site (the `if opts.RestartChain {` block, currently `if err := RestartChain(deps.WorktreeRoot, deps.State, deps.Plan, chainEnd, deps.ReportsDir); err != nil {`), resolve the resetter first: use `deps.Resetter` when non-nil; otherwise construct `f, err := fabricengine.New(deps.Layout.WorktreeRoot, deps.Layout.WeftWorktree())` and `return nil, err` on failure (SpawnBatch returns `(*SpawnResult, error)`), then use `f`. Call `RestartChain(resetter, deps.State, deps.Plan, chainEnd, deps.ReportsDir)`. The member-strand stop loop above and the `SaveState` below are unchanged; `deps.WorktreeRoot` remains in use elsewhere (e.g. `HeadSHA(deps.WorktreeRoot)`) so its variable/field stays.
  - Add the `github.com/Knatte18/loomyard/internal/fabricengine` import to `spawn.go`. `hubgeometry` is already imported (for `Layout`). Reference pattern: `internal/buildercli/weft.go`'s `weftCommit` builds `fabricengine.New(layout.WorktreeRoot, weftWorktree)` from a `*hubgeometry.Layout` identically. In production every hub-managed builder worktree has a paired weft on disk, so `New` succeeds; a missing weft surfaces as a real error (accepted behavior change).
- **Commit:** `refactor(builderengine): construct chain-restart Fabric handle inline via SpawnDeps.Resetter seam`

### Card 9: Inject gitrepo resetters in the chain and spawn integration tests

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/reset.go`
  - `internal/builderengine/chain.go`
  - `internal/builderengine/spawn.go`
- **Edits:**
  - `internal/builderengine/chain_test.go`
  - `internal/builderengine/spawn_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/builderengine/chain_test.go`, change all three `RestartChain(...)` calls to pass a `*gitrepo.Repo` as the first argument instead of the `worktree` string: `builderengine.RestartChain(gitrepo.New(worktree), st, plan, 4, reportsDir)` in `TestRestartChain`, and the analogous change in `TestRestartChain_ChainlessErrors` and `TestRestartChain_UnrecordedAnchorErrors`. `TestRestartChain` still asserts `HeadSHA(worktree) == anchor`, so the injected handle must be a real `gitrepo.New(worktree)` performing an actual reset. Add the `github.com/Knatte18/loomyard/internal/gitrepo` import.
  - In `internal/builderengine/spawn_test.go`, add `Resetter: gitrepo.New(worktree)` to the `builderengine.SpawnDeps{...}` literal inside `newSpawnFixture` (`worktree` is the fixture's scratch repo). This gives every test built on `newSpawnFixture` — including all `RestartChain: true` SpawnBatch tests (those in `TestSpawnBatch_RestartChainPersistsStateBeforeSpawn`, `TestSpawnBatch_RestartChainStopsLiveMemberStrands`, `TestSpawnBatch_RestartChainFromNonLowestMemberSpawnsLowest`, `TestSpawnBatch_RestartChainOnChainlessBatchErrors`, `TestSpawnBatch_RestartChainClearsStaleReportBeforeRefusal`, and any other `newSpawnFixture` consumer that sets `RestartChain: true`) — a real reset over the scratch worktree with no weft needed. Injecting unconditionally at the fixture level covers every such test regardless of enumeration; without it, the migrated `SpawnBatch` would construct `fabricengine.New(...)` against the fixture's empty-`Hub` Layout and fail. Add the `github.com/Knatte18/loomyard/internal/gitrepo` import.
  - Both files are `//go:build integration` (they use `newScratchRepo`), so they are exempt from the Test Tier Purity guard and from the batch-4 production-source regression guard. Do not weaken or delete any existing assertion.
- **Commit:** `test(builderengine): inject gitrepo WarpResetters into the chain-restart integration tests`

## Batch Tests

`verify: go test -tags integration -run 'TestRestartChain|TestSpawnBatch|TestHeadSHA|TestChangedFiles|TestDirty|TestChainMembers|TestChainEndFor' ./internal/builderengine/` compiles the package (proving the production migration and the `ResetHard` deletion build) and runs the chain/gitquery/spawn suites. `TestRestartChain` and the four `RestartChain: true` SpawnBatch tests that reach the reset (`RestartChainPersistsStateBeforeSpawn`, `RestartChainStopsLiveMemberStrands`, `RestartChainFromNonLowestMemberSpawnsLowest`, `RestartChainClearsStaleReportBeforeRefusal`; `RestartChainOnChainlessBatchErrors` errors out before the reset call) exercise the real reset through the injected `WarpResetter`; `TestHeadSHA`/`TestChangedFiles`/`TestDirty` confirm the surviving `gitquery.go` helpers still work after `ResetHard`'s removal. Scoped by `-run` to the suites this batch touches.
