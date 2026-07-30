# Batch: site-adapters

```yaml
task: 'prowler: site-adapter mechanism + github-repo-explorer skill (Claude reading the web)'
batch: site-adapters
number: 1
cards: 8
verify: go -C plugins/prowler test ./...
depends-on: []
```

## Batch Scope

This batch generalizes prowler's single hardcoded Reddit special-case into a pluggable site-adapter mechanism (interface + registry on the `fetcher` seam), migrates Reddit to be the first adapter using only its working `old.reddit.com` HTML strategy (deleting the dead JSON path), adds a Hacker News adapter (Algolia JSON API) as the second, and updates/adds the Go tests plus the now-stale code comments. It is one batch because every card shares the same small Go package (`plugins/prowler`, `package main`) and the same `fetchPage`/`fetcher` seam — splitting would force the same context to be reloaded. External interface consumed by nothing downstream (the skills batch is codeless and independent). All Go; no `PYTHONPATH=` verify prefix.

## Cards

### Card 1: Define the siteAdapter interface and registry

- **Context:**
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/fetch.go`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/adapter.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `plugins/prowler/adapter.go` in `package main` defining the interface `siteAdapter` with two methods: `Matches(url string) bool` and `Fetch(ctx context.Context, f fetcher, url string) (out string, handled bool)`. Also define `func defaultAdapters() []siteAdapter { return []siteAdapter{redditAdapter{}, hackerNewsAdapter{}} }`. The `redditAdapter` and `hackerNewsAdapter` concrete types are created in cards 3 and 4 (same package — the package compiles as a whole once they land; a mid-batch non-compiling state is expected and resolved by batch end). Add a file-header comment and a godoc comment on the interface stating the `handled=false` fall-through contract: a matched adapter that cannot produce usable content returns `handled=false` so `fetchPage` falls through to the generic HTML cascade rather than trying another adapter. Import `context`.
- **Commit:** `feat(prowler): add siteAdapter interface and registry`

### Card 2: Add adapters to fetcher, drive them from fetchPage, wire newFetcher

- **Context:**
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/adapter.go`
- **Edits:**
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `fetcher.go`, add an ordered `adapters []siteAdapter` field to the `fetcher` struct with a doc comment (a nil/empty slice is valid — it just means no site adapters run, and every URL takes the generic cascade). In `fetch.go`, replace the current Reddit special-case block in `fetchPage` (the `if isRedditUrl(url) { if out, handled := fetchReddit(ctx, f, url); handled { return out } }` block) with a loop over `f.adapters`: for the first adapter whose `Matches(url)` returns true, call `Fetch(ctx, f, url)`; if `handled`, return its `out`; otherwise stop scanning adapters and fall through to the existing static/Readability/body-text/browser cascade below (do not try a second adapter). Also in `fetch.go`, rewrite the now-stale comments: the file-header comment ("Reddit special-case first"), `fetchPage`'s docstring reference to the Reddit JSON special-case (describe it as "a matching site adapter first, then the generic cascade"), `fetchOldRedditHTML`'s doc (it is no longer "fetchReddit's fallback for when the JSON API is unreachable" — it is now the Reddit adapter's sole strategy; remove the "falls back to the original JSON-API error" wording), and `errorResult`'s "shared by both fetchPage and fetchReddit (reddit.go)" comment (now used by `fetchPage` only). Do NOT change `fetchOldRedditHTML`'s behavior or signature. In `main.go`, wire `adapters: defaultAdapters()` into the `fetcher` returned by `newFetcher()`.
- **Commit:** `refactor(prowler): drive fetchPage through the adapter registry`

### Card 3: Migrate Reddit to redditAdapter (old.reddit-only), delete JSON path

- **Context:**
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/htmltext.go`
  - `plugins/prowler/headers.go`
- **Edits:**
  - `plugins/prowler/reddit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `reddit.go`, add a `redditAdapter` struct (empty) with `Matches(url string) bool { return redditHostPattern.MatchString(url) }` and `Fetch(ctx context.Context, f fetcher, url string) (string, bool)` that calls `fetchOldRedditHTML(ctx, f, url)` (defined in `fetch.go`) and returns its `(out, ok)` directly — i.e. `(out, true)` on success, `("", false)` on failure so `fetchPage` falls through to the generic cascade. Delete the now-dead JSON code: `isRedditUrl`, `toRedditJsonUrl`, `fetchReddit`, `formatRedditPost`, `formatRedditSubreddit`, `selftextSnippet`, the `selftextSnippetLen` const, and the `redditThing` and `redditData` structs. Keep `redditHostPattern`, `redditHostReplace`, `toOldRedditURL`, and the `maxTopComments` const (the HN adapter reuses `maxTopComments` in card 4). Rewrite the file-header comment and the `redditHostPattern`/`redditHostReplace` comments so they describe the old.reddit HTML strategy, not the ".json special-case". Remove the now-unused `prowler-reader/1.0` UA and any imports left unused after the deletions (e.g. `encoding/json`, `io`, `net/http`, `strconv`, `fmt` — keep only what `redditAdapter`/`toOldRedditURL`/the retained regexes still need: `context`, `regexp`, `strings`).
- **Commit:** `refactor(prowler): migrate Reddit to old.reddit-only adapter`

### Card 4: Add the Hacker News adapter

- **Context:**
  - `plugins/prowler/adapter.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/fetch.go`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/hackernews.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `plugins/prowler/hackernews.go` in `package main` defining a `hackerNewsAdapter` struct with `Matches(url string) bool` (true only for `news.ycombinator.com/item?id=N` URLs — match host `news.ycombinator.com` with an `item` path and a numeric `id` query param, across http/https and optional `www.`; use a compiled `regexp` or `net/url` parsing) and `Fetch(ctx context.Context, f fetcher, url string) (out string, handled bool)`. `Fetch` extracts the numeric `id`, builds `https://hn.algolia.com/api/v1/items/{id}`, issues the request via `f.do` (construct with `http.NewRequestWithContext`; a malformed request or transport error returns `handled=false`), and on a 2xx JSON response unmarshals the Algolia item shape (fields needed: `title`, `points`, `author`, `url`, `text`, and a recursive `children` array of comment items each with `author`, `text`) into local structs. Format markdown mirroring the Reddit post shape: an `# <title>` header, a `HN | <points> points | by <author>` line, the post `text` (HTML-stripped via `htmlToText`) or a `Link: <url>` line when `text` is empty, and up to `maxTopComments` top-level comments (`## Top Comments`, each `**<author>**:` followed by the `htmlToText`-cleaned comment `text`). A non-2xx status, an unparseable body, or a missing id returns `handled=false` (empty `out`); a parsed-but-empty item (no title and no text) may return `handled=false` to fall through. Reuse `htmlToText` from `htmltext.go` for comment/post HTML (Algolia returns HTML in `text`). Set no special User-Agent.
- **Commit:** `feat(prowler): add Hacker News site adapter`

### Card 5: Update reddit_test.go for the adapter migration

- **Context:**
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/fetch.go`
- **Edits:**
  - `plugins/prowler/reddit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete every test covering now-deleted code: `TestToRedditJsonUrl`, `TestFormatRedditPost`, `TestFormatRedditSubreddit`, `TestFetchReddit` (and the `redditPostFixture`, `redditPostFixtureNoSelftext`, `redditPostFixtureEmptyChildren`, `redditSubredditFixture`, `redditSubredditFixtureEmpty` fixtures and the `newTestResponse` helper if no longer referenced by any remaining test in the package — check `fetch_test.go` card 6 first; `newTestResponse` may still be needed there, in which case keep it). Retarget `TestIsRedditUrl` to call `redditAdapter{}.Matches(...)` instead of `isRedditUrl(...)` (rename it e.g. `TestRedditAdapterMatches`, keep the same URL/expectation table). Keep `TestToOldRedditURL` unchanged. Add a small test that `redditAdapter.Fetch` returns `handled=false` when `fetchOldRedditHTML` fails (e.g. a non-2xx stubbed `fetcher.do`) and `handled=true` with the body when it succeeds (stub `toOldRedditURL(url)` → usable HTML), reusing the `stubResponses`/`htmlResponse` helpers from `fetch_test.go` and the `redditLikeHTMLWithComments`-style fixture pattern. Remove now-unused imports.
- **Commit:** `test(prowler): update Reddit tests for old.reddit-only adapter`

### Card 6: Update fetch_test.go's Reddit routing test

- **Context:**
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/adapter.go`
  - `plugins/prowler/fetch.go`
- **Edits:**
  - `plugins/prowler/fetch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `TestFetchPage_RedditUrlRoutesThroughRedditPath` so it no longer references the deleted JSON path (`redditPostFixture`, the `url + ".json"` stub). The rewritten test must construct a `fetcher` that includes `adapters: defaultAdapters()` (or `adapters: []siteAdapter{redditAdapter{}}`) — otherwise no adapter matches and the Reddit URL wrongly takes the generic cascade — stub the `old.reddit.com` URL (`toOldRedditURL(url)`) with usable HTML containing post + comment text, and assert `fetchPage` returns that old.reddit-derived body (rename to e.g. `TestFetchPage_RedditUrlRoutesThroughOldRedditAdapter`). Leave `TestFetchOldRedditHTML` and all non-Reddit `TestFetchPage_*` tests unchanged (they build `fetcher` values with no `adapters` field, which is valid — a nil adapter slice means their `example.com` URLs correctly take the generic cascade). Ensure any helper the deleted assertions used (`newTestResponse`) is either still defined here or in `reddit_test.go`.
- **Commit:** `test(prowler): route Reddit fetch test through the old.reddit adapter`

### Card 7: Add hackernews_test.go

- **Context:**
  - `plugins/prowler/hackernews.go`
  - `plugins/prowler/fetch_test.go`
  - `plugins/prowler/reddit.go`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/hackernews_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `plugins/prowler/hackernews_test.go` (`package main`, no network — stub `fetcher.do`). Cover: `hackerNewsAdapter{}.Matches` accepts `https://news.ycombinator.com/item?id=1`, `http://news.ycombinator.com/item?id=42`, and `https://www.news.ycombinator.com/item?id=7` and rejects non-item HN URLs (`https://news.ycombinator.com/newest`, front page `https://news.ycombinator.com/`), item URLs with a missing/non-numeric id, and non-HN URLs (`https://example.com/item?id=1`); `Fetch` on a stubbed 2xx Algolia JSON fixture (an in-file const modeling `{title, points, author, url, text, children:[{author,text},...]}`) returns `handled=true` with markdown containing the title, the points/author line, the post text, and the comment authors/bodies, bounded to `maxTopComments` (include a fixture with more than `maxTopComments` children to assert the bound); `Fetch` returns `handled=false` on a non-2xx response, on an unparseable body, and on a `Matches`-passing URL whose id extraction yields nothing. Key the stub `fetcher.do` on the `hn.algolia.com/api/v1/items/{id}` URL. Reuse `stubResponses`/`htmlResponse` from `fetch_test.go` where convenient.
- **Commit:** `test(prowler): cover the Hacker News adapter`

### Card 8: Add adapter_test.go for registry routing

- **Context:**
  - `plugins/prowler/adapter.go`
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/fetch_test.go`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/adapter_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `plugins/prowler/adapter_test.go` (`package main`) with a local stub adapter type implementing `siteAdapter` (configurable `matches bool`, a recorded `fetchCalled` flag, and a canned `(out, handled)` return) to test `fetchPage`'s routing loop directly, independent of Reddit/HN: (a) a URL a stub adapter matches with `handled=true` returns the adapter's output and never hits the transport; (b) a matched adapter returning `handled=false` falls through to the generic cascade (assert the generic path ran, e.g. via a stubbed `do`/`browser`); (c) with two matching adapters, only the first is consulted (assert the second's `Fetch` is never called); (d) a non-matching adapter is skipped and the URL takes the generic cascade. Build `fetcher` values with the `adapters` field set to the stub(s) plus stubbed `do`/`browser`. This validates the interface contract in isolation from the concrete adapters.
- **Commit:** `test(prowler): cover site-adapter registry routing`

## Batch Tests

`verify: go -C plugins/prowler test ./...` runs the whole prowler nested module's test suite (`go -C` sets the module directory; the module is small — a handful of files — so the full suite is fast and there is no per-file scoping benefit). It covers the updated `reddit_test.go`, the rewritten Reddit routing test in `fetch_test.go`, the new `hackernews_test.go`, and the new `adapter_test.go`, plus the unchanged cascade/encoding tests that guard against regressions in `fetchPage`. Go project → no `PYTHONPATH=` prefix; the command is git-root-relative (`plugins/prowler` is a path under the worktree root), so the plain-string form (implying `cwd: git_root`) is correct.
