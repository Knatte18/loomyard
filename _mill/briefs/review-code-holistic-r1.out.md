MILL_REVIEW_BEGIN
# Review: fabric: merge-conflict primitive — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-19
```

## Findings

### [NIT:consistency] Stale identifier in `mergeStateOrForeignErr`'s doc comment
**Location:** `internal/fabricengine/mergelifecycle.go:69-72`
**Issue:** The doc comment above the function reads "mergeStartOrForeign resolves the disposition…" but the function itself is named `mergeStateOrForeignErr` — the comment names an identifier that does not exist in the file.
**Fix:** Rename the comment's leading identifier to `mergeStateOrForeignErr` to match the function it documents.

### [NIT:consistency] `MergeIn` resolves the worktree location twice
**Location:** `internal/fabricengine/merge.go:55-62`
**Issue:** `MergeIn` calls `lyxcwd.ResolveWorktree(f.warpPath)` directly for `l`, then immediately calls `resolveMergeGeometry(f.warpPath)`, which internally calls `lyxcwd.ResolveWorktree` again on the same path — a harmless but redundant double resolution not present in `Merge`, which only resolves once.
**Fix:** Have `resolveMergeGeometry` accept the already-resolved `*lyxcwd.Location` (or derive `anchorRel`/`wiredNames` from the `l` `MergeIn` already has) so the worktree is resolved once per call, matching `Merge`'s pattern.

### [NIT:consistency] Envelope-mapping logic duplicated three times in `merge_verbs.go`
**Location:** `internal/fabriccli/merge_verbs.go:42-156`
**Issue:** The card 16 requirement describes one "envelope mapping, shared by all modes" (error → `errWithRecord`, conflicts → `errConflictsWithRecord`, otherwise → `okWithRecord`), but the implementation open-codes this three-way dispatch separately for `merge-in`, `merge --continue`, and `merge --abort`/default, rather than through one shared helper.
**Fix:** Factor the three-branch mapping into one small helper taking `(res MergeResult, err error)` and call it from all three RunE bodies — behavior is currently correct (continue/abort never populate `Conflicts`), so this is a maintainability nit, not a functional defect.

## Verdict

APPROVE
All six batches faithfully implement the plan and Shared Decisions; only minor duplication/comment-naming nits found, no BLOCKING issues.
MILL_REVIEW_END
