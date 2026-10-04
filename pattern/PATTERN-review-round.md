# PATTERN-review-round

One review and fix round: the review is written to disk before any target file is touched, every finding is fixed at every severity, nothing is self-graded, and each fix is its own commit on warp source, never pushed.

- A review segment converges only on a judge verdict over a fresh review, and fixed findings never converge on their own.
- A finding's class decides who decides and when the loop stops, never whether it is fixed.
