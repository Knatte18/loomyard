---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md, D2 and D4
    summary: Ancestor green selection does not require compatible commands and test arguments
    fix: In D2 require every candidate ancestor whole-green entry to match the current tier fingerprint and module and to cover the current invocation's test arguments under D4's directional coverage rule. Explicitly exclude filtered passes as bases for unfiltered requests. Apply the parent-merge-base fallback when no compatible ancestor exists. Add acceptance scenarios for an ancestor tested with an old tier command and for an ancestor tested only with a -run filter.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md, D7
    summary: The zero-test refusal also rejects intentionally filtered runs
    fix: In D7 restrict the missing-tag check to invocations expected to execute tests in the package, accounting for explicit test selection and benchmark-only requests. State how intentional zero-match requests are distinguished from missing-tag commands, preserving D3's manual invocation behavior. Add acceptance coverage for a correctly tagged package with an explicit -run filter matching no tests and for a benchmark-only invocation.
  - id: F3
    severity: LOW
    class: consistency
    location: _lyx/discussion/decision-record.md, D4; _lyx/discussion/support-log.md, Turn 3 and Rejected alternatives
    summary: Exact argument matching does not make -count=1 a repeatable force-rerun mechanism
    fix: Resolve the override policy explicitly in D4. If -count=1 is to force execution as the support log claims, make such requests bypass green-record reads on every invocation and state their write policy. Otherwise remove the force-rerun claim from D4 and the support log and explicitly state that repeated identical -count=1 requests can carry. Align the argument acceptance criterion with the selected policy.
---

### [BLOCKING] Ancestor inheritance needs its own compatibility bound

**Location:** `_lyx/discussion/decision-record.md`, D2 and D4.

**Issue:** D2 selects an ancestor by ancestry, tier and whole-green tree. D4's same-tree carry has command and argument compatibility rules, but D2 does not apply those rules to its different-tree base selection. A whole-target pass with a narrow `-run` argument, or a pass under the previous tier command, can therefore become the base for a new unfiltered or changed-command run. Packages unchanged since that ancestor are omitted even though the required tests have never passed there. D2's statement that a narrowed green never becomes a base addresses package coverage, but does not distinguish whole-package coverage from filtered test execution. This is an inherited-green escape hatch whose required compatibility bound is missing.

### [MEDIUM] Intentional zero-test runs are classified as configuration failures

**Location:** `_lyx/discussion/decision-record.md`, D7.

**Issue:** D7 fails whenever a package emits zero test executions and contains a test function eligible under the tier tag. D3 retains manual flags and D4 explicitly models test arguments and `-run` filtering. A correctly tagged package with `-run` selecting no tests meets D7's failure condition, as can a benchmark-only request using `-run=^$`. Neither indicates a missing tier tag. The stated diagnostic and refusal therefore reject valid retained usage; the design needs to distinguish selection from accidental non-compilation.

### [LOW] The stated rerun mechanism stops working after its first pass

**Location:** `_lyx/discussion/decision-record.md`, D4; `_lyx/discussion/support-log.md`, Turn 3 and Rejected alternatives.

**Issue:** D4 stores arguments and permits an exact match for nonempty arguments, while saying that `-- -count=1` reruns. The support log explicitly presents this as the override and the reason to reject a new rerun flag. After one successful request with those arguments, the next identical request matches the saved entry and runs nothing. Argument identity only separates the first such request from an empty-argument pass; it does not provide the described reusable override.

## Rejected

- D1 bounds `gate_key` to hub-defined tiers and scopes. D8 bounds entry-gate reads, skip routing, resume behavior and the operator's goto bypass. D10 identifies the single setting forced by `--window` and the zero-budget behavior; these edges are sufficiently bounded.
- D6 identifies which guard tiers remain permitted and skips groups only after whole execution or whole carry. D13 limits the legacy tag allowance to guard-marked files and assigns its removal to a follow-up after deployment. The transitional overlap is stated rather than an unbounded permanent guard downgrade.
- D13's discovered-test check is a rollout validation of a stated configuration prerequisite, not a deferred question about an unknown design fact. D7 specifies the count source, and D11 records its event-semantics evidence and the measurement's inclusion of setup time.
- D4's imprecise retention horizon may reduce ancestor-cache hits, but D2 already supplies a fallback when no retained ancestor is available. No unsupported guarantee of indefinite retention is needed for correctness.
- The rationale, assumptions and risks contain relevant design reasons, operational ownership or constraints. Their presence alone does not justify relocation; no substantive deliberation-only passage needs moving in this review.
- D13 asks for a different board entry. The Out list describes scope and asks for no mutation of this task's own entry.
