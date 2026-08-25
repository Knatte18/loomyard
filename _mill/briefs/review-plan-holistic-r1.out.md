MILL_REVIEW_BEGIN
# Review: Add RSS-based Reddit read tier — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-25
```

## Findings

### [BLOCKING:scope] Card 7 Requirements cites redditChild.Kind, file not in Context
**Location:** batch 02-rss-parsing-foundation, card 7 **Issue:** Requirements says the Atom entry's `<id>` prefix is "the tier's only kind discriminator, mirroring `redditChild.Kind` on the OAuth side" — `redditChild` is declared in `redditoauth.go`, which is absent from card 7's Context (`redditformat.go`, `reddit.go`, `blockdetect_test.go`, three testdata files) and Edits (`redditrss.go`, `redditrss_test.go`). **Fix:** Add `redditoauth.go` to card 7's Context.

### [BLOCKING:scope] Card 9 Requirements cites three headers.go/fetch.go symbols, neither file in Context
**Location:** batch 03-rss-limiter-and-fetch, card 9 **Issue:** Requirements says "Do not call `defaultHeaders()`... do not call `decodeContentEncoding`" and discusses `browserUA` — `defaultHeaders`/`browserUA` live in `headers.go`, `decodeContentEncoding` in `fetch.go`; neither file is in card 9's Context (`fetcher.go`, `redditoauth.go`, `blockdetect.go`, `fetch_test.go`, `blockdetect_test.go`, testdata) or Edits. Contrast with card 12, which correctly lists both files. **Fix:** Add `headers.go` and `fetch.go` to card 9's Context.

### [BLOCKING:scope] Cards 11 and 15 call stubRedditRSSLimiter, redditrss_test.go not in Context/Edits
**Location:** batch 04-tier-rewiring-deletion-and-docs, cards 11 and 15 **Issue:** Card 11's Requirements says "Every subtest that reaches the RSS tier calls `stubRedditRSSLimiter(t)`" and card 15 says "Do not call `stubRedditRSSLimiter` in this file" — the helper is declared in `redditrss_test.go` (card 8), which is absent from both cards' Context and Edits (card 11 edits `reddit.go`/`reddit_test.go`/`fetch_test.go`; card 15 edits only `reddit_integration_test.go`). **Fix:** Add `redditrss_test.go` to Context for both cards.

### [NIT:consistency] Card 12's "keep both" instruction contradicts its own deletion
**Location:** batch 04-tier-rewiring-deletion-and-docs, card 12 **Issue:** Card 12 says "Check each of `fetch_test.go`'s two uses of that fixture individually and keep both," but one of those two uses is inside `TestFetchOldRedditHTML`, which the same card deletes wholesale — after the card only one use of `reddit-block-page.html` remains in `fetch_test.go`. **Fix:** Reword to "keep the fixture file" rather than "keep both" uses.

## Verdict

REQUEST_CHANGES
Three cards name production symbols from files absent their Context list, a Context-completeness violation.
MILL_REVIEW_END
