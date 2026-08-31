MILL_REVIEW_BEGIN
# Review: Add cross-repo code search to prowler

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [NIT:design] Dedup matching is not defined as case-folding
**Section:** Decisions → "Duplicate repo refs are deduped silently" **Issue:** Dedup is specified over `<owner>/<repo>` refs with no statement of whether comparison is exact-string or case-insensitive; GitHub refs are case-insensitive, so `Helix-editor/helix` and `helix-editor/helix` survive as two distinct refs, burn two of the ten `code_search` requests on one repo, and emit that repo's records twice under one canonical `.repository.full_name` — the exact failure the decision exists to prevent. **Fix:** State the comparison key (exact string, or lowercased) in the decision, and say whether the emitted record uses the caller's spelling or the API's `full_name`.

### [NIT:scope] No scenario pins all-preflights-before-any-search
**Section:** Testing → scenario list **Issue:** The preflight decision says "before any search call", and the stub-key decision deliberately decouples fixtures from call ordering, but no listed scenario asserts the global ordering: the preflight-failure scenarios assert "no search call was made", which does not distinguish all-preflights-first from per-repo interleaving when the failing repo is not the first. **Fix:** Add a scenario where preflight of repo 2 of 3 fails and the call log is asserted to contain zero search calls.

## Verdict

APPROVE
Decisions are complete and source-verified; two non-blocking gaps in dedup keying and ordering coverage.
MILL_REVIEW_END
