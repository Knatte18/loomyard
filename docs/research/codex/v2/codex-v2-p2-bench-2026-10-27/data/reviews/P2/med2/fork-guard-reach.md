Guard reachability:

- Card 2 alphabet rejection and self-glyph classification: reachable through `ParseRef` and the existing validation dispatch.
- Card 3 rejected handle fallback: reachable; handle bodies bypass glyph-malformed and are subsequently named.
- Card 5 Go-only checks: the proposed gates reach their intended checks; ordinary C# Edit rows introduced earlier do not require caller coverage.
- Card 6 path mismatch: reachable with the prescribed path-only plans.
- Card 6 resolve mismatch: reachable, but its specified input set also admits a false positive described below.
- Card 7 namespace-pair refusal: reachable; existing dispatch filtering drops completed-card findings.
- Card 8 cross-alphabet ambiguity: reachable through status, Create, and done-check consumers.
- Card 9 explicit-interface draft binding: the prescribed successful row cannot succeed under the specified fallback algorithm.
- Card 10 rejected new handle: reachable because handle syntax validation does not parse its C# body.
- Card 10 unsupported-member refusal: reachable in principle, but its named operator test stops at the helper and does not reach the prescribed refusal.
- Card 11 head mismatch and existing-overload, Create-handle, Rename-handle, and other-arrow collisions: the prescribed inputs reach the corresponding passes.
- Card 12 `resign-not-done`: reachable after done checks because ordinary Edit re-signs contribute no existing done-check entries.
- Card 13 re-sign boundary: reachable with the prescribed language-specific plan and Edit-group `Resigns`.

Findings:

1. **MEDIUM — consistency — `_lyx/plan/09-csharp-create-handles.md:24`**
   **The fallback declared-token rule selects `IProvider`, making the explicit-interface draft row fail.**
   When `g.Name` is `draft`, the specified fallback chooses the first identifier followed by one of its stop tokens.
   In `int IProvider<int>.Count { get; }`, that is `IProvider`, followed by `<`, rather than `Count`.
   Consequently `csharpOwner` cannot recognize `IProvider<int>.` immediately before the chosen token and retains `Renderer.IProvider<int>`.
   Quarry refuses that owner’s type arguments, so the row at line 35 produces `handle-name-failed` instead of binding.
   Evidence: card 9 lines 23–30 and 35; `/home/knatte/go/pkg/mod/github.com/!knatte18/quarry@v0.4.0/internal/engine/name.go:333–348`, specifically the type-argument rejection at line 344.
   Clarify the fallback so it finds the actual declaration identifier past generic interface qualifiers, while preserving D7’s explicitly bounded named-token behavior.

2. **MEDIUM — decision — `_lyx/plan/06-plan-language-mismatch.md:31`**
   **The resolve-language refusal mistakes a C# Uses dependency for a C# target.**
   The card says any batched answer containing a `.cs` symbol triggers the refusal when no `.go` evidence exists.
   `collectGlyphTargets` includes Uses, not only Targets.
   A `go` plan editing an existing `README.md` and listing `Glyphchain.Sharp#Renderer.Title` under Uses therefore reaches this refusal even though it changes no C# source.
   D4 requires the C# answer to belong to a **plan target**.
   Evidence: `internal/planglyph/planglyph.go:430–455`, especially Uses collection at lines 447–448; `_lyx/discussion/decision-record.md:87`; card 6 lines 30–31.
   Constrain the positive `.cs` evidence to the plan’s Targets and extend the existing prescribed table with a Uses-only case.
   D4 explicitly allows any `.go` resolve answer to suppress the refusal, so that side is not a separate finding.

3. **BLOCKING — consistency — `_lyx/plan/10-csharp-rename-derivation.md:35`**
   **The unsupported-member refusal has no named test reaching its actual refusal branch.**
   Card 10 prescribes `renameDeclSource` returning `rename-old-unresolved` when `renameCSharpDecl` cannot derive a declaration.
   Its only unsupported-member example is the operator row in `TestRenameCSharpDecl`, which calls the helper and asserts false.
   None of the listed glyph-chain rows reaches the caller’s false-result branch or asserts its way-forward message.
   This violates the rubric’s refusal reachability requirement and `PATTERN-refusal-way-forward`’s reaching-test requirement.
   Evidence: card 10 lines 35 and 40–43; `internal/planglyph/handle.go:98–158` shows the declaration derivation boundary and refusal construction; quarry’s operator glyph form is documented in `/home/knatte/go/pkg/mod/github.com/!knatte18/quarry@v0.4.0/docs/glyph.md:175–185`.
   Extend the already planned glyph-chain Rename table with a resolvable operator old side and an otherwise valid new handle, asserting `rename-old-unresolved` and its remedy.

Tool-call manifest:

1. Read validation entry points, pure validation dispatch, and Create inversion to establish whether earlier checks stop execution and how completed-card filtering works; those entry bodies were absent from inherited excerpts.
2. Read caller-coverage subject construction, handle validation, and repository wrappers; searched assertion helper locations and local quarry cache to establish upstream grammar and infrastructure behavior.
3. Located quarry v0.4.0 sources; obtained card line references; read assertion helpers and done-check entry collection to establish exact test behavior and whether Edit re-signs reach post-batch checking.
4. Searched quarry owner validation, glyph documentation, and recovery/rewrite sites to substantiate owner rejection and available operator forms.
5. Read quarry container validation and C# glyph grammar, plus glyph/rename validation branches, to verify the rejected handle and explicit-interface paths.
6. Located exact card, fasit, Uses-collection, and status-reader lines to ground the reported input-set defect and refusal coverage gap.
