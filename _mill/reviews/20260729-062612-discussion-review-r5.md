MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [NOTE] "two gitrepo.New calls" miscounts current sync.go
**Section:** Decisions § Weft git routing (first bullet)
**Issue:** `internal/boardengine/sync.go` has exactly one `gitrepo.New(boardPath)` call (line 57, in `Sync`), reused for both `StageAllAndCommit` (via `commitDirty`, which takes `*gitrepo.Repo` as a param) and `PushCoalesced` — not "two `gitrepo.New(boardPath)` calls."
**Fix:** Reword to "swap `commitDirty`'s `StageAllAndCommit` and `Sync`'s `PushCoalesced` — the two method calls on the single `gitrepo.New(boardPath)` handle — for the package-level `CommitWeftAt`/`PushWeftAt`"; design substance (two fabricengine functions) is unaffected.

### [NOTE] `_board` adopt-path branch state after clone is imprecise
**Section:** Decisions § `_board` becomes a second weft worktree (bullet 1) + Fresh-bootstrap edge case
**Issue:** On a non-empty weft remote (the assumed default-branch `<hostBranch>`), `git clone` already creates a local `<hostBranch>` ref, which `suffixWeftPrimaryBranch`'s `checkout -b <hostBranch>-weft` leaves intact — so the adopt path is `git worktree add <path> <hostBranch>` against an already-existing, unchecked-out local branch, not "adopt origin/<hostBranch> as a tracking local branch." A literal `-b <branch> --track origin/<branch>` form would fail with "branch already exists."
**Fix:** In planning, distinguish the three post-clone states (local branch already present → plain worktree-add; branch absent on empty remote → `--orphan`) rather than the two-way "adopt-from-origin OR orphan" framing.

## Verdict

APPROVE
Design is sound and source-grounded; two wording-precision NOTEs, no blocking gaps.
MILL_REVIEW_END
