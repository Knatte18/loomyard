# Batch: reddit-oauth-client

```yaml
task: 'Fix prowler: Reddit adapter blocked'
batch: 'reddit-oauth-client'
number: 2
cards: 3
verify: go -C plugins/prowler test -run 'TestRedditCredentials|TestRedditAPIUserAgent|TestRedditToken|TestFormatRedditThread|TestRedditOAuthURL|TestFetchRedditOAuthThread' .
depends-on: [1]
```

## Batch Scope

This batch delivers the whole Reddit OAuth client in one new file, `plugins/prowler/redditoauth.go`: credential resolution, app-only token acquisition with a concurrency-safe in-process cache, thread retrieval from `oauth.reddit.com`, and JSON-to-markdown formatting.
It is one batch because the three cards build one cohesive object with one external entry point and share a single test file;
splitting them across batches would force the middle one to ship a function no caller reaches.
It depends on batch 1 for `looksLikeBlockPage`, which card 6 uses to turn an HTML block page served from a JSON endpoint into a legible tier failure instead of a JSON decode error.

The external interface batch 3 consumes is exactly two identifiers from `plugins/prowler/redditoauth.go`: `redditCredentials()` (so the adapter can decide whether tier 1 is available and name the missing variables when it is not) and `fetchRedditOAuthThread(ctx, f, rawURL)` (tier 1 itself).
Nothing else in this file is referenced from outside it.

Batch-local decisions beyond `## Shared Decisions`:

- The token cache holds its mutex across the token HTTP request rather than releasing it around the call.
  Under `runAll`'s concurrent per-URL goroutines this makes "exactly one token request per process" true by construction, which is what the discussion's `token-caching` decision asks for;
  the cost is that concurrent Reddit fetches serialise on the first token acquisition only.
- Time is read through a package-level `var timeNow = time.Now` so expiry behaviour is testable without sleeping.
  This is the only clock read in the file.

## Cards

### Card 4: Credentials, API User-Agent, and cached token acquisition

- **Context:**
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/headers.go`
  - `plugins/prowler/chrome.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/redditoauth_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the tests in `plugins/prowler/redditoauth_test.go` first, then the implementation in `plugins/prowler/redditoauth.go`, and commit both together.
  Open each of the two new files with a descriptive header comment above the `package main` line, in the style every existing file in this module uses (see `plugins/prowler/headers.go`): what the file is for and why it exists, not a restatement of its identifiers.
  In `plugins/prowler/redditoauth.go`, package `main`, declare the constants `redditClientIDEnv = "PROWLER_REDDIT_CLIENT_ID"`, `redditClientSecretEnv = "PROWLER_REDDIT_CLIENT_SECRET"`, `redditUserAgentEnv = "PROWLER_REDDIT_USER_AGENT"`, `defaultRedditAPIUserAgent = "prowler/1.0"`, `redditTokenURL = "https://www.reddit.com/api/v1/access_token"`, and `redditTokenSafetyMargin = 60 * time.Second`;
  plus `var timeNow = time.Now`.
  This file is the only place in the module that calls `os.Getenv` for the three `PROWLER_REDDIT_*` variables.
  Declare `func redditCredentials() (clientID, clientSecret string, missing []string)`, returning the two values and a sorted slice naming every one of the two credential variables whose value is empty after `strings.TrimSpace`;
  callers treat `len(missing) == 0` as "tier 1 is available".
  Declare `func redditAPIUserAgent() string`, returning `redditUserAgentEnv`'s value when non-empty after trimming and `defaultRedditAPIUserAgent` otherwise;
  it never returns `browserUA`.
  Declare `type redditTokenCache struct` with an unexported `sync.Mutex`, a `token string`, and an `expiresAt time.Time`;
  a package-level `var redditTokens = &redditTokenCache{}`;
  a method `func (c *redditTokenCache) reset()` that clears both fields under the lock (tests only);
  and a method `func (c *redditTokenCache) get(ctx context.Context, f fetcher, clientID, clientSecret string) (string, error)` that takes the lock for its whole body, returns the cached token when it is non-empty and `timeNow()` is before `expiresAt`, and otherwise calls `requestRedditToken`, storing the new token with `expiresAt` set to `timeNow()` plus the returned lifetime minus `redditTokenSafetyMargin`.
  Declare `func requestRedditToken(ctx context.Context, f fetcher, clientID, clientSecret string) (token string, lifetime time.Duration, err error)`, which builds a `POST` to `redditTokenURL` with body `grant_type=client_credentials`, header `Content-Type: application/x-www-form-urlencoded`, header `User-Agent` set from `redditAPIUserAgent()`, and an `Authorization: Basic <base64(clientID + ":" + clientSecret)>` header built with `encoding/base64`'s `StdEncoding`;
  sends it through `f.do`;
  and decodes a JSON object with `access_token` (string) and `expires_in` (number of seconds) on a 2xx.
  A non-2xx status, a transport error, a JSON decode failure, or a 2xx whose `access_token` is empty each yields a non-nil error;
  no error string built anywhere in this file ever contains `clientID`, `clientSecret`, or the token — an authentication failure reports the status code and instructs the reader to check `redditClientIDEnv` and `redditClientSecretEnv` by name.
  Do not use `httpClient` directly and do not construct an `http.Client` here.
  In `plugins/prowler/redditoauth_test.go`, package `main`, write `TestRedditCredentials` (neither variable set, each set alone, both set — asserting the `missing` slice's exact contents in each case), `TestRedditAPIUserAgent` (default, and env override, asserting the result is never equal to `browserUA`), `TestRedditTokenRequestShape` (asserting the stubbed request's method, URL, `Content-Type`, decoded `Authorization` Basic payload, `User-Agent`, and body), `TestRedditTokenErrors` (401 response, malformed JSON body, and a 2xx with an empty `access_token`, each asserting a non-nil error whose message contains neither the fake client id nor the fake secret used by the test), and `TestRedditTokenCaching` (two sequential `get` calls issue exactly one stubbed token request;
  advancing the injected `timeNow` past `expiresAt` triggers a second;
  and a concurrent case launching several goroutines against one cache asserts the stubbed transport was invoked exactly once, with the counter guarded by its own mutex).
  Every test sets the credential variables with `t.Setenv` and calls `redditTokens.reset()` and restores `timeNow` via `t.Cleanup`, so no test leaks process state into another.
  The tests make no network call.
- **Commit:** `feat(prowler): add Reddit OAuth credential resolution and cached token acquisition`

### Card 5: Reddit thread JSON model and markdown formatting

- **Context:**
  - `plugins/prowler/hackernews.go`
  - `plugins/prowler/htmltext.go`
  - `plugins/prowler/reddit.go`
- **Edits:**
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/redditoauth_test.go`
- **Creates:**
  - `plugins/prowler/testdata/reddit-thread.json`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add to `plugins/prowler/redditoauth.go` the types `redditListing` (a struct whose `data` object holds a `children` slice), `redditChild` (`kind string` plus `data redditThing`), and `redditThing` with the fields `Title`, `Selftext`, `Author`, `Subreddit`, `Body`, and `URL` (all `string`), `Score int`, and `Replies json.RawMessage` — Reddit sends `replies` as either the empty string or a nested listing object, so it must be captured raw and decoded conditionally.
  Add `func redditReplies(raw json.RawMessage) []redditChild`, which returns nil when `raw` is empty, is the JSON literal `""`, or fails to decode as a listing, and otherwise returns that listing's children.
  Add `func formatRedditThread(listings []redditListing, sourceURL string) (string, error)`, which returns a non-nil error when `listings` is empty or its first listing has no child whose `kind` is `"t3"` — a response that carries no post is a tier failure, not an empty document.
  On success it renders markdown in this order: an H1 line of the post title;
  a blank line;
  a metadata line of the form `Reddit | r/<subreddit> | <score> points | by u/<author>`;
  a `Source: <sourceURL>` line;
  the post's `Selftext` when non-empty, or a `Link: <url>` line when `Selftext` is empty and `URL` is non-empty;
  then, only when at least one comment is present, a `## Top Comments` heading followed by the comments.
  Take comments from the second listing when `listings` has one, skipping every child whose `kind` is not `"t1"` (Reddit's pagination placeholders use `kind: "more"`), capping top-level comments at the existing `maxTopComments` constant, and rendering exactly one level of replies per top-level comment — a reply's own replies are not rendered — with that reply level also filtered to `kind: "t1"` and also capped at `maxTopComments` so a single heavily-replied comment cannot dominate the output.
  Render each top-level comment as `**u/<author>** (<score> points):` on its own line followed by its body, and each reply as a single-level markdown blockquote line beginning `> **u/<author>**: ` followed by its body.
  Comment and selftext bodies come from Reddit's `body`/`selftext` fields, which are markdown rather than HTML, so pass them through unchanged — do not run them through `htmlToText`, unlike the Hacker News adapter, whose Algolia source really is HTML.
  Apply no minimum-length check anywhere in this function: a thread with a short title and zero comments is a legitimate, usable result, and judging it too short is the exact defect this task exists to remove.
  Create `plugins/prowler/testdata/reddit-thread.json` as a hand-authored but structurally faithful two-element JSON array matching Reddit's documented comments-endpoint Listing shape, containing one `t3` post with a non-empty `selftext`, at least 22 `t1` top-level comments so the `maxTopComments` cap is exercised, one `"kind": "more"` child that must be skipped, one comment carrying a nested `replies` listing of more than `maxTopComments` `t1` replies so the reply-level cap is exercised rather than merely specified, one further comment carrying a nested reply that itself has a non-empty `replies` listing so the "one level only" rule is exercised, and at least one comment whose `replies` field is the empty string.
  Note in a comment at the top of `plugins/prowler/redditoauth_test.go`'s formatting test that this fixture is hand-authored rather than captured, because `oauth.reddit.com` cannot be read without the credentials that do not exist yet;
  batch 4's live integration test is what validates the real response shape.
  Add `TestFormatRedditThread` to `plugins/prowler/redditoauth_test.go`, reading that fixture and asserting: the title and selftext are present;
  exactly `maxTopComments` top-level comment author lines are rendered;
  the `more` child contributes nothing;
  the nested replies are rendered as blockquotes and exactly `maxTopComments` of them appear for the over-capped comment;
  a reply's own nested reply is absent;
  and a separately constructed zero-comment thread still returns a non-empty result with no `## Top Comments` heading and no error.
- **Commit:** `feat(prowler): format Reddit OAuth thread JSON into markdown`

### Card 6: Authenticated thread retrieval from oauth.reddit.com

- **Context:**
  - `plugins/prowler/blockdetect.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/headers.go`
  - `plugins/prowler/testdata/reddit-thread.json`
  - `plugins/prowler/testdata/reddit-block-page.html`
- **Edits:**
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/redditoauth_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func redditOAuthURL(rawURL string) (string, error)` to `plugins/prowler/redditoauth.go`, which parses `rawURL` with `net/url`, replaces its host with the constant `redditOAuthHost = "oauth.reddit.com"` (declare that constant here), keeps the path unchanged, discards any incoming query and fragment, and sets the query to `raw_json=1&limit=100&depth=2`.
  It returns an error for a URL that does not parse or has an empty path.
  Add `func fetchRedditOAuthThread(ctx context.Context, f fetcher, rawURL string) (string, error)`, which: resolves credentials via `redditCredentials()` and returns an error naming every entry of `missing` when any is absent;
  obtains a token via `redditTokens.get`;
  builds a `GET` for `redditOAuthURL(rawURL)` carrying `Authorization: bearer <token>`, `User-Agent` from `redditAPIUserAgent()`, and `Accept: application/json`;
  and sends it via `f.do`.
  Do not call `defaultHeaders()` here and do not set an `Accept-Encoding` header at all — leaving it unset lets Go's transport handle compression transparently, which is why this path needs no `decodeContentEncoding` call.
  On a non-2xx status, read the body and run `looksLikeBlockPage` over it: when it reports blocked, return an error naming the status code and that reason, and otherwise return an error naming the status code alone.
  On a 2xx, decode the body into `[]redditListing` and pass it to `formatRedditThread` with `rawURL` as the source;
  a decode failure is an error, and before returning it run `looksLikeBlockPage` over the body too, so an HTML wall served with a 200 from a JSON endpoint reports as a wall rather than as malformed JSON.
  Add `TestRedditOAuthURL` (a bare, `www`, and `old` Reddit thread URL each mapping to the `oauth.reddit.com` host with the path preserved and the fixed query set, an incoming query being discarded, and an unparseable input yielding an error) and `TestFetchRedditOAuthThread` to `plugins/prowler/redditoauth_test.go`.
  `TestFetchRedditOAuthThread` covers: the happy path, stubbing the token endpoint and then the thread endpoint with `plugins/prowler/testdata/reddit-thread.json`, asserting the thread request's `Authorization` header is `bearer` plus the stubbed token, that its `User-Agent` is not `browserUA`, and that the output contains the post title;
  missing credentials, asserting the error names both variables and that the stubbed transport was never invoked;
  a 403 whose body is `plugins/prowler/testdata/reddit-block-page.html`, asserting the error mentions the detector's reason;
  and a 200 whose body is that same HTML, asserting the error is the wall reason rather than a raw JSON syntax error.
  Every sub-test uses `t.Setenv` and `redditTokens.reset()` as card 4's tests do.
- **Commit:** `feat(prowler): fetch Reddit threads from the authenticated OAuth API`

## Batch Tests

`verify:` runs `go -C plugins/prowler test -run 'TestRedditCredentials|TestRedditAPIUserAgent|TestRedditToken|TestFormatRedditThread|TestRedditOAuthURL|TestFetchRedditOAuthThread' .` from the worktree root.
The filter names every test function this batch adds and nothing else: `TestRedditToken` is a prefix covering `TestRedditTokenRequestShape`, `TestRedditTokenErrors`, and `TestRedditTokenCaching`.
The batch adds no test that needs the network, Chrome, or real credentials — the token endpoint and the thread endpoint are both stubbed through `fetcher.do`, and both fixtures are read from disk.
The concurrency assertion in `TestRedditTokenCaching` is the one that matters most here, because `runAll` in `plugins/prowler/main.go` fetches every URL in its own goroutine against one shared process-wide cache;
run it under `-race` at least once by hand during implementation.
Nothing in this batch changes existing behaviour, so no existing test is edited and the pre-existing suite is unaffected.
