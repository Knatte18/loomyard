# Batch: tier-rewiring-deletion-and-docs

```yaml
task: "Add RSS-based Reddit read tier"
batch: "tier-rewiring-deletion-and-docs"
number: 4
cards: 5
verify: go -C plugins/prowler test . && go -C plugins/prowler test -tags integration -run '^$' .
depends-on: [3]
```

## Batch Scope

This batch makes the RSS tier the live anonymous path and removes what it replaces.
It rewires `redditAdapter.Fetch` from three tiers to two, deletes the measured-dead `old.reddit.com` HTML tier, deletes the no-redirect transport seam that tier was the only production caller of, rewrites the six documentation sites the discussion names, and adds the live two-request integration test.
It is one batch because every card in it is a consequence of the same single behaviour change, and splitting the rewiring from the deletion across a batch boundary would leave a batch boundary at which `fetchOldRedditHTML` is dead but still compiled and still documented as live.

Card order inside the batch is load-bearing: card 11 stops `Fetch` from calling `fetchOldRedditHTML` before card 12 deletes it, and card 12 removes that function before card 13 removes the `doNoRedirect` seam it was the only production caller of.

Batch-local decision: this batch's `verify:` chains a second, compile-only command, `go -C plugins/prowler test -tags integration -run '^$' .`.
`-run '^$'` matches no test name, so the two `//go:build integration` files are type-checked and linked on every implementer and fixer round without a single live Reddit request being issued.
The live suite itself is run by a human, deliberately, with `go -C plugins/prowler test -tags integration .`.

## Cards

### Card 11: Rewire redditAdapter.Fetch onto two tiers

- **Context:**
  - `plugins/prowler/redditrss.go`
  - `plugins/prowler/redditrss_test.go`
  - `plugins/prowler/redditformat.go`
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/adapter.go`
  - `plugins/prowler/blockdetect_test.go`
  - `plugins/prowler/testdata/reddit-thread.rss`
  - `plugins/prowler/testdata/reddit-thread.json`
- **Edits:**
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/reddit_test.go`
  - `plugins/prowler/fetch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `plugins/prowler/reddit.go`, replace `Fetch`'s tier-2 body: the `fetchOldRedditHTML(ctx, f, url)` call and its `- Tier 2 (old.reddit.com HTML): %s` attempt line become `fetchRedditRSS(ctx, f, url)` and `- Tier 2 (Reddit RSS): %s`.
  Everything else about `Fetch` stays: the `redditCredentials()` pre-check that skips tier 1 without issuing any request when credentials are absent, the tier-1 attempt line, the aggregated `errorResult`, and `handled=true` on every return path.
  Tier ordering is unchanged and deliberate — OAuth stays tier 1 when credentials are present, because it is strictly richer there (scores, one level of nested replies, a fuller comment page, and a 100-requests-per-minute budget instead of one per 60 seconds), and it costs nothing when credentials are absent.
  Do not call `f.browser` from any path in this file.

  Rewrite all three stale comment sites in the same commit, each of which currently names three tiers and `old.reddit.com`:
  - The file-level doc comment at the top of `plugins/prowler/reddit.go`.
  - The `redditAdapter` type doc comment.
  - `Fetch`'s own doc comment.
    Keep the never-falls-through-to-the-generic-browser-cascade guarantee all three state, and keep its stated reason: a second headless request against a solvable-looking Reddit challenge has been measured to escalate it into a hard IP-level block rather than recover it.
    Replace the old.reddit description with the RSS tier's: an unauthenticated `.rss` fetch that needs no credentials and no app registration, paced against Reddit's roughly one-request-per-60-seconds per-IP window.

  In `plugins/prowler/reddit_test.go`, update `TestRedditAdapterFetch`'s subtests to the new tier 2, keeping the `fatalBrowser(t)` installation on every one of them:
  - `credentials_absent_tier2_succeeds` — stub the `.rss` URL with `plugins/prowler/testdata/reddit-thread.rss` and assert the rendered thread content, not the old HTML strings.
  - `both_tiers_fail_reports_handled_true_naming_both_tiers` — keep asserting `handled == true`, the `# Error fetching ` prefix, and that the output names both `Tier 1` and `Tier 2`.
  - `credentials_absent_tier2_also_fails` — keep asserting that no token request was issued and that the output names both credential environment variables.
  - `credentials_present_oauth_tier_succeeds_tier2_never_requested` — assert the `.rss` URL is never requested when tier 1 succeeds.
  - `credentials_present_oauth_tier_fails_tier2_succeeds` — stub a failing OAuth response and a successful `.rss` response.
  - Add a subtest asserting no request in this test function ever goes to an `old.reddit.com` host, on any path.
  Every subtest that reaches the RSS tier calls `stubRedditRSSLimiter(t)` as its first statement.

  In `plugins/prowler/fetch_test.go`, retarget `TestFetchPage_RedditUrlRoutesThroughOldRedditAdapter` onto the new tier and rename it `TestFetchPage_RedditUrlRoutesThroughRedditAdapter`.
  Its `success_path` subtest stubs the `.rss` URL rather than the `old.reddit.com` URL and asserts the RSS-rendered output;
  its `all_tiers_fail_still_never_invokes_browser` subtest keeps its existing shape and assertions.
  Both keep `f.adapters = defaultAdapters()` and both call `stubRedditRSSLimiter(t)` first.

  Leave `fetchOldRedditHTML` and `toOldRedditURL` in place in this card;
  they become unused, which Go permits for package-level functions, and card 12 removes them.
- **Commit:** `feat(prowler): make the unauthenticated rss feed reddit's tier 2`

### Card 12: Delete the old.reddit.com HTML tier

- **Context:**
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/headers.go`
  - `plugins/prowler/blockdetect.go`
  - `plugins/prowler/htmltext.go`
  - `plugins/prowler/redditrss_test.go`
  - `plugins/prowler/testdata/reddit-block-page.html`
- **Edits:**
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/reddit_test.go`
  - `plugins/prowler/fetch_test.go`
  - `plugins/prowler/adapter.go`
  - `plugins/prowler/redditrss.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Remove the measured-dead `old.reddit.com` HTML tier.
  Reddit login-gates `old.reddit.com` for anonymous readers, so the tier returns a redirect-to-login on every request;
  keeping it means every uncredentialed Reddit fetch spends one guaranteed-failing request against the per-IP standing the whole RSS tier is built around, and adds a permanently useless line to the error output.

  Delete:
  - `fetchOldRedditHTML` and its doc comment from `plugins/prowler/fetch.go`.
    It is the only user of the `fmt` import in that file, so drop `fmt` from the import block as well.
  - `toOldRedditURL` and `redditHostReplace`, with their doc comments, from `plugins/prowler/reddit.go`.
    `redditHostReplace` exists solely for `toOldRedditURL` and goes with it.
    Keep `redditHostPattern` — `Matches` uses it, and it must keep accepting the `old.` host form so an `old.reddit.com` URL a user pastes still routes to this adapter rather than to the generic cascade.
    Keep `maxTopComments` where it is.
  - `TestToOldRedditURL` from `plugins/prowler/reddit_test.go`, and update that file's own doc comment, which currently advertises "the old.reddit.com host rewrite".
  - `TestFetchOldRedditHTML` and its subtests from `plugins/prowler/fetch_test.go`.

  Keep, explicitly:
  - `decodeContentEncoding`, `stripToBodyText`, `minUsableTextLen`, `errorResult`, and `defaultHeaders` in `plugins/prowler/fetch.go` and `plugins/prowler/headers.go` — the generic cascade uses all of them.
  - `looksLikeBlockPage` in `plugins/prowler/blockdetect.go`, which the RSS tier now calls.
  - `plugins/prowler/testdata/reddit-block-page.html`, which `TestLooksLikeBlockPage`, `TestFetchPage_ChallengePageIsNotReturnedAsContent`, the OAuth wall tests, and the RSS wall test all read.
    Check each of `fetch_test.go`'s two uses of that fixture individually: one sits inside `TestFetchOldRedditHTML`, which this card deletes, and goes with it, while the one in `TestFetchPage_ChallengePageIsNotReturnedAsContent` stays.
    The fixture file itself is kept either way — several surviving tests in other files read it.
  - `redditLikeHTMLWithComments` in `plugins/prowler/fetch_test.go` if any surviving test still reads it — `TestLooksLikeBlockPage` in `blockdetect_test.go` does — and delete it only if nothing does.

  Update `plugins/prowler/adapter.go`'s file-level doc comment, which cites "Reddit's old.reddit.com HTML" as its example adapter strategy.
  Name Reddit's `.rss` feed instead.

  Then confirm the module has no dangling reference: grep the whole of `plugins/prowler` for `fetchOldRedditHTML`, `toOldRedditURL`, and `redditHostReplace` and expect zero hits for each, and confirm `go -C plugins/prowler vet .` and `go -C plugins/prowler build .` are clean.
- **Commit:** `refactor(prowler): delete the login-gated old.reddit.com html tier`

### Card 13: Delete the orphaned no-redirect transport seam

- **Context:**
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/adapter.go`
  - `plugins/prowler/redditrss.go`
- **Edits:**
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/headers.go`
  - `plugins/prowler/main.go`
  - `plugins/prowler/reddit_test.go`
  - `plugins/prowler/fetch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Card 12 removed the seam's only production caller, so remove the seam itself rather than leaving it orphaned.
  `fetcher`'s own doc comment states that none of its fields has a nil fallback, so an unset field is a wiring bug that must fail loudly;
  keeping a field no production path sets or reads silently inverts that contract, and leaves `noRedirectHTTPClient` documented as existing for a tier that no longer exists.
  Re-adding a redirect-observing client is a handful of lines if a future adapter ever needs one.

  Delete:
  - The `doNoRedirect` field and its doc comment from the `fetcher` struct in `plugins/prowler/fetcher.go`, and amend that file's `fetcher` doc comment sentence "do, doNoRedirect, and browser must all be set" to name only `do` and `browser`.
    After this card the seam is `do`, `browser`, `adapters`.
  - The `doNoRedirect: noRedirectHTTPClient.Do` line from `newFetcher` in `plugins/prowler/main.go`, and the mention of `noRedirectHTTPClient.Do` in that function's doc comment.
  - `noRedirectHTTPClient` and its doc comment from `plugins/prowler/headers.go`, and the closing sentence of `httpClient`'s doc comment — "a fetch path that needs to observe a redirect instead of following it uses `noRedirectHTTPClient`".
    Keep the rest of `httpClient`'s doc comment, including its do-not-change-the-redirect-behaviour instruction.
  - The `doNoRedirect:` entry from every `fetcher` literal in `plugins/prowler/reddit_test.go` and `plugins/prowler/fetch_test.go`, including the one inside the `stubResponses` helper.

  Then grep the whole of `plugins/prowler` for `doNoRedirect` and `noRedirectHTTPClient` and expect zero hits for each, and confirm `go -C plugins/prowler vet .` and `go -C plugins/prowler build .` are clean.
- **Commit:** `refactor(prowler): remove the now-orphaned no-redirect transport seam`

### Card 14: Update the user-facing documentation

- **Context:**
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/redditrss.go`
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/adapter.go`
  - `plugins/prowler/hackernews.go`
- **Edits:**
  - `plugins/prowler/README.md`
  - `plugins/prowler/skills/prowler/SKILL.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Both files are verified stale today and describe behaviour this task removes.
  Follow the repository's markdown convention while editing: one sentence per line, no fixed-column hard wrap, and a break at an internal independent-clause boundary inside a long sentence.

  In `plugins/prowler/README.md`, three places:

  1. The lede, which claims prowler reads Reddit posts "by driving a real headless browser plus Mozilla-Readability-style extraction".
     That was never true of the OAuth tier and is further from true now.
     Say that the headless browser plus Readability extraction is the generic cascade, and that Reddit is read from structured sources instead.
  2. The "Runtime prerequisite: Reddit API credentials" section.
     Credentials become optional rather than required.
     State that the unauthenticated `.rss` feed is the zero-setup path that needs no app registration, that credentials only upgrade Reddit reads to the richer OAuth tier when present, that the RSS tier is paced at roughly one request per 60 seconds per IP, and that a burst of several Reddit URLs therefore takes minutes rather than seconds.
     Retitle the section so it no longer reads as a prerequisite.
     Keep the existing facts that are still true: where to register an app, that credentials are read from the environment only and never written to a config file, and that `PROWLER_REDDIT_USER_AGENT` overrides the default `prowler/1.0` API User-Agent.
     Note that Reddit's November 2025 Responsible Builder Policy puts new app registrations behind a manual review that routinely rejects small personal projects, which is why the zero-setup path exists.
  3. The "Site adapters" paragraph, rewritten for the new two-tier order with `old.reddit.com` removed: Reddit tries the authenticated OAuth API when credentials are configured, then the unauthenticated `.rss` feed, then reports a definitive error naming why each attempted tier failed.
     Keep the sentence explaining that a Reddit URL never reaches the generic cascade's headless-browser fallback and why.
     Leave the Hacker News sentence as it is.

  In `plugins/prowler/skills/prowler/SKILL.md`, the sentence reading "a fetched Reddit page especially mixes nav/sidebar chrome with the real content" describes the deleted HTML-scraping tier.
  Reddit output is now formatted markdown rendered from a structured source, so that premise no longer holds for Reddit.
  Rewrite the sentence so the `distill-subagent` rule keeps its justification — RSS and OAuth output is still long, a thread can carry dozens of comments — without claiming Reddit output is full of page chrome.
  Keep the rule itself and the rest of the file unchanged.

  Do not edit `plugins/prowler/fetch.go` in this card: its file doc comment describes the generic static-fetch cascade and enumerates no Reddit tiers, so it does not go stale.
- **Commit:** `docs(prowler): describe the zero-setup rss reddit tier`

### Card 15: The live two-request integration test

- **Context:**
  - `plugins/prowler/redditrss.go`
  - `plugins/prowler/redditrss_test.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/blockdetect.go`
  - `plugins/prowler/main.go`
  - `plugins/prowler/browser_integration_test.go`
- **Edits:**
  - `plugins/prowler/reddit_integration_test.go`
- **Creates:**
  - `plugins/prowler/redditrss_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `plugins/prowler/redditrss_integration_test.go`, guarded by `//go:build integration`, with a file doc comment stating that it drives the real unauthenticated `.rss` endpoint, requires network access but explicitly **no** credentials, and is excluded from the fast unit run.
  It is a new file rather than an addition to `plugins/prowler/reddit_integration_test.go`, whose own file doc comment scopes it to the OAuth API and whose skip-without-credentials contract is the opposite of this test's.

  Add `TestRedditRSSTier_Integration(t *testing.T)`, performing exactly two paced live requests:

  1. Call `fetchRedditRSSFeed(context.Background(), f, "https://www.reddit.com/r/golang/")` and read the first entry's `<link href>` as the discovered thread URL.
     Step 1 cannot go through `Fetch`, whose output is rendered markdown with no machine-readable entry link.
     Fail the test when the feed has no entries.
  2. Call `(redditAdapter{}).Fetch(context.Background(), f, discoveredURL)` on that thread URL.

  Both requests go through the limiter, which is the point of the test: it is the only thing that proves the pacing works across two real calls, which no offline test can do.

  The test must force the RSS tier.
  `Fetch` runs OAuth as tier 1 whenever both credential variables are set, so on a credentialed machine the assertions would otherwise pass without any of this task's code executing.
  Call `t.Setenv(redditClientIDEnv, "")` and `t.Setenv(redditClientSecretEnv, "")` and `redditTokens.reset()` before invoking `Fetch`, which drives tier 1 into its no-request skip branch.
  "Does not require credentials" is not the same as "credentials are absent", and only the second guarantees the RSS tier produced the output.

  Build the fetcher with `newFetcher()` and then replace `f.browser` with a function calling `t.Fatal`, exactly as `TestRedditOAuthThread_Integration` does, so the never-falls-through-to-browser guarantee is enforced here too.
  Do not call `stubRedditRSSLimiter` in this file — the real limiter and the real wait are what this test exists to exercise.

  Assert that `Fetch` reported `handled == true`, that the output does not start with `# Error fetching `, that `looksLikeBlockPage` does not flag it, and that it contains a `Source: ` line carrying the discovered thread URL in its original non-`.rss` form.
  Use no hard-coded thread id anywhere: a hard-coded thread rots, and the one `plugins/prowler/reddit_integration_test.go` uses today already returns a 404 from `.rss`, so a copy of that convention would ship already broken.
  Discovering the thread from the subreddit feed is self-healing.

  In `plugins/prowler/reddit_integration_test.go`, extend the doc comment on `liveRedditThreadURL` — the one justifying "exactly once -- no loop, no retry" — rather than contradicting it silently.
  Its intent is that unpaced request storms degrade this IP's standing;
  say that explicitly, and add that the RSS tier's two correctly-paced requests in `redditrss_integration_test.go` are consistent with that intent because the limiter spaces them against Reddit's own reported window.
  Change no assertion and no behaviour in that file.
- **Commit:** `test(prowler): add the live two-request reddit rss integration test`

## Batch Tests

`verify: go -C plugins/prowler test . && go -C plugins/prowler test -tags integration -run '^$' .` runs two gates.

The first is the module's whole offline package.
The files this batch changes and that it covers are `plugins/prowler/reddit_test.go` (the rewired `TestRedditAdapterFetch` subtests plus the never-requests-old.reddit assertion, card 11;
`TestToOldRedditURL` deleted, card 12;
`doNoRedirect` dropped from every literal, card 13) and `plugins/prowler/fetch_test.go` (the retargeted `TestFetchPage_RedditUrlRoutesThroughRedditAdapter`, card 11;
`TestFetchOldRedditHTML` deleted, card 12;
`doNoRedirect` dropped from `stubResponses` and every literal, card 13).
Every batch-1 through batch-3 suite keeps running here as a regression guard — in particular the OAuth golden file, which must still match after `Fetch` is rewired.

The second gate compiles the `//go:build integration` files without running any of them.
`-run '^$'` matches no test name, so `plugins/prowler/redditrss_integration_test.go` and `plugins/prowler/reddit_integration_test.go` are type-checked and linked on every implementer and fixer round while issuing zero live Reddit requests.
That is deliberate and is this batch's most important scoping decision: the `.rss` endpoint allows about one request per minute per IP, `verify:` re-runs after every round, and executing the live suite on each round would burn the exact resource this task exists to conserve.
The live suite is run by a human with `go -C plugins/prowler test -tags integration .` when they choose to spend the requests.

Card 14 has no runnable surface;
its two files are markdown and neither gate reads them.
Cards 12 and 13 each additionally carry a grep-and-confirm step in their own Requirements — zero hits for `fetchOldRedditHTML`, `toOldRedditURL`, `redditHostReplace`, `doNoRedirect`, and `noRedirectHTTPClient` across the module — because a dangling reference in a comment or a doc string compiles cleanly and neither gate would catch it.
