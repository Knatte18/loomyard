MILL_REVIEW_BEGIN
# Review: Replace reed's header pane with a status-line and Selvage

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Daemon is never told its hub or its cwd
**Section:** `watchdog-is-one-detached-process-per-hub`, `daemon-discovers-worktrees-by-scanning-the-hub`
**Issue:** `internal/reedcli/cli.go:58-96` resolves every reed verb through a `PersistentPreRunE` doing `lyxcwd.CwdFrom` → `lyxcwd.Resolve` → `hubgeom.ReedGeometry`, so a `watchdog` verb inherits a single-worktree, cwd-bound resolution — yet the process is per-hub, detached, and outlives the spawning worktree (`errWorktreeRootGone` is already a live case); the discussion never says how the hub reaches the daemon, whether `watchdog` opts out of the persistent pre-run, or what `cmd.Dir` is. Both existing detached precedents pass the path explicitly (`boardengine/spawn.go:28` `--board-path`, `fabricengine/spawn.go:66` `--weft-path`).
**Fix:** Decide the `lyx reed watchdog` invocation contract — explicit hub flag vs cwd resolution, its relation to `PersistentPreRunE`, and the child's working directory.

### [BLOCKING:decision] Standalone reed geometry's `WorktreeName` left conditional
**Section:** `worktree-token-joins-the-vocabulary`
**Issue:** "plus the standalone teller if it builds a reed geometry" — it does: `internal/standalonegeom/reedgeom.go:45` `ReedGeometry(target, stateDir, hash8)`, which has no worktree at all (`WorktreeRoot: target`, a plain checkout). With `Build` resolving every registry token unconditionally (`internal/tokenvocab/tokenvocab.go:30`), the new default template `{{.repo}}/{{.worktree}} · {{.hub}}` renders `foo/ · <stateDir>` in standalone mode.
**Fix:** State the standalone disposition for `WorktreeName` — value, or an explicit "standalone renders repo/hub only" template decision.

### [BLOCKING:design] Daemon only ever respawned by `reed up`
**Section:** `daemon-single-instance-via-hub-lockfile`
**Issue:** The rationale claims unconditional-attempt-on-`up` makes the daemon "self-healing after a crash, a logout, or an upgrade", but a crash while sessions stay alive leaves no watchdog until someone runs `up` again — while the status-line half explicitly relies on `attach`/`resume` back-filling via `pinGeometryOptionsLocked`. The asymmetry is unstated.
**Fix:** Decide whether `attach`/`resume` also attempt the spawn, or state explicitly that `up` is the sole spawn site and accept the gap.

### [BLOCKING:consistency] Superseded reconcile rationale left in Q&A log
**Section:** Q&A log, entry "Should the plan remove a stale `header:` block"
**Issue:** Its **Why** still reads "`lyx config reconcile` is additive and key-based", which the later round-1-gap entry and the `config-block-replaces-header-with-status_line-and-selvage` gotcha both explicitly retract as wrong; a plan writer reading top-down takes the wrong premise.
**Fix:** Correct or strike that entry's rationale so the log carries one account of reconcile's behaviour.

### [NIT:decision] Daemon lock path named only approximately
**Section:** `daemon-single-instance-via-hub-lockfile`
**Issue:** "`fabricengine.HubLogsDir(hub)`'s `.lyx` sibling" is self-contradictory — `HubLogsDir` is `<hub>/_board/.lyx/logs`, already inside `.lyx` — and the Durable-vs-Ephemeral State Invariant requires a named scratch accessor rather than a derived path.
**Fix:** Name the exact path and its declaring package (`Geometry.LogsDir` is already told to the engine).

### [NIT:design] Daemon exit grace period unquantified
**Section:** `daemon-exits-when-the-hub-server-is-gone-never-on-down`
**Issue:** "a small number of consecutive empty observations" leaves both the count and the cycle period to the plan writer, and it interacts with the `down` + immediate `up` case the decision cites.
**Fix:** State the count (or the constant's home alongside `watchdog.go`'s existing fixed timings).

### [NIT:scope] `Geometry`'s own doc comment counts the fields
**Section:** `worktree-token-joins-the-vocabulary`
**Issue:** `internal/reedengine/geometry.go:1,15` declares "the eight-field struct"; adding `WorktreeName` makes nine, and the `RepoName`/`HubPath` field comments still say "the header pane's ... token".
**Fix:** Note these doc touch-ups in scope with the other same-commit doc edits.

## Verdict

REQUEST_CHANGES
Daemon invocation contract, standalone worktree token, and respawn coverage are undecided.
MILL_REVIEW_END
