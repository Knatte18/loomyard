# `loom` review — round `sonnet5-xhigh-r8`

Round context: NO ASSIGNED RESIDUAL — a genuine, open, no-residual adversarial safety pass over
loom's driver bootstrap / crash-recovery machinery, per the operator's explicit steer this round.
Thread A (the two original refactors) is CONVERGED — light-touch regression pass only.
Thread B/C's prior six instances of the recurring "negative/terminal outcome finalized without
consulting `allOutputFilesExist`" shape are CLOSED-AND-VERIFIED and protected by a sabotage-proofed
AST tripwire (`completionsignal_enforcement_test.go`) — this round deliberately widens away from
`wait.go`/`attach.go` toward `run.go`'s `Start`, `finalize`, `internal/loomengine`,
`internal/loomcli`, `internal/loomshed`.

This report is being built incrementally per the "Log as you go" requirement — the What-was-tested
section and provisional findings are appended as Job 1 proceeds; only the executive summary and
final severity ordering are written last.

## Executive summary

_(written last)_

## Scope assessment (plan vs shipped)

Thread A (`centralize-glyph-shape-enum`, `quarry-bump-v0-2-0-status-helpers`): CONVERGED per two
prior independent rounds (opus5-high-r1, fable5-high-r2). This round's own light-touch pass
(design-doc re-read of `manifest/designs/quarry-glyph-plan-alphabet.md`, plus a live spot-check —
see What-was-tested) found no regression. Not re-litigated in full.

Thread B/C: this round's mandate is a genuinely open, no-residual adversarial pass over
`manifest/designs/loom.md`'s "Crash recovery" section, deliberately widened away from
`wait.go`/`attach.go` (six instances found and fixed across rounds 3-7, now protected by a
sabotage-proofed AST tripwire) toward the surrounding surface: `shuttleengine/run.go`'s `Start`,
`finalize`, `sweepOrphansOpportunistic`, `internal/loomengine/**`, `internal/loomcli/**`,
`internal/loomshed/**`. Read in full (see below); no shipped-beyond-scope or
deferred-that-should-be-v1 gaps found beyond the two already-accepted, already-settled residuals
(the `AddStrand`/`run.json` crash-mid-registration window, and the done-but-not-persisted window) —
neither reopened by this round's reading or driving.

## Code findings (severity-ranked)

_(provisional — appended as found)_

## Docs & operability findings

_(provisional — appended as found)_

## What was tested

_(appended incrementally, one entry per command/scenario)_

### Environment check
- `which gcc clang go tmux` -> gcc `/usr/bin/gcc`, go `/usr/bin/go`, tmux `/usr/bin/tmux` present (clang absent, gcc suffices). `go env CGO_ENABLED` -> `1`. `go version` -> `go1.26.0 linux/amd64`.
  No environment gap blocks any of this round's scenarios.

### Hermetic suite
- `go build ./...` -> clean, no output.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...` -> clean, no output.
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/... ./cmd/lyx/...` -> all `ok`, no failures/flakes across 5 iterations each.
- `go test ./...` (full repo) -> all `ok`, nothing broken downstream (shuttleengine consumers burlerengine/websterengine/shedadapters all green).

### Smoke suite
- `which tmux` -> `/usr/bin/tmux` present, so a skip cannot masquerade as a pass.
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` -> 14 tests, all PASS in 18.7s.
  Notably includes the prior rounds' own regression tripwires still green:
  `TestSmokeSingleLLM_HarvestsAFinishedRunWithReedStateGone` (round 7's F1 shape),
  `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` (round 6's shape),
  `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` (round 7's F5 race fix).
  Zero real LLM subprocesses observed (log lines show `outcome=died` against the
  `/nonexistent/lyx-smoke-has-no-provider` fixture, as the cost declaration promised).
- Teardown check: `pgrep -af tmux` after the run shows no tmux server process (only my own grep
  invocation matching its own command line, not a hit) -> zero stray tmux confirmed.
- `go test -tags integration ./internal/planglyph/... -run "RealDelta"` -> both real-quarry-backed
  rename-repair tests pass (`TestDetectDrift_RealDeltaGateOneRecognizesDeclaredRename`,
  `TestDetectDrift_RealDeltaExactTierRepairsUndeclaredRename`), confirming thread A's exact-tier
  auto-repair against a genuine git delta and a genuine `quarry.Repo.Resolve` call still holds.

### Static read coverage (thread B/C widened surface) — full-file reads, not skims
Personally read in full: `internal/shuttleengine/{run.go,rundir.go,spec.go,wait.go,attach.go,doc.go}`;
`internal/loomengine/{seed.go,coherence.go,discussion.go,plan.go,status.go}`;
`internal/loomcli/{bootstrap.go,drive.go,run.go,pause.go,status.go,seedinput.go,wiring.go,
landingdeps.go,validate.go,cli.go}`; `internal/loomshed/{loompreflight.go,seed.go,discussionwrite.go,
planwrite.go,webster.go,planvalidate.go,ctx.go,batchifier.go}`; `internal/loomrecipe/loomrecipe.go`;
`internal/websterengine/recordbatch.go`; `internal/preflight/preflight.go`; both `Attach` call sites
in `internal/shedadapters/{bouncer.go,burler.go}` (to confirm the Attach-before-Start/Run ordering
`shuttleengine/doc.go` requires of every caller holds at every call site, not just
`SingleLLMProducer`'s). Cross-checked `contracts/recipes/loom-recipe.yaml`'s 17 rows/routing against
`manifest/designs/loom.md`'s 15-row table and `docs/overview.md`'s module descriptions -- both consistent, no drift.

Two independent forks (same session, clean-room re-briefed to hunt race/wrong-layer-check/
unpersisted-transition/partial-error-path shapes, explicitly told not to re-derive the closed
completion-signal shape) read every remaining production file in `internal/loomcli` and
`internal/loomshed` plus `internal/loomengine`'s remaining files (`review.go`, `prompt.go`,
`report.go`, `config.go`, `configtemplate.go`) not covered above. Both reported no findings.

### Race-detector pass (extra adversarial coverage beyond the prompt's floor)
- `go test -race ./internal/shuttleengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/loomengine/...` -> all `ok`, no races.
- `go test -race -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` -> all 14 smoke tests
  PASS under `-race`, including `TestSmokeBootstrap_ConcurrentSpawnHandshakeYieldsOneDriver`
  (the concurrent-bootstrap-invocation scenario) and the two harvest-shaped tripwires. No races
  flagged anywhere in the widened surface.
