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

_(to fill in during/after review)_

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
