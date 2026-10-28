---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: internal/landingshed/finalize.go:741
    summary: The new approval rewrite discards unknown attempt-record keys
    fix: Route the approval record update through internal/state so existing unknown JSON keys survive when Reviewed is saved; replace the raw marshal-and-replace path used by WriteFixerAttempts while preserving its entry and atomic-write semantics.
    origin: lens:code-affected-set
  - id: F2
    severity: BLOCKING
    class: scope
    location: internal/landingshed/fixreview.go:226
    summary: The new mechanical report gate has no CLI self-check or parity check
    fix: Add and register a loom validate-fix-review self-check for the Finalize review report, call ParseFixReviewReport from both that verb and the gate, and add the corresponding parity check and command documentation required by PATTERN-gate-slot.
    origin: handler
  - id: F3
    severity: BLOCKING
    class: design
    location: internal/landingshed/finalize_gate_test.go:370
    summary: Explicit review acceptance criteria and approval persistence lack behavioral coverage
    fix: Extend TestFinalize_GateAndFixer with review-specific timeout and transient-start cases, assertions on the review ModelChoice and Timeout, a malformed-report retry that succeeds on its second turn, and stale-report exclusion; exercise an actual approval followed by interrupted landing and resume at the same and a different HEAD, asserting that only approval writes Reviewed and that the matching HEAD alone skips review.
    origin: lens:code-tests
  - id: F4
    severity: MEDIUM
    class: design
    location: internal/landingshed/fixreview.go:241
    summary: A started review that returns a wait error bypasses the required fallback routing
    fix: Preserve transient handling for failed startup and error handling for an un-stopped seat, but classify cleanliness and HEAD after a started session has stopped even when Run returns a wait error; return fixReviewEndChanged for mutations and fixReviewEndNoVerdict with the wait cause for a clean session without a usable report, retaining errors when branch inspection itself fails.
    origin: lens:code-correctness
  - id: F5
    severity: MEDIUM
    class: design
    location: internal/landingshed/fixreview.go:71
    summary: The parser accepts approval without the required Findings section
    fix: Track whether ParseFixReviewReport encountered the Findings heading and return a spec-naming error when it is absent for either verdict; keep an explicitly empty Findings section valid for approval and invalid for rejection, and extend the existing parser table with the missing-heading case.
    origin: lens:code-correctness
  - id: F6
    severity: LOW
    class: consistency
    location: internal/landingshed/finalize.go:157
    summary: The public Call comment still says every over-cap fix reroutes or halts
    fix: Update the Finalize.Call comment to describe eligible over-cap changes going to the solo review, approval permitting landing, and rejection or no verdict taking the existing reroute.
    origin: lens:code-affected-set
---

Reviewed `161a4a7f..HEAD` against the board snapshot, D1–D7 and the acceptance criteria, with read-only code inspection and no test execution.
The eligibility check, unchanged caps, spent-fixer route, zero-budget and pawl behavior, explicit rejection handling, ordinary dirty-tree and moved-HEAD halts, role registration and stencil inputs match the requested design.
The matching-HEAD approval check is present; the persistence and invalidation coverage remains incomplete as detailed below.

### [BLOCKING] Approval rewrites lose unknown record data

**Location:** `internal/landingshed/finalize.go:741`
**Issue:** The new approval branch calls `WriteFixerAttempts` after setting `Reviewed`.
`ReadFixerAttempts` decodes into the fixed struct, and `WriteFixerAttempts` marshals that struct directly (`fixerattempts.go:47–63`), so an existing unknown key disappears on this newly introduced rewrite.
`writeBytesAtomic` only replaces bytes and does not preserve keys.
This violates `PATTERN-tolerant-records.md:3–8`, which explicitly requires unknown-key preservation on rewrites through `internal/state` or equivalent handling in the type.
The helper predates this diff, but the approval rewrite is new and reaches the violation on a resumed entry whose gate passes.

### [BLOCKING] Report gate lacks its required self-check

**Location:** `internal/landingshed/fixreview.go:226`
**Issue:** The diff adds the `fix-review-report` mechanical gate, whose closure validates the artifact through `ParseFixReviewReport`.
The whole-tree reference search finds its production parser calls only inside `fixreview.go`; no CLI self-check or parity check accompanies it.
`PATTERN-gate-slot.md:24,45–49` requires a new mechanical gate to add its self-check verb and parity check in the same task, including a gate inside a producer closure.
The decision record's lack of a CLI work item does not waive this repository constraint.

### [BLOCKING] Review behavior is not fully pinned by the tests

**Location:** `internal/landingshed/finalize_gate_test.go:370`
**Issue:** The end-routing table covers approval, missing or malformed reports, dirty trees and moved HEADs, but never exercises the new review's timeout or transient startup failure.
The existing fixer tests exercise `fixerRun.run`, not `runFixReview`.
The resume test seeds `Reviewed` directly at line 448, while the approval test completes landing and checks that the record is gone through `requireLanded` at lines 174–185.
Deleting the approval write would therefore leave both cases passing, as would changing the comparison to accept any nonempty reviewed SHA.
No review-table assertion pins the configured model or timeout, and the fake review path does not assert a successful bounded retry or exclusion of a stale approval report.
These are uncovered new behaviors and explicit D3, D5 and D6 acceptance requirements, contrary to the rubric's test-economy requirement and `PATTERN-test-speed`.
This is consolidated as one coverage-enumeration finding rather than one finding per missing case.
No redundant new test or added real-time wait was found.

### [MEDIUM] Session errors skip D3 classification

**Location:** `internal/landingshed/fixreview.go:241`
**Issue:** A started review can finish without a report because shuttle exhausts event-read or status retries (`internal/shuttleengine/wait.go:294–300,385–398`).
`seatengine.Engine.join` stops the chair and returns the wait error (`internal/seatengine/engine.go:376–400`).
The new review runner returns that error immediately, before checking the tree or HEAD.
A clean stopped session therefore becomes a producer error instead of the D3 no-verdict reroute naming its cause; a session that also changed the branch misses the required `LandingStopPrefix` halt.
An actual failure to stop the seat remains an infrastructure error and must not be treated as a safe completed review.

### [MEDIUM] A verdict-only approval passes the report contract

**Location:** `internal/landingshed/fixreview.go:71`
**Issue:** The concrete file `verdict: approved\n` returns an approved report despite lacking the section required by `contracts/specs/landing-fix-review-spec.md:14` and D7.
The parser tracks whether it is inside Findings but never requires that the heading appeared; its final validation only covers rejected verdicts.
The malformed approval consequently passes the report gate and can authorize landing.
The existing rejection-with-empty-Findings case is correctly rejected, and production consumers otherwise use the sole parser.

### [LOW] Public producer documentation describes the old route

**Location:** `internal/landingshed/finalize.go:157`
**Issue:** The `Finalize.Call` comment still says that a spent cap or an over-cap fix reroutes or halts.
The changed implementation now permits an approved over-cap fix to proceed at lines 739–744.
The package documentation and detailed helper comments describe the new behavior, but this directly affected public description was missed.
