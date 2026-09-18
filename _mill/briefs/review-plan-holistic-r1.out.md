MILL_REVIEW_BEGIN
# Review: Replace reed's header pane with a status-line and Selvage — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-09-18
```

## Findings

### [BLOCKING:scope] Card 30's Context omits the two files its own Requirements text cites
**Location:** batch 5 (watchdog-daemon), card 30 (`add the daemon's worktree discovery`)
**Issue:** Requirements has `enterSession` build `reedengine.New` — declared in `internal/reedengine/lock.go` — and justifies not starting a goroutine for a disabled watchdog by describing `watchLoop`'s behavior and calling `go eng.Watch(ctx)`, both declared in `internal/reedengine/watchloop.go`; neither file is in card 30's `Context:` (`lyxcwd.go`, `hubgeom.go`, `config.go`, `server.go`, `logger.go`, `CONSTRAINTS.md`) or `Edits:` (`reedcli/watchdog.go`). Card 31, which also calls `Engine.Watch`, correctly lists `watchloop.go` in its own Context — card 30 is the outlier.
**Fix:** Add `internal/reedengine/lock.go` and `internal/reedengine/watchloop.go` to card 30's `Context:` list.

### [NIT:consistency] Unused "Rename mechanic" section in selvage-pane batch
**Location:** batch 3 (selvage-pane)
**Issue:** The batch carries a `## Rename mechanic` section describing the `git mv` + surgical-edit approach, but every card in the batch declares `Moves: none` — nothing is actually moved in this batch.
**Fix:** Drop the section, since the criterion only requires it when a `Moves:` entry is non-empty.

## Verdict

REQUEST_CHANGES
Card 30's Context list is missing two files its own Requirements text depends on; everything else checked against source held up.
MILL_REVIEW_END
