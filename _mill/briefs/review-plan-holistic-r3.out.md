MILL_REVIEW_BEGIN
# Review: prowler: installable Claude Code plugin (Go), hosted in LoomYard — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-07-30
```

## Findings

### [BLOCKING] fetchReddit's own error/fallback branches are never tested
**Location:** Batch 1, Card 5 (`reddit.go`/`reddit_test.go`) and Card 6 (`fetch_test.go`)
**Issue:** Card 5's `fetchReddit` requirement specifies five distinct branches — transport error, non-2xx, non-JSON `Content-Type` fallthrough, JSON-parse failure fallthrough, and "formatter yields empty → `Could not parse Reddit …`" — but `reddit_test.go`'s test list only table-tests `isRedditUrl`/`toRedditJsonUrl`/`formatRedditPost`/`formatRedditSubreddit` fed raw in-memory JSON bytes, bypassing `fetchReddit` entirely. Card 6's `fetch_test.go` adds exactly one Reddit assertion (happy-path routing via a `Content-Type: application/json` stub). None of `fetchReddit`'s own error/fallthrough branches has a test requirement anywhere in the plan.
**Fix:** Add explicit requirements to `reddit_test.go` (or `fetch_test.go`, since it already stubs `f.do`) exercising `fetchReddit` directly for: a transport error from `do`, a non-2xx status, a non-JSON `Content-Type` (asserting `handled=false`), an unparseable JSON body (asserting `handled=false`), and a parseable-but-empty result (asserting the `Could not parse Reddit …`, `handled=true` message).

### [NIT] `http.NewRequestWithContext` construction error left unaddressed
**Location:** Batch 1, Card 5 (`fetchReddit`) and Card 6 (`fetchPage`)
**Issue:** Both cards describe request-building followed by "on transport error return …", covering only `f.do`'s own error return; neither addresses `http.NewRequestWithContext` itself failing (e.g., a malformed URL), which would risk calling `f.do` with an unchecked/nil request if an implementer doesn't defensively handle it.
**Fix:** Add one clause to each card: treat a `NewRequestWithContext` error identically to a transport error (same `# Error fetching …` formatting).

## Verdict

REQUEST_CHANGES
Sound, well-grounded plan (guard-cleanliness and marketplace-path claims verified against real sources); one real Reddit-path test-coverage gap.
MILL_REVIEW_END
