MILL_REVIEW_BEGIN
# Review: plan-format: drop the v3 suffix and sweep every reference by script — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-09
```

## Findings

### [NIT:consistency] Override note's "verified count" is stale by the time it's written
**Location:** `manifest/designs/shed-followups.md:229` (Block 1, item 2)
**Issue:** The note asserts `grep -c 'plan-format-v3\.md' manifest/designs/shed-followups.md` "returns **five**," but a plain grep of the file as it stands now (post-implementation) returns 6 matching lines — the sixth is the note's own newly-written sub-bullet at line 225 ("the sweeper additionally skipped line 18 of `manifest/roadmap.md`, whose `plan-format-v3.md` → `plan-format.md` would have collapsed..."), which itself contains one literal, unescaped occurrence of `plan-format-v3.md`. The snapshot was evidently taken before that sub-bullet was added, so the "verified, not remembered" count the plan explicitly demanded is now inconsistent with the file it describes.
**Fix:** Re-run the grep after the full block is written and update the figure (or word the sentence to exclude the note's own self-reference), so a future reader (task C/E) re-verifying the count doesn't find a mismatch.

## Verdict

APPROVE
All four batches, shared decisions, and acceptance gates are correctly implemented; only a trivial self-referential count is stale.
MILL_REVIEW_END
