---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:173
    summary: Runner behavior needed for narrowed runs remains an unsupported factual assumption
    fix: Resolve the gated-project and zero-match runner behavior during Discussion using available upstream or repository evidence, recording sources and applicable versions in Decisions or Constraints. Correct the Open risks claim that an existing selected class prevents zero matches to account for the intersection with the tier filter. State the resulting supported runner behavior and any compatibility boundary without assigning this research to Plan or requiring access to the excluded Models checkout.
  - id: F2
    severity: MEDIUM
    class: consistency
    location: _lyx/discussion/decision-record.md:86
    summary: The unconditional inert-scope skip contradicts forced whole runs and the green-only constraint
    fix: Qualify Decision 5 and the inert acceptance criterion to exclude selected carry-false tiers explicitly, with Decision 8 taking precedence before deciding whether commands or the shared build run. Qualify the Scope bullet so whole gates require an eligible ancestor green entry. In the first Constraint and Decision 5 rationale, distinguish omission through an empty narrowed scope from carrying a tier through a green record; retain the clean-tree and record requirements for carrying.
  - id: F3
    severity: MEDIUM
    class: consistency
    location: _lyx/discussion/decision-record.md:62
    summary: The no-class compile guarantee omits the shared-build opt-in required by Decision 6
    fix: Align Decision 3 and its no-class acceptance criterion with Decision 6 by requiring a declared build and an opted-in selected tier for the compile guarantee, and state the outcome when no selected tier opts in. Update the general build acceptance criterion to include a non-empty-seed no-class tier as a build trigger even though its test command does not run, and explicitly preserve the empty-seed and inert-only exclusions after carry-false scope overrides.
---

### [BLOCKING] Resolve the runner facts the design relies on

**Location:** _lyx/discussion/decision-record.md:173; Open risks at line 178; support-log.md, Turn 2.

**Issue:** The record assumes that naming a gated Models project without its MSBuild property succeeds under VSTest, and relies on that assumption when selecting projects independently of the tier. Neither artifact records evidence or applicable versions for this behavior. The support log's researched facts cover filter grammar, build processes and restore, not this premise. Calling it an auto-mode assumption leaves a design-dependent fact for downstream discovery, which the rubric expressly disallows. Moreover, the risk's stated bound is internally unsound: Decision 4 intersects the class terms with the tier filter, so an existing selected class whose tests are all excluded by that filter can still yield zero matches in an otherwise enabled project. The recorded MTP risk therefore does not cover the design's own filter composition.

### [MEDIUM] Make inert skipping and forced execution agree

**Location:** _lyx/discussion/decision-record.md:86; lines 15, 94, 133, 158 and 191.

**Issue:** Decision 5 says no listed command runs for an inert-only computed or base scope, and the acceptance criterion repeats this for all three scopers. Decision 8 expressly requires a carry-false tier to run on that same diff. The exception is specified for ancestor carrying but omitted from the empty-scope rule. This is particularly material for capture and timing tiers, whose execution is the reason for Decision 8. Separately, the Scope bullet promises no tests at whole gates without the ancestor-record qualification in Decision 5, while the first Constraint and Decision 5 rationale say every skip requires a clean tree and a green record even though the narrowed empty-scope rule requires neither. These statements should describe one execution policy.

### [MEDIUM] State the conditions on the no-class build guarantee

**Location:** _lyx/discussion/decision-record.md:62; lines 102, 104, 190 and 197.

**Issue:** Decision 3 promises a build whenever a non-empty seed selects no class and the hub declares a build. Decision 6 instead triggers it only through an opted-in tier. A valid configuration can declare a build while the selected tier has shared_build unset; the former promises compile validation and the latter schedules none. The no-class acceptance criterion drops even the declared-build qualification. Conversely, the general build criterion describes the trigger as the first opted-in tier that runs, omitting Decision 6's explicit trigger when no test command runs. The distinction matters because the compile guarantee is the stated protection against an empty test selection hiding a broken change.

## Rejected

- Decision 5's ancestor carry is not unbounded: it names the whole entry, current fingerprint, empty arguments, module directory, ancestry, inert-only diff, clean tree, carry eligibility and bounded history. The hub's ability to declare test inputs inert is also expressly acknowledged. Decision 8 states both the scope override and the explicit-target exception; its slot-exclusivity bound excludes raw commands. The consistency problem above does not justify inventing an additional missing-bound finding.
- The filtered-green rule and Decision 10 agree for ordinary carry-eligible tiers: Merge-In runs at whole scope, so it can write the whole entry Finalize needs. A configured tier filter is distinct from the scope filter terms that prohibit recording. No change to this rule is warranted.
- The final Constraint changes background guidance about Go builds, not a PATTERN overview entry. The supplied PATTERN-changes contract covers entry additions, changes and removals; the background clarification alone does not require such a declaration.
- The Scope documentation bullet and Decision 9 contain requirements for the documentation and the hub lens. Moving those requirements solely to the support log would hide them from Plan-Write. Their specificity is useful design input, not surrounding deliberation that should be relocated.
- Q2 in the support log still names Webster among whole gates, but Q12 and Decision 10 explicitly record the subsequent scope choice. This historical ledger wording does not leave the final plan-facing decision open.
