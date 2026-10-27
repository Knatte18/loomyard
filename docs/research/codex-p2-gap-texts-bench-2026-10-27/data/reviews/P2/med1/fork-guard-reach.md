Guard inventory:

- Cards 2–3: `ParseRef` and glyph-language guards are reached through parser validation, resolve, drift, and kick-start consumers.
- Card 5: `GoToolchain` covers plan-gate caller/import checks, dispatch’s `LaterDeleteReferences`, verify-package checks, and path mappings.
- Card 6: pure mismatch runs through `Validate`/`ValidateFormat`; resolve mismatch runs through `resolvePass`, with completed-card suppression prescribed.
- Card 7: namespace-pair findings run through shared validation; dispatch drops completed-card findings.
- Card 8: cross-alphabet handling reaches status, Create, and done checks; candidate partitioning remains shared.
- Cards 9–10: C# declaration derivation reaches `CanonicalizeHandles` and quarry `Name`.
- Card 11: re-sign naming and collisions reach plan validation and dispatch through `resolvePass`.
- Card 12: re-sign completion reaches both record and recovery through `postBatchChecks`, after existing done checks.
- Card 13: grouping reaches `renameBoundaries` through `costBatcher.Batch`.

Findings:

1. **MEDIUM — consistency — `_lyx/plan/09-csharp-create-handles.md:24` — The fallback declared-name rule selects the interface qualifier in its own clean test.**  
   For the prescribed draft `Renderer.IProvider<int>.draft` with head `int IProvider<int>.Count { get; }`, `draft` is absent, so the fallback chooses the first identifier followed by an allowed token.
   That identifier is `IProvider`, followed by `<`, rather than `Count`.
   Consequently the qualifier is not immediately before the selected token, `csharpOwner` keeps `Renderer.IProvider<int>`, and quarry rejects that owner.
   The stated clean row at card 9 line 35 cannot succeed under the prescribed algorithm.
   Evidence: card 9 lines 22–30 and 35; quarry v0.4.0 `internal/engine/name.go:330–348` rejects owner components carrying type arguments.
   Correct the fallback selection rule so it skips generic interface qualifiers and reaches the actual declared name.

2. **MEDIUM — decision — `_lyx/plan/06-plan-language-mismatch.md:31` — Resolve mismatch treats a C# Uses reference as a C# target.**  
   The card fires when “some answer” names a `.cs` symbol, but `collectGlyphTargets` includes Uses as well as Targets.
   A `go` plan editing an existing `README.md` and merely listing `Glyphchain.Sharp#Renderer.Title` under Uses reaches the new blocking mismatch despite changing no C# source.
   All upstream parsing and existence checks accept this input; its Uses glyph resolves found.
   D4 requires the positive C# answer to resolve a **plan target**, not any read dependency.
   Evidence: card 6 lines 30–32; `internal/planglyph/planglyph.go:432–452`, especially Uses collection at 447–448; decision record D4.
   Restrict the positive `.cs` trigger to answers corresponding to Targets and add the Uses-only counterexample to the existing planned table.

3. **MEDIUM — design — `_lyx/plan/12-resign-post-batch.md:36` and card 13 — Allowed overload replacement can never reach the new completion pass.**  
   Card 11 explicitly permits deleting an overload on the same or an earlier card before re-signing another member into its ID.
   For an earlier Delete of `Draw(string)` followed by `Draw(int) -> Draw(string)`, the existing boundary rule sees no later cover of the deleted target, and card 13’s new rule sees no earlier re-sign.
   Thus the two cards may share a batch.
   After correct execution, `Draw(string)` exists again, so existing `DoneChecks` returns `delete-not-done`; `postBatchChecks` returns before the new `RebindResigns` call.
   The same-card Delete/re-sign allowance is necessarily affected too.
   Evidence: card 11 lines 39–40 and 59; card 13 lines 12–14; `internal/batcher/boundary.go:12–29`; `internal/batcher/cost.go:71–88`; `internal/planglyph/donecheck.go:96–100` and 201–205; `internal/websterengine/recordbatch.go:829–849`.
   The plan must reconcile its D7 allowance with completion semantics, including a reaching post-batch test; merely testing clean plan-gate findings does not establish executability.

Additional reads, one line per tool call:

1. Located quarry v0.4.0 sources and parser/completion entry points absent from inherited excerpts.
2. Read quarry’s `Name` boundary, parser normalization, Uses ordering, and done-entry construction to trace upstream processing.
3. Read owner validation references, exact card locations, and cost-batcher boundary application.
4. Read C# special-member grammar and owner rejection, plus resolve-target collection line locations.
5. Read recovery/record inputs, rewrite mechanics, and the recovery test’s fixture path to check repeated completion.
6. Read recovery CLI wiring and exact done-check/card locations to establish the completion refusal.
7. Read status handling and post-batch refusal ordering to verify the concrete inputs reach or fail before new checks.
8. Located verify guards and execution-batch loading omitted from inherited context.
9. Read verify guard semantics, batch loading, ambiguity tests, and exact declared-name rule locations.
