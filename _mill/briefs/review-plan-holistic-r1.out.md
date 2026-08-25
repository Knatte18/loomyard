MILL_REVIEW_BEGIN
# Review: Fix prowler: Reddit adapter blocked — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (model id claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-25
```

## Findings

### [BLOCKING:design] Card 8 breaks the build; its "no external callers" premise is false
**Location:** Batch 3, Card 8 (interaction with Card 9).
**Issue:** Card 8 changes `fetchOldRedditHTML`'s signature from `(string, bool)` to `(string, error)` but does not add `plugins/prowler/reddit.go` to its `Edits:`. `reddit.go`'s current `redditAdapter.Fetch` is `func (redditAdapter) Fetch(...) (string, bool) { return fetchOldRedditHTML(ctx, f, url) }` — a direct pass-through of the call's result. Once Card 8 lands, this no longer type-checks (`(string, error)` is not assignable to a `(string, bool)` return), so the module fails to compile until Card 9 (the very next card) rewrites `Fetch`. Card 8's own rationale — "This is a package-internal function with no external callers, so the change is contained" — is factually wrong: `reddit.go` calls it directly.
**Fix:** Add `plugins/prowler/reddit.go` to Card 8's `Edits:` with a minimal interim shim (`out, err := fetchOldRedditHTML(...); return out, err == nil`), or state explicitly that Cards 8 and 9 must land as one atomic commit rather than two.

### [BLOCKING:scope] Card 8 leaves three fetch_test.go subtests set up to nil-panic
**Location:** Batch 3, Card 8 — `plugins/prowler/fetch_test.go`, `TestFetchOldRedditHTML` subtests `non_2xx_fails`, `transport_error_fails`, `unsupported_content_encoding_fails`.
**Issue:** These three subtests build raw `fetcher{do: ...}` literals directly, not via the `stubResponses` helper (which Card 7 updates to also set `doNoRedirect`). Card 8 routes `fetchOldRedditHTML` through `f.doNoRedirect`, and Card 7 explicitly forbids a nil-fallback ("an unset field is a wiring bug and should fail loudly"). Calling `f.doNoRedirect(req)` on these literals will nil-pointer-panic rather than exercise the intended non-2xx/transport-error/decode-error branches. Card 8 explicitly calls out fixing `reddit_test.go`'s analogous raw literal ("keeping it compiling and passing") but gives no equivalent instruction for these three.
**Fix:** Name explicitly in Card 8 that these three subtests' `fetcher{}` literals must also set `doNoRedirect` to the same closure as `do`.

### [NIT:scope] Reply-level maxTopComments cap is specified but never exercised by the fixture
**Location:** Batch 2, Card 5 — `plugins/prowler/testdata/reddit-thread.json` / `TestFormatRedditThread`.
**Issue:** `formatRedditThread`'s requirement caps each top-level comment's own reply list at `maxTopComments` too, but the fixture only requires "at least one comment with a nested one-level replies listing" — no comment needs >20 nested replies, so the reply-level cap branch is unasserted.
**Fix:** Require the fixture's nested-reply comment to carry more than `maxTopComments` replies, and add an assertion that only `maxTopComments` are rendered for it.

## Verdict

REQUEST_CHANGES
Card 8 leaves the build (and two subtests) broken until Card 9 lands; fix the sequencing/edits gap.
MILL_REVIEW_END
