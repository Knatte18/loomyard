---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:100
    summary: A failed later amendment deletes the record of earlier committed amendments
    fix: In D7, require failure rollback to restore the amendment record to its pre-call contents and timestamp, removing it only when this call created it. Update the acceptance criterion at line 178 to distinguish a failed first call from a failed append to an existing record, and require that an earlier committed amendment remains routable after the latter failure.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:80
    summary: The wrong-label bound promises detection that merge and verify cannot guarantee
    fix: Replace D4's never-silently guarantee with the actual bound: current execution retains card order, D6 supplies semantic label review, and a missed wrong label can silently lose an ordering requirement in a future parallel consumer. Keep parallel execution outside scope and state that merge conflicts and verification catch only some such errors.
  - id: F3
    severity: MEDIUM
    class: consistency
    location: _lyx/discussion/decision-record.md:93
    summary: Requiring all documentation work under Extends contradicts the label's own review rule
    fix: In D6, keep documentation on the code-writing card but require its label to reflect the operation: Extends for additions to existing refs, Edit for rewriting existing documentation, and Create for new refs. Add an acceptance criterion covering a code card that updates existing documentation without receiving a false Extends label.
  - id: F4
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:128
    summary: Root-first path resolution can attribute a package test failure to an unrelated root file
    fix: In D9 step 1, resolve bare and dot-relative test locations through the failure identity's package before considering root-relative paths; reserve root-first resolution for locations whose output context establishes that meaning. State that unresolved ambiguity produces no card attribution. Add an acceptance scenario with identically named root and package test files, requiring the package file to win for a package test failure.
---

### [BLOCKING] Preserve previously pending amendments on rollback

**Location:** _lyx/discussion/decision-record.md:100

**Issue:** D7 combines an accumulating record with unconditional removal on append or commit failure. If amendment A commits successfully and amendment B then fails, following the specified cleanup deletes A's pending title and admission timestamp too. A remains in the committed decision record, but the settled-segment guard now refuses the route that was supposed to apply it. The acceptance criterion that a failed commit leaves neither repeats the problem instead of preserving the previous successful transaction. This breaks the amendment-owner goal on an explicitly supported second call.

### [MEDIUM] State the actual limit of wrong-label detection

**Location:** _lyx/discussion/decision-record.md:80

**Issue:** A missing dependency does not necessarily produce overlapping textual edits or a failing verification. For example, two edits to different paragraphs of the same document can merge cleanly while leaving contradictory instructions; verification need not inspect that meaning. The current sequential execution and D6's review are meaningful bounds, but they do not establish the claimed future guarantee. The attack-surface statement must acknowledge what a missed semantic mislabel can let through, without adding parallel machinery to this task.

### [MEDIUM] Documentation must use the label its operation permits

**Location:** _lyx/discussion/decision-record.md:93

**Issue:** D3 permits Extends only for additions to existing refs that change nothing already there. D6 immediately makes rewriting, reordering, or removing content under that label a finding, then instructs every code-writing card to write its docs under Extends. Scope includes changes to the existing plan specification and package documentation, which can require replacing existing statements. Following the unconditional stencil instruction would therefore create a plan-review finding. Keeping docs on the same card is compatible with choosing the correct label.

### [MEDIUM] Package context must beat an unrelated basename match

**Location:** _lyx/discussion/decision-record.md:128

**Issue:** D9 first accepts any path that exists at Head as written, before its package-relative rule. With both `a_test.go` and `internal/p/a_test.go` present, a test failure in package `internal/p` reported as `a_test.go:5` therefore resolves to the root file. This can produce a confident but wrong card attribution rather than an untraced location. `internal/websterengine/verifyparse.go` retains the failure identity's package, and quarry v0.4.0's `quarry/enclose.go` explicitly interprets relative paths as repository-root-relative; quarry cannot repair the mistaken choice after normalization.

## Rejected

- D7/D8 do state meaningful routing bounds: only Plan-Review is reopened, the amendment must postdate the settle, done cards remain frozen, judgment retains the spent budget, and scope widening is handed on. The existing arbiter also has a persisted per-run spawn limit in `internal/loomcli/arbiter.go`. There is no evidence here for an additional unbounded-autonomy finding.
- D1 and D2 use fenced lines as prescriptions consistently. D2 explicitly allows an already-changed worktree bullet to satisfy a declaration and records the remaining undeclared-prose escape and its downstream headroom guards. The stated rework exception is not itself a missing prescription.
- D4's any-other-type-label rule covers Edit, Create and delete pairings; Uses is explicitly covered, and D3 preserves delete-order validation. Missing separate enumerations of those pairs are not findings.
- D9 supplies a location-mapping choice, an explicit matching rule and a pinned quarry source rather than leaving those facts for Plan to discover. The installed v0.4.0 facade confirms revision-based EncloseAt and per-location rejection behavior. F4 concerns the chosen normalization precedence, not missing plan-level interface research.
- D5's estimate explains the actionable decision that measurement comes first without gating the label build. Its short rationale belongs with that decision. The Open risks describe operational limits and recovery consequences, not an interview transcript; moving them would remove useful context from the plan writer. No relocation is required.
- No decision changes this task's own board entry. The absent PATTERN changes section agrees with the explicit constraint that no overview entry is added.
