MILL_REVIEW_BEGIN
# Review: prowler: installable Claude Code plugin (Go), hosted in LoomYard — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-07-30
```

## Findings

### [BLOCKING] Reddit JSON structs omit the nested listing/children shape
**Location:** Batch 1, Card 5 (`reddit.go`)
**Issue:** Card 5 specifies only a flat `{Kind string; Data ...}` thing-wrapper plus one flat data struct (`Title, Subreddit string; Score, NumComments int; Selftext, URL, Author, Body string`) — no `Children`/listing field anywhere. Reddit's `.json` response for a post is `[Listing, Listing]`; each Listing's `data.children[]` holds the nested post thing (kind `t3`) and comment things (kind `t1`). Without a `Children` field on the data struct, `formatRedditPost` cannot reach the post's own data (`raw[0].Data.Children[0].Data`) or iterate the comments listing's children for "up to 20 t1-kind comments," and `formatRedditSubreddit` cannot iterate a listing's children for its bullet list either. Tellingly, the card's own test bullets ("empty-children→empty", "no post/no children") presuppose a field the struct as written never names — the Requirements are internally inconsistent, not merely terse. This also isn't restated inline anywhere else in the batch or in `discussion.md` (which only names the functions, not the JSON shape), so a cold implementer has no source for the missing shape and must invent it, contrary to the "authoritative spec is discussion.md" decision's "every behavioral detail needed... is restated inline" guarantee.
**Fix:** Add an explicit `Children []redditThing` field to the data struct (or a distinct Listing type wrapping it) and state which JSON path each formatter reads: `raw[0].Data.Children[0].Data` for the post fields, `raw[1].Data.Children` filtered to `Kind=="t1"` (cap 20) for comments in `formatRedditPost`; the single listing's `Data.Children` (kind `t3`) for `formatRedditSubreddit`.

## Verdict

REQUEST_CHANGES
Card 5's Reddit JSON schema omits the children/listing field the described parsing logic and its own tests require.
MILL_REVIEW_END
