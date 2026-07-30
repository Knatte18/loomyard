{"status":"success","commit_sha":"64a8c4ae52d7888e56c1b68c0bc7e7290dc4127a","session_id":"5955756b-e947-40d1-99e1-89fa96fa934b","cards_done":[1,2,3,4,5,6,7,8]}

Summary: All 8 of 8 cards in batch `01-site-adapters.md` are committed (verified via `git log 242f81d3..HEAD --oneline`, exact match against each card's `Commit:` message, in order). The batch's `verify: go -C plugins/prowler test ./...` passes cleanly, as does `go vet ./...` and `gofmt -l` (no formatting drift).

Files touched (all under `/home/knatte/Code/loomyard/wts/prowler-web-reading/plugins/prowler/`):
- `adapter.go` (new) — `siteAdapter` interface + `defaultAdapters()` registry
- `adapter_test.go` (new) — registry routing tests via a local `stubAdapter`
- `fetcher.go` — added `adapters []siteAdapter` field
- `fetch.go` — `fetchPage` now loops over `f.adapters`; rewrote stale Reddit-JSON comments (file header, `fetchPage`, `fetchOldRedditHTML`, `errorResult`)
- `main.go` — `newFetcher()` wires `adapters: defaultAdapters()`
- `reddit.go` — rewritten as `redditAdapter` delegating to `fetchOldRedditHTML`; deleted all JSON-path code (`isRedditUrl`, `toRedditJsonUrl`, `fetchReddit`, `formatRedditPost`, `formatRedditSubreddit`, `selftextSnippet`, `redditThing`/`redditData`, the `prowler-reader/1.0` UA); now imports only `context`/`regexp`
- `hackernews.go` (new) — `hackerNewsAdapter` using the Algolia HN API
- `hackernews_test.go` (new) — Matches/Fetch coverage including the `maxTopComments` bound
- `reddit_test.go` — retargeted to `redditAdapter{}.Matches`/`.Fetch`, deleted dead JSON tests/fixtures/`newTestResponse`
- `fetch_test.go` — `TestFetchPage_RedditUrlRoutesThroughRedditPath` rewritten as `TestFetchPage_RedditUrlRoutesThroughOldRedditAdapter`

Untracked (pre-existing, out of scope, left untouched): `plugins/prowler/prowler` (a locally-built binary artifact) and `_mill/briefs/implement-site-adapters-r1.md`.
