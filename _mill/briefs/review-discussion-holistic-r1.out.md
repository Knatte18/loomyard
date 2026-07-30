MILL_REVIEW_BEGIN
# Review: fabric: warp-side commit lock + push coalescing

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-30
```

## Findings

### [GAP] Absorbing push lock ownership is contradictory
**Section:** Decisions §coalescing-loop-in-fabricengine-via-closures / Scope (In)
**Issue:** The decision says the fabricengine primitive "holds one absorbing push lock across the loop," but Scope says board "keeps board.push.lock ... unchanged" and the decision rejects lock-path parameterization — so for the board path it is unspecified whether the primitive acquires its own lock (making board.push.lock redundant/double-locking) or the caller pre-acquires it (contradicting "the primitive holds the lock").
**Fix:** State explicitly who acquires the absorbing push lock (caller vs primitive) for each path, and how board.push.lock (in boardengine.Sync, source-verified at sync.go:43) reconciles with the primitive's lock without passing a lock path.

### [GAP] Unified loop exit-condition contract unspecified
**Section:** Decisions §push-only-loop-exit-on-head-stability vs §coalescing-loop-...-closures
**Issue:** Board's loop terminates on `!committed` (commit-driven, sync.go:78) while fabric's is push-only and terminates on HEAD-SHA stability; one shared "loop-until-clean" skeleton needs a single terminator contract, and the discussion does not say whether the exit condition is itself a closure or how a no-commit-step fabric loop drives continuation — nor which HEAD gates exit given fabric pushes two repos (warp + weft) each with its own HEAD.
**Fix:** Define the primitive's exit-condition signature (e.g. a "did-work" bool returned by the step, or a caller-supplied terminator closure) and specify whether the two-sided fabric exit keys on warp HEAD, weft HEAD, or both.

### [NOTE] Absorbing push lock file name unnamed
**Section:** Decisions §lock-artifacts-never-at-worktree-root / Technical context
**Issue:** The new absorbing push lock is placed "alongside weft.write.lock under .weft/" but is never given a filename (contrast the pinned weftWriteLockFile = "weft.write.lock", weftgit.go:28).
**Fix:** Name the constant (e.g. weftPushLockFile) so plan-writing pins it in the same style as the existing lock constants.

## Verdict

GAPS_FOUND
Shared primitive's lock ownership and loop exit-condition contract are underspecified for the two callers.
MILL_REVIEW_END
