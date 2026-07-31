# Batch: reedengine-adoption

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: reedengine-adoption
number: 9
cards: 2
verify: go test ./internal/reedengine/...
depends-on: [5, 8]
```

## Batch Scope

Applies the `## Shared Decisions` → "Adoption is an audited stop-rule" instruction to `internal/reedengine/lifecycle.go`'s currently-unlogged error-return paths, per discussion.md's `adoption-scope`. Depends on batch 5 (the dual-handler fan-out must exist for a new `logger.Warn`/`Info` call to actually reach the durable sink) and batch 8 (this batch's boot-path edits sit immediately alongside the `LYX_TRACE_ID` filter Card 32 added).

## Cards

### Card 34: Boot-path adoption

- **Context:** none
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Apply the `adoption-scope` done-criterion to `internal/reedengine/lifecycle.go`'s boot path. Research-verified candidate sites, each currently returning/discarding an error with no `logger` call, alongside the sibling sites that already comply (for level-policy consistency):
  - `lifecycle.go:301-303` — `reapSocketProcesses()` failure before boot: wraps and returns `fmt.Errorf("stale tmux socket-holder: %w", err)` with no log call. Add `logger.Warn` naming the socket and the underlying error (retry/fallback-adjacent: this failure aborts the boot attempt).
  - `lifecycle.go:313-315`, `:322-327`, `:331-333` — `os.MkdirAll(logsDir, ...)` and the three `pruneServerLogsLocked` calls: each wraps and returns with no log call. Add `logger.Warn` at each, naming `logsDir` and the failing operation.
  - `lifecycle.go:357-359` — `cmd.Start()` failure inside `spawnSession`: returns `fmt.Errorf("start tmux: %w", err)` with no log call, directly beside the existing success-path `logger.Info` at line 360. Add a sibling `logger.Warn` naming the socket/session and the error, so the spawn attempt is observable on both outcomes, not only success.
  - `lifecycle.go:393-395` — `e.tmux.hasSession` error inside the boot poll loop: returns immediately with no log call. Add `logger.Warn` naming the attempt number and error (this is a retry-loop body, so per `level-policy`'s loop-body hard rule, log the state change — the poll failing this attempt — not every iteration if the same failure recurs across attempts without new information; a reasonable implementation logs once per distinct error message or once per terminal exhaustion, not unconditionally per attempt).
  - `lifecycle.go:406-408` — the "tmux up but session never materialized" branch, sibling to the existing zombie-boot `logger.Warn` at line 409 but itself uninstrumented. Add `logger.Warn` here too, naming the attempt and socket.
  - `lifecycle.go:413-414`, `:416-418` — attempt/deadline exhaustion errors: no log call. Add `logger.Warn` naming the total attempts made and elapsed time before the boot gives up.

  Re-read the full boot path in `lifecycle.go` against the `adoption-scope` stop-rule while making these edits — the list above is a verified floor, not a ceiling; add a call to any further qualifying site the audit finds, and add nothing to a site that already wraps and propagates identifying context (per the criterion's negative case).
- **Commit:** `feat(reedengine): adopt logger.Warn across the tmux boot path's unlogged error returns`

### Card 35: Teardown-path adoption

- **Context:** none
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Apply the same done-criterion to `internal/reedengine/lifecycle.go`'s header-pane and teardown paths:
  - `lifecycle.go:576-578`, `:599-604` — header-pane split/send-keys errors: wrap and return with no log call. Add `logger.Warn` naming the pane ID and the failing tmux operation.
  - `lifecycle.go:584` — `_ = e.tmux.run("kill-pane", ...)`: error explicitly discarded as best-effort, with no log call. Per `adoption-scope`'s criterion (a) ("the returned error is discarded, swallowed by a fallback, or retried"), a discarded error still qualifies — add `logger.Debug` (not `Warn`: this is routine best-effort cleanup, not a notable failure) naming the pane ID, so the discard is at least observable at the step-trace level without upgrading it to a Warn it does not deserve.
  - `lifecycle.go:825` — `_ = e.tmux.run("kill-session", ...)` in `Down`: same discarded-error shape as `:584`. Add `logger.Debug` similarly.
  - `lifecycle.go:839` — `_ = e.tmux.run("kill-server")`: same shape, immediately beside the existing `logger.Warn` at line 842 for `ensureServerGoneLocked`'s failure. Add `logger.Debug` here for the discarded `kill-server` error itself (the Warn at 842 already covers the more significant "server did not confirm gone" outcome — do not duplicate that at Warn level for this earlier, best-effort call).

  Existing suite must stay green — this is instrumentation only, no behavior change (discarded errors stay discarded; only a log call is added alongside each). Per discussion.md's "Adoption-pass tests" Testing bullet, no new assertions on specific log-line wording are wanted (they pin phrasing and rot) — the exception is a previously-silent path that becomes newly observable, where one assertion that the path logs (not what it says) is worth adding; use judgment on whether any of these sites warrant that one assertion, given internal/reedengine's existing test suite already exercises boot/down paths.
- **Commit:** `feat(reedengine): adopt logger calls across the header-pane and teardown paths`

## Batch Tests

`verify: go test ./internal/reedengine/...` — the existing suite (boot, down, header-pane tests) must stay green with no assertion changes beyond whatever Card 35 judges worth adding for a newly-observable path.
</content>
