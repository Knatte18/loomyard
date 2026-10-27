Guard reachability:

- Cards 2–3, alphabet selection and malformed refs: reachable through parsing and both validation entry points.
- Card 3, malformed C# Create handle: reaches `handle-name-failed`, but subsequently also reaches `glyph-rejected`; the prescribed exact finding set is wrong.
- Card 4, scope guard: the C# refusal is reachable, but the proposed Go comparison also reports a finding.
- Card 5, Go-only checks: the planned language gates can suppress caller, import, verify-package and file-unit checks.
- Card 6, language mismatch and C# spelling hint: the named path and member inputs can reach their respective checks.
- Card 7, namespace Rename refusal: reachable; the disk-backed file and earlier-created destination exemptions can also be reached.
- Card 8, cross-alphabet ambiguity: the namespace plus same-named directory supplies the required ambiguous self-glyph answer.
- Card 9, explicit-interface Create binding: the draft-name example cannot reach the prescribed successful binding under the stated token-selection algorithm.
- Card 10, malformed Rename handle: reaches `rename-old-unresolved`; the separate unsupported-member refusal has no named test reaching it.
- Card 11, re-sign mismatch and collision variants: the named fixture inputs reach the respective checks, including handle collisions and two-arrow collisions.
- Card 12, unfinished re-sign: the unchanged fixture reaches `resign-not-done`; the edited fixture reaches rewrite and restamp.
- Card 13, batch boundary: later Targets and Uses entries reach the new boundary rule.

Findings:

1. **MEDIUM · consistency · `_lyx/plan/09-csharp-create-handles.md:24` — The draft-name fallback selects the interface type instead of its declared member.**  
   Card 9 specifies the first identifier followed by `<`, among other delimiters, when the draft name is absent. For its own `Renderer.IProvider<int>.draft` / `int IProvider<int>.Count { get; }` test, that identifier is `IProvider`, not `Count`. Consequently `csharpOwner` finds no interface qualifier immediately before the selected token and retains `Renderer.IProvider<int>`. Quarry rejects that owner before naming the member.  
   Evidence: card 9 lines 23–30 and 35; quarry v0.4.0 `internal/engine/name.go:330–348`, where `nameCSharpContainer` rejects owner components carrying type arguments.  
   Proposed correction: make the fallback skip generic qualifier components whose closing `>` is followed by `.`, so the existing draft-name test actually reaches successful binding.

2. **MEDIUM · consistency · `_lyx/plan/03-planglyph-ref-alphabet.md:65` — The malformed Create test necessarily produces two findings.**  
   After the fallback declaration fails naming, its draft handle remains in the plan. `resolvePass` still calls `createHandleResults`, which resolves every Create handle’s body, including `Glyphchain.Sharp#Renderer.Bad(Int32)`. Neither alphabet accepts that spelling; quarry returns an empty status with a rejection, and `createFindings` reports `glyph-rejected`. The test therefore cannot assert only one `handle-name-failed` at either gate.  
   Evidence: `internal/planglyph/planglyph.go:396–405`; `internal/planglyph/create.go:61–80,195`; quarry v0.4.0 `internal/engine/resolve.go:253–281`.  
   Proposed correction: include the additional `glyph-rejected` in this row’s exact expected findings, preserving the existing fail-closed Create pass.

3. **MEDIUM · consistency · `_lyx/plan/04-scope-guard-plan.md:26` — The Go comparison row does not exercise directory coverage.**  
   `Glyphchain.Sharp#` maps to `Glyphchain.Sharp`, whose final segment contains a dot. `RefScope` therefore classifies it as a file, not a package directory. It cannot cover `Glyphchain.Sharp/notes.txt`, so the prescribed Go row reports `scope-outside-plan` too. The planned differential test fails without demonstrating the language-sensitive guard.  
   Evidence: `internal/planparser/normalize.go:121–130`; `internal/planparser/targetdirs.go:177–178`; `internal/planglyph/scope.go:188–192`; card 4 lines 26–28.  
   Proposed correction: use a slash-free, extensionless overlap such as `Glyphchain#` with `Glyphchain/notes.txt`.

4. **BLOCKING · consistency · `_lyx/plan/10-csharp-rename-derivation.md:35` — The unsupported-member refusal has no test reaching its refusal branch.**  
   Card 10 adds `rename-old-unresolved` when no declared name can be derived, naming operators, indexers, finalizers and static constructors. Its only negative test for this branch calls `renameCSharpDecl` directly and checks `false`; it never invokes `renameDeclSource`, produces the refusal, or asserts its way-forward detail. The separate `Bad(Int32)` gate row reaches the malformed-handle branch instead. This misses the rubric’s refusal-reachability requirement and `PATTERN-refusal-way-forward`.  
   Evidence: card 10 lines 35, 41 and 43; `pattern/PATTERN-refusal-way-forward.md`, requiring a test that reaches a new refusal and asserts its message.  
   Proposed correction: extend the already-planned glyph-chain table and fixture with an unsupported member Rename that reaches this branch and asserts its remedy.

Additional reads, one line per tool call:

1. Located quarry’s owner validation and card 9’s token-selection lines; the manifest did not include quarry’s implementation of declaration-container validation.
2. Read handle grammar and upstream handle checks, plus quarry’s owner-container body; these established whether the malformed-handle and owner cases reach naming.
3. Located quarry’s resolution rejection path and the downstream Create-pass call sites; the inherited context lacked the exact new-version rejection location.
4. Read quarry’s dual-alphabet resolution rejection implementation and searched existing failure tests; this confirmed the additional `glyph-rejected` result.
5. Located scope classification and card 4’s comparison-row lines; the inherited context lacked the implementation of its extension predicate.
6. Read `hasFileExtension` and located the existing scope table; this confirmed that the dotted Go unit is treated as a file.
