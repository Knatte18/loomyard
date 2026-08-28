# Plan: reed: watchdog daemon

```yaml
task: 'reed: watchdog daemon'
slug: 'reed-watchdog-daemon'
approved: false
started: '20260828-093722'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: watchdog-foundations
    file: 01-watchdog-foundations.md
    depends-on: []
    verify: go test ./internal/shell/... ./internal/reedengine/...
  - number: 2
    name: engine-reapply-op
    file: 02-engine-reapply-op.md
    depends-on: [1]
    verify: go test ./internal/reedengine/...
  - number: 3
    name: watch-loop
    file: 03-watch-loop.md
    depends-on: [2]
    verify: go test ./internal/reedengine/...
  - number: 4
    name: cli-tail-docs-and-live-proof
    file: 04-cli-tail-docs-and-live-proof.md
    depends-on: [3]
    verify: go test ./internal/reedcli/... ./internal/reedengine/... ./cmd/lyx/... && go vet -tags integration ./internal/reedengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: engine-hosts-the-loop-cli-only-calls-it

- **Decision:** the watch loop's body lives entirely in `internal/reedengine`, exposed as exactly one new exported symbol, `Engine.Watch(ctx context.Context) error`.
  The re-apply op (`reapplyLayout`), the try-lock (`withTryOpLock`), the state machine (`watchState`), and every helper stay package-internal.
  `internal/reedcli/header.go` calls `Watch` and nothing else.
- **Rationale:** every seam the loop touches is unexported engine state — `e.tmux` and its `execHook` test seam, `stateDir()`, `e.cfg`, `liveBoxLocked`, `applyLayoutLocked`, and the op lock.
  Hosting the loop cli-side would mean exporting all of it, inverting the CLI/Cobra Invariant's direction of knowledge.
- **Applies to:** all batches

### Decision: watch-loop-failures-are-never-fatal

- **Decision:** no code path reachable from `Engine.Watch` may return an error out of the header pane, write to stdout/stderr, call `output.Err`, or panic.
  Every failure is `logger.Warn`-ed (or `logger.Debug`-ed for a polling-probe round trip) and swallowed.
  `Watch` itself never returns while its context is live — including the disabled cases, where it parks on `<-ctx.Done()` rather than returning early.
- **Rationale:** the header pane exists to be an always-on session keepalive.
  A watchdog able to kill it would convert a cosmetic layout failure into a session-survival failure.
- **Applies to:** all batches

### Decision: per-event-retry-cap-not-a-loop-cap

- **Decision:** the bounded thing is one debounced event's retries — at most `watchdogMaxAttempts` (3) attempts with an escalating delay — never the watcher.
  Both the attempt counter and the escalating delay reset on a successful apply **and** on the arrival of the next resize signal.
  Exhausting a streak abandons that one event with a single log line; the loop keeps running and stays responsive to the next signal.
  Poll mode does not use the debouncer or the retry streak at all: each `watchdogPollCycle` cycle is one attempt and the cycle interval is its own cadence.
- **Rationale:** this satisfies the Live-Substrate Spawn Observability rule that a retry loop caps attempt COUNT literally, without self-heal ever silently stopping.
  Poll mode is a steady-state reconcile rather than a retry loop around a spawn, so the cap does not apply to it; capping it would stop self-heal permanently on the fallback platform.
- **Applies to:** batch 3, batch 4

### Decision: five-fixed-constants-no-tunables

- **Decision:** the loop's timings are compile-time constants in `internal/reedengine/watchdog.go`, none configurable: `watchdogDebounceQuiet` = 200ms, `watchdogSignalTick` = 100ms, `watchdogPollCycle` = 2s, `watchdogMaxAttempts` = 3, `watchdogRetryBaseDelay` = 200ms.
  Tests reference the constants, never the literals.
  The loop body reads its timings from a `watchTiming` struct so tests can drive it fast; production builds that struct from these constants alone via `watchDefaultTiming()`.
- **Rationale:** the discussion fixes the first four as exact values, and `watchdogRetryBaseDelay` is the concrete escalation base its "escalating delay" phrase requires — the attempt-`n` delay is `watchdogRetryBaseDelay << (n-1)` (200ms then 400ms).
  Referencing the constants means a later tuning change moves one line and does not break the suite.
- **Applies to:** batch 1, batch 3

### Decision: watchdog-invalid-value-has-three-different-answers

- **Decision:** `watchdogOption(raw string) (bool, error)` is the single validator — pure, trimmed, lowercased, `on`/`off` only, everything else including the empty string returning an error.
  Its three consumers react differently and deliberately: `ensureServerAndSessionLocked` returns the error (hard boot failure naming the value); `pinGeometryOptionsLocked` treats the error as `off`, `logger.Warn`s, and takes the unset side; `Engine.Watch` treats the error as `off`, `logger.Warn`s, and parks.
- **Rationale:** two of the three consumers have no error channel, and a hard error in the header tail would kill the keepalive.
  Exactly one consumer is loud, and it is the one the operator is watching.
- **Applies to:** batch 1, batch 2, batch 3

### Decision: no-unlocked-tmux-query-anywhere-in-the-watcher

- **Decision:** the watcher never issues a tmux round trip of its own in either mode.
  Every tmux interaction it causes happens inside `reapplyLayout`, under that op's non-blocking try-lock — including the box query and the hook-availability probe.
  `reapplyLayout` owns the box-equality guard internally and hands its answer back on `ReapplyResult`.
- **Rationale:** `liveBoxLocked` documents "assumes the op lock is already held"; an unlocked watcher-side query would violate that contract.
  Folding the comparison and the probe into the op gives both modes one code path and one answer for the degraded-box case.
- **Applies to:** batch 2, batch 3

### Decision: no-new-cli-command-and-no-new-package

- **Decision:** this task adds no cobra command, no `internal/reed*` package, and no new module.
  `cmd/lyx/helptree_test.go`, `cmd/lyx/seamsignature_test.go`, and `cmd/lyx/registration_test.go` must stay green unchanged.
  The only CLI-visible change is `reed header`'s `Long` text.
- **Rationale:** the watcher's whole lifecycle is the header pane's lifecycle; there is nothing for an operator to invoke.
- **Applies to:** all batches

### Decision: go-conventions-and-comment-style

- **Decision:** follow the surrounding package exactly — godoc on every new exported and unexported top-level symbol, a file-header comment on every new file stating what the file owns and why, and the existing habit of recording the *rationale* that makes a behaviour load-bearing rather than restating the code.
  New tmux calls go through `TmuxCmd.run`/`TmuxCmd.output` only, never a fresh `exec.Command`.
  Every session `-t` target is built with `exactSessionWindowTarget`.
- **Rationale:** `internal/reedengine` is unusually heavily documented and the review discipline expects it; a bare new file would read as foreign.
- **Applies to:** all batches

### Decision: constraints-this-task-must-not-break

- **Decision:** the following CONSTRAINTS.md invariants are live for every batch — Told-Geometry (no `internal/lyxcwd` import in `reedengine`; the watcher derives no coordinates), Durable-vs-Ephemeral State (the signal file is reached only via `Engine.stateDir()`), Shell Mechanics Seam (the hook's shell fragment is built only via `internal/shell`), Config Strictness (the new key exists in **both** embedded templates), Test Tier Purity (no `exec.Command` and no `time.Sleep(...)` ≥ 1s in an untagged test file), Live-Substrate Spawn Observability, Sandbox Suite Coverage, and CLI/Cobra.
- **Rationale:** every one of them is machine-enforced by a guard in `cmd/lyx/`, so a violation is a red build rather than a review note.
- **Applies to:** all batches

## All Files Touched

- `internal/reedcli/header.go`
- `internal/reedcli/header_test.go`
- `internal/reedengine/apply.go`
- `internal/reedengine/apply_test.go`
- `internal/reedengine/config.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/lifecycle_test.go`
- `internal/reedengine/lock.go`
- `internal/reedengine/lock_test.go`
- `internal/reedengine/reapply.go`
- `internal/reedengine/reapply_test.go`
- `internal/reedengine/template_posix.yaml`
- `internal/reedengine/template_windows.yaml`
- `internal/reedengine/watchdog.go`
- `internal/reedengine/watchdog_integration_test.go`
- `internal/reedengine/watchdog_test.go`
- `internal/reedengine/watchloop.go`
- `internal/reedengine/watchloop_test.go`
- `internal/reedengine/windowsize.go`
- `internal/reedengine/windowsize_test.go`
- `internal/shell/posix.go`
- `internal/shell/pwsh.go`
- `internal/shell/shell.go`
- `internal/shell/shell_test.go`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-REED-SUITE.md`
