# PATTERN-review-round

One review and fix round: the review is written to disk before any target file is touched, every finding is fixed at every severity or disputed by the fixer with evidence that its premise is false, nothing is self-graded, and each fix is its own commit on warp source, never pushed.

- A review segment converges only on a judge verdict over a fresh review, and fixed findings never converge on their own.
- A dispute never converges a segment on its own either: it needs evidence that the finding's premise is false, and severity, size, cost or disagreement with the rubric never justify one.
- The reviewer and the fixer are separate sessions, and the fixer touches nothing before Go has accepted the review and written the ready marker.
- A finding's class decides who decides and when the loop stops, never whether it is fixed.
