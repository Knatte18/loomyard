# Discussion: Add RSS-based Reddit read tier

```yaml
task: Add RSS-based Reddit read tier
slug: reddit-rss-tier
status: discussing
parent: main
```

## Problem

`prowler`'s Reddit adapter can only read Reddit when `PROWLER_REDDIT_CLIENT_ID`/`PROWLER_REDDIT_CLIENT_SECRET` are set:
tier 1 is the authenticated OAuth API, and tier 2 (anonymous `old.reddit.com` HTML) is login-gated and always fails.
Obtaining those credentials is itself blocked — Reddit's November 2025 "Responsible Builder Policy" puts every new API app registration (including plain "script" apps) behind a manual review that routinely rejects small/personal projects with no stated reason and no appeal.
The user already hit that rejection.
So today, in practice, prowler cannot read Reddit at all.

Reddit's unauthenticated `.rss` endpoint still works.
Appending `.rss` to a thread URL returns an Atom feed containing the post and its comments with no login, no OAuth, and no app registration.
This was re-confirmed live during this discussion (see "Live probe findings" under Technical context) — including the exact rate limit, which is **harsher than the task brief assumed**: roughly **one request per ~60 s per IP**, not one per 20–40 s.
This task adds that endpoint as a read tier so prowler reads Reddit again with zero setup, reusing the existing markdown output shape and pacing requests to respect the measured limit.

## Scope

**In:**

- A new `plugins/prowler/redditrss.go` implementing the RSS read tier: URL rewrite to `.rss`, HTTP fetch, Atom parse, and markdown rendering.
- A process-wide rate limiter for the RSS tier, driven by Reddit's `x-ratelimit-reset` response header, with an injectable clock/sleep seam for tests.
- Refactor of `formatRedditThread` in `plugins/prowler/redditoauth.go` onto a tier-neutral intermediate representation, so the OAuth JSON path and the RSS Atom path render through one formatter.
- Rewiring `redditAdapter.Fetch` in `plugins/prowler/reddit.go`: OAuth (when credentials present) → RSS → definitive error. The `old.reddit.com` HTML tier is removed.
- Support for non-thread Reddit URLs (subreddit and user feeds) through the same RSS tier, rendered as a link list.
- Offline fixtures under `plugins/prowler/testdata/` plus unit tests, and one `//go:build integration` live test.
- Doc updates: `plugins/prowler/README.md` and `plugins/prowler/skills/prowler/SKILL.md` (only if it describes the Reddit tiers).

**Out:**

- Pursuing Reddit app approval / the Responsible Builder Policy process. Out of band, not code.
- The Hacker News adapter, the generic static-fetch/Readability/browser cascade, and `blockdetect.go`'s detection rules. `looksLikeBlockPage` is *called* by the new tier but not modified.
- Authenticated RSS, Reddit search, comment pagination beyond what one `.rss` response returns, and reconstructing reply nesting (the feed carries no parent linkage — see Decisions).
- Caching responses to disk. The limiter is in-process only, so a fresh `prowler` invocation starts with an empty limiter.
- Any change to `manifest/roadmap.md`, and any new `CONSTRAINTS.md` invariant — this is a single-plugin change introducing no cross-cutting rule.

## Decisions

### tier-ordering

- Decision: `redditAdapter.Fetch` becomes two tiers — tier 1 the OAuth API *only when* `redditCredentials()` reports no missing variables, tier 2 the RSS endpoint (unconditional) — then the existing `errorResult` listing every attempted tier and its failure reason.
- Rationale: the ordering only ever matters when credentials are present, and in that case OAuth is strictly better than RSS on every axis: it carries scores, one level of nested replies, a fuller comment page (`limit=100&depth=2`), and a 100-requests-per-minute budget instead of one-per-60 s. When credentials are absent, tier 1 is skipped without issuing a request (the `redditCredentials()` pre-check in `Fetch` already does this), so the zero-setup path costs nothing. Putting RSS first would make the credentialed case slower and poorer for no gain.
- Rejected: **RSS unconditionally tier 1, OAuth tier 2** — penalises the credentialed user with a ~60 s wait and a score-less, flat-comment rendering. **RSS tier 1, drop OAuth entirely** — throws away a working, strictly-richer path that costs nothing when unavailable; the approval gate makes OAuth unreliable to *obtain*, not unreliable to *use*.

### drop-old-reddit-html-tier

- Decision: delete the `old.reddit.com` HTML tier — `fetchOldRedditHTML` and `toOldRedditURL` in `plugins/prowler/fetch.go` / `reddit.go`, its slot in `Fetch`, and its tests.
- Rationale: it is a measured-dead tier. Reddit login-gates `old.reddit.com` for anonymous readers, so it returns a redirect-to-login on every request; the prior `prowler-fix-reddit-block` task confirmed this. Keeping it means every uncredentialed Reddit fetch spends one guaranteed-failing request against an IP whose standing is the scarce resource the whole RSS tier is built around, and adds a permanently-useless line to the error output. RSS occupies the "anonymous fallback" slot it was holding.
- Rejected: **keep it as a last tier** — costs a request per fetch and can never succeed; if Reddit ever un-gates it, re-adding it is a small, well-understood change. **Keep it but only when RSS fails** — same cost, same zero success rate.
- Note for mill-plan: `redditHostReplace` exists solely for `toOldRedditURL` and goes with it. `redditHostPattern` (used by `Matches`) stays. `stripToBodyText`, `decodeContentEncoding`, and `minUsableTextLen` are used by the generic cascade too and must NOT be removed. `TestToOldRedditURL` and the `old.reddit.com` cases in `plugins/prowler/reddit_test.go` / `fetch_test.go` go away with the tier; check `fetch_test.go`'s two `testdata/reddit-block-page.html` uses individually — the block-page fixture and `looksLikeBlockPage` both stay.

### neutral-thread-representation

- Decision: introduce a tier-neutral intermediate in `redditoauth.go` (or a new shared file, mill-plan's call):

  ```go
  type redditPost struct {
      Title     string
      Subreddit string
      Author    string   // bare username, no "u/" prefix
      Score     *int     // nil when the source cannot supply it
      Selftext  string
      URL       string
      Comments  []redditComment
  }

  type redditComment struct {
      Author  string
      Score   *int
      Body    string
      Replies []redditComment // one level deep; always nil from the RSS tier
  }
  ```

  `formatRedditThread` is re-signatured to take `(redditPost, sourceURL string)`. The OAuth path gains a mapping step from `[]redditListing` to `redditPost`; the RSS path maps from parsed Atom entries.
- Rationale: the task brief requires the RSS output to match the OAuth output's markdown shape. One formatter guarantees that by construction; two formatters guarantee only that they matched on the day they were written. `*int` for `Score` is what lets the formatter tell "zero points" apart from "this source has no scores".
- Rejected: **synthesize `[]redditListing` from the Atom feed and call `formatRedditThread` unchanged** — the JSON `Score` field is a plain `int`, so every RSS-sourced post and comment would render "0 points", which is not merely ugly but factually wrong output handed to Claude. **A separate `formatRedditRSSThread`** — duplicates the rendering rules and drifts.

### absent-score-rendering

- Decision: when `Score` is nil, omit the points segment rather than substituting a placeholder. Post metadata line becomes `Reddit | r/<sub> | by u/<author>` instead of `Reddit | r/<sub> | <n> points | by u/<author>`; a comment header becomes `**u/<author>**:` instead of `**u/<author>** (<n> points):`.
- Rationale: an omitted field reads as "not available"; `0 points` or `unknown points` reads as data. The OAuth path's exact current output must be byte-identical after the refactor — that is a regression test, see Testing.
- Rejected: rendering `0 points` (wrong), rendering `? points` (noise in every RSS-sourced line).

### flat-comments-from-rss

- Decision: render RSS comments as a flat list under a `## Comments` heading (not `## Top Comments`), capped at the existing `maxTopComments` constant, in the order the feed returns them. `redditComment.Replies` is always nil on this path.
- Rationale: confirmed from a live capture — every comment entry is a sibling at feed level with an `<id>` of `t1_<id>` and no parent reference of any kind. Depth is not recoverable, so the entries are not necessarily top-level and `## Top Comments` would be a false claim. Feed order is Reddit's own ranking, not chronological or structural. The OAuth path keeps `## Top Comments` and its one level of nested replies, because there the structure is real.
- Rejected: reconstructing nesting by re-fetching each comment permalink (one extra ~60 s-throttled request per comment — absurd), or by heuristics on the rendered HTML (no signal present).

### rss-rate-limiter

- Decision: a process-wide limiter in `redditrss.go`, structurally mirroring the existing `redditTokenCache` (package-level singleton + `sync.Mutex` + a `reset()` method for tests):

  - State: `nextAllowed time.Time`, guarded by the mutex.
  - Before each RSS request: acquire the mutex, wait until `nextAllowed`, issue the request, record the new `nextAllowed`, release. Holding the lock across the request serialises every concurrent `runAll` goroutine onto one in-flight RSS request, which is exactly the required behaviour — Reddit's budget is per-IP, not per-URL.
  - After every response, success or failure: `nextAllowed = now + max(x-ratelimit-reset seconds, redditRSSMinSpacing)`. `redditRSSMinSpacing` is the floor used when the header is missing or unparseable; set it to 60 s, the measured window.
  - The wait is context-cancellable (`select` on a timer and `ctx.Done()`), never a bare `time.Sleep`.
  - Both the clock and the wait are injectable, following the existing `var timeNow = time.Now` seam in `redditoauth.go`; add a matching `var redditRSSWait = func(ctx, d) error`. Unit tests must never wait in real time.
- Rationale: the measured limit is a hard per-IP window with an exact remaining-seconds countdown in the response, so honouring the header is both simpler and more accurate than any fixed delay. `x-ratelimit-used` stayed at 1 across a 429 storm, so 429s do not consume budget — but they do nothing useful either, and pacing avoids them entirely.
- Rejected: **a fixed sleep between requests, ignoring the headers** — the window is not actually constant (observed resets of 3 s, 45 s, 52 s, 53 s, 54 s, 59 s), so a fixed delay is either wasteful or ineffective. **No pacing, return an error carrying an expected-wait hint** — pushes the retry loop onto Claude, and the burst case the user actually has ("spin up, want several threads") degenerates into a 429 storm.

### rss-wait-bounds

- Decision: two independent bounds. (1) Queue wait: a URL blocks in the limiter for at most `redditRSSMaxWait` = 5 minutes before the tier fails with an error naming the wait. (2) 429 retry: after the limiter grants a turn, a 429 response is retried at most twice more (3 attempts total), re-honouring the header countdown each time; a third 429 is a tier failure.
- Rationale: `main` builds the fetcher with `context.Background()` — no deadline — so an unbounded queue wait can hang the process indefinitely when many Reddit URLs are passed at once. At ~60 s per request, 5 minutes lets a realistic burst of ~5 threads through and fails the rest with a clear reason instead of hanging. The retry budget covers the genuine race where another process on the same IP consumed the window.
- Rejected: **unbounded queue wait governed only by ctx** — with `context.Background()` that is "unbounded" full stop. **A single attempt, no 429 retry** — loses to any transient contention on the shared IP.

### rss-progress-visibility

- Decision: when a request must wait longer than `redditRSSLogWaitThreshold` = **2 seconds**, write one line to **stderr**: `prowler: reddit rss rate limit, waiting <n>s before fetching <url>`. Never to stdout. A wait of exactly the threshold or shorter logs nothing, so the boundary is assertable.
- Rationale: the task brief asks for "a queue with visible progress". `main` prints exactly one line to stdout — the output file path — and the invoking skill wrapper captures that single line, so stdout is off limits. stderr is already used by `main` for errors and is visible to the operator.
- Rejected: silence (a 5-minute wait looks like a hang), or stdout (breaks the skill wrapper's contract).

### non-thread-reddit-urls

- Decision: the RSS tier handles any Reddit URL, discriminating on the parsed feed rather than the URL. If the first entry's `<id>` has the `t3_` prefix **and** the URL path contains `/comments/`, render the post+comments thread shape. Otherwise render a listing: an H1 from the feed `<title>`, a `Source:` line, then one bullet per entry with its title, author, and link.
- Rationale: `redditAdapter.Matches` claims every `reddit.com` URL, so a subreddit or user URL already routes here and today produces a confusing failure. `.rss` works identically on those URLs (confirmed live against `https://www.reddit.com/r/golang/.rss`), and the extra rendering branch is a few lines over parsing that is needed anyway. Failing a URL form the adapter advertises it handles is a worse outcome than a modest branch.
- Rejected: **thread URLs only, error otherwise** — leaves an advertised URL family permanently broken. **A separate listing adapter** — same endpoint, same parser, same limiter; splitting it duplicates all three.

### rss-url-construction

- Decision: a `redditRSSURL(rawURL string) (string, error)` built on `net/url`, mirroring `redditOAuthURL`'s structure: parse, force `https`, force host `www.reddit.com`, drop the query and fragment, ensure exactly one trailing `/` on the path, append `.rss`. Error on a parse failure, on an empty path, and when the path already ends in `.rss` is *not* an error (idempotent — normalise and reuse).
- Rationale: `Matches` accepts bare, `www.`, and `old.` hosts plus arbitrary query strings; all of those must land on one canonical feed URL. `old.reddit.com/…/.rss` also serves the feed, but normalising to `www` keeps one host in errors and fixtures.
- Rejected: string concatenation — breaks on a missing trailing slash, on `?utm_source=…`, and on `#comment-anchor`.

### rss-user-agent-and-headers

- Decision: send `User-Agent: redditAPIUserAgent()` (the existing `prowler/1.0`, overridable via `PROWLER_REDDIT_USER_AGENT`) and `Accept: application/atom+xml`. Do not set `Accept-Encoding` and do not call `defaultHeaders()` or `decodeContentEncoding` — let Go's transport handle compression transparently, exactly as `fetchRedditOAuthThread` does. Use `f.do` (redirect-following), not `f.doNoRedirect`.
- Rationale: consistency with the other Reddit tier, and Reddit's API rules penalise generic/impersonating User-Agents with harsher rate limiting — which is the one resource this tier cannot afford to lose. `browserUA` and the browser header set belong to the HTML cascade that this task removes from the Reddit path.
- Rejected: `browserUA` + `defaultHeaders()` (invites harsher throttling and drags in the `decodeContentEncoding` path for no benefit).

### rss-failure-detection

- Decision: the RSS tier reports a tier failure — never a partial or empty success — on each of:
  1. Transport error, or a non-2xx status. A `429` is named as such together with the seconds from `x-ratelimit-reset` (its body is empty, so status is the only signal).
  2. XML decode failure. Run the body through `looksLikeBlockPage` first, so an HTML wall served in place of the feed reports as a wall rather than an XML syntax error — mirroring `fetchRedditOAuthThread`'s handling.
  3. A well-formed feed with **zero** `<entry>` elements. Reddit's not-found response is a valid, entry-less Atom feed whose `<title>` is `<subreddit>: page not found` — captured in `.scratch/reddit-rss-capture/notfound.rss`. It arrived with a 404 status, so rule 1 catches it too, but the entry-count check must stand on its own because a genuinely empty feed is a failed read either way.
  4. A `/comments/` URL whose first entry is not `t3_` — the post is missing, so there is nothing to render as a thread.
- Rationale: `Fetch` aggregates per-tier reasons into `errorResult`, so every failure needs a distinguishable, human-readable cause. Rule 2 matters because Reddit has a history of serving HTML walls with 200 statuses from non-HTML endpoints.
- Rejected: returning an empty-but-successful document on rule 3 or 4 — hands Claude a document that looks like "this thread has no content" when the real cause is a bad URL or a block.

### rss-fixtures-and-live-test

- Decision: commit three fixtures under `plugins/prowler/testdata/` — `reddit-thread.rss` (real capture, trimmed to the post plus ~4 comments), `reddit-listing.rss` (real subreddit capture, trimmed to ~3 entries), and `reddit-rss-notfound.rss` (the entry-less page-not-found feed, committed verbatim; it is already tiny). The 429 case is constructed in-test from a status code and empty body, no fixture needed.
- Rationale: matches the existing `testdata/` convention (`reddit-block-page.html`, `reddit-thread.json`). Real captures are the only thing that proves the parser handles Reddit's actual escaping, its `<!-- SC_OFF -->`/`<!-- SC_ON -->` wrappers, and the `submitted by … [link] … [comments]` trailer.
- Note for mill-plan: **the live captures already exist** at `.scratch/reddit-rss-capture/` in this worktree — `thread.rss` (19 entries, r/golang), `subreddit.rss` (25 entries), `notfound.rss`, plus `headers-429-and-404.txt` and `ratelimit-probe.log` as evidence for the rate-limit numbers. Trim and copy these into `testdata/`; **do not spend fresh live requests re-capturing them.** `.scratch/` is gitignored, so the trimmed copies under `testdata/` are what gets committed.
- Rejected: hand-authoring the fixtures (the existing `testdata/reddit-thread.json` is hand-authored and its own test file flags that as a limitation).

### rss-integration-test-target

- Decision: the `//go:build integration` live test performs **two** paced requests: fetch `https://www.reddit.com/r/golang/.rss`, take the first entry's `<link href>`, then fetch that thread's `.rss` through `redditAdapter.Fetch`. Assert the result is not an `errorResult`, is not a block page, and carries the `Source:` line for the resolved URL. No hard-coded thread id anywhere.
- Rationale: a hard-coded thread rots — confirmed during this discussion, the thread `plugins/prowler/reddit_integration_test.go` currently uses (`/r/announcements/comments/5e19z2/…`) returns 404 from `.rss` today, so an RSS test copying that convention would ship already-broken. Discovering the thread from the subreddit feed is self-healing. It also exercises the limiter for real across two requests — the one behaviour no offline test can prove — at a cost of about two minutes in a suite that is opt-in and never runs in the fast tier.
- Rejected: **hard-code a thread and `t.Skip` on 404** — a test that silently skips forever is worth less than no test. **Reuse the OAuth test's thread URL** — measured to 404 over RSS.
- Note: the existing `reddit_integration_test.go` comment justifying "exactly once — no loop, no retry" is about *unpaced* request storms degrading the IP's standing. Two correctly-paced requests are consistent with that intent; mill-plan should extend that comment rather than contradict it silently.

### limiter-scope

- Decision: the limiter governs the RSS tier only. The OAuth tier and the OAuth token request are untouched.
- Rationale: they draw on a different, far larger authenticated budget (100 QPM). Routing them through a one-per-60 s limiter would make the credentialed path dramatically worse for no reason.
- Rejected: one shared Reddit limiter.

### file-layout

- Decision: new `plugins/prowler/redditrss.go` (URL rewrite, limiter, fetch, Atom types, parse, render) and `plugins/prowler/redditrss_test.go`. The neutral `redditPost`/`redditComment` types and the re-signatured `formatRedditThread` stay in `redditoauth.go` unless mill-plan judges a third shared file cleaner. `reddit.go` keeps only the adapter, `Matches`, `Fetch`, and `maxTopComments`.
- Rationale: matches the one-file-per-tier layout the module already uses (`redditoauth.go`, `blockdetect.go`, `hackernews.go`), each with a file-level doc comment stating its role.
- Rejected: growing `reddit.go` into a multi-tier file.

### docs

- Decision: update `plugins/prowler/README.md` in the same commit — the "Runtime prerequisite: Reddit API credentials" section becomes optional-not-required, states that RSS is the zero-setup path, names the ~60 s per-IP pacing and the resulting burst latency, and the "Site adapters" paragraph is rewritten for the new two-tier order with `old.reddit.com` removed. Check `plugins/prowler/skills/prowler/SKILL.md` for Reddit-tier or credential claims and update any that exist.
- Rationale: the README currently tells the reader that credentials are a prerequisite and that Reddit fetches fail without them. Both statements become false with this change.
- Rejected: deferring the doc update (the repo's task-completion rule requires docs in the same commit).

## Technical context

### Files in play

- `plugins/prowler/reddit.go` — `redditAdapter` (`Matches`, `Fetch`), `redditHostPattern`, `redditHostReplace`, `toOldRedditURL`, `maxTopComments = 20`. `Fetch` always returns `handled=true` so a Reddit URL never reaches the generic browser cascade; **this guarantee must survive**, and the existing tests enforce it by installing a `t.Fatal`-ing `f.browser`.
- `plugins/prowler/redditoauth.go` — credentials, token cache, `formatRedditThread`, `redditOAuthURL`, `fetchRedditOAuthThread`, `redditAPIUserAgent`, `var timeNow = time.Now`.
- `plugins/prowler/fetch.go` — `fetchOldRedditHTML` (to be deleted), plus `decodeContentEncoding`, `stripToBodyText`, `minUsableTextLen`, `errorResult`, `defaultHeaders` (all shared, all stay).
- `plugins/prowler/blockdetect.go` — `looksLikeBlockPage`, called by the new tier.
- `plugins/prowler/htmltext.go` — `htmlToText(fragment string) string`; strips script/style/noscript and normalises whitespace. The Hacker News adapter already uses it for HTML-bodied comments.
- `plugins/prowler/fetcher.go` — the `fetcher` injection seam: `do`, `doNoRedirect`, `browser`, `adapters`.
- `plugins/prowler/main.go` — `runAll` fans out one goroutine per URL, so anything process-wide in the RSS tier must be concurrency-safe. `main` uses `context.Background()`.
- `plugins/prowler/go.mod` — module `github.com/Knatte18/loomyard/plugins/prowler`, Go 1.26, its own module separate from the repo root. `encoding/xml` is stdlib; **no new dependency is needed or wanted.**

### Live probe findings (2026-08-25, this discussion)

Raw evidence is in `.scratch/reddit-rss-capture/`.

**Rate limit — harsher than the brief stated.** The window is about **60 seconds, one request per window, per IP**, not 20–40 s. Response headers on *every* response (200, 404, and 429 alike):

```
x-ratelimit-used: 1
x-ratelimit-remaining: 0.0
x-ratelimit-reset: <seconds until the window resets>
```

`x-ratelimit-remaining` reads `0.0` even immediately after a successful 200, so it is useless as a gate — `x-ratelimit-reset` is the only actionable signal. Observed reset values: 3, 13, 14, 33, 45, 52, 53, 54, 59. Probing at 20 s intervals produced `429 (reset 53) → 429 (reset 33) → 429 (reset 13) → 200`, i.e. the countdown decrements normally and a 429 does **not** restart the window or increment `x-ratelimit-used`. There is no `Retry-After` header. **429 responses have `content-length: 0`** — an empty body, so the status code is the only thing to key on.

**Thread feed shape** (`.scratch/reddit-rss-capture/thread.rss`, `https://www.reddit.com/r/golang/comments/1vxc255/small_projects/.rss`, 19 entries):

- `Content-Type: application/atom+xml; charset=UTF-8`, namespace `http://www.w3.org/2005/Atom` (plus an unused `media:` namespace).
- Feed level: `<category term="golang" label="r/golang"/>`, `<title>Small Projects : golang</title>` (post title + `" : "` + subreddit — prefer the post entry's own `<title>` and the feed `<category term>` over splitting this), `<link rel="alternate" href="…"/>`, `<subtitle>` (the subreddit description), `<id>`, `<updated>`, `<icon>`, `<logo>`.
- Each `<entry>` has `<author><name>/u/<username></name><uri>…</uri></author>`, `<category term=… label=…/>`, `<content type="html">…</content>`, `<id>`, `<link href="…"/>`, `<updated>`, `<published>` (post only), `<title>`.
- **`<id>` carries Reddit's fullname with kind prefix** — `t3_1vxc255` for the post, `t1_p5nsitu` for each comment. This is the kind discriminator, mirroring `redditChild.Kind`.
- The post entry is first; every remaining entry is a comment. **All comments are siblings — no parent id, no depth, no nesting.** Order is Reddit's ranking, not chronological.
- **No score field exists anywhere in the feed**, at post or comment level.
- Author names arrive **`/u/`-prefixed** and must be trimmed before formatting, since the formatter emits its own `u/` prefix.
- `<content type="html">` is XML-escaped HTML. `encoding/xml` unescapes it into a normal HTML string; that string then needs `htmlToText`. The body is wrapped as `<!-- SC_OFF --><div class="md">…</div><!-- SC_ON -->`.
- **The post entry's content has a trailer after `<!-- SC_ON -->`**: `&#32; submitted by &#32; <a href="…/user/x"> /u/x </a> <br/> <span><a href="…">[link]</a></span> &#32; <span><a href="…">[comments]</a></span>`. Comment entries have no trailer. Extracting the span between `<!-- SC_OFF -->` and `<!-- SC_ON -->` handles both uniformly; fall back to the whole content when the markers are absent (e.g. a link post with no selftext).
- A thread with more comments than the feed carries is silently truncated by Reddit (19 entries here). That is a hard cap of the endpoint, not something to work around.

**Listing feed shape** (`.scratch/reddit-rss-capture/subreddit.rss`, `https://www.reddit.com/r/golang/.rss`, 25 entries): structurally identical, except every entry is `t3_` and the feed `<title>` is the subreddit's display name.

**Not-found shape** (`.scratch/reddit-rss-capture/notfound.rss`): HTTP 404, a well-formed Atom feed with **zero entries** and `<title>announcements: page not found</title>`.

**Stale integration-test URL:** `https://www.reddit.com/r/announcements/comments/5e19z2/every_time_you_write_reddit_in_all_caps_you_are/` — the thread `reddit_integration_test.go` uses today — returns that 404 feed from `.rss`. Old/archived threads appear not to serve `.rss`. This is the direct evidence behind the `rss-integration-test-target` decision.

### Conventions to follow

- Every file opens with a file-level doc comment stating the file's role (see the top of `reddit.go`, `redditoauth.go`, `fetcher.go`).
- Every exported and unexported declaration carries a doc comment explaining *why*, not just *what* — the module's existing comments are unusually dense and set the bar (`mill:code-comments`, `golang:golang-comments`).
- Errors are wrapped with `fmt.Errorf("<action>: %w", err)` and never leak secrets.
- Tests are table-driven with `t.Run` subtests, and use the failure format `got X; want Y` naming the call (`plugins/prowler/reddit_test.go`).
- `stubResponses(t, map[string]*http.Response{…}, browserFn)` is the existing helper for stubbing `fetcher.do` by URL; fixtures are read via `os.ReadFile("testdata/<name>")`.

## Constraints

`CONSTRAINTS.md` at the hub root encodes invariants for the `lyx` Go module (`cmd/lyx`, `internal/…`). `plugins/prowler` is a **separate Go module** with its own `go.mod`, so the `cmd/lyx`-enforced invariants (Cwd Resolution, gitkit Leaf, CLI/Cobra, Test Tier Purity, Hermetic Git Test Environment, …) do not mechanically apply here and no `go test ./...` at the repo root will police this code. That makes the following review obligations rather than enforced rules:

- **No real-time waits in untagged tests.** The spirit of the Test Tier Purity Invariant applies: the unit tests for the limiter must drive it through the injected clock/wait seam and must not sleep. This is the single most likely way for this task to ship a slow or flaky test suite.
- **No new dependency.** `encoding/xml` covers the parse; `goquery`/`go-readability` are already present for the HTML cascade and are not needed here.
- **Live network only behind `//go:build integration`.** The fast unit run must stay fully offline, per the existing `reddit_integration_test.go` convention.
- **`redditAdapter.Fetch` must keep returning `handled=true` on every path**, and must never call `f.browser` — a second headless request against a Reddit challenge escalates it into a hard IP block.
- **Documentation Lifecycle:** docs land in the same commit as the change (`CLAUDE.md`, "Task completion"). `manifest/roadmap.md` is not touched; no new `CONSTRAINTS.md` invariant arises from this task.
- **Rate limit is the scarce resource.** No code path may issue a speculative or retry-on-anything Reddit request outside the limiter.

## Testing

TDD candidates — write the test first for each of these; they are pure functions or fully seam-injected:

- **`redditRSSURL`** — table-driven over: bare/`www.`/`old.` hosts; `http` and `https` schemes; trailing slash present and absent; a query string and a fragment (both dropped); a path already ending in `.rss` (idempotent); a subreddit path; empty path (error); unparseable URL (error).
- **The Atom parser** — against `testdata/reddit-thread.rss`: post title, subreddit, author with the `/u/` prefix stripped, selftext extracted from between the `SC_OFF`/`SC_ON` markers with the `submitted by … [link] … [comments]` trailer excluded, comment count, comment authors and bodies, `Score` nil throughout, `Replies` nil throughout. Against `testdata/reddit-listing.rss`: every entry `t3_`, titles and links present. Against `testdata/reddit-rss-notfound.rss`: zero entries → error.
- **Wait logging** — a stubbed wait of `redditRSSLogWaitThreshold` exactly emits nothing to stderr; a wait one second longer emits exactly one line naming the seconds and the URL; nothing is ever written to stdout.
- **The limiter** — with the clock and wait both stubbed: first request proceeds immediately; a second request waits until `nextAllowed`; the wait duration comes from `x-ratelimit-reset`; a missing or garbage header falls back to `redditRSSMinSpacing`; a cancelled context aborts the wait and returns `ctx.Err()`; the queue wait cap (`redditRSSMaxWait`) is enforced; concurrent callers serialise (drive N goroutines through a stubbed clock and assert exactly one in-flight request at a time). Must complete in milliseconds.
- **429 retry budget** — a stubbed transport returning 429 twice then 200 succeeds; three 429s produce a tier failure whose message names the reset seconds.

Other required coverage:

- **Formatter regression (highest value).** Before refactoring, capture `formatRedditThread`'s exact current output for `testdata/reddit-thread.json` as a golden string, then assert the refactored `[]redditListing → redditPost → markdown` path reproduces it byte-for-byte. This is what proves the neutral-representation refactor changed nothing on the OAuth path.
- **Nil-score rendering** — a `redditPost` with `Score == nil` at post and comment level omits the points segment; `Score` pointing at `0` still renders `0 points`.
- **`redditAdapter.Fetch` tier wiring**, via `stubResponses` with a `t.Fatal`-ing `f.browser` on every subtest: credentials present and OAuth succeeds → RSS never requested; credentials present, OAuth fails, RSS succeeds; credentials absent → OAuth skipped with no request issued, RSS succeeds; both tiers fail → `errorResult` naming both, with `handled == true`; `old.reddit.com` is never requested on any path.
- **Block-page detection on the RSS path** — an HTML wall body (reuse `testdata/reddit-block-page.html`) returned with a 200 from the `.rss` URL reports as a wall, not as an XML parse error.
- **Listing rendering** — a non-`/comments/` Reddit URL renders the link-list shape and not the thread shape.
- **Integration (`//go:build integration`)** — the two-request self-discovering live test from the `rss-integration-test-target` decision. It must not require credentials (unlike the existing OAuth integration test, which skips without them).
- **Deletions** — remove `TestToOldRedditURL` and the `old.reddit.com` subtests; confirm `go vet ./...` and `go build ./...` are clean in `plugins/prowler` and that no dangling reference to `fetchOldRedditHTML`/`toOldRedditURL`/`redditHostReplace` remains.

## Q&A log

- **Q:** Tier ordering — RSS first unconditionally, or keep OAuth first when credentials are present? **A:** [auto-pick] OAuth tier 1 when credentialed, RSS tier 2 unconditionally. **Why:** ordering only matters when credentials exist, and there OAuth is strictly richer (scores, nested replies, `limit=100&depth=2`, 100 QPM vs one-per-60 s); when credentials are absent tier 1 is skipped without a request, so the zero-setup path pays nothing.
- **Q:** Keep the `old.reddit.com` HTML tier? **A:** [auto-pick] Delete it. **Why:** measured always-failing (login-gated for anonymous readers), and each attempt spends a request against the IP standing this whole task is built around.
- **Q:** Keep the OAuth tier at all, given the approval gate? **A:** [auto-pick] Keep it. **Why:** the gate makes OAuth hard to *obtain*, not unreliable to *use*; it costs zero requests when credentials are absent.
- **Q:** How should RSS reuse `formatRedditThread`'s markdown shape? **A:** [auto-pick] Extract a tier-neutral `redditPost`/`redditComment` intermediate and re-signature the one formatter onto it. **Why:** guarantees the two tiers match by construction; synthesizing `[]redditListing` would render a fabricated "0 points" on every RSS line.
- **Q:** RSS carries no scores — how to render the points segment? **A:** [auto-pick] Omit it (`Score *int`, nil ⇒ segment absent). **Why:** an omitted field reads as unavailable; `0 points` reads as data and is false.
- **Q:** Can reply nesting be reconstructed from the feed? **A:** [auto-pick] No — render a flat `## Comments` list. **Why:** confirmed live, every comment is a feed-level sibling with a `t1_` id and no parent reference; `## Top Comments` would falsely claim they are top-level.
- **Q:** Pacing mechanism for the rate limit? **A:** [auto-pick] Process-wide mutex + `nextAllowed`, driven by `x-ratelimit-reset`, ctx-cancellable, with injectable clock and wait. **Why:** the window is not constant (observed 3–59 s), so only the header is accurate; `x-ratelimit-remaining` reads `0.0` even after a 200 and is unusable.
- **Q:** How long may a fetch wait, and how many 429 retries? **A:** [auto-pick] 5-minute queue-wait cap plus at most 3 attempts per turn. **Why:** `main` uses `context.Background()`, so an unbounded queue wait can hang the process; 5 minutes clears a realistic ~5-thread burst and fails the rest with a stated reason.
- **Q:** Show waiting progress to the operator? **A:** [auto-pick] One stderr line per non-trivial wait. **Why:** stdout is reserved for the single output-path line the skill wrapper captures; a silent 5-minute wait is indistinguishable from a hang.
- **Q:** Handle non-thread Reddit URLs (subreddit/user feeds)? **A:** [auto-pick] Yes — same fetch and parser, rendered as a link list. **Why:** `Matches` already claims those URLs and they currently fail confusingly; `.rss` serves them with an identical feed shape, so it is one rendering branch over parsing already needed.
- **Q:** User-Agent and headers for the RSS request? **A:** [auto-pick] `redditAPIUserAgent()` + `Accept: application/atom+xml`, no `defaultHeaders()`, no `Accept-Encoding`. **Why:** Reddit throttles generic/impersonating User-Agents harder, and rate budget is the scarce resource.
- **Q:** What counts as an RSS tier failure? **A:** [auto-pick] Non-2xx (429 named with its reset seconds), XML decode failure (block-page-checked first), zero entries, and a missing `t3_` post on a `/comments/` URL. **Why:** `Fetch` aggregates per-tier reasons, and Reddit serves HTML walls with 200 statuses from non-HTML endpoints.
- **Q:** Fixture source — capture live or hand-author? **A:** [auto-pick] Use the live captures already taken during this discussion, trimmed into `testdata/`. **Why:** real captures are the only proof the parser survives Reddit's actual escaping and `SC_OFF`/`SC_ON` wrappers, and re-capturing would burn scarce rate budget for nothing.
- **Q:** What should the live integration test target? **A:** [auto-pick] Two paced requests — discover a thread from `r/golang/.rss`, then fetch it. **Why:** hard-coded threads rot (the existing OAuth test's thread already 404s over `.rss`), and the two-request form is the only test that exercises the limiter for real.
- **Q:** Should the OAuth tier share the RSS limiter? **A:** [auto-pick] No, RSS-only. **Why:** the authenticated budget is ~100 QPM; throttling it to one-per-60 s would make the credentialed path far worse for no reason.
