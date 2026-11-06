---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    class: scope
    origin: lens:code-affected-set
    location: contracts/stencils/webster/webster-template-master.md:251
    summary: The post-fixer retry unconditionally re-enters the fixer rung for a new plan conflict
    fix: In the fixed-verdict branch at line 251, repeat steps 1–3 only when the retried begin-batch returns plan_defect or tree_moved; explicitly send plan_order to the immediate-stuck instruction at line 242 and quote the new refusal verbatim.
  - id: F2
    severity: MEDIUM
    class: scope
    origin: lens:code-affected-set
    location: docs/overview.md:448
    summary: Public documentation still routes every plan conflict through the cold fixer
    fix: Qualify the fixer route in docs/overview.md at lines 448 and 645, internal/webstercli/cli.go at lines 239–240, and internal/webstercli/doc.go at line 62 with plan_defect or tree_moved. State that plan_order ends Merriam stuck immediately, preserving the conflict record for the existing plan-review route.
  - id: F3
    severity: MEDIUM
    class: scope
    origin: lens:code-tests
    location: internal/websterengine/beginbatch_test.go:837
    summary: No test pins that unrelated informational findings preserve plan_order
    fix: Add a row to TestBeginBatch_ConflictCauseNamesAPlanOrderConflict containing a blocking CheckDeleteBeforeReference finding and an informational finding with a different check, expecting ConflictCausePlanOrder and moveDelete. Use the existing fixture and assertions.
---

### [MEDIUM] Reclassify the refusal after a fixer retry

**Location:** contracts/stencils/webster/webster-template-master.md:251

**Issue:** The initial routing explicitly skips the fixer for `plan_order` at line 242, but the fixed-verdict branch subsequently directs every `plan_conflict` back to step 1, whose next step runs the fixer. Repairing a stale Edit target can expose a previously unresolved reference and produce `plan_order` on the retry. These instructions conflict at that transition, contrary to the commit's requirement that a plan-order refusal ends stuck immediately. The plan-fix stencil at lines 31–36 limits the fixer to mechanical target edits.

### [MEDIUM] Document the plan-order bypass at the remaining readers

**Location:** docs/overview.md:448

**Issue:** The overview still says a begin-batch plan conflict goes first to the cold fixer, and line 645 describes plan-review rerouting only after the fixer could not clear it. The root CLI help at internal/webstercli/cli.go:239 and package documentation at internal/webstercli/doc.go:62 retain the same unqualified route. The new master stencil sends `plan_order` directly to a stuck outcome, and internal/shedadapters/webster.go:106 claims its conflict record without requiring a fixer attempt. These descriptions leave readers with the recovery route the commit explicitly intended to replace for this cause.

### [MEDIUM] Cover an unrelated advisory beside a blocking order finding

**Location:** internal/websterengine/beginbatch_test.go:837

**Issue:** Both successful `plan_order` rows contain only a blocking delete-order finding. The informational row instead places an informational delete-order finding beside another blocking check. None pins the requirement that an unrelated advisory must not prevent `plan_order`. Moving the orderOnly update outside the severity guard at internal/websterengine/beginbatch.go:500–506 would violate that requirement while the new table still passed.

The engine's classification and error rendering match the commit's requested behavior on inspection. No BLOCKING finding was identified. This was a read-only source review; no tests were executed. All three lens reports are saved alongside this report.
