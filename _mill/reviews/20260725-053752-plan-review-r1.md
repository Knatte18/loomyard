MILL_REVIEW_BEGIN
# Review: board: use gitrepo as its git operator — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-25
```

## Findings

### [NIT] Stale gitrepo doc references to board's pushUnpushed/hasUnpushed
**Location:** Batch 2, Card 4 (fallout in `internal/gitrepo/push.go`, `doc.go`)
**Issue:** `push.go` (rebaseRetryTriggers comment, PushCoalesced comment) and `doc.go`'s Push-surface section cite "board's sync.go:pushUnpushed matches" and "hasUnpushed's contract" as the trigger-set justification; Card 4 deletes both board functions, leaving these as dangling cross-references, and no card touches these two files.
**Fix:** Reword those gitrepo comments to drop the reference to the now-deleted board symbols (e.g. cite the trigger set on its own merit).

### [NIT] gitrepo.go file-header primitive list omits StageAllAndCommit
**Location:** Batch 1, Card 1
**Issue:** `gitrepo.go`'s top-of-file comment enumerates "New, ... CurrentSHA, StageAndCommit, ChangedFilesSince, and SHAExists"; Card 1 adds `StageAllAndCommit` to this file but does not update that header list, so it goes stale.
**Fix:** Add `StageAllAndCommit` to the file-header enumeration in the same Card 1 edit.

## Verdict

APPROVE
Sound, complete, well-sequenced; two doc-staleness NITs, neither blocking.
MILL_REVIEW_END
