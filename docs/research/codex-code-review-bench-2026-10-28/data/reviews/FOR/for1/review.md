---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: lens:code-affected-set
    location: internal/landingshed/fixreview.go:226
    summary: The new report gate lacks its required CLI self-check and parity check
    fix: Add a CLI self-check for landing fix-review reports under the existing loom command subtree, calling ParseFixReviewReport just as the new gate does, and add a parity test comparing the CLI and gate results for valid and invalid reports.
  - id: F2
    severity: BLOCKING
    class: design
    origin: lens:code-correctness
    location: internal/landingshed/finalize.go:741
    summary: Persisting an approval discards unknown attempt-record keys
    fix: Change WriteFixerAttempts in internal/landingshed/fixerattempts.go to preserve unknown JSON keys through internal/state or equivalent retained fields, and use that preserving writer for the new approval update.
  - id: F3
    severity: MEDIUM
    class: design
    origin: lens:code-tests
    location: internal/landingshed/finalize_gate_test.go:370
    summary: The new review lifecycle leaves several acceptance guarantees untested
    fix: Extend TestFinalize_GateAndFixer and its fake runner to assert approval persistence before an interrupted landing and reuse on resume, re-review after HEAD changes, reviewer timeout rerouting, transient reviewer-start failure, the configured fixer model and timeout, and exactly one successful report-repair re-prompt. Exercise the review runner rather than the separate merge-fixer runner.
  - id: F4
    severity: LOW
    class: design
    origin: lens:code-correctness
    location: internal/landingshed/fixreview.go:71
    summary: A bare approval passes without the required Findings section
    fix: Track whether ParseFixReviewReport encountered the Findings heading and return a spec-naming error when it is absent, including for approvals; retain acceptance of an explicitly present empty Findings section on approval and cover the distinction in the existing parser test table.
---

Reviewed `161a4a7f..HEAD` against the board snapshot and decision record, checking the landing summary as a claim rather than an answer key.
No tests or other executable checks were run, as the review instructions require read-only source analysis.

### [BLOCKING] The report gate needs a self-check

**Location:** `internal/landingshed/fixreview.go:226`

**Issue:** The new `fix-review-report` mechanical gate validates the report through `ParseFixReviewReport`, but the parser's only production callers are this closure and the final report read in the same file.
There is no CLI self-check or parity check for it in the diff.
`pattern/PATTERN-gate-slot.md` explicitly requires a verb and parity check when adding a mechanical gate, including a gate implemented inside a producer closure.
This is a missing implementation obligation, not a report of a failed mechanical gate.

### [BLOCKING] The approval rewrite loses unknown fields

**Location:** `internal/landingshed/finalize.go:741`

**Issue:** The newly added approval path rewrites the attempt record after setting `Reviewed`.
`ReadFixerAttempts` decodes only the known struct fields, and `WriteFixerAttempts` marshals that struct before `writeBytesAtomic` replaces the file.
For example, a resumed record with an additional `future_key` loses that key when the solo reviewer approves.
The previous over-cap reroute did not perform this rewrite.
Although the helper predates the diff, this new rewrite path violates `PATTERN-tolerant-records`, whose background requires preserving unknown keys through `internal/state` or equivalent retained fields.
The matching findings from the correctness and affected-set lenses are consolidated here.

### [MEDIUM] Lifecycle guarantees lack coverage

**Location:** `internal/landingshed/finalize_gate_test.go:370`

**Issue:** The approval case reaches Done, where the attempt record is removed; the resume case separately seeds `Reviewed` directly at line 448.
Neither observes that a real approval wrote the record before an interrupted landing, and neither checks that a changed HEAD invalidates that approval.
Deleting the approval write or accepting any nonempty approved HEAD would therefore escape these assertions.
The new runner's timeout and transient-start branches, its configured model and timeout, and successful bounded report re-prompting likewise lack covering assertions.
Existing tests of those behaviors in the merge fixer exercise a different implementation.
These are coverage gaps under the rubric's Test economy rule, not requests for helper-by-helper tests.

### [LOW] The parser accepts an incomplete approval report

**Location:** `internal/landingshed/fixreview.go:71`

**Issue:** The spec requires a `## Findings` section after the verdict, allowing its contents to be empty on approval.
The parser tracks whether it is currently inside that section but never requires the heading to exist.
Thus `verdict: approved` alone passes the report gate and authorizes landing, despite not matching the declared report shape.
The existing empty-rejection check is correct; this gap concerns the missing section on an approval.

The remaining focused routes are consistent with the requested behavior in source: only an over-cap change with downstream review starts the solo reviewer; spent, zero-budget and pawl paths retain their previous routing; rejection and unusable reports reroute with a cause; dirty tree or moved HEAD halts; startup failure is marked transient.
The approval field is assigned only after approval and compared with the current HEAD, although its persistence and invalidation lack the coverage described above.
The stencil names the fixer range, cap reason, fixer report and decision record, references the spec, and renders both directives through the registered role.
The recipe comments and updated landing documentation describe the implemented routing.
