# `loom` — independent review, round 4 (`opus5-high-r4`)

Reviewer: Opus 5, high reasoning effort.
Scope: thread A (converged, regression-alertness only), thread B (the three bootstrap/crash-recovery commits `d0e5a0e7b`/`aba2c270a`/`69886823e`), thread C (open adversarial pass over loom's driver bootstrap + crash-recovery machinery).
Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-refshape-registry`, branch `crucible-loom-refshape-registry`, HEAD at review start `c8e948d13`.

Clean-room: no prior `_mill/loom-review-*` file was opened before this report's findings list was complete.

## Executive summary

_(written last)_

## Scope assessment — plan-vs-shipped

_(written last)_

## Findings

_(appended as formed)_

## What was tested

_(appended as each command/scenario returns)_

### Environment

- `tmux` present at `/usr/bin/tmux`; `go` at `/usr/bin/go`; `gcc` at `/usr/bin/gcc` (cgo build prerequisite per `CONSTRAINTS.md`'s Quarry CGO Requirement Invariant satisfied).

### Hermetic gates (round-start baseline)

- `go build ./...` — exit 0.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...` — exit 0, no diagnostics.
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/... ./cmd/lyx/...` — exit 0, every package `ok`.
- `go test ./...` (full repo, mandated by the broadened scope) — exit 0. Nothing downstream of the shared `shuttleengine` change (burler, webster, treadle) regressed.
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` — exit 0; `tmux` present, so no case skipped-as-pass. All ten smoke cases ran, including `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` (the case `aba2c270a` rewrote).

### Live fixture (built by hand, driven through the real built binary)

Built a real wired fabric hub with the freshly-built `cmd/lyx` binary rather than a test helper:

- bare warp repo carrying a real Go package tree (`pkg/widget` with `Alpha`/`Beta`/`Gamma` plus a deliberately duplicated `Duplicate` name at two kinds, for a genuinely ambiguous quarry resolve) + an empty bare weft;
- `lyx fabric clone <weft-bare> <warp-bare> --into <dir>` → `ok:true`, full hub wired (`_board`, warp/weft primes, junctions, per-module configs);
- weft config patched to `claude: /nonexistent/lyx-r4-has-no-provider`, `startup_timeout_s: 2`, `discussion_timeout_min: 1` and committed — zero real provider subprocesses, per the cost declaration;
- `lyx fabric add r4task` → `ok:true`, pair created and pushed.

Scenario L1 — cold bootstrap through the real binary (`lyx loom run` in the pair's warp worktree):

- The verb ran all seven steps and reached the tmux handover, which failed only for lack of a TTY (`open terminal failed: not a terminal`), exit 1 — expected for a non-interactive invocation.
- `_lyx/loom/status.json` after: `state=failed`, `current_producer=Discussion-Write`, `error="shedadapters: Discussion-Write (shuttle): shuttle run outcome died"`, `history=[(Preflight,done),(Loom-Preflight,done)]`.
- `.lyx/shuttle/<runID>/run.json` after: `"outcome": "died"`, **`"started": false`** — the new persisted field is written by the real binary and carries the correct value for a launch that never reached `StartupReady`.
- Matches the design: a launch against a nonexistent binary classifies `OutcomeDied` at `startup_timeout_s`, not `OutcomeTimeout` at `discussion_timeout_min`.
- Confirms `aba2c270a`'s claim independently: the failing producer call reached no verdict, so `history` did **not** grow past the two preflight rows — `current_producer`/`state`/`error` carry the failure instead.
