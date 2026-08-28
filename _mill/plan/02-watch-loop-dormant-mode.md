# Batch: watch-loop-dormant-mode

```yaml
task: 'reed: resume/down leak lock directories at the stale pre-rename session-name path'
batch: 'watch-loop-dormant-mode'
number: 2
cards: 1
verify: go test ./internal/reedengine/... && go vet -tags integration ./internal/reedengine/...
depends-on: [1]
```

## Batch Scope

Batch 1 stops the leak but converts it into noise: the resize watch loop's poll mode calls `reapplyLayout` every two seconds with no gating, so an abandoned session's header pane would log a re-apply failure every two seconds for the rest of its life.
This batch adds a third watch mode — dormant — entered on batch 1's `errWorktreeRootGone` sentinel and nothing else, running at a sixty-second cadence, logging exactly one line on entry and exactly one on recovery, and returning to whichever mode it came from once the worktree root exists again.

It is one batch, and one card, because the timing constant, the mode value, the ticker period, the loop's dormant tick, and the outcome handler's sentinel branch are a single state-machine change: splitting them would leave an intermediate commit whose own tests could not pass.
It depends on batch 1 solely for the sentinel `errWorktreeRootGone`; it adds no error of its own and changes no message batch 1 wrote.

This batch consumes no interface from anywhere else and exposes none.
`Watch` stays the package's only exported watchdog symbol.

Batch-local decision: recovery is triggered by a dormant tick that does NOT return the sentinel, including a tick that returns some other error.
Any non-sentinel outcome means the stat succeeded and the worktree root is a directory again, which is exactly what recovery is about; whether the re-apply then failed for an unrelated reason is the existing error path's business, not dormancy's.

## Cards

### Card 4: A dormant watch mode that backs off on the sentinel and recovers on its own

- **Context:**
  - `internal/reedengine/lock.go`
  - `internal/reedengine/server.go`
  - `internal/reedengine/geometry.go`
  - `internal/reedengine/reapply.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/reapply_test.go`
  - `internal/logger/logger.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/watchloop.go`
  - `internal/reedengine/watchloop_test.go`
  - `internal/reedengine/doc.go`
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/reedengine/watchdog.go`, add `watchdogDormantCycle = 60 * time.Second` to the existing fixed-timings const block, beside `watchdogPollCycle`, with a doc comment stating it is the cadence a watcher runs at once its told worktree root is provably gone, and that it exists so a session abandoned by `down` costs one log line and a minute-scale poll rather than a warning every two seconds forever.

  In `internal/reedengine/watchloop.go`:
  - Add a `Dormant time.Duration` field to `watchTiming` and set it from `watchdogDormantCycle` in `watchDefaultTiming`.
  - Add a third `watchMode` constant `watchModeDormant` after `watchModeSignal`, documented as the mode a watcher enters when `reapplyLayout` reports the told worktree root is provably gone: it neither polls geometry nor consumes signals, it re-tries at the dormant cadence purely so it can notice the directory coming back, and it is the only mode that remembers where it came from.
  - Make `tickerPeriodFor` return `t.Dormant` for `watchModeDormant`, keeping its existing signal and poll answers unchanged.
  - In `watchLoop`, declare a `dormantFrom watchMode` local beside `mode`, and add a `case watchModeDormant:` to the tick's mode switch that calls `e.reapplyLayout(lastApplied, false)`. Pass `false`: a dormant tick must not spend a hook probe, and the mode it returns to on recovery is the remembered one rather than one the probe re-decides.
  - Change `handleWatchOutcome`'s signature to take `dormantFrom *watchMode` alongside the existing `lastApplied *render.Box`, and update its one call site.
  - At the very top of `handleWatchOutcome`, before its existing `if err != nil` branch, handle the sentinel: when `errors.Is(err, errWorktreeRootGone)` is true and `mode` is not already `watchModeDormant`, log exactly one `logger.Warn` naming the socket, the session, and the vanished told worktree root, set `*dormantFrom = mode`, and return `watchModeDormant`. When it is true and `mode` is already `watchModeDormant`, return `watchModeDormant` with no logging at all — a dormant watcher logs nothing while dormant.
  - Immediately after that, handle recovery: when `mode` is `watchModeDormant` and the outcome is NOT the sentinel, log exactly one `logger.Info` saying the told worktree root is back and the watcher is resuming, set `mode = *dormantFrom`, and fall through into the function's existing logic under that restored mode so the tick is handled normally. A watcher that had been promoted to signal mode must come back as signal mode, never be silently demoted to poll.
  - Dormancy must neither advance nor reset the signal-mode retry streak: do not call `state.Failed`, `state.Succeeded`, or `state.Deferred` on the sentinel branch. A vanished worktree root is not a resize-event failure.
  - `watchLoop` must NOT return while ctx is live, and must NOT stop, kill, or otherwise disturb the header pane. The abandoned session may still be hosting the operator's live strand processes.
  - Keep the existing ticker-swap mechanism as-is: the loop already stops and re-creates the ticker whenever `handleWatchOutcome` reports a different mode, and dormancy needs nothing more than that.

  In `internal/reedengine/watchloop_test.go`:
  - Update `TestWatchDefaultTiming_MatchesTheFiveConstants` for the new field, renaming it and its doc comment to say six rather than five.
  - Give `watchdogTestTiming` a `Dormant` value that differs from its `PollCycle` — a small multiple such as fifteen milliseconds against the existing five — so the ticker swap is genuinely exercised while every assertion still lands inside a bounded poll.
  - Add a direct `tickerPeriodFor` test asserting it answers `t.Dormant` for `watchModeDormant`, `t.SignalTick` for `watchModeSignal`, and `t.PollCycle` for `watchModePoll`. This is the deterministic pin on the cadence itself: while dormant the loop refuses before any tmux round trip, so the recording hook observes nothing and cannot measure the interval.
  - Add a poll-mode dormancy test: start the loop against a fixture whose worktree root exists, wait until tmux calls are accumulating, then remove the worktree root directory, then assert that tmux calls STOP accumulating, that the loop does not return while ctx is live, and that exactly one warning line was logged. Capture logs with `logger.SetOutput` over a mutex-guarded buffer, restoring `os.Stderr` in a `t.Cleanup`, following the pattern other packages in this repo already use — the loop writes from its own goroutine while the test goroutine reads, so an unguarded `bytes.Buffer` would race.
  - Add the same test in signal mode, so the per-event retry-streak machinery cannot swallow the transition.
  - Add a recovery test: drive a watcher into dormancy, recreate the worktree root directory, then assert tmux calls resume, that exactly one informational line was logged for the recovery, and that a watcher which had been promoted to signal mode before going dormant comes back as signal mode rather than poll. This is the regression guard for the rename-away, test-the-refusal, rename-back workflow the sandbox suite prescribes.
  - Add a test that a NON-sentinel failure does not go dormant: script the recording hook to fail, and assert the loop keeps re-applying at its existing cadence exactly as it does today. This is the regression guard on the narrowing, and the reason the sentinel is matched rather than "any error from the anchor check".
  - Keep every timing in this file in single- or low-double-digit milliseconds, and add no sleep of a second or longer — the Test Tier Purity Invariant binds these untagged tests.

  In `internal/reedengine/doc.go`, extend the geometry-lifetime bullet card 3 added to the bullet list introduced by the line "Load-bearing behavioral assumptions, each with the rationale that makes it", recording what the watcher does once it learns its told worktree root is gone: one warning, then a sixty-second dormant cadence rather than the two-second poll, automatic return to its previous mode when the directory comes back, and no teardown of the header pane, because the session reed walked away from may still be hosting live strands.

  In `tools/sandbox/SANDBOX-REED-SUITE.md`, add one line to each of the M24 and M25 "Watch:" sections asking the checker to confirm the abandoned session's header pane stopped logging reconcile failures rather than spinning at the two-second poll cadence.
  This line lands here rather than with card 3's no-stray-directory assertions because dormancy is this card's behaviour, and a milestone must not ask a checker to observe something the commit under test does not yet do.
  Do not restate or reword card 3's no-stray-directory assertions, do not add a new milestone, do not renumber existing milestones, and do not change the verdict-summary block at the end of the file.
  Follow the file's existing prose conventions and the repo's semantic-line-break markdown rule.
- **Commit:** `fix(reed): drop the resize watcher to a dormant cadence on a vanished worktree root`

## Batch Tests

`verify:` runs `go test ./internal/reedengine/...`, which covers every untagged test in the package, and `go vet -tags integration ./internal/reedengine/...`, which type-checks the `integration`-tagged files without running them.
The new coverage all lands in `internal/reedengine/watchloop_test.go`: the six-constant timing pin, the `tickerPeriodFor` cadence pin, poll-mode and signal-mode entry into dormancy, recovery back into the prior mode, and the non-sentinel-failure regression guard.

Running the `integration`-tagged tests for real is deliberately NOT part of `verify:`, for the reason recorded in the overview's `a-pre-existing-integration-failure-will-trip-the-done-gate` Decision.
The implementer should run `go test -tags integration ./internal/reedengine/...` once by hand after this card and confirm that `TestWatchdogSelfHeal_HookProbeMatchesLiveTmux` is the ONLY failure — a second failure would be a real regression from this batch.
The live-tmux behaviour this batch changes is covered by the M24 and M25 sandbox-suite line this card adds, not by a Go test: dormancy over a real abandoned session is an operator-observable, agent-driven check.
