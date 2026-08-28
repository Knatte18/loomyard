MILL_REVIEW_BEGIN
# Review: reed: watchdog daemon — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-28
```

## Findings

### [BLOCKING:design] reapplyLayout re-probes the hook on every signal-mode apply
**Location:** Batch 2 Card 9 (`reapplyLayout`/`hookInstalledLocked`) and Batch 3 Card 16 (mode-promotion rule)
**Issue:** discussion.md's `hook-availability-decides-poll-fallback` decision states, repeatedly and with safety rationale, that "signal-driven mode never re-probes"; Card 16's own Requirements repeat "Signal mode never demotes and never re-probes." But Card 9 has `reapplyLayout` call `hookInstalledLocked()` (a live `show-options -v` round trip) unconditionally on every invocation, in both modes — Card 16's promotion rule only gates on `mode == watchModePoll`, so a genuine resize handled in signal mode still fires the probe; only the mode transition, not the round trip, is suppressed.
**Fix:** either thread the current mode into `reapplyLayout` so it skips the probe once already in signal mode, or correct "never re-probes" in Card 16's Requirements (and any doc.go bullet drawing on it) to state precisely what happens: the probe always runs as part of the one shared code path, and only the poll→signal transition is one-way.

### [NIT:scope] LivePane's defining file is never listed in any card's Context
**Location:** Batch 2 Cards 7, 9, 12, 13; Batch 3 Card 18
**Issue:** `LivePane` is declared in `internal/reedengine/parse.go`, which never appears in the plan's manifest or any card's `Context:`/`Edits:`, even though several cards write new signatures naming `[]LivePane` directly (e.g. Card 7's `applyLayoutLockedOpts`, Card 9's `reapplyLayout`). The type's field shapes are only inferable indirectly via `apply.go`/`overlay.go` usage, which are included elsewhere.
**Fix:** add `internal/reedengine/parse.go` to `Context:` for the cards that introduce new `[]LivePane`-typed signatures.

## Verdict

REQUEST_CHANGES
Fix the reapplyLayout/never-re-probes discrepancy between discussion.md and Cards 9/16 before implementation.
MILL_REVIEW_END
