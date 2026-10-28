# An over-cap Finalize fixer change gets a solo review of its own commits

When Finalize's merge fixer makes the landing gate pass with a change over the caps, `FixCap.Exceeded` used to send the run back along Finalize's `on_reroute` edge to the whole Code review segment, so a ten-line test lock got a full lens fan over the entire task.
Now, on a run whose reroute reaches a review, Finalize first runs one solo fix review session that reads only the fixer's commits, `Baseline..HEAD`, the range the reroute reason already named.
The caps are unchanged: what they guard against, a fixer weakening or skipping a test to pass the gate, is still judged by a reviewer before the landing.

## What changes

- `internal/landingshed/fixreview.go` adds the fix review: `fixReviewTable` builds the one-seat table (role `fix-reviewer`, stencil `landing-template-fix-review`), `runFixReview` runs it on `Deps.FixerSeats` and judges how it ended, and `ParseFixReviewReport` is the sole parser of its report.
- `Finalize.judgeFixerChange` routes an over-cap change through the fix review when `Deps.RerouteReviewed` answers true:
  - `approved` lands the change and stores the approved HEAD in the attempt record's new `Reviewed` field, so a later pass of the same entry at that HEAD lands without a second review;
  - `rejected`, or a session with no usable verdict (timeout, no report, a report that does not parse after one re-prompt), reroutes exactly as before, the reason adding the findings and the review report's path, or why there was no verdict;
  - a session that leaves the tree dirty or moves the task head halts with `LandingStopPrefix`, naming the way forward;
  - a session that fails to start is a transient error, as the fixer's is.
- A spent fixer, and an over-cap change where no review follows the reroute (Code budget 0, or a pawl run), behave as before and spawn no fix review.
- `keepFixerCommits` now takes whether a review follows as an argument, read once through `Finalize.reviewFollowsReroute`; `archiveStaleReport` is shared by the fixer and the fix review.
- New contract `contracts/specs/landing-fix-review-spec.md` (registered in `contracts/specs`) pins the report: `verdict: approved` or `verdict: rejected`, then `## Findings` bullets, required on a rejection.
- Docs: `internal/landingshed/doc.go`, the darn and loom recipe comments, `docs/overview.md`, `landing.yaml`'s `fixer` and `fixer_timeout_min` comments and `pattern/PATTERN-sole-parsers.md`.

## Design decisions

- The review runs inside Finalize, under the parent lock it already holds, rather than as a new recipe row: no recipe edge changes, and the parent cannot move under an approved change.
  The lock is held for one more session, bounded by `fixer_timeout_min`.
- It runs on the merge fixer's `fixer` model-spec and `fixer_timeout_min`; no new `landing.yaml` key.
- Only an explicit approval lands; every uncertain end falls back to today's full review.
- The review sees the fixer's report and the decision record besides the range, and its prompt names exactly what the caps guard: weakened, skipped or deleted tests, loosened assertions, and gate, build-constraint or test-data changes that dodge the failure.

## Tests

`TestFinalize_GateAndFixer` covers the review's table (role, range, cap reason, fixer report), each end's route (approval lands, rejection and missing or unparseable report reroute, dirty tree and moved head halt), the no-second-review pass at an approved HEAD, and that no review runs with no review after the reroute.
`TestParseFixReviewReport` pins the parser's verdicts, findings and refusals.
