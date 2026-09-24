# batten follow-up — opus-medium-r1 fixer report

Review: `_mill/batten-review-opus-medium-r1.md`.
All nine findings fixed, one commit each, none deferred.

## Implemented

| Finding | Sev | Commit | Fix | Regression test (proven failing without the fix) | Live re-proof |
|---|---|---|---|---|---|
| F4 | BLOCKING | `1bd758f2f` | Webster row commits webster's durable directory on Done: `websterengine.DirRel()`, `shedrecipe.Env.CommitWebster` (required by `websterEntry`), `loomshed.NewWebsterProducer(..., commit)`, wired in `loomcli/wiring.go` like `CommitPlan` | `loomshed.TestWebsterProducer_CommitsTheRunRecordOnDoneOnly` (sabotaged commit call → `DoneCommitsOnce` and `CommitFailureIsReturnedError` fail); `CommitWebster` added to the shedrecipe under-filled-Env table and `TestWire_PlanSeamsFilled` | `lyx batten run t8 --child-driver go` ran unattended to `{"halted_producer":"Worktree-Teardown","outcome":"done"}`; `t8-weft` carries `loom: webster run record for t8`; `main` got `a292426 Add t8 comment` |
| F1 | MEDIUM | `ef703936c` | `Run-Shed` spawns when the child has no status OR is `running` with no spawn confirmed; marker `SpawnConfirmedFile(scratchDir, producer)` cleared before each spawn, written only after `Spawn` succeeds; halted/done children never spawned | `TestInnerRun_SpawnsUntilASpawnIsConfirmed`, `TestInnerRun_FailedSpawnIsRetriedOnTheNextCall` (condition reverted to `!found` → both fail); `TestInnerRun_ReentryAgainstExistingStatusDoesNotRespawn` now seeds the marker | `t9`: `LYX_REED_TMUX=/nonexistent/tmux lyx batten step t9` → named error "(resuming this run retries the spawn)"; plain `lyx -v batten step t9` → "spawning inner shed run ... status_found=true", `lyx loom run` driver alive, `Run-Shed-spawned` written; next step spawns nothing (0 spawn lines) |
| F5 | MEDIUM | `993ed9439` | both teardown halves probe `taskWorktreePresent` and treat an absent pair as their post-condition met | `TestWire_TeardownIsIdempotentAgainstAnAlreadyRemovedWorktree` (probes disabled → Shutdown "not present", Remove "not initialized here") | the stranded `t5` (blocked since L9): `lyx -v batten step t5` → both "skipped, the task worktree is already gone" lines, `state: done` |
| F3 | MEDIUM | `894f5df7c` | absent-worktree refusal offers restore-by-hand, or abandon = delete the named run directory + local and remote branches, stating the work loss | `TestTaskWorktreeLocation_AbsentPairIsNamed` extended (run-dir path, "local and remote", loss wording; forbids "so a resumed run reaches a fresh create") | `t7` moved away, status set to `Run-Shed`: new text shown; followed the abandon path verbatim (`worktree prune`, `branch -D` both, `push origin --delete` both, run dir removed and committed) → `lyx batten step t7 --child-driver go` reached a fresh `Worktree-Create` (first attempt tripped fabric's portal-link refusal left by my mv-simulation and fabric's `rollbackAdd` left `t7` behind; after `git branch -D t7`, create done) |
| F2 | LOW | `05b2cca3e` | scan stamps each hit with its enclosing top-level function; carve-out keyed `internal/battencli/arm.go:refuseAdoptedSeed`; CONSTRAINTS.md Driver Choice bullet updated | `TestScanFileForDriverFieldReads_StampsTheEnclosingFunction` | the planted-gate archive copy from L5 now FAILS the tripwire naming `internal/battencli/arm.go:604` |
| F7 | LOW | `af081a157` | `writeSeed` calls `fabricengine.RequireDrivableWorktree` first; overview's shed entry notes it | `TestWriteSeed_RefusesFabricsOwnCheckouts` | `lyx shed seed zz --recipe loom` from `_board` and `warp-weft` → "shedcli: seed refuses to write here: ... not a warp worktree", both checkouts clean |
| F8 | LOW | `05b1668f3` | `battenshed/doc.go` residual names the self-stopping driver and where its report lands | doc only | — |
| F6 | NIT | `0be1f57b4` | reason: "the task worktree already has its own seed, and it disagrees with the one Seed-Child would write: ..." | `TestSeedChild_DisagreeingChildSeedIsStuck` asserts the new text and forbids "disagrees with recipe" | `t7` with a hand seed disagreeing only in `driver` → new reason |
| F9 | NIT | `c901e7257` | `waitOrCancel` / `InnerRunDeps.Sleep` docs say who can cancel (an in-process caller), and that the CLI never does | doc only | — |

Also `70883fb70`: `tools/sandbox/SANDBOX-FABRIC-SUITE.md` F22 gains four lines (F3 abandon path, F5 teardown re-entry, F1 retried bootstrap, F7 seed refusal); `sandbox_coverage_test.go` green.
Docs moved in the same commit as each behaviour: `docs/overview.md` batten entry (F1, F5) and shed entry (F7); `CONSTRAINTS.md` (F2).

## Deferred

None of the nine.
Re-evaluated deferred items (unchanged, still accepted): recreate-from-branch stays absent (F3's text now describes the true manual paths); `step`-mode pacing unchanged; the design residuals remain, with F8's text extension; #263 untouched.

Observation handed to the orchestrator, not fixed (fabric internals, out of this round's scope): when `Topology.Add` rolls back a failed create, `rollbackAdd`'s warp-branch deletion is refused by the destructive gate ("check=ownership") and the branch is left behind, so the next create refuses the pre-existing branch until the operator runs `git branch -D` — seen twice (`t2`, `t7`) after a create failed on a leftover portal link or a remote non-fast-forward.

## Test commands and results (final)

- `go build ./... && go vet ./... && go test -count=1 ./...` — green after every fix commit (final run after `70883fb70`: no non-ok package).
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./internal/shedrun/... ./cmd/lyx/...` — ok.
- `go test -tags integration -count=1 ./internal/battencli/...` — ok.
- `./deploy-dev` re-run after the fixes; live checks above ran on that binary.

## Changed files

`internal/websterengine/state.go`, `internal/loomshed/webster.go`, `internal/loomshed/{webster,cancellation,gatefindings}_test.go`, `internal/shedrecipe/{recipe,entries_simple}.go`, `internal/shedrecipe/{entries_simple,fixture}_test.go`, `internal/shedbuild/fixture_test.go`, `internal/loomrecipe/{fixture,shape}_test.go`, `internal/loomcli/wiring.go`, `internal/loomcli/{wiring,bootstrap}_test.go`, `internal/battenshed/{innerrun,deps,seamchild,doc}.go`, `internal/battenshed/{innerrun,seamchild}_test.go`, `internal/battencli/wire.go`, `internal/battencli/wire_test.go`, `internal/shedcli/seed.go`, `internal/shedcli/seed_test.go`, `docs/overview.md`, `CONSTRAINTS.md`, `tools/sandbox/SANDBOX-FABRIC-SUITE.md`.
