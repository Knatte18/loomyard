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

### [NIT:scope] RSS non-2xx + block-page path has no test case
**Location:** batch 3 (rss-limiter-and-fetch), card 9 **Issue:** Requirements explicitly mandates "Apply the same block-page check to a non-2xx body before reporting the bare status" (mirroring `fetchRedditOAuthThread`'s `403_block_page` behavior), but card 9's enumerated test list only covers the 200-with-wall-body case (`reddit-block-page.html`) and a separate plain-status case ("a 500, and a 404 each return an error naming the cause") with no overlap between the two — the non-2xx+wall-body combination is never asserted. **Fix:** Add a subtest stubbing a non-2xx status (e.g. 403) with `testdata/reddit-block-page.html` as the body and assert the error names the wall reason rather than a bare status code, matching `redditoauth_test.go`'s `403_block_page` precedent.

## Verdict

REQUEST_CHANGES
One NIT: card 9's failure-detection tests omit the non-2xx+block-page-body case its own Requirements mandate.
MILL_REVIEW_END
