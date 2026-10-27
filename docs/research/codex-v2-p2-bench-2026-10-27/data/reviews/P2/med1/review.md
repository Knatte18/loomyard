---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: lens:guard-reach
    location: _lyx/plan/10-csharp-rename-derivation.md:35
    summary: The unsupported-member refusal is tested only below the refusal boundary
    fix: Revise card 10's test intent so TestRenameCSharpDecl exercises CanonicalizeHandles with an unsupported old member and asserts rename-old-unresolved and its way forward, rather than calling the private derivation helper and asserting false. Remove the private-head assertions already represented by TestGlyphChain_CSharp's public Rename cases; keep distinct uncovered behavior at the public surface.
  - id: F2
    severity: BLOCKING
    class: design
    origin: handler
    location: _lyx/plan/14-package-docs-pattern.md:11
    summary: Required package documentation and the new invariant are deferred beyond their implementation commits
    fix: Make the package documentation and alphabet-invariant updates land in the commits that introduce their respective behavior, as the repository instructions require. A concrete redistribution of targets is withheld because the mandatory overlay validation cannot be executed under this round's read-only analysis rule; redundant-file-target and redundant-package-target must constrain that redistribution.
  - id: F3
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/09-csharp-create-handles.md:24
    summary: The fallback token rule selects the interface type instead of the declared member
    fix: In card 9, revise the missing-name fallback to skip a generic qualifier whose closing angle bracket is followed by a dot before selecting the declared member. Preserve the specified known-name rule and retain the existing draft explicit-interface Create row as the reaching regression case.
  - id: F4
    severity: MEDIUM
    class: design
    origin: lens:affected-set
    location: _lyx/plan/06-plan-language-mismatch.md:6
    summary: New quarry-status consumers omit the existing status-enforcement registration
    fix: Complete the enforcement work in cards 6, 8, 11 and 12 for languageMismatchFindings, crossAlphabetDetail, nameResigns, resignCollisionFindings and RebindResigns, with reviewed file/function entries and reasons in allowedStatusConsumers. A concrete target-list amendment is withheld because this round forbids executing the mandatory overlay validator; any eventual amendment must satisfy redundant-file-target and redundant-package-target.
---

### [BLOCKING] Test the unsupported-member refusal through its public surface

**Location:** `_lyx/plan/10-csharp-rename-derivation.md:35–43`.
**Issue:** Card 10 introduces an unsupported-member `rename-old-unresolved` branch, but its operator example only calls `renameCSharpDecl` and checks `false`.
That does not reach the refusal in `renameDeclSource`, which `CanonicalizeHandles` invokes at `internal/planglyph/handle.go:220`, or assert its recovery detail.
The separate glyph-chain rejection exercises a malformed new handle, not this branch.
This violates the prescribed-refusal reachability rule and the decision record's refusal constraint.
The private-helper test also violates `PATTERN-test-economy`'s public-surface requirement: the same card already assigns const, generic-field and property Rename behavior to `TestGlyphChain_CSharp`.
Testing exact private intermediate heads does not make those assertions public behavior.

### [BLOCKING] Documentation must accompany the implementing commits

**Location:** `_lyx/plan/14-package-docs-pattern.md:11–23`.
**Issue:** Card 14 collects the package documentation and PATTERN changes after cards 2–13 have introduced the alphabet, validation and execution changes.
The supplied repository instructions require observable CLI changes and cross-cutting infrastructure to update their package documentation in the same commit, and require a new cross-cutting invariant in PATTERN in the same commit as its code.
For example, card 2 admits `language: csharp` and introduces the alphabet-selection API, while its package documentation and the new alphabet invariant wait until card 14.
Cards 12 and 13 likewise defer their post-batch and batching documentation.
The final wording proposed in card 14 implements the declared PATTERN change correctly; its placement is the defect.

### [MEDIUM] Draft interface handles select the wrong token

**Location:** `_lyx/plan/09-csharp-create-handles.md:23–35`.
**Issue:** For the prescribed draft handle ending in `Renderer.IProvider<int>.draft` and head `int IProvider<int>.Count { get; }`, the requested name is absent.
The fallback selects `IProvider`, the first identifier followed by an allowed token, because `<` is allowed.
It therefore cannot find the qualifier immediately before `Count`, and `csharpOwner` retains `Renderer.IProvider<int>`.
Quarry v0.4.0 rejects precisely that container at `internal/engine/name.go:340–345` in the installed dependency, because an Owner component carries type arguments.
The card promises successful binding for this very row, but its algorithm produces `handle-name-failed`.
The handler independently confirmed the same defect; it is recorded once.

### [MEDIUM] New status readers need enforcement coverage on their own cards

**Location:** `_lyx/plan/06-plan-language-mismatch.md:6`, with the same omission in cards 8, 11 and 12.
**Issue:** `internal/planglyph/status_enforcement_test.go:64–99` declares a closed allowlist for readers of selectors named `Status`, `Known`, `Rejected` or `Unit`.
`TestStatusEnforcement_NoOutOfAllowlistConsumer` rejects new enclosing functions outside that list at lines 135 onward.
The plan introduces status readers in `languageMismatchFindings`, `crossAlphabetDetail`, `resignCollisionFindings` and `RebindResigns`; `nameResigns` also reads the parsed glyph's `Unit`, which the scanner explicitly includes even on non-status types.
No card covers their allowlist registrations, so the implementation encounters the existing test failure starting with card 6.
This is a single affected-set enumeration gap across the new consumers, not a separate finding per function.

## Review limits

The instruction to read files and run nothing conflicts with executing `lyx board get` and `lyx loom validate-plan --overlay`.
The read-only analysis constraint was followed: no board command, validator, build or test was executed.
Consequently target-list redistribution proposals requiring overlay validation are explicitly withheld above.
No prior review or support log was read.
All three fork reviews completed before consolidation; the recovery lens found no defective way forward.
The focused re-sign collision variants, post-batch restamp and rewrite, language-check halves, namespace-pair bounds, and card 15's pointer to the plan spec are carried by the plan.
No finding is raised for absent Verify lines, which are optional, or for dependency completeness already assigned to the mechanical gate.
