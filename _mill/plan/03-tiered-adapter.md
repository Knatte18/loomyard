# Batch: tiered-adapter

```yaml
task: 'Fix prowler: Reddit adapter blocked'
batch: 'tiered-adapter'
number: 3
cards: 3
verify: go -C plugins/prowler test -run 'TestReddit|TestFetchOldRedditHTML|TestFetchPage|TestRunAll' .
depends-on: [1, 2]
```

## Batch Scope

This batch is where the user-visible behaviour changes: it adds a redirect-suppressing transport seam, hardens the `old.reddit.com` HTML tier so a login redirect is reported instead of silently followed, rewires `redditAdapter.Fetch` into the three-tier strategy, and documents the new credentials prerequisite.
It is one batch because the three cards are a single behaviour change viewed at three depths — the seam exists only to serve the hardened tier, and the hardened tier exists only to be tier 2 of the adapter — and because they share the same two test files.
It depends on batch 1 for `looksLikeBlockPage` and on batch 2 for `redditCredentials` and `fetchRedditOAuthThread`.

After this batch the adapter is feature-complete and every offline test passes;
batch 4 adds only the live credentialed proof.

Batch-local decisions beyond `## Shared Decisions`:

- `fetchOldRedditHTML` changes its signature from `(string, bool)` to `(string, error)`.
  The discussion records that its bare `return "", false` gives the caller no reason for the failure, and tier 3's error message is required to name why each tier failed, so an `error` is the minimum useful return.
  It has exactly one caller outside `plugins/prowler/fetch.go` — `redditAdapter.Fetch` in `plugins/prowler/reddit.go`, which today is a direct pass-through of its result — so card 8 carries a one-line interim adaptation of that call site to keep every commit compiling, and card 9 then replaces it with the real tier logic.
- Redirect suppression is a *new transport field* on `fetcher` rather than a change to the shared `httpClient`'s `CheckRedirect`.
  `httpClient` also backs the generic cascade, where following redirects is correct behaviour for ordinary sites;
  changing it globally would silently alter every non-Reddit fetch.
- The README update lands in the same commit as the adapter change that makes it true, per this repo's `CLAUDE.md` rule that a task changing observable CLI behaviour updates its docs in the same commit.

## Cards

### Card 7: Redirect-suppressing transport seam

- **Context:**
  - `plugins/prowler/browser.go`
- **Edits:**
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/headers.go`
  - `plugins/prowler/main.go`
  - `plugins/prowler/fetch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/fetcher.go`, add a third function field to the `fetcher` struct, `doNoRedirect func(*http.Request) (*http.Response, error)`, documented as the transport that returns a 3xx response to the caller instead of following it, so a fetch path can observe that it was redirected.
  In `plugins/prowler/headers.go`, add `var noRedirectHTTPClient = &http.Client{...}` with the same 60-second timeout as the existing `httpClient` and a `CheckRedirect` function returning `http.ErrUseLastResponse`;
  leave `httpClient` itself untouched, because it also backs the generic cascade where following redirects is correct.
  In `plugins/prowler/main.go`, set `doNoRedirect: noRedirectHTTPClient.Do` in the `fetcher` literal returned by `newFetcher`.
  In `plugins/prowler/fetch_test.go`, set `doNoRedirect` on the `fetcher` returned by the `stubResponses` helper to the same canned-response closure already used for `do`, so every test built on that helper exercises both transports identically.
  Do not add a nil-fallback in production code that silently substitutes `do` for a missing `doNoRedirect` — an unset field is a wiring bug and should fail loudly in tests rather than quietly change transport semantics.
  Make no behavioural change in this card: after it, `doNoRedirect` is wired but not yet read.
- **Commit:** `refactor(prowler): add a redirect-suppressing transport to the fetcher seam`

### Card 8: Harden the old.reddit.com tier against the login redirect

- **Context:**
  - `plugins/prowler/blockdetect.go`
  - `plugins/prowler/htmltext.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/testdata/reddit-login-page.html`
- **Edits:**
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/fetch_test.go`
  - `plugins/prowler/reddit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/fetch.go`, change `fetchOldRedditHTML`'s signature to `func fetchOldRedditHTML(ctx context.Context, f fetcher, url string) (string, error)` and replace every bare `return "", false` with a `return "", <error>` naming the specific failure.
  Send the request through `f.doNoRedirect` instead of `f.do`.
  When the response status is in the 3xx range, return an error naming the status code and the response's `Location` header value, phrased so a reader sees that anonymous access is login-gated rather than that the request merely failed;
  this is the branch that fixes the reported defect, because the shared client previously followed that redirect and reported the resulting login page as content.
  Keep the existing non-2xx branch for the remaining status codes, the read-error branch, and the `decodeContentEncoding` branch, each returning a distinct error.
  After decoding, run `looksLikeBlockPage` over the decoded HTML and return an error naming the reported reason when it is flagged, before the `stripToBodyText` length check;
  the existing "too little content" case becomes its own error too.
  The success return is unchanged in shape: `"# " + url + "\n\n" + bodyText, nil`.
  Update `TestFetchOldRedditHTML` in `plugins/prowler/fetch_test.go` to the new two-value form throughout, and add three sub-tests: a `302` whose `Location` is `https://old.reddit.com/login/?reason=lor2&dest=%2Fr%2Fgolang%2F`, asserting a non-nil error whose message contains `login` and that no second request was issued to follow it;
  a `200` whose body is the contents of `plugins/prowler/testdata/reddit-login-page.html`, asserting a non-nil error carrying the detector's login-wall reason rather than a successful result;
  and a `200` whose body is the existing `redditLikeHTMLWithComments` constant, asserting success and a nil error, which is the guard that the new checks do not reject genuine Reddit content.
  `redditAdapter.Fetch` in `plugins/prowler/reddit.go` is this function's only caller outside `plugins/prowler/fetch.go` and today returns its result directly, so the signature change stops that file compiling unless it is adapted in this same card.
  Adapt it minimally — assign both results and return `out, err == nil` — and change nothing else in `plugins/prowler/reddit.go`;
  card 9 replaces this one line with the real tier logic.
  Three sub-tests of `TestFetchOldRedditHTML` in `plugins/prowler/fetch_test.go` — `non_2xx_fails`, `transport_error_fails`, and `unsupported_content_encoding_fails` — build raw `fetcher{do: ...}` literals rather than going through the `stubResponses` helper card 7 updated, so they would nil-panic the moment this function starts calling `f.doNoRedirect`.
  Set `doNoRedirect` to the same closure as `do` in each of those three literals.
  Update the `handled_false_when_fetchOldRedditHTML_fails` sub-test in `plugins/prowler/reddit_test.go` only as far as keeping it compiling and passing against the interim adapter behaviour, setting `doNoRedirect` on its raw `fetcher` literal for the same reason;
  card 9 rewrites that test's expectations when the adapter itself changes.
  Change no other existing assertion in either test file.
- **Commit:** `fix(prowler): detect old.reddit.com's login redirect instead of following it`

### Card 9: Three-tier Reddit adapter that never reaches the browser

- **Context:**
  - `plugins/prowler/adapter.go`
  - `plugins/prowler/blockdetect.go`
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/redditoauth.go`
- **Edits:**
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/reddit_test.go`
  - `plugins/prowler/fetch_test.go`
  - `plugins/prowler/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `redditAdapter.Fetch` in `plugins/prowler/reddit.go` to run three tiers in order and to always report `handled=true`.
  Tier 1: call `redditCredentials()`;
  when `missing` is empty, call `fetchRedditOAuthThread` and return its output with `handled=true` on a nil error, otherwise record the tier's failure reason;
  when `missing` is non-empty, record a skip reason naming every missing variable and issue no request at all.
  Tier 2: call `fetchOldRedditHTML` and return its output with `handled=true` on a nil error, otherwise record its error.
  Tier 3: return `errorResult(url, ...)` with `handled=true`, where the detail is a markdown bullet list with one bullet per attempted tier, each naming the tier and its recorded reason.
  Never return `handled=false` from this method and never call `f.browser` from it, directly or indirectly — the discussion measured that a second headless request escalates a solvable-looking challenge into a hard IP-level block.
  Update the file's leading package comment, which currently asserts that `old.reddit.com` is not gated;
  that premise has expired and the comment must describe the tier order instead.
  Leave `redditHostPattern`, `redditHostReplace`, `toOldRedditURL`, and `maxTopComments` unchanged.
  In `plugins/prowler/reddit_test.go`, replace `TestRedditAdapterFetch`'s failure sub-test and add sub-tests covering: both tiers stubbed to fail with a `fetcher` whose `browser` field calls `t.Fatal` if invoked, asserting `handled` is `true`, that the output starts with `"# Error fetching "`, and that it names both tiers — this is the single most important behavioural assertion in the task and must be written so it fails if anyone reintroduces a browser fall-through;
  credentials absent, asserting the output names `PROWLER_REDDIT_CLIENT_ID` and `PROWLER_REDDIT_CLIENT_SECRET` and that no request went to the token endpoint;
  credentials present with the OAuth tier stubbed to succeed, asserting tier 2 was never requested;
  and credentials present with the OAuth tier stubbed to fail and tier 2 stubbed to succeed, asserting the tier-2 output is returned.
  Every sub-test in `plugins/prowler/reddit_test.go` and every Reddit-touching sub-test in `plugins/prowler/fetch_test.go` must call `t.Setenv` to clear both credential variables unless it is deliberately setting them, and must call `redditTokens.reset()` with `t.Cleanup`, so the suite's result does not depend on whether the developer running it happens to have real credentials exported.
  In `plugins/prowler/fetch_test.go`, extend `TestFetchPage_RedditUrlRoutesThroughOldRedditAdapter` so its stubbed browser calls `t.Fatal` when invoked even on the all-tiers-fail path, proving the adapter terminates the cascade.
  In `plugins/prowler/README.md`, add a `## Runtime prerequisite: Reddit API credentials` section after the existing Chrome prerequisite section, stating that Reddit content now comes from the official OAuth API;
  that the operator must register a "script"-type app at `https://www.reddit.com/prefs/apps` and export `PROWLER_REDDIT_CLIENT_ID` and `PROWLER_REDDIT_CLIENT_SECRET`;
  that the credentials are read from the environment only and are never written to a config file;
  that `PROWLER_REDDIT_USER_AGENT` optionally overrides the descriptive `prowler/1.0` API User-Agent;
  and that without credentials prowler falls back to an `old.reddit.com` HTML fetch that Reddit currently login-gates for anonymous readers, so Reddit fetches will report a definitive error rather than returning content.
  Update the `## Site adapters` section's Reddit sentence to describe the three tiers and to state that Reddit URLs never reach the headless-browser fallback.
  Write both README sections with one sentence per line, per this repo's semantic-line-break rule.
- **Commit:** `fix(prowler): tier Reddit fetches through the OAuth API and stop returning walls`

## Batch Tests

`verify:` runs `go -C plugins/prowler test -run 'TestReddit|TestFetchOldRedditHTML|TestFetchPage|TestRunAll' .` from the worktree root.
`TestReddit` is a prefix covering `TestRedditAdapterMatches`, `TestRedditAdapterFetch`, and every test function batch 2 added whose name begins `TestReddit`, so batch 2's suite is re-run here as a regression guard against the signature and behaviour changes this batch makes.
`TestFetchOldRedditHTML` and `TestFetchPage*` cover the two files this batch edits most heavily, and `TestRunAll` covers `runAll`, which this batch touches indirectly through `newFetcher`'s new field.
The run is offline: no test in it makes a network call, spawns Chrome, or requires real credentials, and card 9 explicitly requires every Reddit-touching test to neutralise the credential environment with `t.Setenv` so a developer with real credentials exported gets the same result as one without.
Run the batch once by hand with `-race` as well, because card 9's tier-1 path reaches the shared token cache introduced in batch 2.
