MILL_REVIEW_BEGIN
# Review: Scope the Shed producer-model rewrite into buildable tasks — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [NIT] Off-by-one line citation transcribed into Card 4 (task C body)
**Location:** batch 2 / Card 4 (`format-docs-name-producers`) **Issue:** discussion.md's `discussion-stays-two-files-with-current-names` decision, and Card 4's own Requirements, cite `discussion-format.md:15` for "filenames are self-describing rather than terse" — the actual file has that sentence at line 16 (line 15 is the preceding sentence about the filesystem boundary), confirmed by direct read of `docs/reference/discussion-format.md`. **Fix:** correct the citation to `:16` when C's body is authored, or note the discrepancy so C doesn't propagate it into the wiki page.

## Verdict

APPROVE
Structure, decision alignment, dependency wiring, and source citations all check out; only one trivial inherited off-by-one line citation found.
MILL_REVIEW_END
