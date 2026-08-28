# Review: reed: attach's layout computation scales header pane height with terminal height

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [NIT:scope] Hook install site list omits the zero-live-pane / unknown-size attach gap
**Section:** Scope ("Hook installation/refresh at the two sites...") and `attach-chain-is-kept-and-its-docs-corrected`
**Issue:** The two named install sites both have an early-return that skips hook install/refresh: `applyLayoutLocked` returns before select-layout when `len(live) < 2` (`apply.go:142-144`, e.g. a session with only the header pane and zero strands), and `AttachArgv` returns the bare argv before entering `withOpLock` when `cols<=0||rows<=0` (`attach.go:65-68`, e.g. `reedcli/attach.go`'s piped-stdout/no-TTY fallback). A session that has never had ≥2 live panes and is first attached with no known terminal size therefore has no window-resized hook installed yet, so a subsequent resize of the underlying pty (the exact mechanism the Problem section's `AttachArgv(0,0)` synthetic repro already demonstrates) can still balloon the header on that one narrow path.
**Suggested fix:** Either extend Scope to note this as an accepted, self-healing known limitation (parallel to the already-documented shrink-below-clamp-threshold limitation in `pins-are-a-snapshot-refreshed-at-every-apply`) — it resolves on the next `reed add` or full-preflight attach — or add hook install to the zero-pane/no-size branches explicitly. Non-blocking: narrow combination (zero strands AND unknown terminal size AND a later real resize), bounded, and self-correcting.

## Verdict

APPROVE
Exceptionally well-grounded discussion (live tmux-verified behavior table, all decisions have rationale + rejected alternatives); one narrow, non-blocking scope gap noted above.
