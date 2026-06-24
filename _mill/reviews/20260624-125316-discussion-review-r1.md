`WorktreeRoot` is the git toplevel of the caller's cwd — so for a mid-work subtask spawn it correctly reflects the parent task's host worktree (on branch Y). The discussion's resolution mechanism (`git -C <WorktreeRoot> rev-parse --abbrev-ref HEAD`) is sound.

I now have enough verified context to assess the discussion holistically. The discussion is technically thorough and well-grounded. Let me check a couple of edge cases the discussion may not fully address: (1) detached HEAD on the host worktree (abbrev-ref returns "HEAD"), and (2) the prime weft worktree being checked out on `main` while a subtask branch needs to base on weft-Y — git allows `worktree add -b new <path> <branch>` even when `<branch>` is checked out elsewhere, so that's fine.

The detached-HEAD case is a real failure mode the discussion does not address. Let me also verify the "Pushed" semantics and whether the start-point arg threading affects the signature change in callers — but that's a plan concern. I have what I need.

MILL_REVIEW_BEGIN
# Review: Ensure weft branches are orphan branches

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-24
```

## Findings

### [GAP] Detached-HEAD / unborn host branch unspecified
**Section:** Technical context (parent weft branch resolution) / Failure modes
**Issue:** Resolving the parent via `git rev-parse --abbrev-ref HEAD` returns `"HEAD"` on a detached host worktree (and fails on an unborn branch); the discussion only handles "parent weft branch missing," not a non-branch host HEAD, which would yield a bogus start-point name.
**Fix:** State the expected behavior when the host worktree is detached/unborn (e.g. fail with a clear error), so the plan does not silently pass `"HEAD"` as the start-point.

### [GAP] No explicit assertion that root is the parent *tip at spawn*, not just shared-base
**Section:** Testing (createWeftWorktree start-point)
**Issue:** The test text mixes two claims — "share a merge-base" and "new branch's root is the parent's tip at spawn time" — but the start-point fix only guarantees the new branch *descends from* the parent tip; later parent commits would still share a merge-base, so a merge-base-only assertion cannot distinguish forking-from-tip from forking-from-an-older-ancestor (e.g. prime main).
**Fix:** Specify the discriminating assertion (e.g. `git merge-base <new> <parent>` equals the parent's tip SHA captured at spawn), not merely "non-empty," so the subtask anti-regression test actually rejects the old prime-main behavior.

### [NOTE] Signature change ripple to callers/tests not called out
**Section:** Technical context (Primary change site / Caller)
**Issue:** Threading a start-point arg changes `createWeftWorktree`'s signature; `add.go` step 8 and any other callers/tests referencing it must update, but the discussion names only the one call site.
**Fix:** Note that the plan must sweep all callers of the new signature (currently only `add.go:142`, but confirm) to avoid a half-migrated build.

### [NOTE] Proposal rewrite target lives on the wiki, not in-tree
**Section:** Scope (Rewrite proposal-weft-orphan-branches.md)
**Issue:** `proposal-weft-orphan-branches.md` does not exist in this worktree; per CLAUDE.md the proposal is a wiki artifact written only via the mill wiki module, not editable in-tree.
**Fix:** Clarify that the proposal rewrite goes through the wiki daemon/`/mill-*` skills, so the plan does not attempt a raw in-tree Edit/Write.

## Verdict
GAPS_FOUND
Two gaps: detached-HEAD parent resolution and a non-discriminating subtask root assertion.
MILL_REVIEW_END