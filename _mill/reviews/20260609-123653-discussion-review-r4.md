All claims verified. The current template at init.go:111-114 uses different trailing comments than the discussion's new template (e.g. current "relative to cwd; may contain $env:NAME" vs new "relative to cwd or absolute"), but the discussion provides explicit complete replacement strings, so that is well-specified.

I have verified the core claims. The discussion is highly detailed and well-grounded. Let me produce the review.

MILL_REVIEW_BEGIN
# Review: extract-shared-infra

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\mhgo\wts\extract-shared-infra\_mill\discussion.md
date: 2026-06-09
```

## Findings

### [NOTE] spawn constants ownership after hideProcWindow moves
**Section:** `hideProcWindow` placement / package layout
**Issue:** `spawn_windows.go` defines `createNoWindow` and `createNewProcessGroup`; `spawnSync` uses both, but `hideProcWindow` (moving to internal/git) is the only user of `createNoWindow` outside spawnSync — the discussion does not state these consts stay in board.
**Fix:** Add one line confirming `createNoWindow`/`createNewProcessGroup` remain in `spawn_windows.go` (spawnSync still needs them) and internal/git redeclares its own `CREATE_NO_WINDOW`.

### [NOTE] Line-number references drifted by one
**Section:** Key files to read
**Issue:** Cited lines are slightly off vs source: `store.go` read-lock is line 62 (not 63), `sync.go` RunGit at 72/173 (not 73/174), AcquireWriteLock at 66 (not 67).
**Fix:** Treat line numbers as approximate; the plan writer should grep call sites rather than trust exact lines.

### [NOTE] Template trailing comments differ from current source
**Section:** init.go template comment update
**Issue:** Current template comments (e.g. "relative to cwd; may contain $env:NAME") differ from the new strings; discussion supplies full replacement literals so this is intentional, but the "preserving per-key descriptions" wording slightly overstates continuity.
**Fix:** None required — the explicit replacement block is authoritative; note the descriptions are reworded, not preserved verbatim.

## Verdict
APPROVE
Discussion is fully decided, source-grounded, and testable; only minor non-blocking notes.
MILL_REVIEW_END
