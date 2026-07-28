MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-28
```

## Findings

### [NIT] composePrompt described as filesystem-free but calls os.Stat
**Location:** batch 7 / card 25
**Issue:** Card 25 calls `composePrompt(p *Profile)` in `internal/burlerengine/prompt.go` "a pure string function with no filesystem access," but it calls `formatFileSet`, which runs `os.Stat` on each Target/Fasit path to detect directories — the claim is not quite accurate.
**Fix:** Reword to something like "does no filesystem access of its own beyond formatFileSet's existing directory check," so the stated rationale for taking a `patternDirective string` parameter (rather than a `*hubgeometry.Layout`) stays factually accurate; the actual required change is unaffected.

### [NIT] Master-template directive placement clause is imprecise against the general rule
**Location:** batch 7 / card 27
**Issue:** Card 27 places `{{.pattern_directive}}` "after both leading orientation blockquotes," but `master-template.md` has a plain (non-blockquote) paragraph — "You are the long-lived Master session for one webster plan run…" — between the second blockquote and the first `##` heading (`## Orientation`). That makes card 27's clause describe a position earlier than the batch-wide rule's own "immediately before the template's first `##` heading … one mechanical rule with no per-template exceptions."
**Fix:** Clarify that the marker goes after that trailing paragraph too, immediately before `## Orientation`, matching the general rule verbatim. No test distinguishes the two candidate positions (the only positional assertion checks the marker lands "ahead of the first work instruction"), so this is a wording-precision fix only, not a functional defect.

## Verdict

APPROVE
Plan is thorough, exhaustively grounded in the actual source across all seven batches, and internally consistent; only two prose-accuracy NITs found.
MILL_REVIEW_END
