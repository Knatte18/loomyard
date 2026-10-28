---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: lens:code-affected-set
    location: internal/landingshed/finalize.go:741
    summary: The new approval rewrite drops unknown attempt-record keys
    fix: Route approval persistence through internal/state's unknown-key-preserving JSON writer, updating WriteFixerAttempts to use that primitive or retaining unknown fields in the record type; preserve existing entry semantics and verify that saving Reviewed retains an extra JSON key.
  - id: F2
    severity: MEDIUM
    class: design
    origin: lens:code-correctness
    location: internal/landingshed/fixreview.go:241
    summary: A started review returning a wait error bypasses integrity checks and the no-verdict reroute
    fix: In runFixReview, distinguish failed startup and unsafe termination from an error returned after a started session was stopped successfully; for the latter, inspect tree cleanliness and HEAD before returning a changed-branch outcome or a no-verdict outcome carrying the error cause. Retain errors when termination or inspection cannot be established, and cover clean and changed-tree wait-error cases.
  - id: F3
    severity: MEDIUM
    class: scope
    origin: lens:code-tests
    location: internal/landingshed/finalize_gate_test.go:443
    summary: Approval persistence and invalidation are not covered by the seeded cache test
    fix: Extend the Finalize fixture tests to approve a review and stop landing after the approval write, read the persisted Reviewed value, resume at that HEAD without another session, and resume at a different HEAD with another session; assert rejection and unusable reports never persist an approval.
  - id: F4
    severity: MEDIUM
    class: scope
    origin: lens:code-tests
    location: internal/landingshed/finalize_gate_test.go:370
    summary: Review timeout, startup failure, stale reports and successful report repair lack coverage
    fix: Add fake-driven review cases for timeout, ErrNotStarted, Chair.NotStarted, a stale approval followed by no fresh report, and an invalid report corrected on the one allowed re-prompt; assert routing, transient classification, cause text and bounded turn count without real waits.
  - id: F5
    severity: MEDIUM
    class: scope
    origin: lens:code-tests
    location: internal/landingshed/finalize_gate_test.go:359
    summary: The new review builder's configured model and timeout are untested
    fix: Extend the existing captured review-table assertions with distinctive fixer model and effort settings and a nondefault timeout, asserting the review seat's ModelChoice and table Timeout match those settings.
  - id: F6
    severity: LOW
    class: design
    origin: lens:code-correctness
    location: internal/landingshed/fixreview.go:71
    summary: The parser accepts approval without the required Findings section
    fix: Track whether ParseFixReviewReport encountered the required Findings heading and return a spec-naming format error when absent for either verdict; keep an empty section valid for approval and add the missing-heading parser case.
  - id: F7
    severity: LOW
    class: consistency
    origin: lens:code-affected-set
    location: internal/landingshed/finalize.go:157
    summary: The Call overview still describes every over-cap fix as rerouting or halting
    fix: Update Finalize.Call's overview comment to describe solo-review approval proceeding to landing and reserve reroute or halt for the other over-cap outcomes and the spent fixer.
---

### [BLOCKING] Preserve unknown keys when recording approval

**Location:** internal/landingshed/finalize.go:741
**Issue:** The new approval branch rewrites the attempt record through `WriteFixerAttempts`, which marshals only the known struct fields in `fixerattempts.go:58–63` and atomically replaces the previous file.
An additional key written by another binary is therefore lost on approval.
`pattern/PATTERN-tolerant-records.md` requires every rewrite to preserve unknown keys through `internal/state` or a type that retains them itself.
This finding concerns the newly added rewrite, not unrelated existing writes; the constraint makes it BLOCKING.

### [MEDIUM] Handle started-session errors through the review outcome path

**Location:** internal/landingshed/fixreview.go:241
**Issue:** A reviewer whose events file becomes unreadable can return a wait error after starting without producing a report (`internal/shuttleengine/wait.go:294–300`).
`seatengine.join` stops that chair and still returns its wait error even when stopping succeeds (`internal/seatengine/engine.go:376–391`).
The immediate return in `runFixReview` bypasses both the dirty-tree/HEAD checks and the no-usable-verdict reroute.
Consequently a clean failed review errors instead of falling back to full review, and one that edited the tree before the wait failure misses the promised `LandingStopPrefix` halt.
The change description promises those routes, and D3 describes the same intended behavior.

### [MEDIUM] Exercise approval persistence across a real resumed fixture pass

**Location:** internal/landingshed/finalize_gate_test.go:443
**Issue:** The cache test seeds `Reviewed: "f1"` itself, so removing the new approval write would not fail it.
The approval test checks successful landing, whose helper expects the attempt record to have been deleted.
No row checks a nonempty approved HEAD differing from the current HEAD, or that rejection leaves approval unset.
The added persistence and invalidation behavior therefore has no covering test, contrary to the rubric's test-economy requirement.

### [MEDIUM] Cover the review-specific failure and retry safeguards

**Location:** internal/landingshed/finalize_gate_test.go:370
**Issue:** The review outcome table covers approval, missing and malformed reports, dirty tree and moved HEAD, but never a timeout or either failed-start representation.
Existing timeout/start tests exercise the separate fixer runner.
No review case starts beside an old approved report or supplies a corrected report on its repair turn, leaving stale-approval prevention and successful bounded repair unpinned.
These are added behaviors, not failures of an already-run mechanical gate.

### [MEDIUM] Pin the review's model and timeout wiring

**Location:** internal/landingshed/finalize_gate_test.go:359
**Issue:** Captured-table assertions check role, range, cap reason and fixer report but never the new builder's resolved model choice or timeout.
`TestFixerTable` covers a different builder that `fixReviewTable` does not call.
Hardcoding the review model or timeout could therefore pass the tests despite changing the configured behavior.

### [LOW] Require the report section that the contract specifies

**Location:** internal/landingshed/fixreview.go:71
**Issue:** The complete input `verdict: approved\n` returns a valid approval despite containing no Findings section.
The contract at `contracts/specs/landing-fix-review-spec.md:12–14` requires that section; approval permits empty contents, not an absent heading.
The parser currently lets this malformed report pass its gate and authorize landing instead of taking the repair/fallback path.
Rejection with an empty Findings section is correctly refused, and the parser remains the sole report parser.

### [LOW] Update the affected public behavior comment

**Location:** internal/landingshed/finalize.go:157
**Issue:** The Call overview still says that a spent cap or an over-cap fix reroutes or halts.
The new approval branch proceeds to landing, so this affected description contradicts the implemented behavior even though the package documentation and recipe comments were updated.

The range reviewed was `161a4a7f..HEAD`, including its commit message, against the board snapshot and the decision record's amendment status.
The core over-cap-only routing, spent-fixer and no-downstream-review paths, matching-HEAD cache check, prompt inputs, parent/edit directives and role registration are implemented as described.
No declared PATTERN change was omitted, and no new test sleeps or production-duration waits were found.
Review was read-only; no tests were run and no target files were changed.
