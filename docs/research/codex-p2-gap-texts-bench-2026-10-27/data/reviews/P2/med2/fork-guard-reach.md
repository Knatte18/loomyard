Guard reachability list:

- Cards 2–4: `ValidateFormat`, `Validate`, `ValidateDispatch`, kick-start, and post-batch scope checks reach the plan-aware parsing paths.
- Card 5: plan-gate caller/import checks, dispatch’s `LaterDeleteReferences`, and pure verify/handle checks receive the Go-only guards.
- Card 6: pure validation reaches the path-extension check; `resolvePass` reaches the symbol-extension check, with completed-card suppression.
- Card 7: shared pure validation reaches the namespace-pair check; dispatch and after-completion validation suppress completed-card findings.
- Card 8: status, Create, and done checks reach cross-alphabet ambiguity rendering.
- Cards 9–10: handle canonicalization reaches owner derivation and Rename head derivation; the draft-name fallback fails its prescribed explicit-interface test.
- Card 11: resolve-pass re-sign naming precedes batched resolution and collision checks; the named collision and mismatch inputs reach those branches.
- Card 12: both record-batch and recover-batch call `postBatchChecks`; ordinary Edit arrows pass existing done checks and reach the new rebind check.
- Card 13: cost batching reaches the shared boundary computation.

Finding:

**MEDIUM — consistency — `_lyx/plan/09-csharp-create-handles.md:24` — The declared-name fallback selects the interface type instead of `Count`.**

The prescribed fallback selects the first identifier followed by `<`, among other tokens.
For the card’s own test input `int IProvider<int>.Count { get; }`, with draft name `draft`, that identifier is `IProvider`, not `Count`.
Consequently `csharpOwner` finds no qualifier immediately before the selected identifier and retains `Renderer.IProvider<int>`.
Quarry rejects that owner for carrying type arguments, so the row prescribed as clean on line 35 instead produces `handle-name-failed`.
This also affects Rename derivation when the renamed head no longer contains the old member name.

Evidence:
- `_lyx/plan/09-csharp-create-handles.md:23–31`: fallback selection, qualifier stripping, and stated quarry restriction.
- `_lyx/plan/09-csharp-create-handles.md:35`: concrete draft-name input and expected clean binding.
- `/home/knatte/go/pkg/mod/github.com/!knatte18/quarry@v0.4.0/internal/engine/name.go:333–348`: `nameCSharpContainer` rejects owner components carrying type arguments.
- `internal/planglyph/handle.go:288–299`: naming errors become `handle-name-failed`.

Proposed correction: specify how the fallback skips generic interface qualifiers before choosing the declared member token, preserving the existing explicit-interface draft-name row as the reaching test.
This requires an Intent correction, not a target change.

Tool-call manifest:
1. Read validation dispatch, malformed-handle guards, and test/caller locations missing from the manifest.
2. Read glyph-chain helpers, re-sign scenarios, dispatch-after-completion behavior, and batching boundary implementation to trace test inputs.
3. Read delete-order and Create inversion paths, plus numbered card 9 text, to check upstream handling and the fallback algorithm.
4. Located cached quarry v0.4.0 sources and read ambiguity/rewrite behavior to check external naming restrictions and post-batch substitutions.
5. Located numbered card 11–12 rules and the rewrite arrow implementation; the guessed quarry filename was absent.
6. Located quarry’s actual owner validation and post-batch test/caller definitions.
7. Read actual quarry owner validation and record/recover test fixtures to confirm rejection and recovery-path reachability.
