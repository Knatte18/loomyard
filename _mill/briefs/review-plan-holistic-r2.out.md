MILL_REVIEW_BEGIN
# Review: Audit internal/logger coverage across spawn/hard-error paths — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-28
```

## Findings

### [BLOCKING:scope] "covered" verdicts conflate file-level import with per-call spawn logging
**Location:** Batch 1 / Card 1, spawn-sites table (feeds `manifest/designs/logger-coverage.md`).
**Issue:** `internal/fabricengine/spawn.go`'s `SpawnDetachedPush` calls `cmd.Start()` with only a `Warn` on `Start` failure and no spawn-announcement line at all, yet is marked `covered`; once card 2's sharpened invariant text lands ("...logs its spawn via internal/logger... a detached spawn... logs the spawn alone"), this site no longer satisfies the invariant it just sharpened, and batch 5's guard cannot catch the gap because it only checks file-level `logger` import presence — exactly the blind spot the guard's own header comment documents. `internal/reedcli/attach.go`'s sole `exec.Command` (the tmux attach handover) and one of `internal/loomcli/run.go`'s two sites are the same shape: the file imports `logger` only for an unrelated terminal-size warning, and the spawn/wait itself is never logged, so their `covered` verdict rests on the file's import list rather than the call site.
**Fix:** Either give `fabricengine/spawn.go` (and re-examine the two attach sites) an `add` verdict with its own card, or add an explicit "pre-existing, out of this task's scope" rationale for each to card 1's requirements so `logger-coverage.md` records a deliberate exemption instead of a verdict a stricter per-call read contradicts.

### [NIT:consistency] Wrong enclosing function named for mergeresolve's Warn insertion
**Location:** Batch 2 / Card 4.
**Issue:** Requirements say to add the `logger.Warn` "inside `(*Resolver).Resolve`", but the `if runResult.Outcome != shuttleengine.OutcomeDone` branch and the following `r.abortAndStuck` call actually live in the unexported `resolveConflicts` helper that `Resolve` calls at line 59, not in `Resolve`'s own body.
**Fix:** Correct the named enclosing function to `resolveConflicts` (the branch and adjacent return already uniquely pin the location either way).

## Verdict

REQUEST_CHANGES
The audit's `covered` classification is unreliable for at least one clear-cut pre-existing spawn site the new invariant text itself would then contradict.
MILL_REVIEW_END
