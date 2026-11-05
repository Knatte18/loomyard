```yaml
lens: code-correctness
outcome: reported
```

Branch cases reviewed:

- `ParseFixReviewReport`: absent/unreadable file returns its wrapped error; empty or unknown first line is rejected; valid approval and rejection parse; rejection with zero findings is rejected; **approval without the required Findings section is incorrectly accepted**.
- `fixReviewTable`: empty required values, missing stencil, invalid model-spec and failed resolution return errors; optional fixer report and skills remain optional; valid values build one seat with the configured model and timeout.
- `archiveStaleReport`: absent source succeeds; rename failure propagates; existing report is archived before launch.
- `runFixReview`: failed startup is transient; dirty tree and moved HEAD halt when `Run` returns nil; timeout, missing report and malformed report reroute; approval and rejection map correctly; **a started session returning a wait error bypasses both branch-integrity checks and the no-verdict reroute**.
- `judgeFixerChange`: absent baseline and under-cap change proceed; read failures propagate; matching approved HEAD proceeds; changed HEAD requires review; false/nil reroute-review seam retains the old halt; approval persists HEAD before proceeding; rejection reroutes; changed branch halts.
- `reviewFollowsReroute`, `endSpent`, `keepFixerCommits`: nil seam means no review; seam errors propagate; spent failing gate does not review; reroute retains commit range; no-review halt persists its cause.
- `FixerAttempts`: `Reviewed` is written on approval and checked against the current HEAD; entry mismatch discards the old record.

Findings:

1. **MEDIUM — design — `internal/landingshed/fixreview.go:241–243`: Started-session errors bypass the required fallback and branch-integrity check.**  
   A real path exists where the reviewer starts, writes no report, and its events file becomes unreadable: `internal/shuttleengine/wait.go:294–300` returns a mechanism error, and `internal/seatengine/engine.go:376–391` stops the chair and returns that error even when stopping succeeds. `runFixReview` immediately returns it before checking the working tree or HEAD. Finalize therefore produces an infrastructure error rather than D3’s no-usable-verdict reroute; if the reviewer edited the tree first, it also misses D3’s required `LandingStopPrefix` halt. After a started session has safely stopped, check branch integrity and classify an unusable result with its cause; retain hard errors for cases where safe termination or inspection cannot be established.

2. **LOW — design — `internal/landingshed/fixreview.go:71–85`: An approval can omit the mandatory Findings section.**  
   For the concrete contents `verdict: approved\n`, the loop sees no heading, the rejection-only validation is skipped, and the parser returns approval. The gate consequently passes and Finalize lands this malformed report. `contracts/specs/landing-fix-review-spec.md:12–14` requires a section opened by `## Findings`; only its contents may be empty for approval. Track whether that section was present and reject its absence, allowing the existing re-prompt/fallback path to handle malformed approvals.

Tool-call manifest:

1. Read seatengine launch/join, fixer session callers, and shuttle failure locations because the inherited context lacked actual session error propagation.
2. Read cleanliness and atomic-write helpers, shuttle wait-error paths, numbered review code, seat-table validation, and Finalize call-site locations to ground concrete failure cases and their routing.
