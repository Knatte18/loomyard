MILL_REVIEW_BEGIN
# Review: modelspec: bare-effort shorthand and version-to-v rename

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [NIT:scope] Grammar block disposition less precise than :64/:66/:112
**Section:** § Scope, `contracts/specs/llm-model-spec.md` bullet
**Issue:** The four `version` mentions get per-line dispositions, but the Grammar section itself gets only "documents the shorthand" — `:8` (`<alias>[key=value,key=value,...]`), `:13` ("each `key=value` overrides that parameter"), and `:24` (escape-form production) all state a key=value-only bracket and are the lines the shorthand actually contradicts.
**Fix:** Name those three lines with the same per-line treatment used for `:64`/`:65`/`:66`/`:112`.

### [NIT:consistency] Inventory heading mislabels registry_test.go lines
**Section:** § Technical context, "`version=` occurrences repo-wide"
**Issue:** `registry_test.go:66,69` (verified) hold `"version"` map keys in `Spec`/`wantParams` literals, not the string `version=`; a repo grep for `version=` returns no `registry_test.go` hit, so the heading overstates the inventory it introduces.
**Fix:** Retitle the list to "`version` bracket-and-key occurrences" or split the two groups.

## Verdict

APPROVE
Decisions complete and source-verified; only two cosmetic precision gaps remain.
MILL_REVIEW_END
