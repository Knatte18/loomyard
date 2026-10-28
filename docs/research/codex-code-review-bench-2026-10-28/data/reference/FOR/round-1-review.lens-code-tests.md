```yaml
lens: code-tests
outcome: reported
```

Lens code-tests: for each behaviour the diff adds or changes, the test row that pins it, or none.

**Per behaviour**
- Over-cap change with a review after the reroute spawns one fix review over `Baseline..HEAD` (D1, D4): pinned by `TestFinalize_GateAndFixer/"a fix over the caps goes to the fix review…"`. It asserts the role, `baseline`, `head`, `cap_reason` and `fixer_report_path`, and that there are two sessions.
- Approval lands with no reroute (D3): pinned by the "fix review's end decides" row "an approval lands".
- Approval writes `Reviewed` into the attempt record (D6): none. The only `Reviewed` test seeds the field by hand, so dropping the write in `judgeFixerChange` (`finalize.go:741-244`) breaks nothing.
- A later pass at the approved HEAD lands with no second review (D6): pinned by "a pass at the HEAD the fix review approved lands…".
- A HEAD that moved since the approval is reviewed again (D6): none. Changing `rec.Reviewed == head` to `rec.Reviewed != ""` fails no test.
- Rejection reroutes with the cap, the findings, the report path and the fixer commits (D3): pinned by the over-cap table (`requireReroute` with `rejectedReview`) and the "two turns past the cap together" row.
- No usable verdict reroutes naming why (D3):
  - Missing report: pinned.
  - Unknown verdict: pinned.
  - Session timeout (`OutcomeTimeout`, "ran past fixer_timeout_min", `fixreview.go:609-610`): none. The fake's `sessionTimeout` exists but no review row uses it.
  - Re-prompt over an unparseable report (`fixReviewReprompts`, the gate closure): none. No row scripts a bad first report followed by a good second one.
- Dirty tree after the review halts with `LandingStopPrefix` (D3): pinned by the "dirties the tree" row.
- Moved head after the review halts with `LandingStopPrefix` (D3): pinned by the "commits" row.
- Failed review start is a transient error (D3): none. The fake has `startErr`, but no review row sets it, so `fixreview.go:579-585` is uncovered.
- No review after the reroute still halts as today (D2): pinned. The `reviewed=false` test asserts one session only and that `Halt` is `over-cap`.
- Spent fixer still reroutes with no review session (D2): weakly pinned. The resumed-entry test (`finalize_gate_test.go:~305`) only errors on a non-review session, so a spent fixer that wrongly spawned a review would pass.
- `RerouteReviewed` nil or erroring (`reviewFollowsReroute`, `endSpent` error path): none. This is a new error path.
- `ParseFixReviewReport` (D7): pinned by `TestParseFixReviewReport`. It covers both verdicts, an empty Findings section on approval, an unknown verdict, a rejection with no findings, bullets outside the section, and a missing file wrapping `fs.ErrNotExist`.
- Fix review runs under the `fixer` model-spec and `fixer_timeout_min` (D5): none. No test checks `table.Timeout` or the seat's `ModelChoice`.
- `fixReviewTable` required-field and missing-stencil errors: none. These are internal misuse paths, so I treat them as LOW or no finding.
- Stale review report archived before a new session (`archiveStaleReport` refactor): the fixer path is covered by existing fixer tests. The fix review path has none.
- Spec registry entry: pinned by `specs_test.go` (names and relpath).
- `roleOpeningStencils` and stencil registry entries: no direct row visible in the diff. The existing registry and role tests may cover them, and I did not check.

**Findings**
1. MEDIUM, design, `internal/landingshed/finalize_gate_test.go` ("a pass at the HEAD…") and `finalize.go:741`. The approval-writes-`Reviewed` behaviour (D6) has no test. Evidence: `fx.seed(... Reviewed: "f1")` is the only source of the field. Fix: after the "an approval lands" row, read `ReadFixerAttempts(FixerAttemptsPath(...), 1)` and assert `Reviewed == "f1"`.
2. MEDIUM, design, `finalize.go:722`. "HEAD moved since approval is reviewed again" (D6 bound) has no row. Fix: add a row seeding `Reviewed: "old"` with HEAD `f1` and a rejecting review, and assert a review session runs.
3. MEDIUM, design, `fixreview.go:579-585`. The transient-on-failed-start path (D3) is uncovered. Fix: add a row with `fx.seats.startErr` set for the review turn, asserting `shedengine` transient marking and no landing.
4. MEDIUM, design, `finalize_gate_test.go:~305`. The D2 "spent fixer reroutes with no review session" criterion is not pinned, because the check only rejects non-review roles. Fix: assert `len(fx.seats.tables) == 0` for the spent rows (those with no fixer turns), or assert that no table has `fixReviewRole`.
5. LOW, design, `fixreview.go:609`. The timeout-to-no-verdict route is untested. Fix: add a row in "fix review's end decides" using a review-session timeout, expecting a reroute naming `fixer_timeout_min`.
6. LOW, design, `fixreview.go:571-576`. The single re-prompt over an unparseable report is untested. Fix: add a row with `reviewTurns` of an unparseable report then `approvedReview`, expecting a landing.
7. LOW, design, `fixreview.go:491-494`. The D5 acceptance criterion (fixer model-spec and `fixer_timeout_min` bound) is unasserted. Fix: in the over-cap table row, assert `review.Timeout == time.Duration(cfg.FixerTimeoutMin)*time.Minute` and compare the seat's `ModelChoice` with the fixer's.
8. LOW, design, `finalize.go:252-262`. The new `reviewFollowsReroute` error path (`RerouteReviewed` returning an error) has no row. It is a new error path in `endSpent` and `judgeFixerChange`. Fix: add one row asserting an error stop.

**Tool calls:** none. The manifest and inherited context covered everything.
