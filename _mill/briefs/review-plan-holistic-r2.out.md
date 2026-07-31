MILL_REVIEW_BEGIN
# Review: fabric: fold snapshot-tracking into the Warp-SHA trailer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (claude-sonnet-5 per system context)
reviewed_file: plan/
date: 2026-07-31
```

## Findings

### [BLOCKING] Card 1 references snapshot.go identifiers without listing it in Context/Edits
**Location:** batch 01-retire-ref-mechanism, Card 1
**Issue:** Requirements names nine identifiers defined in `internal/gitrepo/snapshot.go` (`SnapshotSHA`, `SetSnapshotSHA`, `snapshotPushMaxAttempts`, `advanceAndPushSnapshotRef`, `adoptSnapshotRef`, `isStrictDescendant`, `remoteName`, `validSnapshotKey`, `snapshotRef`) and requires grep-confirming callers of two of them, but `snapshot.go` appears only under `Deletes:`, never `Context:`/`Edits:` — the brief's Context-completeness rule only grants implicit-read status to `Edits:` files, not `Deletes:` ones.
**Fix:** Add `internal/gitrepo/snapshot.go` to Card 1's `Context:` list.

### [NIT] Card 2's gogit.go location description is imprecise
**Location:** batch 01-retire-ref-mechanism, Card 2
**Issue:** "the refs/loomyard/snapshot/* mention in the fingerprint-gate rationale near the top of the file" actually sits in `goGit`'s "Why EnableDotGitCommonDir" section (line ~40), not near `lookupObjectRetrying`/`packFingerprint`'s fingerprint-gate rationale; the mandatory grep sweep will still surface it regardless.
**Fix:** Reword the pointer to name the EnableDotGitCommonDir section, or drop the sub-location detail and rely on the grep.

## Verdict

REQUEST_CHANGES
One BLOCKING Context-completeness gap in batch 1 Card 1; otherwise exceptionally well-grounded and internally consistent.
MILL_REVIEW_END
