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

### [BLOCKING:design] Standalone watcher seam cannot carry the decision
**Section:** `### standalone-runs-the-watch-loop-in-process`
**Issue:** The named seam is `reedUp func() error` (`internal/burlercli/cli.go:41`, `internal/webstercli/cli.go:69`) — it takes no context, so "cancelled by that run's own context" has no way in from wiring, where no ctx exists; and in webster the seam has **two** callers, `run.go:104` and `recoverbatch.go:129`, the latter a short-lived verb that returns right after spawning a cold recovery strand, so a watcher bound to its context dies immediately while the session lives on.
**Fix:** State the seam's new signature (does `reedUp` take a `context.Context`, or does a second seam start the watcher?) and decide `recover-batch`'s disposition — start no watcher there, or accept a watcher that ends with the verb.

### [BLOCKING:design] A departing session's watcher is never cancelled
**Section:** `### daemon-discovers-worktrees-by-scanning-the-hub` / `### watch-loop-internals-are-rehosted-not-rewritten`
**Issue:** Discovery is decided only for *newly-appeared* names; `Engine.Watch` "never returns while ctx is live" (`watchloop.go:147-154`), so a worktree whose session goes away while siblings remain keeps a goroutine polling a dead session forever — and `watchLoop` reads `cfg.Watchdog` once at start (`watchloop.go:168-171`), so the "daemon re-reads a worktree's config when it (re)enters the watched set" claim is unreachable if nothing ever leaves.
**Fix:** Decide the per-worktree teardown half: on a name dropping out of the live set, cancel that worktree's context and drop its engine, so re-entry is a fresh `Watch` with a fresh config read.

### [BLOCKING:design] Windows/psmux disposition of the new status-line options unstated
**Section:** `### status-line-content-and-pins`
**Issue:** Four new options (`status-position`, `status-left`, `status-left-length`, `window-status-format`) are all live-verified against tmux 3.6 only, while reed's Windows multiplexer is psmux, whose support this module treats as unverified and branches on explicitly (`windowsize.go:135-143`, `reapply.go:54-55`); since every `set-option` failure is Warn-only, an unsupported option means the identity text silently disappears on Windows, where today's header pane shows it.
**Fix:** State the Windows disposition — verified-supported, or an accepted degrade named as such (and whether `status-position` falling back to `top` is acceptable given the band pane is bottom-most).

### [BLOCKING:scope] "Every header-adjacent file" inventory is incomplete
**Section:** `## Testing` (smoke dispositions) and `## Scope` (In)
**Issue:** The inventory claims exhaustiveness ("Disposition for every existing header-adjacent smoke file, decided rather than deferred") but at least these are unlisted: `internal/reedcli/smoke_lifecycle_test.go` (four `st.HeaderPaneID` assertions plus `pollPaneContains(..., "hub: "+HubPath)` against the header pane, which becomes unassertable once the text moves to a tmux option); `internal/reedengine/testmain_test.go` and `internal/reedcli/testmain_test.go`, whose `os.Args[1] == "reed"` keepalive stand-ins exist only for the deleted re-exec, together with `cmd/lyx/tiersleep_test.go`'s two `allowedLongSleepers` entries citing `lyx reed header --blocking`; `cmd/lyx/stencilseedgate_test.go:82-85`, which pins the annotation on `reed header` by name; `.gitattributes:29`, which names `console-header.md`; and `internal/loomcli/bootstrap.go:183`, whose comment points at `headerpane.go`.
**Fix:** Re-derive the inventory by a stated method (a repo-wide grep for `reed header` / `HeaderPaneID` / `console-header`) and record a disposition per site, rather than enumerating from the reed packages alone.

### [NIT:design] Receiver field for the spawn left as an either/or
**Section:** `### daemon-spawn-is-owned-by-reedcli`
**Issue:** "stores the resolved `*lyxcwd.Location` (or just its `HubPath`)" leaves the plan writer a choice the discussion elsewhere makes for them.
**Fix:** Pick one.

## Verdict

REQUEST_CHANGES
Four gaps: standalone seam shape, watcher teardown, Windows disposition, incomplete call inventory.
MILL_REVIEW_END
