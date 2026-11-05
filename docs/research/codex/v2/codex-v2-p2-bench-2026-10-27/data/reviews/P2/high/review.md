---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/10-csharp-rename-derivation.md:35
    summary: The new unsupported-member refusal is tested only below the refusal boundary
    fix: >-
      Revise card 10's already-planned TestRenameCSharpDecl to exercise the public CanonicalizeHandles entry point with synthetic resolved symbols and a valid C# new-side handle.
      Include the operator case and assert rename-old-unresolved together with the prescribed way forward.
      Test distinct tuple and generic-return cases through their observable canonicalization outcomes; leave the const and map success cases to the already-planned TestGlyphChain_CSharp rows.
      Remove the instruction to test renameCSharpDecl directly and pin its private return strings.
      This changes test intent within the existing targets.
  - id: F2
    severity: MEDIUM
    class: design
    origin: lens:affected-set
    location: _lyx/plan/08-multi-alphabet-answers.md:22
    summary: New status and Unit readers omit the existing status-consumer enforcement registry
    fix: >-
      Rework the affected-set accounting of cards 6, 8, 11 and 12 against TestStatusEnforcement_NoOutOfAllowlistConsumer before execution, including allowedStatusConsumers and each new enclosing function that reads Status or Unit.
      Preserve the scan, its fail-closed review requirement and its stale-entry check.
      A concrete target-list patch is withheld: the required baseline and combined validate-plan --overlay checks cannot be run under this round's read-files/run-nothing constraint.
      Complete that validation before finalizing a target-list correction.
  - id: F3
    severity: MEDIUM
    class: consistency
    origin: handler
    location: _lyx/plan/09-csharp-create-handles.md:24
    summary: The draft-name fallback selects IProvider instead of Count in its own acceptance row
    fix: >-
      Correct card 9's fallback token-selection rule so an identifier whose generic argument list is followed by a dot is treated as part of the qualifier and skipped when locating the declared member.
      Continue to Count in int IProvider<int>.Count { get; } even when the draft name is absent.
      Preserve the existing draft-handle row, its expected Owner of Renderer, and the decision record's separately declared generic-return-type bound.
  - id: F4
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/03-planglyph-ref-alphabet.md:65
    summary: The malformed Create row also produces glyph-rejected
    fix: >-
      Change card 3's malformed Create row to expect both blocking handle-name-failed and blocking glyph-rejected for its card at both gates.
      Keep the existing fail-closed Create inversion; do not suppress its rejection to make the single-finding expectation pass.
  - id: F5
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/04-scope-guard-plan.md:26
    summary: The dotted Go self glyph is a file scope and cannot provide the proposed passing comparison
    fix: >-
      Change the two planned ScopeGuard test inputs in card 4 from Glyphchain.Sharp# and Glyphchain.Sharp/notes.txt to Glyphchain# and Glyphchain/notes.txt.
      Keep the expected Go pass and C# scope-outside-plan result.
      These are test inputs in the existing TestScopeGuard_FileLevelCoverage target, not changes to the card's target list.
---

### [BLOCKING] Reach the Rename refusal through the public surface

**Location:** `_lyx/plan/10-csharp-rename-derivation.md:35`

**Issue:** Card 10 prescribes a new `rename-old-unresolved` branch when declaration derivation fails, but its operator test calls only `renameCSharpDecl` and asserts `false`.
That never reaches `renameDeclSource`'s refusal or verifies its recovery message.
The separate `Bad(Int32)` row reaches the malformed-handle branch, not this branch.
This violates the rubric's refusal-reachability rule and `PATTERN-refusal-way-forward`.
The direct private-helper test also conflicts with `PATTERN-test-economy`, which requires unexported helpers to be tested through their public surface.
Card 10 already gives the const and map Rename successes public coverage in `TestGlyphChain_CSharp`; the new test need not repeat them below that surface.
Evidence is card 10 lines 35 and 41–43, `internal/planglyph/handle.go:98`, and the two named PATTERN background files.

### [MEDIUM] Account for the status-consumer scan as each card lands

**Location:** `_lyx/plan/08-multi-alphabet-answers.md:22`, also cards 6, 11 and 12

**Issue:** The new `languageMismatchFindings`, `crossAlphabetDetail`, `nameResigns`, `resignCollisionFindings` and `RebindResigns` introduce status or glyph-unit reads, but no card covers `allowedStatusConsumers`.
`internal/planglyph/status_enforcement_test.go:64` scans selectors named `Status`, `Known`, `Rejected` and `Unit`; its comment at line 57 explicitly includes `glyph.Glyph.Unit` despite that field not being a status.
The registry at line 87 contains only the existing consumers, and `TestStatusEnforcement_NoOutOfAllowlistConsumer` at line 135 rejects new readers outside it.
Consequently the corresponding card gates fail as the new readers land.
Card 11 updates the separate `quarry.Name` call-site registry, which does not satisfy this scan.
This is one affected-set enumeration defect across the cards, rather than one finding per omitted function.

### [MEDIUM] Locate the declared member after a generic interface qualifier

**Location:** `_lyx/plan/09-csharp-create-handles.md:24`

**Issue:** The fallback is specified as the first identifier followed by any listed delimiter, including `<`.
For the card's own draft handle `Renderer.IProvider<int>.draft` and head `int IProvider<int>.Count { get; }`, `draft` is absent and that first identifier is `IProvider`, not `Count`.
There is no interface qualifier immediately before the selected token, so the next rule retains `Renderer.IProvider<int>` as Owner.
Quarry v0.4.0's `internal/engine/name.go:333` rejects an owner component carrying type arguments before extracting the declaration.
The row at card 9 line 35 therefore cannot bind as prescribed, contrary to D7's interface-qualifier rule.
This finding was independently identified by the handler and the guard-reach fork and is recorded once.

### [MEDIUM] Preserve both findings from a malformed Create handle

**Location:** `_lyx/plan/03-planglyph-ref-alphabet.md:65`

**Issue:** Failed naming leaves the draft handle unchanged.
`resolvePass` still invokes `createHandleResults` at `internal/planglyph/planglyph.go:396`, and `internal/planglyph/create.go:61` resolves every Create handle's body.
Neither alphabet accepts `Glyphchain.Sharp#Renderer.Bad(Int32)`; quarry v0.4.0's `internal/engine/resolve.go:253` returns a rejection with no status when neither parse succeeds.
The default arm at `internal/planglyph/create.go:194` consequently adds `glyph-rejected` alongside `handle-name-failed`.
The card requires exact finding sets, so its single-finding expectation fails at both gates.

### [MEDIUM] Use an extensionless unit for the scope comparison

**Location:** `_lyx/plan/04-scope-guard-plan.md:26`

**Issue:** Under Go, `Glyphchain.Sharp#` yields the path `Glyphchain.Sharp`, but `hasFileExtension` at `internal/planparser/normalize.go:124` treats any dot in the final segment as an extension.
`RefScope` at `internal/planparser/targetdirs.go:177` therefore classifies this as a file scope.
It cannot cover `Glyphchain.Sharp/notes.txt`, so the proposed Go row also reports `scope-outside-plan`.
An extensionless overlap exercises the intended distinction between a Go directory and a C# namespace without changing Go's existing scope rules.

## Focus coverage

D7's principal sub-rules are assigned to cards 9–13, including collision variants, parameter-list presence, post-batch rewriting, recovery parity, restamping and the scope union.
F1 and F3 identify problems in the prescribed declaration tests and algorithm.
Cards 6 and 7 carry the language-check halves and the disk-backed namespace check; their named refusal inputs are reachable.
Completed-card filtering in `dispatchFindings` supplies card 7's pending-card behavior.
No forward dependency was found among the named ParseRef, GoToolchain, ScopeGuard and nameDeclarations changes.
The specified Edit targets use member glyphs, and the explicit signature changes in the focused cards carry arrows.
The quarry facts named by the focus are stated in the plan rather than left for implementation research.
Cards 14 and 15 carry the declared alphabet chokepoint and contract changes.
No card supplies a raw test Verify line; Verify is optional under the writer's instructions.
The added tests prescribe no production-duration sleeps, and the recovery test uses its existing injected clock.

## Review constraints

This review used file reads and searches; it ran no tests, board command or validation executable and changed only this review file.
Step 1's instruction to read the board through `lyx board get` conflicts with its explicit read-files/run-nothing rule, so that command was not run and no board content grounds a finding.
The same rule prevents the overlay execution required before proposing a concrete target-list correction for F2; that correction is expressly withheld rather than presented as validated.
The other fixes change Intent or test-case descriptions within existing targets.
The affected-set fork reported that one broad search inadvertently returned two lines from `_lyx/plan/lens-review.md`; those lines were not used as evidence or grounds for any finding.
