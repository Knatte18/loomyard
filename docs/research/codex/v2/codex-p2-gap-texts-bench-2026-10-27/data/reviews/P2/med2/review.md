---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/09-csharp-create-handles.md:24
    summary: The fallback selects IProvider instead of the declared Count member
    fix: Revise card 9's declaredNameIndex fallback to skip a generic qualifier whose balanced type-argument list is followed by a dot before selecting the declared member token. Preserve the existing draft-to-Count row and explicitly require that it selects Count and strips IProvider<int> from Owner. Apply the same rule when card 10 uses the helper after renaming the head.
  - id: F2
    severity: MEDIUM
    class: design
    origin: lens:affected-set
    location: _lyx/plan/06-plan-language-mismatch.md:30
    summary: The new quarry readers omit the status-consumer registry required by their package gate
    fix: Account for allowedStatusConsumers in the commits introducing languageMismatchFindings, crossAlphabetDetail, nameResigns, resignCollisionFindings and RebindResigns, retaining the guard's reviewed-consumer and no-stale-entry rules. A concrete target-list amendment is withheld because this review's read-only analysis constraint prohibits the required overlay validation; validate that amendment before prescribing its target bullets.
  - id: F3
    severity: MEDIUM
    class: design
    origin: lens:affected-set
    location: _lyx/plan/10-csharp-rename-derivation.md:23
    summary: The Rename derivation requires the actual plan but its function receives no plan
    fix: Specify that card 10 threads the actual plan from CanonicalizeHandles into renameDeclSource for ParseRef, with a corresponding re-sign contract and caller update in the same commit. The concrete arrow and target-list amendment is withheld because this review's read-only analysis constraint prohibits the required overlay validation; validate that amendment before prescribing its target bullets.
---

### [MEDIUM] Explicit-interface draft binding contradicts its algorithm

**Location:** `_lyx/plan/09-csharp-create-handles.md:24`

**Issue:** For the card's own input `int IProvider<int>.Count { get; }`, the draft name `draft` is absent, so the fallback searches for the first identifier followed by a listed stop token.
`IProvider` is followed by `<` and therefore wins before `Count`.
No interface qualifier immediately precedes that selected token, so `csharpOwner` retains `Renderer.IProvider<int>` instead of returning `Renderer`.
Quarry v0.4.0's `internal/engine/name.go:333` rejects owner components carrying type arguments, and the existing `internal/planglyph/handle.go:288` turns that naming error into `handle-name-failed`.
The clean binding prescribed at card 9 line 35 cannot follow the specified algorithm, contrary to D7's owner rule.
The handler independently found the same defect; it is recorded once.

### [MEDIUM] New reader enumeration misses the enforcement registry

**Location:** `_lyx/plan/06-plan-language-mismatch.md:30`; cards 8, 11 and 12

**Issue:** The new functions inspect resolve status, while `nameResigns` also reads the parsed glyph's `Unit`.
`internal/planglyph/status_enforcement_test.go:64` scans selectors named `Status`, `Known`, `Rejected` and `Unit`, including `Unit` on non-status types.
Its fixed `allowedStatusConsumers` list at line 88 contains none of the new functions, and `TestStatusEnforcement_NoOutOfAllowlistConsumer` at line 134 rejects unregistered consumers.
No card carries this registry update, so the corresponding implementation commits cannot pass their existing package guard as planned.
This is one enumeration defect across the affected cards, separate from the Name-call chokepoint that card 11 does cover.

### [MEDIUM] Plan-aware Rename parsing lacks its input and caller change

**Location:** `_lyx/plan/10-csharp-rename-derivation.md:23`

**Issue:** Card 2 defines `ParseRef(plan *Plan, ref string)`, and card 10 requires `renameDeclSource` to call it on the new handle body.
The actual function at `internal/planglyph/handle.go:98` receives four strings and a result map, with no plan.
Its sole caller at line 220 likewise supplies no plan.
Card 10 lists the function without a re-sign arrow and does not cover its caller; no earlier card supplies the missing argument.
The implementation therefore requires an unplanned signature/caller change to honor D3's actual-plan alphabet selection.

## Review limits

The instruction to perform read-only analysis and run nothing conflicts with the requirement to execute `validate-plan --overlay` before proposing target amendments.
The read-only constraint takes precedence, so F2 and F3 identify the required repairs but withhold concrete target amendments rather than claim they were validated.
No tests, board command, or overlay validator were run.
The verdict follows the specified format: these findings are MEDIUM, with no BLOCKING-severity finding.

## Focus departures

- The directive says prior review material must not supply findings.
  An initial wildcard read inadvertently included the current plan directory's `lens-review.md`; no finding is grounded in that file, and every retained issue was checked against the numbered cards and actual source independently.
  No archived plan or prior-round review was opened.
