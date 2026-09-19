MILL_REVIEW_BEGIN
# Review: reed: per-hub daemon reaps orphaned sessions

```yaml
duration_s: 118.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-4-class (self-assessed; brief names "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] --shell pre-flight test has no reachable seam
**Section:** Testing → `internal/reedcli` pure, untagged ("The `--shell` pre-flight, asserted as the *inverse* of `--tmux`'s")
**Issue:** The negative cases (`--tmux`/`--hub-path` empty) return before any side effect, but the positive case — "an absent or empty `--shell` starts the daemon anyway" — falls through `watchdogCmd`'s RunE into `logger.SetDurableSinkDir`, a global `logger.SetOutput(io.Discard)`, `lock.TryAcquireWriteLock` under `fabricengine.HubScratchDir` (which `watchdogCmd` does not `MkdirAll`, so a temp hub yields a lock *error* → `output.Err`, i.e. the assertion fails for a reason unrelated to `--shell`), and then `runWatchdogLoop` with `watchdogDefaultTiming()`, whose first non-cancelled tick calls `reedengine.ListSessions` → `exec.Command` in an untagged file (Test Tier Purity Invariant). The discussion's only timing seam is a parameter on `runWatchdogLoop`, which `watchdogCmd` is specified to feed with `watchdogDefaultTiming()` — nothing lets a CLI-level untagged test stop before the loop.
**Fix:** State how the positive `--shell` case is asserted without entering the loop — e.g. assert the pre-flight through a separated pure validator, or name a pre-cancelled `cmd.Context()`/injection point in `watchdogCmd` — or move that assertion to the tagged tier.

### [NIT:design] ReapSession error disposition unstated
**Section:** `ReapSession-is-a-second-engine-less-exported-function` / `the-reap-runs-off-loop-not-inline`
**Issue:** `ReapSession` returns `error`, and `reapSessionTmux`'s `list-panes` failure is explicitly handled, but nothing says what the reap goroutine does with a failed `kill-session` (log level, and whether the name simply re-confirms across three more cycles given the counter was deleted at dispatch).
**Fix:** One line naming the log level for a failed reap and confirming the re-confirmation path is the whole recovery.

### [NIT:consistency] Scope line invites the validator the decision forbids
**Section:** Scope → In ("A `--shell` flag … exactly as `--tmux` already is")
**Issue:** Read alone, "exactly as `--tmux` already is" reads as including `--tmux`'s non-empty pre-flight, which `the-daemon-is-told-its-shell` explicitly rejects; the discussion itself anticipates the mistake by writing a regression test for it.
**Fix:** Qualify the Scope line to "told the same way as `--tmux`, but never required — see `the-daemon-is-told-its-shell`".

## Verdict

REQUEST_CHANGES
One prescribed untagged test cannot run as described; two minor clarifications.
MILL_REVIEW_END
