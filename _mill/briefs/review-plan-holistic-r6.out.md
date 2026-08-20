MILL_REVIEW_BEGIN
# Review: shedadapters: Burler-round producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Sonnet-class (harness-reported model id: claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-20
```

## Findings

### [BLOCKING:consistency] Card 14's roadmap fix leaves the judge-call clause on the old single-artifact predicate
**Location:** batch 4 / card 14 (`manifest/roadmap.md`, the `Bouncer` item's seed-vs-judge sentence)
**Issue:** The Bouncer item's seed-vs-judge text is two sentences: "...if the round producer's report artifact for the current round does not exist yet, this is the **seed call**... unconditionally. If the artifact exists, this is the **judge call**...". Card 14 quotes and amends only the first (seed) sentence to the pair predicate ("review and fixer-report artifacts are not both present"); it never mentions the second (judge) sentence, which is that seed clause's logical else-branch and still reads "If the artifact exists" (singular). Following the card literally leaves the two complementary clauses testing different predicates in the same paragraph — a self-contradiction, and specifically a regression of the single-artifact ambiguity the pair-predicate Decision (`Applies to: batch 3, batch 4`) exists to close: a review-only orphan would satisfy "the artifact exists" and wrongly read as a judge call.
**Fix:** Extend card 14's instruction to also reword the judge-call trigger to the pair predicate (e.g. "if both artifacts exist, this is the judge call"), so both halves of the if/else state the same test.

### [NIT:consistency] Card 8's parenthetical names two of three same-bucket error cases
**Location:** batch 3 / card 8
**Issue:** "a runner error whose Result.Outcome is shuttleengine.OutcomeDone (the cluster-audit and verdict-parse cases, reached only after every output file already exists on disk)" — `burlerengine.Engine.Run` actually has three post-Done error returns sharing this shape: the cluster-audit failure, the `os.ReadFile(p.ReviewPath)` failure, and the `ParseReview` failure. The archive rule's actual gate (`err != nil && Outcome == Done`) already covers all three, so this is a documentation-precision nit only, not a functional gap.
**Fix:** Either name all three cases or make the parenthetical explicitly non-exhaustive ("e.g., the cluster-audit and verdict-parse cases").

## Verdict

REQUEST_CHANGES
Card 14's roadmap correction is internally inconsistent — it fixes only half of the seed/judge if-else pair.
MILL_REVIEW_END
