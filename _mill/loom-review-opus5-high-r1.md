# `loom` — independent review, round 1 (`opus5-high-r1`)

> Clean-room round-1 review of the `loom` module per `_mill/loom-review-prompt.md`.
> This file is written incrementally as work proceeds (crash-resilience discipline, see `crucible/README.md`).

## Status

IN PROGRESS — Job A (review) underway.

## Executive summary

_(filled at end of Job A)_

## Scope assessment

_(filled at end of Job A)_

## Findings

_(provisional; appended as spotted — severity and CONFIRMED/PLAUSIBLE per entry)_

### F1 — BLOCKING (CONFIRMED live) — a fast-halting driver makes `lyx loom run` report a false bootstrap failure and skip the terminal handover

`internal/loomcli/run.go:257-262`.

`awaitRunLock` is a deliberate **three-way** result (`awaitRunLockReady` / `awaitRunLockChildDied` / `awaitRunLockDeadline`, `internal/loomcli/bootstrap.go:34-45`),
but `run.go` collapses the last two into one error:

```go
if result != awaitRunLockReady {
    _ = bootstrapLock.Release()
    driverLogPath := loomengine.LoomDriverLog(c.location)
    clihelp.SetExit(ctx, output.Err(out, "loom: driver did not take the run lock; see "+driverLogPath))
    return nil
}
```

`awaitRunLockChildDied` means the driver **ran to completion and exited** before the handshake's first poll — which `run.go`'s own comment at :223-231 calls "the common case".
Every fast-halting run hits it: a blocked `Preflight`, a blocked `Loom-Preflight`, an exhausted bounce budget, a `drive` that errors on the envelope.

Failure scenario, reproduced end to end on a real hub:

- `lyx fabric clone` + `lyx fabric add loom-e2e`, then `lyx loom run` in the pair.
- The detached driver ran, hit `Preflight` `Stuck` (weft dirty), wrote `{"halted_producer":"Preflight","outcome":"blocked",...}` to `.lyx/loom/driver.log`, and exited — **correct behaviour**.
- `lyx loom run` printed `{"error":"loom: driver did not take the run lock; see .../driver.log","ok":false}`.

Two things are wrong: the message asserts something false (the driver did take the lock, did its work, and released it), and step 7 — the tmux attach that is the entire point of the bootstrap — is skipped, so the operator's terminal is never handed to the session that is sitting right there with the status strand in it.
The operator is told the bootstrap broke when in fact their *task* halted, and is given no way to see it.

Fix: branch on the three-way result. `awaitRunLockChildDied` is not an error — it means the driver already finished; fall through to step 7 and attach (the status strand shows the halt), or at minimum report the driver's own recorded outcome rather than a lock claim that is untrue.
Only `awaitRunLockDeadline` deserves the "did not take the run lock" refusal.

### F2 — MEDIUM (CONFIRMED live) — `Preflight`/`Loom-Preflight` discard every failure detail on the one row a human is the only recovery for

`internal/preflightshed/preflight.go:52-58` and `internal/loomshed/loompreflight.go:67-72`.

Both compute a `preflight.Report` carrying `Failures []Failure{Check CheckID, Reason string}` and then discard it:

```go
if !report.OK {
    ...
    return shedengine.Stuck, shedengine.OutputPointer{}, nil
}
```

Neither row carries an `on_stuck` in `contracts/recipes/loom-recipe.yaml` (by design — "a human is the only thing that can fix any of the five"),
so the *only* thing the operator ever sees is the generic Shed text.
Observed on the real run above: `lyx loom status` reported

```
"error": "stuck with no OnStuck target",
"activity": {"now":"Preflight","last":"Preflight → stuck","wait":"stuck with no OnStuck target"}
```

and `.lyx/loom/driver.log` carried exactly one line, the same envelope. Nothing anywhere named `worktree-clean`, `fabric-sync`, or which path was dirty.
The operator has to re-derive the cause by hand.

Fix: log the determined failures (`logger.Warn` with the check IDs and reasons) before returning `Stuck`, in both producers — the same instrumentation posture every other adapter in `shedadapters` already takes on a degraded exit.

### F3 — MEDIUM (CONFIRMED live) — a freshly-added pair has no module config at all, so the shipped `run.sh` launcher fails on first use

`lyx fabric clone` writes the nine module configs into the weft **prime's working tree** but never commits them:
after a clean clone, `git -C <hub>/<name>-weft status --short` reports `?? _lyx/`, and the only commit on the weft primary branch is `fabric clone: initialise weft primary branch main-weft`.

`lyx fabric add <slug>` then branches the weft from that commit, so the new pair's `_lyx/config/` does not exist.
The first `lyx loom run` in the pair fails on the envelope:

```
{"error":"config file .../loom-e2e/_lyx/config/loom.yaml not found; run \"lyx config reconcile\"","ok":false}
```

This is squarely on loom's own bootstrap path — `manifest/designs/loom.md`'s "The run-launcher" section has `lyx fabric add` drop a `run.sh` into the pair's launcher directory whose whole promise is "a double-click shortcut makes this one click".
On a fresh pair that shortcut cannot work.

The defect itself lives in `fabricengine`'s clone (the configs are written but not staged/committed), not in `loom`.
Recording it here because loom is where it surfaces, and because the remedy the error text names (`lyx config reconcile`) is again the bare dry-run verb rather than `--apply`.

## What was tested

Appended after each command/scenario returns.

### Hermetic gates

- `go build ./...` — **PASS** (rc=0, no output).
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/...` — **PASS** (rc=0, no output).
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/... ./cmd/lyx/...` — **PASS**, rc=0, all ten packages `ok`.

### Live driving — the real hub

A fresh, real fabric hub was built for this review (nothing in the operator's own repos was touched):

```
remotes: <scratch>/remotes/loomdrive.git, <scratch>/remotes/loomdrive-weft.git   (bare)
hub:     <scratch>/hubs/loomdrive-HUB
pair:    <scratch>/hubs/loomdrive-HUB/loom-e2e  +  loom-e2e-weft
```

where `<scratch>` is `/tmp/claude-1000/-home-knatte-Code-loomyard-wts-loomyard/e1daed9b-2504-4766-96de-a1a7ca997c32/scratchpad`.
The toy warp repo is a two-file Go module (`greet/greet.go`, `README.md`); the board task `loom-e2e` asks for a `Goodbye` helper beside `Hello`.
`loom.yaml` in the pair was set to `sonnet[effort=medium]` with 20-minute per-role timeouts so a full pipeline run fits in review wall-clock.

- `lyx fabric clone <weft.git> <warp.git>` — OK, after `git symbolic-ref HEAD refs/heads/main` on both bare remotes
  (a bare `git init --bare` leaves HEAD pointing at `master`; `fabric clone` reports this clearly and refuses, which is correct behaviour).
- `lyx board upsert '{"slug":"loom-e2e",...}'` — OK.
- `lyx fabric add loom-e2e` — OK; wrote `_launchers/loom-e2e/run.sh` as documented.
- `lyx loom run` (1st) — **failed on the envelope**: `loom.yaml not found`. See finding F3.
- `lyx config reconcile --apply` in the pair — OK.
- `lyx loom run` (2nd) — driver spawned, ran, halted at `Preflight`, exited; bootstrap reported
  `{"error":"loom: driver did not take the run lock; ...","ok":false}`. See findings F1 and F2.
- `lyx loom status` — round-tripped `slug`/`parent`/`current_producer`/`state`/`activity`/`history_length` correctly.
- `lyx reed status` — `{"ok":true,"session":"loom-e2e","socket":"lyx-loomdrive-HUB-3225f10b","strands":[{"name":"loom-status","live":true,"paneId":"%0"}]}`.

**Operator watch commands for this session** (both take no flags — they resolve from cwd):

```sh
cd /tmp/claude-1000/-home-knatte-Code-loomyard-wts-loomyard/e1daed9b-2504-4766-96de-a1a7ca997c32/scratchpad/hubs/loomdrive-HUB/loom-e2e
lyx reed status     # prints the JSON session/socket/strands
lyx reed attach     # hands this terminal to the live tmux session
```

## Could NOT verify

_(filled at end of Job A)_
