MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus (environment reports claude-opus-5); self-assessment only, not independently verifiable
reviewed_file: C:\Code\loomyard\wts\trace-logging\_mill\discussion.md
date: 2026-07-31
```

## Findings

### [GAP] Warn+ sink drops the spawn events that motivate the task
**Section:** `dual-handler-fan-out` + `level-policy` vs. Problem/"Why now"
**Issue:** The durable sink is pinned at Warn+, but `level-policy` (and CONSTRAINTS' Live-Substrate Spawn Observability, verified: `lifecycle.go:360` logs the tmux spawn at `logger.Info`) puts spawn/teardown lifecycle at Info — so the 2026-07-30 "what spawned, how many times" record still would not land in the durable file on a default run, only the retry Warns.
**Fix:** State the reconciliation explicitly — durable sink at Info+ (with the retention/spam consequences re-argued), or a rule that spawn/teardown counts emit at Warn, or accept and say why the sink deliberately records only failures.

### [GAP] No trace-ID mint path for entry points that never reach the root hook
**Section:** `trace-id-mint-and-propagate` + `test-entry-activation` + `no-arming-under-test`
**Issue:** Minting/adoption is specified only in `cmd/lyx/main.go`'s `PersistentPreRunE`, which a `go test` binary never runs, yet `test-entry-activation` arms the durable sink under `LYX_TRACE=1` for exactly those binaries; `no-arming-under-test` says all three "activate only through the explicit test seam or `LYX_TRACE=1`" without saying where the mint then happens (logger `init`? first write?) or whether `LYX_TRACE_ID` is adopted there.
**Fix:** Name the second mint/adopt site and its precedence relative to the root hook, so live-substrate test lines carry a real `trace=`.

### [GAP] Trace-ID propagation contradicts "one file per trace"
**Section:** `one-file-per-trace` + `trace-id-mint-and-propagate`
**Issue:** A nested `lyx` re-exec (`boardengine/spawn.go:27`, `fabricengine/spawn.go:62` — both verified to inherit env) adopts the parent's ID, so one trace spans N processes: either N files share one ID (the "one file per trace" name and the anti-interleaving rationale both break), or two processes opening within the same UTC second collide on `trace-<ts>-<id>.log` and interleave into it via `O_APPEND`.
**Fix:** Decide and state it — per-process disambiguator in the filename (pid/counter), or rename the unit to "one file per process, keyed by trace-ID", and say how a reader reassembles a multi-process trace.

### [GAP] `proc.DetachBreakaway` does not touch `cmd.Env`
**Section:** Technical context → "Spawn sites relevant to propagation"; `long-lived-child-env`
**Issue:** The claim that `scoutengine/ensureserver.go:520` touches `cmd.Env` "indirectly, via `proc.DetachBreakaway`" is false — `internal/proc` sets `SysProcAttr` only (no `cmd.Env` anywhere in the package), so that spawn inherits `os.Environ()` implicitly and has no filtering chokepoint like reed's `CleanClaudeEnv`.
**Fix:** Correct the claim and size the work as adding a new explicit `cmd.Env` assignment plus a scout-side env filter, not editing an existing filter.

### [NOTE] `SetOutput`'s semantics under the composite handler are unstated
**Section:** `dual-handler-fan-out`, Testing → "Test seam required"
**Issue:** `SetOutput` today rebuilds the whole logger (`logger.go:140-143`); under fan-out it must rebind only the stderr half or it silently drops the durable handler — the discussion adds a new durable seam but never restates `SetOutput`.
**Fix:** One sentence pinning `SetOutput` to the stderr handler only, and say whether `LYX_LOG_FILE` output duplicates Warn lines already in the trace file.

### [NOTE] Concurrency contract omits the non-sink globals
**Section:** `concurrency-contract`
**Issue:** It covers the sink's open/counter/marker/write, but not the arming flag, the seam-injected directory, or the existing `out`/`log` globals, which are written by the root hook or test seams and read from the three verified production goroutines (`lifecycle.go:1091`, `lspclient.go:224`/`:241`).
**Fix:** State that arming/seam writes happen-before any emit, or put them behind the same mutex.

### [NOTE] Adoption pass has no done-criterion
**Section:** `adoption-scope`
**Issue:** "Audit error-return paths that swallow context" across six packages (perch greenfield, three near-greenfield) gives a plan writer no boundary for the task's largest work item.
**Fix:** Add a rough per-package target (e.g. call-site count or the specific paths) or a stop rule.

## Verdict

GAPS_FOUND
Four gaps: sink level vs. spawn events, mint site off the CLI path, multi-process trace files, false env claim.
MILL_REVIEW_END
