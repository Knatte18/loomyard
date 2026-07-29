MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnetmax
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Fresh-orphan `_board` branch creation mechanism unspecified
**Section:** `_board` becomes a second weft worktree — Fresh-bootstrap edge case
**Issue:** Every existing `git worktree add -b <branch> <path> <startpoint>` call in this codebase (`weftwiring.go`'s `createWeftWorktree`, `clone.go`'s `suffixWeftPrimaryBranch`) requires a start point; a truly history-less orphan (no shared history with `main-weft`, as the decision demands) needs either `worktree add --orphan` (git ≥2.42, newer than README.md's stated "Git 2.35+" minimum) or an unaddressed multi-step `--detach` + `checkout --orphan` + index-clear sequence.
**Fix:** Record which git invocation produces the orphan branch, and whether the stated minimum git version needs to move.

### [GAP] Cross-store slug uniqueness has no stated access path
**Section:** Global slug uniqueness + length cap
**Issue:** The cited chokepoint (`validateUpsertFields`/`NewTask`/`ApplyPatch` in `internal/boardengine/task.go`/`store.go`) takes only a single field map or existing `Task`; `Store.validateWrite`'s dangling-dependency/isolated/deferred checks are likewise scoped to one `Store`'s own `s.tasks` snapshot. Nothing describes how an upsert into one store sees the other store's slug index (or `depends_on` targets) to enforce the claimed global uniqueness.
**Fix:** State the mechanism — e.g. `Store` gains a sibling reference, or `Board`'s facade pre-checks both indices before delegating.

### [GAP] `promote-note` cross-file atomicity unaddressed
**Section:** `promote-note` — pure mechanical cross-store move
**Issue:** `Store.Save`/`state.WriteJSON` (`internal/state/state.go`) is atomic per file only (rename-based); there is no cross-file transaction primitive in this codebase. A crash between the `notes.json` save and the `tasks.json` save is unaddressed, and "mirrors `MergeTasks`'s all-or-nothing discipline" doesn't generalize — `MergeTasks`'s atomicity comes from discarding one in-memory `Store` before any file is saved, not from coordinating two independent writes.
**Fix:** State the intended save order (duplicate-on-crash preferred over loss-on-crash) and whether recovery/idempotent-retry is in scope.

### [NOTE] Existing command help text goes stale, not flagged
**Section:** CLI verb surface
**Issue:** `boardcli.Command()`'s parent `Long` ("home, sidebar, and proposal_prefix filenames") and `rerenderCmd`'s `Short` ("Rebuild Home.md and _Sidebar.md from tasks.json", both confirmed in `internal/boardcli/cli.go`) name the config keys/files this task removes; Scope calls out `Short` for the new `notes`/`promote-note` commands but not this wording fix for the unrenamed `rerender` command and the parent `Long`.
**Fix:** Add a help-text pass on `rerender` and the parent `Long` to the same-commit doc list.

### [NOTE] weftwiring.go doc comment conflicts with `_board` reuse
**Section:** Technical context — `internal/fabricengine/` `weftwiring.go`
**Issue:** The file's header states every branch argument there "is ALWAYS...already-suffixed" and that "this file never derives a branch name itself" — `_board`'s branch is deliberately the unsuffixed `main`, so a new function implementing the decision above would falsify that claim if placed in this file.
**Fix:** Site `_board`'s worktree-add function elsewhere, or update `weftwiring.go`'s header comment in the same commit.

## Verdict

GAPS_FOUND
Three unresolved technical mechanisms (orphan-branch creation, cross-store validation, promote-note atomicity) need answers before planning.
MILL_REVIEW_END
