# Review: loom: Preflight phase (precondition validation)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] state.ReadJSON does not do strict parsing
**Section:** Decisions → status-json-typed-and-strict
**Issue:** The decision says the seed is read "via `internal/state.ReadJSON[T]` with a strict, fail-loud parse (`KnownFields(true)` discipline)", but `internal/state/state.go:71` uses plain `json.Unmarshal` — unknown fields are silently accepted. Builder's `ParseOutcome` gets its `KnownFields(true)` from a manual `yaml.NewDecoder`, not from `state.ReadJSON`.
**Fix:** Pin the mechanism: either (a) change `state.ReadJSON` to use `json.Decoder.DisallowUnknownFields()` (and accept the blast radius on existing callers), or (b) have `loomengine` bypass `ReadJSON`'s unmarshal step and run its own strict `json.Decoder` on the bytes/file, or (c) add a strict-variant to `state`. The plan cannot pick without this being decided in scope.

### [NOTE] JSON strictness uses yaml API name
**Section:** Decisions → status-json-typed-and-strict
**Issue:** `KnownFields(true)` is a `yaml.Decoder` method; the JSON equivalent is `json.Decoder.DisallowUnknownFields()`. The naming risks the plan copying builder's YAML idiom into a JSON path.
**Fix:** Rename the discipline in the decision to "DisallowUnknownFields discipline" (or "strict-unknown-field discipline") to keep it API-accurate for JSON.

### [NOTE] PairInSync reason → CheckID mapping is implicit
**Section:** Decisions → weft-pairing-composition; the-five-checks (check 3)
**Issue:** `PairInSync` returns a single opaque `reason` string ("host on X, weft on Y" / "junction missing" / "junction points elsewhere"). The discussion enumerates the strings but does not state how loomengine classifies them into `weft-sync` vs `junction` (prefix match? exact set? a returned enum?).
**Fix:** State the classification mechanism (e.g. "match by known reason-string prefix, unknown reason → treat as `weft-sync`"), or promote the distinction to `PairInSync`'s return shape in a preceding warp change.

## Verdict

GAPS_FOUND
Strict-parse mechanism vs actual `state.ReadJSON` behaviour must be resolved before planning.