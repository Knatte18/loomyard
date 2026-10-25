---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:173
    summary: Runner exit behaviour remains an unsupported design assumption
    fix: Research the gated-project VSTest behaviour and unmatched-filter MTP behaviour at the SDK and runner versions the design supports. Record the answers and named versioned evidence in Decisions or Constraints, replace the factual auto-mode assumption, and revise the MTP risk and filtering policy to account for the intersection of a selected class filter with the tier filter being empty even in a real test project. Settle the intended handling of that case in the discussion rather than leaving it to Plan or implementation.
  - id: F2
    severity: MEDIUM
    class: consistency
    location: _lyx/discussion/decision-record.md:62
    summary: The empty-test-scope build promise omits the shared-build opt-in condition
    fix: Align Decision 3 and the empty-closure acceptance criterion with Decision 6 by specifying that the declared build runs for an empty test scope from a non-empty seed only when a listed tier opts in with shared_build. State that no opted-in tier means no shared build. Update the build scheduling acceptance criterion to include the non-empty-seed empty-test-scope trigger and the empty-seed exclusion, as well as the existing carried and inert-only exclusions.
  - id: F3
    severity: MEDIUM
    class: consistency
    location: _lyx/discussion/decision-record.md:168
    summary: A prescribed PATTERN background change is declared to be no PATTERN change
    fix: Remove the no-PATTERN-changes assertion. Add a PATTERN changes section after Acceptance criteria with a changes bullet for PATTERN-gate-slot, retaining its existing overview requirement about held slots and green-record carry. Keep the instruction to narrow the background trimpath rule to Go builds as the detailed requirement associated with that entry, with the edit landing in the build-step card.
---

### [BLOCKING] Runner facts have not been settled with evidence

**Location:** _lyx/discussion/decision-record.md:173; _lyx/discussion/decision-record.md:178

**Issue:** The design relies on a gated Models project returning success without discovery, but records this as an auto-mode assumption. The support log's researched facts cover filter grammar, node reuse and restore, not this exit behaviour or the MTP claim. Neither claim names the applicable SDK/runner version and supporting evidence. These are facts on which project selection rests, not operator choices, so the rubric requires their resolution in the decision record. The MTP risk also says an unmatched filter arises only in a project not running as a test project. Decision 4 composes a tier filter with the class terms: a selected class can exist while all its tests are excluded by the tier filter. The stated bound therefore does not cover an ordinary supported filtering case.

### [MEDIUM] Empty-scope build scheduling has two contracts

**Location:** _lyx/discussion/decision-record.md:62; _lyx/discussion/decision-record.md:102; _lyx/discussion/decision-record.md:190; _lyx/discussion/decision-record.md:197

**Issue:** Decision 3 promises a build whenever the hub declares one and a non-empty seed selects no class. Decision 6 instead schedules it through the first listed opted-in tier. A hub declaring a build while the listed tiers do not opt in is valid under the stated load rules, but receives incompatible instructions for this case. The dedicated empty-closure acceptance criterion repeats the unconditional promise, while the general build criterion mentions only a tier that runs and omits the special empty-test-scope trigger. The plan writer should receive one explicit scheduling rule.

### [MEDIUM] The PATTERN edit is not recorded as a change

**Location:** _lyx/discussion/decision-record.md:168

**Issue:** The last Constraint both denies any PATTERN changes and directs an edit to the PATTERN-gate-slot background. The existing background actually says every gate build keeps trimpath, so limiting it to Go changes that entry's requirement. The discussion spec requires every PATTERN entry the design changes to be listed under PATTERN changes; putting this edit only in Constraints contradicts that declaration and bypasses the intended change record.

## Rejected

- Decision 5 and Decision 8 state substantive bounds: inert patterns are hub-only, opt-in and fingerprinted; ancestor carry requires a matching whole record on a clean tree; Decision 8 explicitly overrides inert and no-class narrowing for carry-disabled tiers. The broader no-tier wording in Scope and the inert acceptance criterion should be read with that explicit exception, not treated as a separate missing guard.
- The filtered-pass green-record rule does not prevent Finalize carrying a genuinely whole Merge-In pass. Scope filter terms differ from the tier's own declared filter, which is fingerprinted. Decision 10 and that Constraint can hold together.
- The documentation inventory and Decision 9's repo-rule text prescribe deliverables the plan writer needs. Moving those requirements to the support log would lose design input because Plan does not read it. No relocation finding is warranted.
