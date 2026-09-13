MILL_REVIEW_BEGIN
# Review: modelspec: bare-effort shorthand and version-to-v rename

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [BLOCKING:scope] Two accept-table cases break, discussion says none do
**Section:** § Testing ("Unchanged and expected to stay green") + § Scope (`parse_test.go` bullet)
**Issue:** `parse_test.go:29-33` (`"alias multiple params"`, `sonnet[effort=high,version=4.5]`) and `:44-48` (`"dotted version value"`, `sonnet[version=4.5]`) are **accept** cases that will now error under the `version=` removal, yet Scope says the only test edit besides new cases is "one existing reject case is deleted" and Testing asserts every other case in both tables stays green.
**Fix:** State the disposition of these two accept cases (rewrite to `v=` vs delete) in Scope and drop the blanket stay-green claim.

### [NIT:consistency] Conflicting line cite for the deleted reject case
**Section:** § Decisions (`opus[effort]` becomes valid) vs § Testing (deleted case)
**Issue:** Decisions cites `parse_test.go:186`; the `"param with no equals"` case is actually at `175-179` (186 is a `t.Fatalf` in the loop body), as Testing correctly says.
**Fix:** Use `175-179` in both places.

### [NIT:design] Migration-hint scope unstated for other unknown keys
**Section:** § Decisions (`version=` is removed…)
**Issue:** The example error appends the `v` hint, but the discussion never says whether the hint is emitted only for key `version` or unconditionally for every unknown key (`sonnet[speed=fast]` at `parse_test.go:165-169` must keep passing on substring `unknown param key` either way).
**Fix:** Say explicitly that the hint clause is conditional on the offending key being `version`.

### [NIT:consistency] Duplicate error names a key absent from the input
**Section:** § Decisions (duplicate-key) vs § Decisions (empty bracket segments)
**Issue:** `opus[v=4.8,v=4.5]` will report `duplicate param key "version"`, naming a spelling the operator did not write — the exact objection used to reject folding empty segments into `empty param key`.
**Fix:** Note the accepted asymmetry, or state that the duplicate error echoes the as-written spelling.

### [NIT:scope] `internal/loomengine/template.yaml` omitted from the `effort=` inventory
**Section:** § Problem and § Scope ("Out")
**Issue:** `internal/loomengine/template.yaml` also carries `effort=` specs; Problem names only `websterengine`/`landingshed` templates, and Out names loom's *tests* but not its template.
**Fix:** Add it to the untouched-sites list so the plan writer does not treat it as an unlisted surprise.

## Verdict

REQUEST_CHANGES
Test-impact inventory contradicts itself: two `version=` accept cases will fail.
MILL_REVIEW_END
