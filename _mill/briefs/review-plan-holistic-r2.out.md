MILL_REVIEW_BEGIN
# Review: landing: Publish + Finalize producers — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-19
```

## Findings

### [BLOCKING:scope] Card 4's Context omits mergepaths.go, which its Requirements directly name
**Location:** batch 1 / Card 4 (MergeStageResolved verb and StageResult)
**Issue:** Requirements state "`unifyConflictPaths` already treats that collision as unmappable and self-aborts the merge" — `unifyConflictPaths` (and the total-function `weftPathVisible` the batch's own discriminator rationale leans on) is declared in `internal/fabricengine/mergepaths.go`, not in any file Card 4's Context lists (fabric.go, mergelifecycle.go, merge.go, mergeerrors.go, mutation.go, gitrepo/merge.go). Verified: `mergepaths.go` is where both functions live and where the "collision"/no-transformation behavior the card's partition logic depends on is documented.
**Fix:** Add `internal/fabricengine/mergepaths.go` to Card 4's Context.

### [BLOCKING:design] Finalize's "dirty-worktree reason" detection has no stable, cross-package identifier
**Location:** batch 4 / Card 24 (Finalize), step 6
**Issue:** Requirements say "On a guard error carrying a dirty-worktree reason, surface that reason verbatim and return stuck" and forbid stash/reset/force on it — a real safety behavior. The only identifier for this condition, `mergeReasonWorktreeDirty = "worktree dirty"` in `internal/fabricengine/mergeerrors.go`, is unexported; `MergeGuardError.Reasons []string` is the only exported surface. The plan never states the mechanism landingshed uses to recognize this specific reason from outside the package (no exported constant, no instruction to mirror the literal, no cross-package test tying the two together) — landingshed's only option is to hardcode the literal string `"worktree dirty"`, with no guard against fabricengine's reason text drifting later (the Shared Decision on "pinned guard tables… move in the same commit" does not name this string among the tracked tables).
**Fix:** Either export a `fabricengine.ReasonWorktreeDirty` constant (or a `MergeGuardError.HasReason(string)`/typed helper) for Finalize to match against, or explicitly instruct landingshed to declare its own constant mirroring the literal and add a cross-package test pinning the two together.

### [NIT:consistency] roadmap.md's "loom: phase-machine scaffolding" Done entry is already stale and this task doesn't touch it
**Location:** batch 6 (documentation lifecycle) — no card
**Issue:** `manifest/roadmap.md`'s Done entry for "loom: phase-machine scaffolding" still says `internal/loomshed` "carries loom's full 12-row producer list" and lists only seven stubbed rows, omitting `Publish` (pre-existing staleness from the prior "add missing Publish stub row, 12-row -> 13-row" fix, which this task's own Card 40 corrects in docs/overview.md but never touches here). After this task, the entry becomes further wrong (neither Publish nor Finalize is a stub any longer, and it's still described as 12-row).
**Fix:** While in `manifest/roadmap.md` for Card 39, also correct the "loom: phase-machine scaffolding" Done entry's row count/stub list, or note explicitly why it's left for a later task.

## Verdict

REQUEST_CHANGES
Two Context/design gaps (mergepaths.go omission; unexported dirty-worktree reason with no cross-package contract) need fixing.
MILL_REVIEW_END
