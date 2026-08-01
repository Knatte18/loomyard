MILL_REVIEW_BEGIN
# Review: fabric: clone-does-everything + subpath-in-weft + init dissolution — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-01
```

## Findings

### [NIT] Stale doc comment: removeHostJunction still describes pre-migration name-set source
**Location:** `internal/fabricengine/weftwiring.go:129-133`
**Issue:** The doc comment for `removeHostJunction` says "Its caller (Remove) sources names from the removed slug's own weft base, best-effort" — but batch 2 card 7 migrated `Topology.Remove` to `RepoWiredNames(l)` (the repo-wide `hubgeometry.BoardDir(l.Hub)` base), and `remove.go`'s own comment was updated accordingly. This one call-site doc comment was missed by the batch 2/6 sweeps.
**Fix:** Reword to "sources names from the repo-wide `BoardDir` base" to match `remove.go`'s current comment.

### [NIT] Stale reference to deleted `internal/initengine/undo.go` in test comment
**Location:** `internal/fabricengine/weftgit_pathspec_integration_test.go:108`
**Issue:** `TestCommitWeft_IndexOnlyDeletionCountsAsMatch`'s doc comment names `internal/initengine/undo.go`'s `lyx init --undo` as the rationale for the "index-only must count" predicate; that file and command are deleted (batch 6 card 25). This file was not on card 28's or batch 7's explicit sweep list.
**Fix:** Reword to reference `fabricengine.Unwire` (`unwire.go`)'s `_lyx` clear-and-commit step instead of the deleted `initengine/undo.go`.

## Verdict

APPROVE
All 7 batches are faithfully implemented, cross-batch contracts hold, and both prior non-blocking items stay fixed; only two stale-comment NITs remain.
MILL_REVIEW_END
