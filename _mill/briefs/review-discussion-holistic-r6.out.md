MILL_REVIEW_BEGIN
# Review: Replace reed's header pane with a status-line and Selvage

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (claude-opus-5), per the runtime environment; no independent means to confirm
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Daemon has no API to list hub sessions
**Section:** `daemon-discovers-worktrees-by-scanning-the-hub` **Issue:** The daemon "runs one `list-sessions` round trip against the hub socket" from `internal/reedcli`, but `reedengine` exposes no engine-less session listing — `TmuxCmd.run`/`output` are unexported (`internal/reedengine/overlay.go:47,64`), every exported `*Engine` method is bound to one session (`Watch`, `Up`, `Resume`, `Down`, `Status`, `AttachArgv`, `Socket`, `SessionName`, `TmuxPath`, strand/io verbs), and the daemon has no Engine before discovery. It also never says where the daemon gets `cfg.Tmux` (the binary path) and the socket key with no worktree in hand. **Fix:** Decide and state the new seam — e.g. an exported `reedengine.ListSessions(tmuxPath, socketKey)` plus which `LoadConfig` baseDir supplies `cfg.Tmux` — and note it lands in `manifest/designs/reed-fabric-standalone-api.md`'s public-surface inventory, which this task already edits.

### [BLOCKING:design] Idle-exit premise: error vs empty enumeration
**Section:** `daemon-exits-when-the-hub-server-is-gone-never-on-down` / `daemon-idle-exit-timings-are-fixed-constants` **Issue:** The exit rule is "three consecutive enumerations that find none", but the normal last-`down` case is not an empty list — the tmux server is gone and `list-sessions` fails, so the daemon's main exit path is undefined; conversely a transient round-trip failure counted as "none" exits a daemon whose sessions are alive. `internal/reedengine/proctree_windows.go:4-5` records that under psmux `list-sessions`/`has-session`/`kill-server` "exit identically with and without a server on the socket", so the mapping is platform-dependent too. **Fix:** State explicitly which `list-sessions` outcomes count toward `watchdogHubIdleCycles` (no-server error yes, other errors no — or whatever is chosen) and what answers the Windows indistinguishability.

### [BLOCKING:design] Daemon's durable-log claim is false at a hub cwd
**Section:** `daemon-single-instance-via-hub-lockfile`, `daemon-verb-is-the-new-interactive-handoff-exception` **Issue:** The design leans on "logs at Error to the durable sink" and "the durable handler keeps recording at Info and above" while `logger.SetOutput(io.Discard)` and stdio nil, but the sink's cwd-anchored fallback arms only inside a lyx-owned worktree: `armDurableSinkLocked` returns false when `lyxcwd.Resolve(cwd)` fails or `isLyxWorktree` is false (`internal/logger/sink.go:125-142,191-193`), and `sink.go:189-190` states outright that commands run from the hub never arm it. With `cmd.Dir` pinned to the hub, the daemon's lock-failure Error, spawn Info, and Debug resolve lines all go nowhere. **Fix:** Decide where the daemon points its sink (e.g. an explicit `logger.SetDurableSinkDir` at a hub-anchored logs dir) before it discards stderr, and restate the lock-failure/diagnosis story against that.

### [NIT:design] Spawn placement in `attach` is unstated
**Section:** `daemon-spawn-is-owned-by-reedcli` **Issue:** "Immediately after their engine op returns without error" is unambiguous for `up`/`resume`, but `attach`'s RunE has three engine touches and ends in a blocking `attach.Run()` handover (`internal/reedcli/attach.go:55-83`); a plan writer could place the spawn after the handover, where it fires only on detach. **Fix:** Name the call site for `attach` (after the `Status()` pre-flight, before the handover).

### [NIT:design] `watchdog: off` worktree — parked or never started
**Section:** `watchdog-config-key-keeps-its-meaning-and-scope` **Issue:** "Discovered but not watched" does not say whether the daemon skips the goroutine or starts `Engine.Watch`, which parks until ctx cancel rather than returning (`internal/reedengine/watchloop.go:179-185`) — the two differ in map/cancel bookkeeping. **Fix:** State which, since the re-entry config re-read depends on entries leaving the map.

## Verdict

REQUEST_CHANGES
Daemon's tmux seam, idle-exit signal, and durable-logging premise are unresolved or false.
MILL_REVIEW_END
