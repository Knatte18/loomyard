I have verified the load-bearing claims. The shuttle `Spec`/`Result`/`Outcome` types, `stencil.Fill`'s top-level-marker rule, the design doc's frontmatter format, roadmap milestone 11 (joint burler+perch), and the sandbox/reviews tooling all match the discussion's descriptions. The r1 items (reject APPROVED-with-BLOCKING; unconditional fixer-report; prior-round existence validation) are resolved in the Decisions and Q&A log.

Two fresh issues remain.

MILL_REVIEW_BEGIN
# Review: Build burler - the review+fix round worker

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-08
```

## Findings

### [GAP] FixScope validation left unspecified
**Section:** Decisions › profile-shape / fix-scope-commit-rules; Testing
**Issue:** Every profile field enumerates its validation (Rubric non-empty, ClusterN>0 typed error, Target/Fasit/Prior* existence) except `FixScope`, whose two values select safety-critical behavior (source = git commit-per-fix on host; markdown = no git at all) — an empty or typo'd value's handling is undefined.
**Fix:** State that `Run` rejects a `FixScope` that is not exactly `markdown` or `source` (typed/validation error, fail loud), and add it to the profile-validation unit-test list.

### [NOTE] Rubric "severity rules" vs fixed parser vocabulary
**Section:** Decisions › profile-shape (Rubric) vs review-file-format-and-parse
**Issue:** `Rubric` is described as carrying "severity rules for this artifact type," but the parser strictly enforces a fixed `BLOCKING|MEDIUM|LOW|NIT` set — a rubric author (or plan writer) could read the former as license to define custom severities the parser will reject.
**Fix:** Clarify that a rubric maps its criteria onto the fixed four-value severity vocabulary (and verdict `APPROVED|BLOCKING`); it does not define new severity names.

## Verdict

GAPS_FOUND
One unresolved validation gap on the safety-critical FixScope field; otherwise the discussion is complete and source-accurate.
MILL_REVIEW_END
