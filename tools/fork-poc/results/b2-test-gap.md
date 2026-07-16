{"in": 1676, "cache_cr": 74826, "cache_rd": 54492, "out": 14124, "turns": 2, "compute": 90626, "tool_calls": 0, "models": ["claude-sonnet-5"], "found_marker": true}
**Test-gap findings — internal/modelspec**

1. [registry.go, `Resolve`] No test constructs a `Spec` with an invalid shape (both `Alias` and `Engine` set, or neither) bypassing `Parse` — `Resolve` is exported and `Spec` fields are public, so this boundary condition is completely unverified. — HIGH

2. [load.go, `validateAlias`] No test case in `TestLoadRegistry_RejectsInvalidEntries` exercises an invalid alias charset (e.g. an uppercase or otherwise malformed alias key in models.yaml) — the alias-charset check is the first line of `validateAlias` and has zero coverage. — HIGH

3. [load.go, `LoadRegistry`] The non-`ErrNotExist` read-error branch (`fmt.Errorf("modelspec: read %s: %w", ...)`, e.g. permission-denied) is never triggered by any test — only the absent-file and successful-read paths are covered. — MED

4. [registry.go, `Resolve`/`sortedKeys`] `TestRegistry_Resolve_UnknownAlias` uses a registry with only one alias, so the docstring's "sorted" guarantee for the known-aliases list is never actually exercised; the assertion only checks substring containment, not order. — MED

5. [load.go, `validateAlias`] No test covers an empty-string `Defaults` value (e.g. `defaults: {effort: ""}`), though `validateAlias` has an explicit branch for it. — MED

6. [parse.go, `Parse`/`isIdentChar` vs `isModelIDChar`] No accepted/rejected case distinguishes alias/engine charset (no `.`/`_`) from model-id/value charset (allows `.`/`_`) — e.g. an alias like `"son.net"` is never tested for rejection, so the narrower alias charset is only indirectly covered via the uppercase case. — MED

7. [parse.go, `Parse`] No test places whitespace inside the bracket part (e.g. `"sonnet[effort= high]"`) — all whitespace-rejection cases are outside/around the bracket, leaving the interaction between the top-level whitespace scan and bracket parsing unverified. — MED

8. [load_test.go, `TestLoadRegistry_FileOverridesWholeEntry`] Only asserts the overridden `sonnet` entry's fields; never checks that `opus`/`haiku`/`fable` retain their original built-in defaults untouched after the same load, so "merge onto builtins" for untouched aliases is unverified in this scenario. — MED

9. [leaf_enforcement_test.go, `TestLeafInvariant_AllowlistOnly`] The detection logic itself (the `isStdlib` heuristic and `allowedImports` check) has no fixture-based negative test proving it actually flags a disallowed import — it only runs against current real files, so a latent bug in the heuristic would go undetected. — MED

10. [registry_test.go, `TestRegistry_Resolve`] No case distinguishes a registry `Entry` with `Defaults: nil` from one with `Defaults: map[string]string{}` — `copyParams`'s nil-map handling is exercised only incidentally, never asserted as a distinct case. — LOW

11. [parse_test.go, `TestParse_Accepts`] No accepted case uses a dash in a param *value* (only in aliases/engines) or otherwise isolates the `isModelIDChar` dash-allowance for values — coverage of that branch is incidental, not direct. — LOW

12. [package-wide] No test chains `Parse` → `LoadRegistry` → `Resolve` together as a real consumer would; each layer is unit-tested in isolation, so bracket-over-default precedence combined with a file-loaded registry is never verified end-to-end. — LOW-MED
