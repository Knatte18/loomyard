MILL_REVIEW_BEGIN
# Review: reed: watchdog daemon

```yaml
duration_s: 133.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Invalid `watchdog` value undefined off the Up path
**Section:** `watchdog-single-toggle-no-tunables`
**Issue:** The hard-error validator is only sited in "the same boot path that validates `mouse`" — `ensureServerAndSessionLocked` (`lifecycle.go:176`) — but two other named consumers read the key with no error channel: `AttachArgv` → `pinGeometryOptionsLocked` (`attach.go:80`, a `func()` returning nothing, all-non-fatal), and the header-pane watch loop in `reedcli/header.go`'s `--blocking` tail, which never runs `Up`.
A hard error in the header tail would kill the keepalive, contradicting `watch-loop-failures-are-never-fatal`; a silent choice in `pinGeometryOptionsLocked` leaves install-vs-unset-vs-skip undecided.
**Fix:** State, per consumer, what an invalid value does — which side (install/unset/no-op) `pinGeometryOptionsLocked` takes and whether the watcher declines to start rather than erroring out of the pane.

### [NIT:design] Box-equality guard's box source vs `liveBoxLocked`'s silent fallback
**Section:** `self-apply-does-not-retrigger-and-is-guarded-anyway`, `reapply-layout-is-a-new-public-engine-op`
**Issue:** The guard compares the "live box" against the last successfully-applied box, but `liveBoxLocked` (`windowsize.go:42`) never reports failure — it `logger.Warn`s and returns `render.Box{W: cfg.Width, H: cfg.Height}`, so a degraded query yields a plausible box that can spuriously match (skip forever) or spuriously differ.
`ReapplyLayout()`'s result type is also left as `<result>`, yet the guard needs the applied box back from it.
**Fix:** Say whether the guard consumes the box `ReapplyLayout` returns (and that the result carries it), and whether a fallback box counts as an observation at all.

### [NIT:decision] Signal file has no stated lifecycle outside consume
**Section:** `hook-touches-a-signal-file`, `watchdog-single-toggle-no-tunables`
**Issue:** `<stateDir()>/reed-resize.signal` has a stated creation and consume step but no disposition for a stale file at watcher startup, on `watchdog: off` (the hook is unset, the orphan file is not mentioned), or on `Down`.
**Fix:** State the file's disposition at watcher start, at `off`, and at teardown — even if the answer is "left alone, harmless".

### [NIT:consistency] Roadmap edit described as a "planned entry"
**Section:** `## Constraints` (Documentation Lifecycle bullet) vs `## Problem`
**Issue:** The Problem section correctly calls this a **Someday** item (`manifest/roadmap.md:32`), while the constraints bullet says the roadmap moves "because it completes part of a planned entry"; CLAUDE.md limits roadmap movement to completing or adding a **planned** item, and roadmap Maintenance only documents Planned/Someday → Done on ship, not partial in-place rewrites.
Additionally the entry's text ("a standalone per-worktree daemon") is the shape this task rejects.
**Fix:** Say plainly that the Someday entry's prose is amended in place (resize half shipped, pane-reap remains, host is the header pane, not a standalone daemon) and that no section move occurs.

## Verdict

REQUEST_CHANGES
Invalid-`watchdog`-value behaviour is undefined on the attach and header-pane paths.
MILL_REVIEW_END
