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

### F1 — `classifyStartupWindow`'s `OutcomeDied` ignores a satisfied file contract (MEDIUM, CONFIRMED live)

`internal/shuttleengine/wait.go:401-406` (`classifyStartupWindow`), reached from `wait.go:365`, `wait.go:389`.

`checkLivenessTick`'s own doc comment (`wait.go:325-327`) states the rule plainly:

> A satisfied file contract wins over every negative answer: the agent's output files ARE its return value, so their existence classifies OutcomeDone whether the pane died or reed simply stopped tracking or addressing it.

Its two negative branches honour that — the not-tracked branch (`wait.go:336`) and the not-live branch (`wait.go:342`) both call `allOutputFilesExist` before returning a negative answer.
The third negative answer, the startup window expiring, does not: `classifyStartupWindow` returns `OutcomeDied` on the clock alone.
`manifest/designs/loom.md:344-348` (crash recovery, step 1) says the same thing from the design side — inside a started or attached run's own wait loop, a complete output file means the step finished.

**Failure scenario (reproduced live, not traced).** A run whose pane stays live, whose provider never classifies `StartupReady` inside `startup_timeout_s` (a capture that keeps failing — `wait.go:362` explicitly routes that case here — or a provider whose viewport does not carry the ready markers), and whose `events.jsonl` carries no parseable event, but which HAS written every file in `spec.OutputFiles`.
`pollEventsTick` never classifies it, because it only tests the file contract after successfully parsing NEW event bytes (`wait.go:259`); the startup deadline then classifies the run `OutcomeDied`.

**Observed.** Driven through the real built binary against a real wired hub, with the provider replaced by a script that writes both of Discussion-Write's declared output files and then stays alive without rendering a TUI or appending to `events.jsonl`:

- `_lyx/discussion/decision-record.md` and `_lyx/discussion/support-log.md` both present on disk;
- `events.jsonl` never created;
- `run.json`: `"outcome": "died"`, `"started": false`;
- `_lyx/loom/status.json`: `state=failed`, `error="shedadapters: Discussion-Write (shuttle): shuttle run outcome died"`.

**Cost.** The step genuinely finished, and is recorded as a failure. On the next resume `SingleLLMProducer.Call` finds nothing attachable (the record's `Outcome` is now terminal), so it runs `archiveStaleOutputs` over the two completed files and respawns a fresh agent — finished work archived and redone, which is exactly the outcome the file contract exists to prevent.

**Suggested fix.** Consult the file contract before the deadline turns into `OutcomeDied`, the same way the two sibling branches already do — the check is `allOutputFilesExist(run.spec.OutputFiles)`, already in this file. Placing it on the expiry path alone cannot change behaviour for any run that has not reached its startup deadline.

### F2 — `Wait`'s run-deadline `OutcomeTimeout` ignores a satisfied file contract (MEDIUM, CONFIRMED live)

`internal/shuttleengine/wait.go:206-208` (`Wait`'s deadline branch).

The same asymmetry, one level up and on the other deadline: when `run.deadline` passes, `Wait` finalizes `OutcomeTimeout` without ever asking whether the output files are present.
A run whose agent wrote every declared output file but emitted no parseable terminal event is classified as "the agent was still working when the clock ran out", when in fact its file contract — the thing `manifest/designs/loom.md:340-348` makes the authority on whether a step finished — was satisfied.

This is one defect class with F1, at the campaign's named shape: a check answering a question (*did this run finish?*) using only the fact it happens to hold (*the clock*), while the fact that actually owns the answer (*the output files*) sits one line away and is consulted by every neighbouring branch.

**Suggested fix.** Same as F1: test the file contract before finalizing the negative classification.

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
