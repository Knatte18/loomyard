Guard inventory:

- Cards 2–3: alphabet rejection and malformed-handle fallback are reachable; quarry v0.4.0 explicitly rejects `Draw(Int32)` as noncanonical.
- Card 5: Go-only check exclusions are reachable with the prescribed C# plans.
- Card 6: both language-mismatch halves and the C# suggestion on `glyph-malformed` are reachable; completed-card filtering supports the stated dispatch exception.
- Card 7: namespace-pair refusal is reachable; root-file, earlier-created-file and directory exceptions are reachable.
- Card 8: cross-alphabet ambiguity and non-Go partition exclusion are reachable.
- Card 9: the draft explicit-interface Create row cannot reach its prescribed successful binding under the stated token-selection algorithm.
- Card 10: invalid-new-handle refusal is reachable; the unsupported-member refusal lacks a test reaching the refusal itself.
- Card 11: naming mismatch, parameter-list mismatch, existing-overload collision, handle collision and distinct-arrow collision have reachable prescribed inputs.
- Card 12: `resign-not-done` is reachable after ordinary done checks because Edit arrows do not contribute Create/Delete/Rename done-check entries.
- Card 13: Uses/Targets boundaries are reachable; the Go exclusion is reachable.

Findings:

1. **MEDIUM — consistency — `_lyx/plan/09-csharp-create-handles.md:24`**
   **The fallback declared-token rule selects `IProvider`, so the draft explicit-interface Create cannot bind successfully.**
   With the prescribed input `plan:Glyphchain.Sharp#Renderer.IProvider<int>.draft` and head `int IProvider<int>.Count { get; }`, `draft` is absent.
   The fallback therefore selects the first identifier followed by an allowed token: `IProvider`, followed by `<`, rather than `Count`.
   Consequently `csharpOwner` cannot remove the qualifier immediately before `Count`; it retains `Renderer.IProvider<int>`.
   Quarry rejects that Owner because it contains type arguments, producing `handle-name-failed` rather than the clean result card 9 promises.
   Evidence: card 9 lines 23–30 and 35; `/home/knatte/go/pkg/mod/github.com/!knatte18/quarry@v0.4.0/internal/engine/name.go:340–345`.
   Correct the fallback selection rule so generic interface qualifiers are skipped before identifying the actual declared member.

2. **BLOCKING — consistency — `_lyx/plan/10-csharp-rename-derivation.md:35`**
   **The new unsupported-member refusal has no named test reaching `rename-old-unresolved`.**
   Card 10 prescribes a refusal for operators/indexers/finalizers/static constructors, but its sole unsupported-member row calls the private `renameCSharpDecl` helper and asserts `false`.
   That never reaches `renameDeclSource`’s new refusal or asserts its way forward.
   The glyph-chain rejection row instead exercises an invalid *new handle*, a separate branch.
   This violates the prescribed-refusal reachability rubric and `PATTERN-refusal-way-forward`.
   Evidence: card 10 lines 35, 41–43; `internal/planglyph/handle.go:98–159` shows the derivation/refusal boundary.
   Exercise an unsupported old member through the existing glyph-chain validation table and assert the refusal and recovery detail.

Tool-call manifest:

1. Read validation entry points and dispatch scoping, handle validation, re-sign test execution, and quarry canonicalization references; inherited excerpts lacked the complete upstream path.
2. Read `isInterfaceMethod`, malformed-handle checks, quarry’s noncanonical-type rejection cases, post-batch done-check sequencing, and rebaseline guards; these settled reachability and upstream rejection.
3. Located quarry’s Owner validation and C# member extraction, and read numbered card 9/10 clauses; inherited context lacked dependency implementation and precise finding locations.
4. Read C# special-member extraction, Owner rejection implementation, test helpers, and numbered collision/language-mismatch clauses; these completed refusal-input and test-path checks.
