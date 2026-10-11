# PATTERN-review-round

One review and fix round: the review is written to disk and accepted by Go before any target file is touched, every finding is fixed at every severity, disputed by the fixer with evidence on one of two grounds, or fixed with the fixer's own change when the proposed one is wrong, nothing is self-graded, and each fix is its own commit on warp source, never pushed.

- A review segment converges only on a judge verdict, and from round 2 on a fix of a key first raised in the closing round may converge, carried to the decision record's `## Open risks`; every other fixed finding never converges on its own.
- A dispute rests on `premise false` (the premise is false against the code the fixer reads) or `every fix worse` (the premise holds, but every change that addresses it makes the target worse).
  One sound change means the finding is fixed, not disputed, and "every change" includes changes outside the fixer's write surface, so a finding whose only sound change leaves the surface is deferred with that reason.
- The fixer holds neither the fasit nor the rubric, so neither is a dispute ground: the reviewer applies both before it writes, and the judge rules on a finding that breaks them.
  A finding that contradicts the fasit or that the rubric's `Do not flag` list excludes may therefore be applied.
  That is bounded by the judge's CONTINUE and a later round reverting it, and unbounded when that CONTINUE is auto-accepted at a spent budget.
- When the premise holds and the review's proposed change is wrong or harmful but another change resolves the premise, the fixer fixes with its own change and marks the finding as fixed differently in the fixer-report.
- A dispute never converges a segment on its own either: it needs cited evidence, and severity, size, cost, taste or disagreement with the rubric never justify one.
- The reviewer and the fixer are separate sessions, and the fixer touches nothing before Go has accepted the review.
  That is sequencing: the fix half spawns only once the round's acceptance is recorded, and it refuses a review whose bytes changed since.
- A finding's class decides who decides and when the loop stops, never whether it is fixed.
