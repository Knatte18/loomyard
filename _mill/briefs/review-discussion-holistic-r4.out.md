MILL_REVIEW_BEGIN
# Review: Replace reed's header pane with a status-line and Selvage

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Daemon timings declared where the daemon cannot read them
**Section:** `daemon-idle-exit-timings-are-fixed-constants` **Issue:** `watchdogHubDiscoveryCycle`/`watchdogHubIdleCycles` are placed "alongside the existing `watchdog*` timings in `internal/reedengine/watchdog.go`", but those consts are unexported (`watchdog.go:36-53`) while the discovery loop and process lifetime they govern are owned by `internal/reedcli` (Constraints §Told-Geometry). **Fix:** decide the home explicitly — export them from `reedengine`, or declare the two new ones in `reedcli` beside the loop that consumes them.

### [BLOCKING:design] Discovery rationale rests on a false "SessionName is lossy" premise
**Section:** `daemon-discovers-worktrees-by-scanning-the-hub` **Issue:** the scan-every-subdir + `ResolveWorktree` + forward-match design, and the rejection of "parse the worktree back out of the session name", both rest on "`SessionName` is a lossy one-way derivation and deliberately sanitizes" — but hub-mode `reedengine.SessionName` is `filepath.Base(worktreeRoot)` verbatim (`internal/reedengine/server.go:105-107`), sanitization is refused rather than applied in hub mode (`server.go:126-133`), and hub = `filepath.Dir(worktreeRoot)`, so the mapping is exactly `filepath.Join(hub, sessionName)`. **Fix:** restate the rationale against the real derivation and say why a per-candidate scan is still needed (e.g. the anchor marker / `Location` the geometry requires), or adopt the direct join.

### [BLOCKING:design] Lock failure conflated with lock contention
**Section:** `daemon-single-instance-via-hub-lockfile` **Issue:** "a daemon that cannot take the lock logs at Info and exits 0" collapses two distinct answers — `lock.TryAcquireWriteLock` returns `(nil,false,nil)` for contention but `(nil,false,err)` when the path is unusable, and it does not `MkdirAll`, so a hub whose `<hub>/_board/.lyx` is absent silently yields a permanently watchdog-less hub at Info. **Fix:** state who creates `HubScratchDir(hub)` and that a non-nil lock error is a loud failure, distinct from the exit-0 contention path.

### [BLOCKING:decision] Three header smoke files left without a disposition
**Section:** Testing → Smoke/integration **Issue:** `smoke_headerscrollback_test.go`, `smoke_headerseed_test.go` and "the dot-fill/header smokes" (`smoke_dotfill_test.go`, `smoke_dotfill_measure_test.go`, all present) are deferred with "need review one by one — some … should be deleted rather than adapted", which is a named artifact set with no decision. **Fix:** state keep / adapt / delete per file in the discussion, since the ED3-scrollback and stencil-seed subjects are removed by this task's own scope.

### [BLOCKING:consistency] Q&A contradicts the standalone-token decision
**Section:** Q&A log (round-2 gap, `{{.worktree}}` in standalone) vs `worktree-token-joins-the-vocabulary` **Issue:** the Q&A answers "the **normalized** target basename", while the decision requires the **raw** `filepath.Base(target)` matching `reedgeom.go:57`'s `RepoName` — and the raw form is what the testing section's byte-identity assertion for a symlinked spelling depends on. **Fix:** correct the Q&A entry to "raw target basename", or mark it superseded in the same explicit style used for the two other round-2 corrections.

### [NIT:consistency] Superseded wording left in the spawn-sites decision
**Section:** `daemon-spawn-attempted-by-up-resume-and-attach` **Issue:** it still says the three verbs spawn "exactly as they each already call `pinGeometryOptionsLocked`", the precise phrasing `daemon-spawn-is-owned-by-reedcli` corrects as wrong-layer (`pinGeometryOptionsLocked` is engine-internal, `windowsize.go:117`). **Fix:** reword to "on the same three paths" and point at the ownership decision.

### [NIT:design] tmux's window-status segment is undisposed
**Section:** `status-line-content-and-pins` **Issue:** the decision pins `status-left`, `status-right ""` and the length cap, but says nothing about the window list tmux renders between them, which will appear beside reed's identity text. **Fix:** state whether the window-status segment is left at tmux's default deliberately or suppressed.

## Verdict

REQUEST_CHANGES
Four blocking issues: const placement, a false SessionName premise, lock-error handling, and undisposed smokes.
MILL_REVIEW_END
