{"in": 1676, "cache_cr": 74856, "cache_rd": 54492, "out": 15750, "turns": 2, "compute": 92282, "tool_calls": 0, "models": ["claude-sonnet-5"], "found_marker": true}
1. **[load.go, LoadRegistry]** `decoder.Decode` reads only the first YAML document in the file; if `models.yaml` contains multiple `---`-separated documents, everything after the first is silently discarded with no error or warning — MED

2. **[load.go, LoadRegistry / validateAlias]** `Entry.Model` and the alias-adjacent free-form strings are never charset/content-validated (only checked non-empty, and `Engine` against `knownEngines`) — a model string containing whitespace, newlines, control characters, or unbounded length sails through and reaches `Resolved.Model` unsanitized, inconsistent with `Parse`'s strict `isModelIDChar` enforcement for the escape form — MED

3. **[load.go, LoadRegistry]** Duplicate alias keys in the YAML mapping are not detected — decoding into `map[string]Entry` silently keeps only the last occurrence of a repeated key via yaml.v3's default behavior, so an operator's typo'd duplicate block fails silently instead of erroring — MED

4. **[load.go, LoadRegistry]** `os.ReadFile` loads the entire `models.yaml` into memory with no size cap, and the yaml.v3 decode has no guard against anchor/alias expansion ("billion laughs")-style bombs — a corrupted or hostile config file could cause unbounded memory use with no defensive check — LOW/MED

5. **[parse.go, parseBracket]** A spec with more than one bracket (e.g. `sonnet[a=1][b=2]`) isn't rejected with a clear "multiple brackets" error; the code finds only the first `[` and the *original* string's `HasSuffix("]")`, so the extra `][b=2` text gets absorbed into the previous param's value and fails (if at all) with a confusing "invalid character in param value" error rather than naming the actual problem — LOW

6. **[registry.go, Resolve]** On the unknown-alias error path, `Resolved{}` is returned with a nil `Params` map, breaking the documented "Params is never nil" invariant for that one failure case — if a caller mistakenly proceeds past an ignored/mis-checked error, downstream code that unconditionally ranges over or reads `Resolved.Params` degrades silently rather than the invariant holding universally — LOW

7. **[leaf_enforcement_test.go, TestLeafInvariant_AllowlistOnly]** A `go/parser.ParseFile` failure is only logged via `t.Logf` and then swallowed (`return nil` from the walk callback) instead of failing the test — a source file that fails to parse is silently excluded from the leaf-import enforcement rather than surfacing as a hard failure, defeating the "catch a stray dependency with no list maintenance" guarantee for that file — LOW
