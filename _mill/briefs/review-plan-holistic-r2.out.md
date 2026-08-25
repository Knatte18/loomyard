MILL_REVIEW_BEGIN
# Review: Fix prowler: Reddit adapter blocked — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-25
```

## Findings

### [BLOCKING:scope] Card 2's Context omits fetch_test.go
**Location:** Batch 1, Card 2 (`blockdetect.go`/`blockdetect_test.go`).
**Issue:** Requirements instruct asserting `blocked` is `false` for `redditLikeHTMLWithComments` and `readableArticleHTML`, both declared in `plugins/prowler/fetch_test.go` — but Card 2's `Context:`/`Edits:` list omits that file (only `htmltext.go`, `fetch.go`, and four testdata fixtures are listed).
**Fix:** Add `plugins/prowler/fetch_test.go` to Card 2's `Context:` list.

### [NIT:consistency] Fetch's stale doc comment not scheduled for update
**Location:** Batch 3, Card 9 (`reddit.go`).
**Issue:** Card 9 instructs updating the file's leading package comment but not `redditAdapter.Fetch`'s own doc comment ("Reports handled=false when request fails or content is insufficient"), which becomes false once `Fetch` always reports `handled=true`.
**Fix:** Add an instruction to update `Fetch`'s doc comment alongside the package comment.

### [NIT:consistency] Batch 3's regression-coverage claim overstates its own `-run` filter
**Location:** Batch 3, `## Batch Tests`.
**Issue:** States "`TestReddit` is a prefix covering ... every test function batch 2 added whose name begins `TestReddit`, so batch2's suite is re-run here" — but two of batch 2's eight test functions (`TestFormatRedditThread`, `TestFetchRedditOAuthThread`) don't begin with `TestReddit` and aren't matched by the batch's `-run` regex, so it is not "batch2's suite," only a subset. No functional gap results (`redditoauth.go` is untouched by batch 3), but the prose overclaims.
**Fix:** Reword to "the subset of batch 2's tests whose names begin `TestReddit`" or add the two missing names to the `-run` filter.

## Verdict
REQUEST_CHANGES
One BLOCKING context-completeness gap in Card 2; two cosmetic consistency nits.
MILL_REVIEW_END
