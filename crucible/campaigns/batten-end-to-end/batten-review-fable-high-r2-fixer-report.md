# batten — fixer report, round `fable-high-r2`

Companion to `_mill/batten-review-fable-high-r2.md`.
Every finding recorded there is either fixed below, one commit each, or listed under "Deferred" with the specific reason.

## Implemented (one commit per finding, in landing order)

| Finding | Commit | What changed | Test that would have caught it |
|---|---|---|---|
| F3 (MEDIUM) weft prime admitted by the guard | `dc9690e71` | `armAt` calls `fabricengine.RequireWarpWorktree` ahead of the name check; `refusal.go` doc; Batten Bookend Invariant gains the "prime means the WARP prime" clause in `CONSTRAINTS.md` | `TestBattenIntegration_WeftPrimeRefusal` (integration; all four verbs from `<hub>/<warp>-weft`, no seed written there) |
| F1 (MEDIUM) `status`/`pause` unreadable on a cold machine | `56867e299` | `EnsureStatusLockDir: true` in `specFor`, with the durable-file/ephemeral-lock reasoning | `TestStatusAndPause_ReadADurableStatusWhoseLockDirIsAbsent` (sabotage-proven: fails with the flag flipped back) |
| F5 (MEDIUM) done-slug remedy names the scratch dir | `c6241a96e` | both refusal sites name `shedrun.RunDir` and say it is a weft change | `TestRunCmd_StateDoneRefusesNamingTheRunDir` (also asserts the scratch dir is NOT named) |
| F2 (MEDIUM) non-loom Board type fails late inside the child | `64bd740ed` | new `battenshed.ErrUnsupportedChildRecipe`; the wired `WriteSeed` seam refuses `recipe != loom` before any write; `Seed-Child` lands `Stuck` naming the Board type; overview states the loom-or-empty rule | `TestSeedChild_UnsupportedChildRecipeIsStuck` (no commit/push), `TestWire_WriteSeedRefusesANonLoomChildBeforeTouchingTheWorktree` (order: refusal precedes the worktree resolve) |
| F8 (LOW) no way to restart a halted child from batten, and nothing says so | `63d4f968b` | `haltedChildRemedy` appended to `Run-Shed`'s blocked/paused/failed error; overview paragraph | `TestInnerRun_StuckIsReturnedForRunningAndNoOtherCase` now asserts the remedy on every halted state |
| N2 (NIT) "three producers" + F6 residual docs | `8cc1c2e12` | `doc.go`/`ctx.go`/`stuck.go` count four; the dead-strand residual now also covers a live-but-parked strand | docs |
| N1 (NIT) dead `InnerRunDeps.Now` | `ad8230e08` | seam removed from `deps.go`, `NewInnerRun`, the fake clock, the registry comment and its test | compile-time (`shedrecipe` test renamed `TestInnerRunEntry_NilSleepIsAccepted`) |
| N5 (NIT) write-only `params.slug` | `2f03fd2e8` | auto-seed no longer writes it; `arm_seed_test` asserts absence | `TestArmSeed_RunAndStepSeedBeforeWire` |
| N3 + N4 (NIT) group help / status help | `a811c01b7` | four verbs listed, prime-only sentence names the weft and `_board` refusals; status help states the unseeded-slug refusal | help-tree tests (`cmd/lyx`) |
| F9 + F4 (LOW) docs, F22 extension | `a9f55a847` | overview: teardown debris ("clean and re-step, never `--force`") and inherited run dirs; F22 gains four LLM-free checks (weft-prime refusal, cold-machine status read, done-slug remedy, non-loom Board type) | `cmd/lyx/sandbox_coverage_test.go` stays green |

## Deferred (with the specific reason)

- **F6 — llm driver strand parks on Claude Code's workspace-trust dialog.** The mechanism lives in `internal/loomcli`'s llm arm (it never calls shuttle's `Run.Wait`, which is where `TrustDismissSequence` is played) and `internal/shuttleengine`; both are outside batten's three packages and the round's declared scope. Batten's side is fixed (the residual is documented as dead-or-parked). Needs its own task: play the trust dismissal in `startLLMDriverArm`'s pane-liveness probe, or pre-trust the worktree path at bootstrap.
- **F7 — dev binary does not reach agent panes.** Wiki task `lyx-bin-pane-path`, being fixed on `main` by the operator during this round; not re-reported.
- **R1-F9 recreate-from-branch half** — still blocked on a fabric capability (`Topology.Add` refuses a pre-existing branch); re-confirmed live (A4c, A5), not built.
- **R1-F6 `step`-mode pacing** — still one `poll_interval_s` per step (30.0–30.4 s measured across 80 steps); moving pacing into `shedengine` is out of scope; accepted.
- **Fixture recipe gap** — loom's `Publish` row needs a GitHub-shaped origin; a no-network fixture reaches `Finalize` only with `require_pr_to_base: []` in the child's `landing.yaml`. A campaign-prompt note, not a batten change.

## Test commands run (all green after the last fix, HEAD `a9f55a847`)

```
go build ./...
go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...
go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...
go test -tags integration -count=1 ./internal/battencli/...
go test -count=1 ./internal/shedrecipe/... ./internal/shedverbs/... ./internal/shedcli/...
```
`ok` for every package (`cmd/lyx` 69.5 s under `-count=5`; integration `battencli` 8.6 s).

## Live re-verification (re-deployed dev binary `a9f55a847`, same fixture hub)

- F3: `run|step|status|pause greet-world` from `<hub>/warp-weft` and `status` from `_board` → `this verb runs from the hub's prime worktree only: fabricengine: not a warp worktree: ... is the weft sibling of a pair ...` exit 1; from the warp prime `status` answers as before.
- F1: `.lyx/shed/other/` removed under a durable paused status → `batten status other` `found: true, paused`; `batten pause other` ok; `shed status other` ok.
- F5: `batten run greet-world` (done) names `<hub>/warp/_lyx/shed/greet-world`; removing it and re-running gets past the done gate to `Worktree-Create`, which then blocks on fabric's own leftover-branch refusal (`branch "greet-world" already exists; ... "git branch -D greet-world"`) — a fabric remedy fabric names itself.
- F2: Board `type: batten` → `Worktree-Create` done, `Seed-Child` `blocked` with `stuck_reason: Board task type "batten" cannot be the task worktree's own run: ... only "loom" has a bootstrap verb ...`; child `_lyx/shed/` has no `self`, child weft log stops at `fabric: record parent branch`. (The first attempt surfaced `read Board task type failed: board task "typed-batten" not found` — honest: the review's sabotage-8 raw commit had corrupted prime's `board.yaml`, so the upsert itself had failed.)
- F8: hand-blocked child status under `other` → `Run-Shed` error now ends `... ; the task worktree's own run must be resumed from inside that worktree (its recipe's bootstrap verb, e.g. "lyx loom start") before this run is resumed; resuming this run alone only resumes the watch`.
- Not re-driven: the full real campaign (create → real loom `done` → teardown) — no fix touched `Run-Shed`'s `Done` arm, `Worktree-Teardown`, or the spawn; re-proving it would cost another hour of real agents for no changed code path (the cost declaration's "only if a fix requires it").

## Changed files

`CONSTRAINTS.md`, `docs/overview.md`, `tools/sandbox/SANDBOX-FABRIC-SUITE.md`,
`internal/battenshed/{deps.go,seamchild.go,seamchild_test.go,innerrun.go,innerrun_test.go,doc.go,ctx.go,stuck.go}`,
`internal/battencli/{arm.go,arm_seed_test.go,cli.go,refusal.go,run_test.go,wire.go,wire_test.go,lifecycle_integration_test.go}`,
`internal/shedrecipe/{entries_batten.go,entries_batten_test.go}`.

## Teardown discipline

Fixture hub `scratchpad/fixture` (and the take-2 `fixhub`, `parked`) `rm -rf`'d after re-verification; no tmux server on any `lyx-*` socket; no `lyx` process; the only `claude` processes are the two baseline sessions (1506, 47275) and the operator's own `lyx-bin-pane-path` worktree sessions, none spawned by this round. The operator's standing `lyx-test-LYXHUB` bench was never touched. `~/.claude.json` was not modified (the trust-entry attempt was denied and not retried).
