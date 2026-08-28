MILL_REVIEW_BEGIN
# Review: reed: resume/down leak lock directories at the stale pre-rename session-name path

```yaml
duration_s: 102.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Standalone target existence is asserted, not true
**Section:** `### the-predicate-is-worktreeroot-exists-not-anchorpath-exists` (and its Q&A entry)
**Issue:** The predicate rests on "`WorktreeRoot` … is the operator's own target repository in standalone mode, which always exists by the time reed runs"; source contradicts this — `burlercli/wiring.go:173-181` / `webstercli/wiring.go:232-239` (`resolveStandaloneTarget`) only absolutise `--target-dir` with no `Stat`, and `standalonestate.derive` explicitly "fall[s] back to Clean alone when the target does not exist on disk yet" (`standalonestate.go:57-59`), so a standalone `--target-dir /nope` reaches `withOpLock` with a non-existent `WorktreeRoot` that today proceeds and creates `stateDir`.
**Fix:** State the disposition for a non-existent standalone target (refuse is defensible) and say which message it gets — the vanished-path text tells a standalone operator to fix a rename that never happened, which is precisely the misdiagnosis `### the-error-names-the-vanished-path-and-the-remedy` exists to prevent.

### [NIT:consistency] Named integration fixture inventory is wrong
**Section:** `## Testing` → Fixture (and the matching `## Technical context` bullet)
**Issue:** `attachgeometry_integration_test.go` is listed as an inline-`Geometry`-literal site; it contains no `Geometry` literal and no `WorktreeRoot` reference — it builds engines via `newIntegrationEngine` (`mouse_boot_integration_test.go:27`), a second shared helper the discussion never names, which already `os.MkdirAll`s its worktree dir.
**Fix:** Drop `attachgeometry_integration_test.go` from the named list and name `newIntegrationEngine` as the second helper (already compliant); the stated criterion and the sweep instruction are correct and unaffected.

## Verdict

REQUEST_CHANGES
One false premise about standalone target liveness leaves the refusal message undecided.
MILL_REVIEW_END
