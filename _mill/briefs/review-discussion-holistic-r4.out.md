MILL_REVIEW_BEGIN
# Review: scoutengine told-geometry (optional uniformity pass)

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [NIT:decision] seeded servers.yaml must satisfy validateEntry
**Section:** Testing → "The registry assertion must seed an overlay" **Issue:** The seed is specified only as "<yaml with a recognisable entry>", but `LoadRegistry` runs `validateEntry` (`internal/scoutengine/registry.go:110-124`) requiring non-empty `markers`, `match` ∈ {`all`,`any`}, non-empty `command`, non-empty `install_hint`, under `KnownFields(true)` — an under-specified seed makes `lookupContext` return an error and case 2 fail on the wrong axis. **Fix:** State the four required keys (or name an existing valid override sample) so the plan writer does not have to rediscover them.

### [NIT:scope] test-comment enumeration misses refs_integration_test.go:196
**Section:** Decisions → documentation, "Known test-comment sites" **Issue:** `internal/scoutengine/refs_integration_test.go:196` ("layout is an isolated temp dir so the supervised daemon this…") is a surviving prose "layout" mention not on the five-site list; the closed rule plus the pre-commit grep covers it, but the list reads as complete. **Fix:** Add `:196` to the known-sites list, or mark the list explicitly non-exhaustive.

## Verdict

APPROVE
Source-verified: enumeration, line refs, invariant claims, and both r2/r3 fixes hold.
MILL_REVIEW_END
