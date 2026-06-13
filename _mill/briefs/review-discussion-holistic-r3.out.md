The `mustRun` helper referenced in the discussion's test examples does not yet exist. That's fine — it's a helper to be created. Now I have enough verified facts to produce the review.

Key verifications:
- `internal/config.Load` signature and behavior: confirmed, emits `not initialized: _mhgo/ directory not found`.
- `board.LoadConfig` re-wraps to `not initialized here; run "mhgo init"`: confirmed.
- `RunGit` signature and exit-code semantics: confirmed.
- `RunInit` JSON output and structure: confirmed.
- Board `RunCLI` signature `(out io.Writer, args []string) int`: confirmed.
- main.go dispatch and doc-comment modules list (init, board, muxpoc — no worktree yet): confirmed.
- Roadmap milestone numbering: discussion says milestone 4 = worktree; roadmap confirms milestone 4 = worktree. BUT docs/modules/worktree.md line 4 says "roadmap milestone 5" — a stale inconsistency. The discussion scope says "update open questions to answered decisions" in worktree.md but does not mention fixing the milestone-5 reference.

Now let me produce the review.

MILL_REVIEW_BEGIN
# Review: mhgo-worktree-module

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-13
```

## Findings

### [NOTE] worktree.md says milestone 5, roadmap says 4
**Section:** Scope (out) / docs update
**Issue:** `docs/modules/worktree.md:4` states "roadmap milestone 5", but `docs/roadmap.md` and the discussion both place worktree at milestone 4; the scope only commits to updating "open questions to answered decisions", leaving this stale reference unaddressed.
**Fix:** Add the milestone-number correction to the worktree.md edit scope so the doc update is internally consistent.

### [NOTE] config defaults map drops a key silently on rewrap
**Section:** Technical context — Existing shared infrastructure
**Issue:** `board.LoadConfig` rewraps any error containing "not initialized" into the friendly message; if `config.Load` ever returns a different "not initialized" variant the message is masked, but for worktree this matches board exactly so the mirror is sound.
**Fix:** None required; noted only to confirm the mirror behaviour is intended and identical to board.

### [NOTE] list main-worktree ordering assumption
**Section:** Decisions — `list` is a thin wrapper
**Issue:** "First block = main worktree → main:true" relies on `git worktree list --porcelain` always emitting the main worktree first; this is git's documented behaviour but is not asserted as a guarantee in the discussion.
**Fix:** Note in the plan that a list_test.go case should assert the first entry is the main checkout to pin this ordering contract.

## Verdict

APPROVE
Decisions are complete with rationale and rejected alternatives; only minor doc/test notes remain, none blocking.
MILL_REVIEW_END
