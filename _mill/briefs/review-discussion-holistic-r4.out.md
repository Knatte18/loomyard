MILL_REVIEW_BEGIN
# Review: fabric: Fabric.Commit classify+dispatch + unified diff/status

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Fabric.Status working-tree primitive unspecified
**Section:** § unified-diff-status-warp-anchor / Scope / Technical context (`status.go`)
**Issue:** `Fabric.Status()` promises a per-file list of uncommitted working-tree changes merged across both repos, but no `gitrepo.Repo` method returns such a list — `ChangedFilesSince` is committed-tree-vs-HEAD only (gitrepo.go:449), and `StatusWeft` collapses `git status --porcelain` to a single `dirty` bool with no warp analog; the "no new git primitive" rationale is stated for `Diff` only, leaving Status's source unnamed.
**Fix:** State that `Fabric.Status` requires a new per-repo working-tree changed-file primitive (a porcelain/go-git worktree diff on both warp and weft), name where it lives, and confirm it against the gitrepo Client Boundary Invariant.

### [NOTE] Diff/Status return-type shape/placement not an Open item
**Section:** § unified-diff-status-warp-anchor / Open items
**Issue:** The side-labelled merged-entry type and the "no weft correspondence" flag/field are described inline, but unlike `CommitResult` placement there is no Open item pinning the Diff/Status result struct's shape and file.
**Fix:** Add a brief Open item for the unified diff/status result type (entry struct with side label + no-correspondence flag) and its file placement.

## Verdict

GAPS_FOUND
Fabric.Status's uncommitted working-tree file-list primitive is unspecified and unbacked by existing gitrepo methods.
MILL_REVIEW_END
