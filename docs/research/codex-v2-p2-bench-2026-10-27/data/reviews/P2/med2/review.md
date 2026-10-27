---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    location: _lyx/plan/10-csharp-rename-derivation.md:35
    summary: The unsupported-member refusal has no test reaching the refusal and its remedy.
    fix: Extend card 10's already targeted TestGlyphChain_CSharp table with a Rename of a resolvable operator to an otherwise valid C# handle, using the table's per-row fixture files, and assert rename-old-unresolved and the drop-the-pair remedy at both gates. The helper-only false-result assertion does not substitute for this row.
    origin: lens:guard-reach
  - id: F2
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/09-csharp-create-handles.md:24
    summary: The draft-name fallback selects the interface qualifier instead of the declared member.
    fix: Amend card 9's fallback rule to skip a generic identifier whose balanced type-argument list is followed by a dot, continuing to the declared member after the qualifier. Keep the named-token rule and its D7 bound unchanged, and keep the existing draft-to-Count row as the regression case.
    origin: lens:guard-reach
  - id: F3
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/06-plan-language-mismatch.md:31
    summary: The resolve-language check treats a C# Uses dependency as a changed C# target.
    fix: Require the positive .cs resolve evidence in card 6 to have a result Target present in the plan's Targets. Preserve D4's separate allowance for any .go resolve answer to suppress the finding. Extend the already targeted C# table with a go plan editing an existing README.md and naming Renderer.Title only under Uses, expecting no language mismatch.
    origin: lens:guard-reach
  - id: F4
    severity: MEDIUM
    class: design
    location: _lyx/plan/06-plan-language-mismatch.md:30
    summary: The affected-set enumeration omits the enforcement registry for new status and Unit readers.
    fix: Reconcile the affected sets of cards 6, 8, 11 and 12 with the status-consumer scan before these helpers are implemented, recording each new reader and its fail-closed justification on its introducing card. A concrete target-list patch is withheld because the mandatory validate-plan overlay prerequisite conflicts with this round's read-files-only tool rule; validate such a patch before adding registry targets, subject to redundant-file-target and redundant-package-target.
    origin: lens:affected-set
  - id: F5
    severity: MEDIUM
    class: design
    location: _lyx/plan/10-csharp-rename-derivation.md:23
    summary: The Rename parser migration requires the enclosing plan but carries no parameter or caller migration.
    fix: Specify in card 10 that the enclosing plan must reach renameDeclSource from CanonicalizeHandles for ParseRef, preserving D3's actual-plan alphabet choice. The concrete re-sign and caller target patch is withheld pending the required overlay validation, which this round's read-files-only tool rule prohibits; the repair must satisfy caller-uncovered and the re-sign-arrow rule together.
    origin: lens:affected-set
  - id: F6
    severity: MEDIUM
    class: decision
    location: _lyx/plan/09-csharp-create-handles.md:36
    summary: Quarry v0.4.0 rejects the verbatim spaced interface head that card 9 promises will bind.
    fix: Record the demonstrated quarry limitation in card 9 and obtain a parent-recorded resolution before retaining this clean-row promise. The concrete adaptation needed is canonicalizing the explicit-interface qualifier in the declaration sent to Name, but that changes D7's verbatim-head requirement and must be authorized in the decision record first. Preserve the spaced-qualifier case as coverage of the resolved behavior rather than silently replacing its input with canonical text.
    origin: handler
---

### [BLOCKING] Reach the unsupported-member refusal

**Location:** `_lyx/plan/10-csharp-rename-derivation.md:35`

**Issue:** Card 10 prescribes a `renameDeclSource` refusal when `renameCSharpDecl` cannot derive an identifier, but its only unsupported-member test calls `renameCSharpDecl` directly and asserts false.
The listed glyph-chain rows never exercise this refusal or its way forward.
This violates the rubric's refusal-reachability requirement and `PATTERN-refusal-way-forward`.
An operator is a reachable input: quarry v0.4.0's `docs/glyph.md:176` documents member glyphs such as `Box.operator +(Box,Box)`, and a valid new handle can pass the upstream handle checks.

### [MEDIUM] The fallback stops at the qualifier

**Location:** `_lyx/plan/09-csharp-create-handles.md:24`

**Issue:** For the card's own `Renderer.IProvider<int>.draft` handle and `int IProvider<int>.Count { get; }` head, the requested name is absent.
The prescribed fallback's first identifier followed by a stop token is `IProvider`, because `<` is a stop token.
It therefore does not select `Count`, contrary to lines 25 and 35.
The qualifier is not immediately before the selected token, so `csharpOwner` retains `Renderer.IProvider<int>`.
Quarry's cached v0.4.0 `internal/engine/name.go:333–348` rejects type arguments in an Owner component, yielding `handle-name-failed` instead of the required binding.

### [MEDIUM] Read dependencies can trigger a language refusal

**Location:** `_lyx/plan/06-plan-language-mismatch.md:31`

**Issue:** D4 requires the positive C# evidence to be a resolved plan target.
Card 6 instead says that some batched answer containing a `.cs` symbol is sufficient.
`collectGlyphTargets` in `internal/planglyph/planglyph.go:430–455` collects Uses as well as Targets.
A default-Go plan editing an existing `README.md` and reading `Glyphchain.Sharp#Renderer.Title` through Uses therefore gets a blocking language mismatch although it changes no C# source.
The `.go` suppression side is not a defect: D4 expressly allows a resolve answer to provide that evidence.

### [MEDIUM] New consumers are absent from the enforcement plan

**Location:** `_lyx/plan/06-plan-language-mismatch.md:30`, with the same omission in cards 8, 11 and 12.

**Issue:** `internal/planglyph/status_enforcement_test.go:65–68` scans `Status`, `Known`, `Rejected` and `Unit` selectors, including `Unit` reads on glyphs.
Its `allowedStatusConsumers` registry at line 87 contains only existing consumers, and `TestStatusEnforcement_NoOutOfAllowlistConsumer` at line 135 rejects an unregistered enclosing function.
The prescribed `languageMismatchFindings`, `crossAlphabetDetail`, `nameResigns`, `resignCollisionFindings` and `RebindResigns` introduce such reads, but no card carries the registry changes.
Their implementation gates therefore fail even if their behavior is otherwise correct.
This is one affected-set enumeration defect across the new helpers, rather than a separate finding for each reader.

### [MEDIUM] The Rename derivation has no plan to parse with

**Location:** `_lyx/plan/10-csharp-rename-derivation.md:23`

**Issue:** The prescribed `ParseRef` call needs the plan, but `renameDeclSource` at `internal/planglyph/handle.go:98` receives only strings and a resolve-result map.
Its caller at line 220, inside `CanonicalizeHandles`, has the actual plan.
Card 10 neither specifies the parameter migration nor carries that caller; the earlier cards do not introduce the parameter either.
Inventing a C# plan locally would not implement D3's requirement to choose the alphabet from the enclosing plan, particularly when a Go-parsed spelling resolves to a C# symbol.

### [MEDIUM] Canonical comparison in LoomYard does not cure quarry's raw comparison

**Location:** `_lyx/plan/09-csharp-create-handles.md:36`

**Issue:** This is separate from the fallback defect: the `IPair<string,int>.First` row already names the correct member.
Even with the correct Owner of `Renderer`, the prescribed declaration retains `int IPair<string, int>.First { get; }` verbatim.
Quarry v0.4.0's `internal/engine/csharp.go:364–372` canonicalizes the extracted interface qualifier to `IPair<string,int>`.
Its `internal/engine/name.go:398–413` then requires the raw declaration text to contain that canonical qualifier followed by a dot, using `strings.Contains`.
The spaced head fails that check and returns `invalid_container` through `nameCSharp` at lines 302–303.
Canonicalizing only LoomYard's owner comparison, as card 9 prescribes, cannot make this row pass.
The plan therefore promises an unsupported dependency behavior and leaves no implementation compatible with both its pinned dependency and D7's verbatim-head rule.

## Consolidation notes

All three forks returned before consolidation.
The way-forward lens found no defect in the record/recover/rebaseline route.
The handler independently identified F2, which is recorded once under the guard-reach origin.
The direct helper assertions of exact derived declaration heads were not treated as duplicate glyph-chain assertions: the latter observe minted identifiers, not the exact heads those assertions pin.
Cards 12–13 explicitly carry the re-sign rewrite, restamp, refreshed drift input, scope union and batch boundary; no omission is reported for those requirements.
Cards 14–15 carry the declared PATTERN and contract updates, and no card has a raw-test Verify line.

## Focus departures

- The focus instruction says that it never licenses reading prior review files before the review is saved.
  An initial broad plan-directory glob inadvertently included `lens-review.md`; it was not used as evidence for any finding, and all reported issues were independently grounded in the current cards, decision record and source.
- The exploration instructions request `lyx board get`, while the tool-use rules require reading files and running nothing.
  The board command was not executed, and no finding depends on board content.
- Proposed target-list fixes require `validate-plan --overlay`, while the same tool-use rule prohibits executing it.
  No validator, tests or quarry command was run; the target/signature patches for F4 and F5 are explicitly withheld rather than represented as validated.
