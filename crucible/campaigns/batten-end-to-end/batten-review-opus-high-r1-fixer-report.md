# batten — fixer report (round `opus-high-r1`)

Companion to `_mill/batten-review-opus-high-r1.md`.
Every finding in that review was fixed. One finding has a deliberately deferred half, recorded below with its reason.

## What was implemented

One commit per finding, on `crucible-batten-end-to-end`, never pushed.

| Finding | Severity | Commit | Fix |
| --- | --- | --- | --- |
| F0 | BLOCKING | `c6e789b60` | `ResolveStatus` creates the child's ephemeral status-lock directory, so `Run-Shed`'s read-before-spawn check no longer hard-errors on every fresh task worktree |
| F1 | BLOCKING | `e9c60eb96` | `childSeedParams` carries the pair's recorded parent branch into the child seed's `params.parent`, so the child's own bootstrap accepts the seed instead of refusing it |
| F2 | MEDIUM | `f8d9b51cc` | `Spawn` captures the child's output; `childSpawnError` folds it into the returned error, capped and explicitly truncated |
| F3 | MEDIUM | `3c11aa679` | `refuseAdoptedSeed` refuses a found seed whose recipe is not batten's |
| F4 | LOW | `3c11aa679` | The same helper refuses an explicitly typed `--driver`/`--child-driver` that disagrees with a seeded run; a defaulted flag is still ignored |
| F5 | LOW | `9130317db` | `PrimeLock.Acquire` ensures the prime lock's own directory rather than relying on a deeper `MkdirAll` |
| F6 | LOW | `a36eec2f9` | `InnerRunDeps.Sleep` takes a context and resolves to `waitOrCancel`, so an operator stop is not held for the poll interval |
| F7 | LOW | `fc3a6140a` | The status envelope bounds `history` and reports `history_length` / `history_truncated` |
| F8 | NIT | `65c740222` | `Spawn`'s contract says which process it waits for; the dead-strand residual is written into `battenshed`'s package doc |
| F9 | MEDIUM | `5b3f10fae` | `taskWorktreeLocation` names an unmaterialized pair and its remedy instead of surfacing a `chdir` ENOENT |
| F10 | MEDIUM | `fc3a6140a` | `status` surfaces the blocked run's producer-supplied `stuck_reason`; `battenshed.StuckReasonFile` is exported so writer and reader share one declarer |
| F11 | LOW | `8bd7d4d0a` | The teardown row records the abandoned session; `status` reads it back, so `step` reports it as well as `run` |
| F12 | MEDIUM (docs) | `c7658d1b1` | F22 and the integration test header say what is really covered, and that no automated test drives a real child bootstrap |
| F13 | NIT (docs) | `12eb36256` | `lyx board upsert --help` lists `type` and `short_name` |
| F14 | — | — | Verified non-defect: a re-driven `Worktree-Create` does not double-create. Pinned live, no code change needed |
| F15 | MEDIUM | `d213843c5` | The CommitStatus seam commits prime's own seed alongside its status, so a resumed machine knows what the run is |

`515d76edf` is a follow-up pass tightening the doc comments this round added, after the user pointed out they were too long and carried edit-history narrative.

## Tests added

All deterministic; no new test spawns a real provider, per this round's cost declaration.

Tier 1 (untagged):

- `TestChildSpawnError` — a child's output is folded into the error, a silent child passes through, over-long output truncates visibly.
- `TestTaskWorktreeLocation_AbsentPairIsNamed` — an unmaterialized pair is named on its own terms.
- `TestRefuseAdoptedSeed` — seven cases over the recipe and both driver flags.
- `TestRecentHistory` / `TestReadStuckReason` — the two new status-envelope behaviours.
- `TestBattenRunCommitPaths` — the seed joins the status once one exists, and only then.
- `TestRecordAbandonedSession` / `TestTeardown_DoneRunRecordsTheAbandonedSession` — the record is written, cleared, and reached from the producer's own Done path.
- `TestWaitOrCancel_*` — deadline-based, never sleep-based, so they hold under `-count=5`.

Integration-tagged, and deliberately **not** stubbing the two seams every other test in that file replaces:

- `TestBattenIntegration_RealReadStatus_OnAFreshPairReportsAbsentRatherThanErroring` — drives the real `ResolveStatus`/`ReadStatus` pair. Verified to fail without F0's fix, with the exact production error.
- `TestBattenIntegration_SeedChild_WritesASeedTheChildBootstrapAgreesWith` — drives `Worktree-Create` + `Seed-Child`, then asserts the seed the child's bootstrap writes is accepted. Verified to fail without F1's fix, with the exact production refusal.

## Deliberately deferred

One item, with its reason — not bucketed as low priority.

**F9's recreate-from-branch half.** The design doc requires batten's rows to "self-heal machine-local resources a durable status promises but a cold machine lacks (recreate the child worktree from its branch before watching it)". The honest-reporting half is fixed; the recreation half is not.

Reason: materializing a pair whose branches already exist needs a fabric capability that does not exist — `Topology.Add` refuses a pre-existing branch by design, which this round confirmed live (`Worktree-Create` on an existing branch blocks, naming `lyx fabric checkout` as the remedy). Building that capability is a change to fabric's own topology behaviour, which this round's scope explicitly excludes, and choosing between adopt-in-place and a new verb is an operator decision about fabric's contract rather than a batten bug fix.

The gap is now legible rather than silent: an operator gets a message naming the run, the expected path, and fabric's own recovery verb.

**F6's pacing half**, noted rather than deferred as a finding: the 30s poll sleep still runs in `step` mode. The producer cannot see which verb drove it, and moving the pacing out of the producer body is a `shedengine` change. What was a defect — the wait being uninterruptible — is fixed.

## Commands run, and results

Hermetic, green throughout and at the end:

```
go build ./...                                                          clean
go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...   clean
go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...
    ok battenshed 0.325s | ok battencli 0.637s | ok battenrecipe 0.025s | ok cmd/lyx 10.183s
go test -count=1 ./tools/sandbox/...                                    ok (sandbox coverage guard green)
go test -count=2 ./internal/boardcli/...                                ok
```

Integration tier:

```
go test -tags integration -count=1 ./internal/battencli/...             ok 7.237s
```

Live driving: see the review's "Live driving" section and its post-fix addendum for the exact commands and observed output. The headline is that `lyx batten` walked create → seed → a real child bootstrap that spawned a real provider in the child's own reed session, which had never happened before this round.

## Changed files

- `internal/battencli/`: `wire.go`, `arm.go`, `cli.go`, `commitstatus.go`, plus `wire_test.go`, `arm_seed_test.go`, `run_test.go`, `commitstatus_test.go`, `lifecycle_integration_test.go`
- `internal/battenshed/`: `deps.go`, `doc.go`, `innerrun.go`, `stuck.go`, `teardown.go`, plus `innerrun_test.go`, `teardown_test.go`
- `internal/boardcli/cli.go`
- `docs/overview.md`, `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- `_mill/batten-review-opus-high-r1.md`, this file

`manifest/roadmap.md` was deliberately not touched: this is a hardening pass, not a planned item completing.
`CONSTRAINTS.md` was not touched either — no invariant moved, and nothing here is a new cross-cutting rule.

## Merge-readiness

**Mergeable.** The two blocking defects are fixed and each carries a regression test proven to fail without its fix; the whole suite is green at `-count=5` plus the integration tier; the live path the module exists to walk was driven end to end for the first time; and all substrate was torn down with zero stray processes.
