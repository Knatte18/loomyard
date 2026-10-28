```yaml
lens: code-correctness
outcome: reported
```

Branch/case list:

- `ParseFixReviewReport`: absent file and unreadable file return wrapped errors; empty input and unknown verdict are rejected; both valid verdicts parse; rejection with an empty Findings section is rejected; bullets outside Findings are ignored.
- `ParseFixReviewReport`: `verdict: approved\n` without the required Findings section is incorrectly accepted.
- `fixReviewTable`: empty required inputs, missing stencil, malformed model-spec and failed registry resolution return errors; valid inputs produce one reviewer seat with the configured model, timeout, output and skills; an absent optional fixer report is supported.
- `archiveStaleReport`: absent old report proceeds; existing report is renamed; rename failure returns an error.
- `runFixReview`: a failed start marked `ErrNotStarted`, or a result marked `NotStarted`, becomes a transient error.
- `runFixReview`: a started session returning a wait/mechanism error exits before cleanliness, HEAD and report classification; this does not implement D3’s fallback routing.
- `runFixReview`: clean timeout yields no verdict; missing or malformed report yields no verdict; rejection retains findings; approval yields approval.
- `runFixReview`: dirty tree or changed HEAD with no runner error yields the changed-branch outcome; cleanliness and HEAD read failures propagate.
- `Finalize.judgeFixerChange`: empty baseline proceeds; HEAD/change-read failures stop; within-cap changes proceed; approval at the same HEAD proceeds; a different HEAD requires review again when eligible.
- `Finalize.judgeFixerChange`: no following review preserves the over-cap halt; approval persists Reviewed before proceeding; persistence failure stops; rejection/no verdict reroutes; changed branch halts with `LandingStopPrefix`.
- `reviewFollowsReroute`: nil seam returns false; a seam error is wrapped; true/false is preserved.
- `endSpent` and `keepFixerCommits`: spent attempts do not spawn the reviewer; eligible reroutes retain the baseline-to-HEAD range; ineligible paths persist Halt and halt; seam, HEAD and persistence errors stop.
- `fixReviewOutcome.rerouteCause`: rejection names findings and report; no-verdict names cause and session log.
- Resume and retry: attempt records are read by entry, Reviewed survives reads, and the same-HEAD bypass is reached through the passing-gate path.

Findings:

1. **MEDIUM — design — `internal/landingshed/fixreview.go:71–85` — An approval without the required Findings section is accepted.**  
   Evidence: `contracts/specs/landing-fix-review-spec.md:14` requires a `## Findings` section after the verdict; `ParseFixReviewReport` tracks only whether it is currently inside that section and validates missing findings only for rejection.
   The concrete report `verdict: approved\n` therefore passes the report gate at `fixreview.go:226–230` and becomes approval at `fixreview.go:272–273`, although it does not satisfy D7’s format contract.
   Track whether the required heading appeared and reject a report that lacks it, while continuing to accept an explicitly empty Findings section on approval.

2. **MEDIUM — design — `internal/landingshed/fixreview.go:241–242` — Errors after a reviewer starts bypass D3’s no-verdict fallback and branch-change halt.**  
   Evidence: a real started session can return a wait error without a report when shuttle exhausts events/status retries (`internal/shuttleengine/wait.go:294–300`, `385–398`); `seatengine.Engine.join` stops that chair and returns its wait error (`internal/seatengine/engine.go:376–400`).
   `runFixReview` immediately propagates that error, before inspecting cleanliness or HEAD.
   Consequently a clean failed review does not reroute with the no-verdict cause, and a failed review that changed the branch does not halt with `LandingStopPrefix`, contrary to D3.
   Preserve the special transient startup handling, then classify a started session’s branch state before mapping its ended-without-verdict error to `fixReviewEndNoVerdict`; retain infrastructure errors where checking the branch itself fails.

Tool-call manifest:

- Call 1: read previously truncated `seatengine.Run`/`join`, searched shuttle gate timeout semantics and production landing seams; the attempted `record.go` lookup found no file.
- Call 2: read production `TaskHead`, `startSeat` error propagation, and located shuttle gate/finalization and atomic writer definitions missing from inherited excerpts.
- Call 3: read shuttle deadline/mechanism-failure completion and finalization, the atomic writer, exact new-code line numbers, and the review format.
- Call 4: read `Fabric.HeadSHA`, `verifytree.DirtyPaths`, relevant dependency/config declarations, and shuttle’s concrete runtime failure returns to establish the started-session error route.
