MILL_REVIEW_BEGIN
# Review: modelspec: bare-effort shorthand and version-to-v rename

```yaml
duration_s: 100.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Claude-family Opus, per environment self-report)
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [NIT:consistency] Spec-doc line 112 rename contradicts normalization
**Demoted-from:** BLOCKING
**Section:** § Scope ("the three `version=` mentions (lines 64, 66, 112) become `v=`") vs § Decisions ("`v=` normalizes to the canonical param key `version`")
**Issue:** `contracts/specs/llm-model-spec.md:112` sits under "Provider seam" and names "`version=` id translation" — the provider engine reads `resolved.Params["version"]`, never `v`, so blanket-renaming that line to `v=` documents a spelling the provider side never sees; lines 64 and 66 are genuine bracket-grammar mentions and do want `v=`.
**Fix:** State per-line disposition: 64 and 66 → `v=` (bracket spelling), 112 → keep the canonical `version` (drop the `=`), and say the same test for `llm-model-spec.md:65`'s "generic `version` param", which the Scope inventory does not mention at all.

### [NIT:consistency] "`Parse` is untouched" vs parse.go's grammar doc comments
**Section:** § Scope (`internal/modelspec/parse.go` bullet)
**Issue:** `parse.go:1-4`, `:36-38` ("alias, alias[k=v,...]") and `:122-123` ("comma-separated key=value list") describe the bracket grammar as key=value only; the shorthand makes them incomplete, yet Scope names only `modelspec.go`'s package doc as a prose target.
**Fix:** Add parse.go's own file/function doc comments to the edit inventory, or state explicitly that they stay as-is.

### [NIT:design] No stated relationship between `bracketKeys` and `knownParams`
**Section:** § Decisions ("Bracket key vocabulary is separate…")
**Issue:** Two closed vocabularies now exist with no stated sync obligation; a later param added to `knownParams` alone would be settable in `Defaults` but silently unsettable in a bracket, with no test catching it.
**Fix:** State the intended relation (bracketKeys' value set ⊆ knownParams) and whether a sync assertion is in scope or deliberately deferred.

## Verdict

APPROVE
Doc-rename inventory contradicts the normalize-to-canonical decision at spec line 112.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
