---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: lens:guard-reach
    location: _lyx/plan/12-resign-post-batch.md:36
    summary: An allowed overload replacement can fail completion before RebindResigns runs
    fix: Revise the completion design in cards 11–13 to account for an overload deliberately removed and replaced by a re-sign in the same batch, including the same-card case D7 permits. Specify how completion distinguishes that replacement from an unperformed deletion, and extend the already-targeted record and recovery tests with this case. Preserve D7's allowance and its rewrite-after-done-check ordering.
  - id: F2
    severity: BLOCKING
    class: design
    origin: handler
    location: _lyx/plan/10-csharp-rename-derivation.md:41
    summary: The new Rename test directly tests a private helper contrary to PATTERN-test-economy
    fix: Replace the private-helper testing prescription with public-surface coverage through CanonicalizeHandles or the existing TestGlyphChain_CSharp table. Retain uncovered tuple, operator and generic-name-bound behavior, and let the existing planned const and dictionary-field rows own their observable Rename behavior. A concrete target-list deletion or reassignment is withheld because this round forbids executing the required overlay validation.
  - id: F3
    severity: BLOCKING
    class: design
    origin: handler
    location: _lyx/plan/14-package-docs-pattern.md:11
    summary: The invariant and package documentation are deferred beyond the implementing commits
    fix: Revise the card boundaries so the alphabet invariant lands with its implementation and each changed module's built behavior is documented in the implementing commit, as the repository instructions require. Keep only independently justified documentation work in card 14. Concrete target moves are withheld because this round forbids executing the required overlay validation.
  - id: F4
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/09-csharp-create-handles.md:24
    summary: The fallback name rule selects IProvider rather than Count in its own clean test
    fix: Correct card 9's fallback algorithm to skip a generic interface qualifier followed by a dot and locate the actual declared member token. Keep the existing draft-name explicit-interface row and require it to bind with Owner Renderer and canonical member IProvider<int>.Count.
  - id: F5
    severity: MEDIUM
    class: scope
    origin: lens:guard-reach
    location: _lyx/plan/06-plan-language-mismatch.md:31
    summary: The resolve mismatch rule mistakes a CSharp Uses dependency for a changed target
    fix: State that the positive CSharp resolve trigger is restricted to answers for the plan's Targets, as D4 requires. Extend the already-planned TestGlyphChain_CSharp table with a go plan editing an existing README.md and listing Glyphchain.Sharp#Renderer.Title only under Uses; it must not report plan-language-mismatch.
  - id: F6
    severity: MEDIUM
    class: design
    origin: lens:affected-set
    location: _lyx/plan/06-plan-language-mismatch.md:30
    summary: New status consumers are omitted from the existing enforcement registry
    fix: Complete the affected-set plan for cards 6, 8, 11 and 12 so each introducing commit registers its new Status or Unit reader in internal/planglyph/status_enforcement_test.go's allowedStatusConsumers with its reviewed reason. Cover languageMismatchFindings, crossAlphabetDetail, nameResigns, resignCollisionFindings and RebindResigns where they read those selectors. A concrete target-list addition is withheld because this round forbids executing the required overlay validation.
  - id: F7
    severity: MEDIUM
    class: design
    origin: handler
    location: _lyx/plan/06-plan-language-mismatch.md:4
    summary: The exported PathEntryPath surface is bundled with its first consumer
    fix: Use insert-card to give PathEntryPath its own independently reviewable card before the language-mismatch card, with its public contract and coverage. Keep the mismatch implementation and its two test tables on the consumer card, and retain the namespace-pair consumer after both.
  - id: F8
    severity: LOW
    class: consistency
    origin: lens:affected-set
    location: _lyx/plan/05-go-only-checks.md:48
    summary: The claim that none already skips every gated check contradicts an existing test
    fix: Correct card 5's ImpactSummary to acknowledge the changed none behavior for verify-nested-module, and explicitly update the already-targeted TestValidate_VerifyNestedModule row named language none still fires to expect no finding under GoToolchain gating.
  - id: F9
    severity: LOW
    class: scope
    origin: lens:affected-set
    location: _lyx/plan/02-planparser-ref-alphabet.md:51
    summary: The public Plan.Language field contract is left describing only Go and none
    fix: Carry the Plan.Language field-comment update with CSharp language acceptance so its legal-value description names go, csharp and none while preserving the absent-value default. A concrete glyph-target addition is withheld because this round forbids executing the required overlay validation.
---

### [BLOCKING] Accepted overload replacement cannot complete

**Location:** `_lyx/plan/12-resign-post-batch.md:36`, with cards 11 and 13.
**Issue:** Card 11 permits removing `Draw(string)` and re-signing `Draw(int)` into that signature on the same or a later card.
The existing boundary algorithm only detects a later cover of an earlier target (`internal/batcher/boundary.go:12`), and card 13 only adds boundaries after an earlier re-sign whose old target a later card names.
An earlier Delete of `Draw(string)` followed by that re-sign can therefore share a batch; the same-card form cannot be separated by batching at all.
After correct execution, `Draw(string)` exists again.
`DoneChecks` constructs a deletion check for that ID (`internal/planglyph/donecheck.go:96`) and treats every status other than `not_found` as still existing.
`postBatchChecks` returns on the resulting `delete-not-done` before the proposed re-sign pass (`internal/websterengine/recordbatch.go:829`).
Thus the plan-gate exception prescribed by D7 is not executable through the completion path the plan prescribes.

### [BLOCKING] Private-helper testing violates the test boundary

**Location:** `_lyx/plan/10-csharp-rename-derivation.md:41`.
**Issue:** `TestRenameCSharpDecl` is explicitly a new test of the unexported `renameCSharpDecl`, using hand-built symbols and inspecting its derived heads.
`pattern/PATTERN-test-economy.md` requires an unexported helper to be tested through the public surface using it.
The distinction between exact-head assertions and minted-ID assertions means these are not simply duplicate assertions, but it does not exempt a direct private-helper test from that rule.
The same card already extends the public plan-gate Rename table, `TestGlyphChain_CSharp`, for const and dictionary-field behavior.
The uncovered cases can be exercised through naming and canonicalization without introducing a private-function testing surface.

### [BLOCKING] Documentation and invariant changes land too late

**Location:** `_lyx/plan/14-package-docs-pattern.md:11`.
**Issue:** Card 14 reserves the changed alphabet invariant and the built package rules for a separate commit after cards 2–13 implement them.
The supplied repository instructions require a new cross-cutting invariant in the same commit as its code, and require module documentation in the same commit as observable behavior changes.
The plan format also defines each card as independently committable and reserves standalone Prosa work for module headers not triggered by a code change (`.bench/specs/loom/loom-plan-spec.md:320`).
Card 14's updates are explicitly triggered by these earlier implementation cards, including the new plan refusals and re-sign completion behavior.
The final documents are in the right locations, but the proposed commit boundaries do not satisfy the repository rule.

### [MEDIUM] Draft-name fallback stops at the qualifier

**Location:** `_lyx/plan/09-csharp-create-handles.md:24`.
**Issue:** Apply the prescribed fallback to its own row, `Renderer.IProvider<int>.draft` with head `int IProvider<int>.Count { get; }`.
The draft name is absent, and the first identifier followed by an admitted token is `IProvider`, followed by `<`.
It is not `Count`, contrary to the next sentence and the clean-row expectation.
No interface qualifier immediately precedes the selected `IProvider` token, so `csharpOwner` retains `Renderer.IProvider<int>`.
Quarry v0.4.0 rejects an owner component carrying type arguments in `internal/engine/name.go:330–348`.
This defeats the explicit-interface binding that D7 and the acceptance criteria require.

### [MEDIUM] Read dependencies trigger the language refusal

**Location:** `_lyx/plan/06-plan-language-mismatch.md:31`.
**Issue:** D4's positive resolve trigger is a plan target declared in a `.cs` file.
Card 6 instead specifies that some batched answer has such a symbol.
`collectGlyphTargets` includes both Targets and Uses (`internal/planglyph/planglyph.go:432–452`).
A Go plan editing an existing `README.md` and merely reading `Glyphchain.Sharp#Renderer.Title` therefore has a `.cs` symbol answer, no `.go` symbol answer, and no `.go` path target.
It meets the card's refusal predicate although it changes no C# source.
The property glyph parses under Go, and the fixture supplies its declaration, so this input reaches the new check.

### [MEDIUM] The affected-set enumeration misses a guard registry

**Location:** `_lyx/plan/06-plan-language-mismatch.md:30`, also cards 8, 11 and 12.
**Issue:** `statusVocabularySelectors` includes `Status`, `Known`, `Rejected` and `Unit`, including `Unit` selectors on non-status types (`internal/planglyph/status_enforcement_test.go:64`).
`TestStatusEnforcement_NoOutOfAllowlistConsumer` scans every production file and fails for a reader absent from `allowedStatusConsumers` (`:86`, `:135`).
The new mismatch, cross-alphabet, collision and completion helpers read status, while re-sign naming reads the parsed glyph's unit.
No card covers the registry update.
The corresponding implementation gates will fail even if the new runtime logic works.
This is one enumeration gap across the new readers, not a separate finding per helper.

### [MEDIUM] A new exported surface needs its own card

**Location:** `_lyx/plan/06-plan-language-mismatch.md:4`.
**Issue:** `PathEntryPath` is an exported planparser surface with two independent consumers: language mismatch and namespace-pair validation.
Card 6 bundles its creation with the mismatch implementation in two packages.
The rubric's granularity rule explicitly gives a public surface its own card even when one card is its first consumer; the private-helper bundling exception does not apply.

### [LOW] Existing none behavior is misdescribed

**Location:** `_lyx/plan/05-go-only-checks.md:48`.
**Issue:** `checkVerifyNestedModule` currently has no language guard (`internal/planparser/verifymodule.go:37`).
Its existing table explicitly expects a finding in the row `language none still fires` (`internal/planparser/validate_test.go:1700`).
Card 5's new gate changes that behavior, but its test instructions only add a C# row and its ImpactSummary says `none` already skipped every check.
The table is already targeted, so correcting its existing expectation needs no additional test or target.

### [LOW] Public language documentation is omitted

**Location:** `_lyx/plan/02-planparser-ref-alphabet.md:51`, with card 14.
**Issue:** `internal/planparser/plan.go:51` describes the implemented alphabets as today Go, plus `none`.
Card 2 changes the accepted alphabet without covering that field contract, and card 14 updates package `doc.go` files but not this public model comment.
It remains inconsistent with the built C# support.

## Rejected

- No way-forward finding was retained: the prescribed rebaseline/recover sequence is supported by the failed-batch and recovery paths examined by that lens.
- Copying fixture glyph spellings after creating the named fixture is not deferred design research; the plan already specifies the fixture and library behavior.
- No missing Verify line is reported: the writer contract makes it optional, and none of the cards supplies a raw-test Verify line.
- The explicit generic-return-type substitution bound is not reported as a defect because D7 deliberately settles that bound.

## Review limits

This review used file reads and source searches only; no tests, board command, quarry command or overlay validator was executed, following the round's instruction to read files and run nothing.
That restriction conflicts with the requested board lookup and with executable validation of target-changing fix proposals.
Accordingly, concrete target-list edits requiring an overlay check are withheld above; their defects and required work remain recorded.
The proposed insert-card split is exempt from overlay validation under the rubric.
An initial broad read of the plan directory also returned `lens-review.md`; it was not used as evidence for the findings above.
All retained findings are grounded in the current cards, decision record, repository rules and source reads described here.
