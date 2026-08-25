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
- Rewiring `redditAdapter.Fetch` in `plugins/prowler/reddit.go`: OAuth (when credentials present) → RSS → definitive error. The `old.reddit.com` HTML tier is removed, and with it the now-orphaned no-redirect transport seam (`fetcher.doNoRedirect`, its `newFetcher` wiring, and `noRedirectHTTPClient`).
- Support for non-thread Reddit URLs (subreddit and user feeds) through the same RSS tier, rendered as a link list.
- Offline fixtures under `plugins/prowler/testdata/` plus unit tests, and one `//go:build integration` live test.
- Doc updates at the five sites named in the `docs` decision: `plugins/prowler/README.md`, `plugins/prowler/skills/prowler/SKILL.md`, and the file/declaration doc comments in `adapter.go` and `headers.go`.

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
- **The no-redirect seam goes too.** `plugins/prowler/fetch.go:186` is the *only* production caller of `f.doNoRedirect` (verified by grep during this discussion — every other hit is a test wiring it up), so deleting `fetchOldRedditHTML` orphans the whole seam. Remove all of it in the same change: the `doNoRedirect` field and its doc comment in `plugins/prowler/fetcher.go:22-26`, the "do, doNoRedirect, and browser must all be set" sentence in that file's `fetcher` doc comment, the `doNoRedirect: noRedirectHTTPClient.Do` wiring and its mention in `newFetcher`'s doc comment (`plugins/prowler/main.go:17,22`), `noRedirectHTTPClient` itself (`plugins/prowler/headers.go:37-45`, whose doc comment cites old.reddit's login redirect as its only reason to exist), and the `httpClient` doc comment's closing "a fetch path that needs to observe a redirect instead of following it uses `noRedirectHTTPClient`" sentence (`plugins/prowler/headers.go:30-32`). Every test that constructs a `fetcher` literal with a `doNoRedirect` field drops it (`reddit_test.go`, `fetch_test.go`).
- Rationale for removing rather than retaining: `fetcher`'s own doc comment states that none of its fields has a nil fallback, so an unset field is "a wiring bug that must fail loudly". Keeping a field no production path sets or reads inverts that contract, and leaves `noRedirectHTTPClient` documented as existing for a tier that no longer exists. Re-adding a redirect-observing client is a handful of lines if a future adapter needs one.

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
      Flat      bool     // true when the source cannot express reply structure
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

  **The `maxTopComments` caps stay in the formatter; mappings never truncate.** `formatRedditThread` today applies the cap twice — once over top-level comments and once over each comment's rendered replies — interleaved with its `Kind == "t1"` filter. After the refactor the *kind filtering* moves into each tier's mapping (the OAuth mapping drops `more` placeholders, the RSS mapping drops non-`t1_` entries), but **both caps remain in the formatter**, applied to `redditPost.Comments` and to each `redditComment.Replies`. Mappings hand over every comment they parsed, untruncated. Keeping the caps in one place is what makes them apply identically to both tiers, and what keeps the byte-identical OAuth regression meaningful — note that a fixture with fewer than 20 comments cannot detect a misplaced cap, so the golden test needs a synthetic over-cap case alongside the real fixture.

  **`Flat` is the comments-heading discriminator, set explicitly by each tier — never inferred.** The OAuth mapping sets `Flat = false`, the RSS mapping sets `Flat = true`, and the formatter emits `## Top Comments` when `Flat` is false and `## Comments` when it is true. It must not be derived from "every `Replies` is nil": an OAuth thread whose top-level comments genuinely have no replies is a real and common case, and inferring from it would silently flip that thread's heading and break the byte-identical OAuth regression the Testing section requires.

  **`sourceURL` is always the caller's original URL, never the derived `.rss` URL.** The OAuth tier already does this — `fetchRedditOAuthThread` passes `rawURL`, not the rewritten `oauth.reddit.com` URL — so the rendered `Source:` line stays a URL a human can open. The RSS tier follows the same rule, with the one refinement that the integration test's "resolved URL" means the thread URL discovered from the subreddit feed, still in its non-`.rss` form. The listing-rendering branch does the same.
- Rationale: the task brief requires the RSS output to match the OAuth output's markdown shape. One formatter guarantees that by construction; two formatters guarantee only that they matched on the day they were written. `*int` for `Score` is what lets the formatter tell "zero points" apart from "this source has no scores".
- Rejected: **synthesize `[]redditListing` from the Atom feed and call `formatRedditThread` unchanged** — the JSON `Score` field is a plain `int`, so every RSS-sourced post and comment would render "0 points", which is not merely ugly but factually wrong output handed to Claude. **A separate `formatRedditRSSThread`** — duplicates the rendering rules and drifts.

### absent-score-rendering

- Decision: when `Score` is nil, omit the points segment rather than substituting a placeholder. Post metadata line becomes `Reddit | r/<sub> | by u/<author>` instead of `Reddit | r/<sub> | <n> points | by u/<author>`; a comment header becomes `**u/<author>**:` instead of `**u/<author>** (<n> points):`.
- Rationale: an omitted field reads as "not available"; `0 points` or `unknown points` reads as data. The OAuth path's exact current output must be byte-identical after the refactor — that is a regression test, see Testing.
- Rejected: rendering `0 points` (wrong), rendering `? points` (noise in every RSS-sourced line).

### flat-comments-from-rss

- Decision: render RSS comments as a flat list under a `## Comments` heading (not `## Top Comments`), capped at the existing `maxTopComments` constant, in the order the feed returns them. `redditComment.Replies` is always nil on this path, and the heading is selected by `redditPost.Flat`, which the RSS mapping sets to `true` (see `neutral-thread-representation`) — not inferred from the nil `Replies`.
- Rationale: confirmed from a live capture — every comment entry is a sibling at feed level with an `<id>` of `t1_<id>` and no parent reference of any kind. Depth is not recoverable, so the entries are not necessarily top-level and `## Top Comments` would be a false claim. Feed order is Reddit's own ranking, not chronological or structural. The OAuth path keeps `## Top Comments` and its one level of nested replies, because there the structure is real.
- Rejected: reconstructing nesting by re-fetching each comment permalink (one extra ~60 s-throttled request per comment — absurd), or by heuristics on the rendered HTML (no signal present).

### rss-rate-limiter

- Decision: a process-wide limiter in `redditrss.go`, structurally mirroring the existing `redditTokenCache` (package-level singleton + `sync.Mutex` + a `reset()` method for tests):

  - **Serialisation is a 1-token buffered channel, not a held mutex** — `var redditRSSToken = make(chan struct{}, 1)`, pre-filled with one token. A caller acquires by `select`ing the token against `ctx.Done()` and the deadline timer, and returns the token in a `defer`. A `sync.Mutex` is deliberately *not* used for this: `Mutex.Lock()` is uncancellable and takes no deadline, so a goroutine queued behind others could observe neither `ctx` cancellation nor `redditRSSMaxWait` — the bounds in `rss-wait-bounds` would be unimplementable. Reddit's budget is per-IP, not per-URL, so one token for the whole process is exactly right.
  - **State: `nextAllowed time.Time`, owned by the token holder.** Only the goroutine currently holding the token reads or writes it, so the channel already provides the mutual exclusion and no separate mutex is needed. `reset()` (tests only) is the one exception and is called between tests, never concurrently with a fetch.
  - Once the token is held: wait until `nextAllowed`, issue the request, record the new `nextAllowed`, release.
  - **The token is held across 429 retries, not released and re-acquired per attempt.** One tier call takes the token once, and keeps it through every paced retry wait until it succeeds or gives up. Releasing between attempts would let a sibling `runAll` goroutine overtake into a window that is still exhausted — earning another 429 and making the storm worse — which is the exact outcome one process-wide token exists to prevent. The cost is that siblings queue behind a retrying call for up to its remaining deadline, which `redditRSSMaxWait` already bounds.
  - **Spacing rule, one rule only:** after every response — success or failure — `nextAllowed = now + d`, where `d` is the value parsed from `x-ratelimit-reset` when that header is present and parses as a non-negative number of seconds, and `redditRSSMinSpacing` = 60 s **only** when the header is absent, empty, or unparseable. The header value is used verbatim, including small values — a reset of 3 s was observed live and is legitimate, since it means the current window is nearly over. `redditRSSMinSpacing` is a missing-header fallback, never a floor applied on top of a parsed value: clamping with `max(reset, 60s)` would make the header dead code, because every reset ever observed was under 60 s, and would silently collapse this design into the fixed-delay alternative it rejects.
  - Every wait — the pacing wait and the token acquisition alike — is a `select` on a timer, `ctx.Done()`, and the call's deadline, never a bare `time.Sleep`.
  - Both the clock and the wait are injectable, following the existing `var timeNow = time.Now` seam in `redditoauth.go`; add a matching `var redditRSSWait = func(ctx context.Context, d time.Duration) error`. Unit tests must never wait in real time.
- Rationale: the measured limit is a hard per-IP window with an exact remaining-seconds countdown in the response, so honouring the header is both simpler and more accurate than any fixed delay. `x-ratelimit-used` stayed at 1 across a 429 storm, so 429s do not consume budget — but they do nothing useful either, and pacing avoids them entirely.
- Rejected: **a fixed sleep between requests, ignoring the headers** — the window is not actually constant (observed resets of 3 s, 45 s, 52 s, 53 s, 54 s, 59 s), so a fixed delay is either wasteful or ineffective. **No pacing, return an error carrying an expected-wait hint** — pushes the retry loop onto Claude, and the burst case the user actually has ("spin up, want several threads") degenerates into a 429 storm. **A `sync.Mutex` held across the request** — uncancellable and deadline-free, see above.

### rss-wait-bounds

- Decision: **one deadline, computed once, covering the whole tier call**, plus a separate retry count.
  1. On entry to the RSS tier, compute `deadline = timeNow() + redditRSSMaxWait`, with `redditRSSMaxWait` = 5 minutes. That single deadline bounds **every** blocking step of the call: token acquisition, the pacing wait before the first request, and the pacing wait before each 429 retry. `redditRSSMaxWait` therefore measures queue time *and* turn time *and* retry time together — not queue time alone. Exceeding it fails the tier with an error naming how long the call waited and what it was waiting for.
  2. Independently, a 429 response is retried at most twice more (3 attempts total), re-honouring the header countdown each time and still subject to the same deadline; a third 429 is a tier failure.
- Rationale: `main` builds the fetcher with `context.Background()` — no deadline — so an unbounded wait can hang the process indefinitely when many Reddit URLs are passed at once. Making the deadline a single value computed on entry (rather than a per-step budget) is what makes the 5-minute promise honest: a per-step cap would let queue time and retry time stack to ~8 minutes while every individual step stayed "within bounds". At ~60 s per request, 5 minutes lets a realistic burst of ~5 threads through and fails the rest with a clear reason instead of hanging. The retry budget covers the genuine race where another process on the same IP consumed the window.
- Rejected: **unbounded queue wait governed only by ctx** — with `context.Background()` that is "unbounded" full stop. **A single attempt, no 429 retry** — loses to any transient contention on the shared IP. **Separate per-step budgets** — the sum, not any single step, is what the operator experiences as a hang.

### rss-progress-visibility

- Decision: when a request must wait longer than `redditRSSLogWaitThreshold` = **2 seconds**, write one line to `redditRSSLogOut`: `prowler: reddit rss rate limit, waiting <n>s before fetching <url>`. `var redditRSSLogOut io.Writer = os.Stderr` is a package-level seam, matching `timeNow` and `redditRSSWait`, so a test can capture the line into a `bytes.Buffer` instead of asserting on the process's real stderr. Never to stdout. A wait of exactly the threshold or shorter logs nothing, so the boundary is assertable.
- Rationale: the task brief asks for "a queue with visible progress". `main` prints exactly one line to stdout — the output file path — and the invoking skill wrapper captures that single line, so stdout is off limits. stderr is already used by `main` for errors and is visible to the operator.
- Rejected: silence (a 5-minute wait looks like a hang), or stdout (breaks the skill wrapper's contract).

### non-thread-reddit-urls

- Decision: the RSS tier handles any Reddit URL, discriminating on the parsed feed rather than the URL. **Evaluation order is fixed and failure detection comes first:** rules 1–3 of `rss-failure-detection` (transport/status, decode, zero entries) run before any rendering decision; then, if the URL path contains `/comments/`, the feed must have a `t3_` first entry or rule 4 fails the tier — the listing branch is never a fall-through for a thread URL. Only a **non**-`/comments/` URL reaches the listing branch, which renders an H1 from the feed `<title>`, a `Source:` line, then one bullet per entry with its title, author, and link.
- Note: this ordering is what resolves the apparent overlap between "otherwise render a listing" and failure rule 4. A `/comments/` URL whose feed has no post is a broken read — most likely a removed thread or a wall — and rendering its comments as an anonymous link list would disguise that as a successful fetch.
- Rationale: `redditAdapter.Matches` claims every `reddit.com` URL, so a subreddit or user URL already routes here and today produces a confusing failure. `.rss` works identically on those URLs (confirmed live against `https://www.reddit.com/r/golang/.rss`), and the extra rendering branch is a few lines over parsing that is needed anyway. Failing a URL form the adapter advertises it handles is a worse outcome than a modest branch.
- Rejected: **thread URLs only, error otherwise** — leaves an advertised URL family permanently broken. **A separate listing adapter** — same endpoint, same parser, same limiter; splitting it duplicates all three.

### rss-marker-absent-body

- Decision: on the RSS path, `redditPost.Selftext` is **only** ever the text between `<!-- SC_OFF -->` and `<!-- SC_ON -->`, run through `redditHTMLToMarkdown` (see `rss-body-html-to-markdown`). When those markers are absent, `Selftext` is the empty string — never the whole `<content>`. Separately, `redditPost.URL` is set from the `href` of the trailer's `[link]` anchor when that anchor is present and its href is not the thread's own permalink; otherwise `URL` is empty. Comment bodies use the same marker extraction, and a marker-absent comment likewise yields an empty body and is skipped rather than rendered.
- Rationale: a link post has no selftext and no markers, so its entire `<content>` is the `<table>` thumbnail plus the `submitted by … [link] … [comments]` trailer. Falling back to the whole content would render that scaffolding as the post's body — 4 of 25 entries in the captured subreddit feed take exactly this branch, so it is the common case, not an edge case. With `Selftext` empty and `URL` populated, `formatRedditThread`'s existing "no selftext but there is a URL → emit `Link: <url>`" branch fires and produces the right output with no new formatter branch. The permalink comparison is what stops a self-post's `[comments]`-only trailer from being rendered as a `Link:` pointing back at the page the reader is already looking at.
- Rejected: **falling back to the whole content** — renders the trailer as body text. **Stripping the trailer with a regex over the whole content** — the marker extraction already does this correctly for the marker-present case, and the marker-absent case has no body to salvage.

### rss-body-html-to-markdown

- Decision: the RSS tier converts entry bodies with a Reddit-specific `redditHTMLToMarkdown(fragment string) string` in `redditrss.go`, **not** with a bare `htmlToText` call. It runs three steps over the fragment with `goquery` (already a dependency), then hands the result to the existing `htmlToText` for tag-stripping and whitespace normalisation:
  1. **Anchors become markdown links.** Each `<a href="X">Y</a>` is replaced by the literal text `[Y](X)`. A relative href — Reddit emits `/r/golang` and `/u/name` — is absolutized against `https://www.reddit.com` first. An anchor whose text already equals its href collapses to the bare URL rather than `[url](url)`.
  2. **Block boundaries become newlines.** `</p>`, `<br>`, and `</blockquote>` become a blank-line break; each `<li>` is prefixed with `- ` and terminated by a single newline.
  3. The result goes through `htmlToText`, whose `normalizeWhitespace` collapses the runs this leaves behind.
  `htmlToText` itself is **not modified** — the generic fetch cascade and the Hacker News adapter both depend on its current behaviour, and neither is in this task's scope.
- Rationale: the OAuth tier emits Reddit markdown verbatim, so its comment bodies keep their links and paragraphs; `htmlToText` is built on `goquery`'s `.Text()`, which discards every `href` and every block boundary. Feeding RSS bodies through it unmodified would mean a comment written `[the docs](https://…)` arrives as the bare word `the docs` with the URL gone, and a five-paragraph post arrives as one run-on line. Links are the substance of the use case this task exists for — Claude reading a Reddit thread as a *research source* — so silently dropping them defeats the feature while still passing every structural test. This is also what makes the task brief's "same markdown shape as the OAuth path" true rather than approximately true.
- Rejected: **accept the flattening, citing the Hacker News adapter's `htmlToText` precedent** — HN's Algolia comments are mostly prose and the precedent is a limitation, not a design goal; copying it here would knowingly ship the worse output on the tier that is now the *default* Reddit path. **Change `htmlToText` itself to preserve links** — would silently alter the generic cascade's fallback output and HN's rendering, which is out of scope and untested here. **Pull in a general HTML-to-markdown dependency** — a new module dependency for three rules this task fully specifies.

### rss-url-construction

- Decision: a `redditRSSURL(rawURL string) (string, error)` built on `net/url`, mirroring `redditOAuthURL`'s structure. Steps, **in this order**: parse; force scheme `https`; force host `www.reddit.com`; drop the query and fragment; **strip a trailing `.rss` from the path if present**; then ensure exactly one trailing `/`; then append `.rss`. Error on a parse failure and on an empty path. The strip step is what makes the function idempotent — `redditRSSURL(redditRSSURL(u)) == redditRSSURL(u)` — without it, normalising the slash first would turn an already-`.rss` path into `/.rss/.rss`.
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
  4. A `/comments/` URL whose first entry is not `t3_` — the post is missing, so there is nothing to render as a thread. Rules 1–3 are evaluated before the thread/listing discriminator; rule 4 *is* the discriminator's thread-branch precondition and takes precedence over the listing fall-through (see `non-thread-reddit-urls`).
- Rationale: `Fetch` aggregates per-tier reasons into `errorResult`, so every failure needs a distinguishable, human-readable cause. Rule 2 matters because Reddit has a history of serving HTML walls with 200 statuses from non-HTML endpoints.
- Rejected: returning an empty-but-successful document on rule 3 or 4 — hands Claude a document that looks like "this thread has no content" when the real cause is a bad URL or a block.

### rss-fixtures-and-live-test

- Decision: commit three fixtures under `plugins/prowler/testdata/` — `reddit-thread.rss` (real capture, trimmed to the post plus ~4 comments), `reddit-listing.rss` (real subreddit capture, trimmed to ~3 entries), and `reddit-rss-notfound.rss` (the entry-less page-not-found feed, committed verbatim; it is already tiny). The 429 case is constructed in-test from a status code and empty body, no fixture needed.
- Rationale: matches the existing `testdata/` convention (`reddit-block-page.html`, `reddit-thread.json`). Real captures are the only thing that proves the parser handles Reddit's actual escaping, its `<!-- SC_OFF -->`/`<!-- SC_ON -->` wrappers, and the `submitted by … [link] … [comments]` trailer.
- Note for mill-plan: **the live captures already exist** at `.scratch/reddit-rss-capture/` in this worktree — `thread.rss` (19 entries, r/golang), `subreddit.rss` (25 entries), `notfound.rss`, plus `headers-429-and-404.txt` and `ratelimit-probe.log` as evidence for the rate-limit numbers. Trim and copy these into `testdata/`; **do not spend fresh live requests re-capturing them.** `.scratch/` is gitignored, so the trimmed copies under `testdata/` are what gets committed.
- Rejected: hand-authoring the fixtures (the existing `testdata/reddit-thread.json` is hand-authored and its own test file flags that as a limitation).

### rss-integration-test-target

- Decision: the `//go:build integration` live test lives in a **new `plugins/prowler/redditrss_integration_test.go`**, not in `reddit_integration_test.go` (whose file doc comment scopes it to the OAuth API and whose skip-without-credentials contract is the opposite of this test's). It performs **two** paced requests: step 1 calls the tier's own internal fetch-and-parse helper — `fetchRedditRSSFeed(ctx, f, url)`, the function that acquires the limiter token, issues the request, and returns the parsed feed, sitting one level below the markdown rendering — on `https://www.reddit.com/r/golang/`, and reads the first entry's `<link href>`; step 2 fetches that thread URL through `redditAdapter.Fetch`. Both requests therefore go through the limiter, which is the point: the test proves the pacing works across two real calls. Step 1 cannot use `Fetch`, whose output is rendered markdown with no machine-readable entry links. Assert the result is not an `errorResult`, is not a block page, and carries the `Source:` line for the resolved thread URL in its original non-`.rss` form. No hard-coded thread id anywhere.
- **The test must force the RSS tier.** `Fetch` runs OAuth as tier 1 whenever `PROWLER_REDDIT_CLIENT_ID` and `PROWLER_REDDIT_CLIENT_SECRET` are both set (`plugins/prowler/reddit.go`), so on the maintainer's own credentialed machine the assertions would pass without the new code ever executing. The test therefore calls `t.Setenv(redditClientIDEnv, "")` and `t.Setenv(redditClientSecretEnv, "")` and `redditTokens.reset()` before invoking `Fetch`, which drives tier 1 into its no-request skip branch and guarantees the RSS tier is what produced the output. "Does not *require* credentials" is not the same as "credentials are absent" — this is the difference.
- Rationale: a hard-coded thread rots — confirmed during this discussion, the thread `plugins/prowler/reddit_integration_test.go` currently uses (`/r/announcements/comments/5e19z2/…`) returns 404 from `.rss` today, so an RSS test copying that convention would ship already-broken. Discovering the thread from the subreddit feed is self-healing. It also exercises the limiter for real across two requests — the one behaviour no offline test can prove — at a cost of about two minutes in a suite that is opt-in and never runs in the fast tier.
- Rejected: **hard-code a thread and `t.Skip` on 404** — a test that silently skips forever is worth less than no test. **Reuse the OAuth test's thread URL** — measured to 404 over RSS.
- Note: the existing `reddit_integration_test.go` comment justifying "exactly once — no loop, no retry" is about *unpaced* request storms degrading the IP's standing. Two correctly-paced requests are consistent with that intent; mill-plan should extend that comment rather than contradict it silently.

### limiter-scope

- Decision: the limiter governs the RSS tier only. The OAuth tier and the OAuth token request are untouched.
- Rationale: they draw on a different, far larger authenticated budget (100 QPM). Routing them through a one-per-60 s limiter would make the credentialed path dramatically worse for no reason.
- Rejected: one shared Reddit limiter.

### file-layout

- Decision: new `plugins/prowler/redditrss.go` (URL rewrite, limiter, fetch, Atom types, parse, `redditHTMLToMarkdown`, render), `plugins/prowler/redditrss_test.go`, and `plugins/prowler/redditrss_integration_test.go` (build-tagged `integration`). The neutral `redditPost`/`redditComment` types and the re-signatured `formatRedditThread` stay in `redditoauth.go` unless mill-plan judges a third shared file cleaner. `reddit.go` keeps only the adapter, `Matches`, `Fetch`, and `maxTopComments`.
- Rationale: matches the one-file-per-tier layout the module already uses (`redditoauth.go`, `blockdetect.go`, `hackernews.go`), each with a file-level doc comment stating its role.
- Rejected: growing `reddit.go` into a multi-tier file.

### docs

- Decision: five named sites, all updated in the same commit — no conditional "check whether it needs it" wording, every one of these is verified stale today:
  1. `plugins/prowler/README.md` — the "Runtime prerequisite: Reddit API credentials" section becomes optional-not-required, states that RSS is the zero-setup path, and names the ~60 s per-IP pacing and the resulting burst latency; the "Site adapters" paragraph is rewritten for the new two-tier order with `old.reddit.com` removed.
  2. `plugins/prowler/skills/prowler/SKILL.md:10` — "a fetched Reddit page especially mixes nav/sidebar chrome with the real content" describes the deleted HTML-scraping tier. Reddit output is now formatted markdown from a structured source, so this line's premise for the `distill-subagent` rule no longer holds for Reddit and must be rewritten (the rule itself stays — RSS output is still long).
  3. `plugins/prowler/adapter.go:1-5` — the file doc comment names "Reddit's old.reddit.com HTML" as the example adapter strategy.
  4. `plugins/prowler/headers.go:37-45` — `noRedirectHTTPClient`'s doc comment cites old.reddit's login redirect; the declaration is deleted outright per `drop-old-reddit-html-tier`.
  5. `plugins/prowler/headers.go:30-32` — `httpClient`'s doc comment closes by pointing at `noRedirectHTTPClient`, which no longer exists.
  6. `plugins/prowler/reddit.go` — three separate comment sites all naming three tiers and `old.reddit.com`: the file-level doc comment, the `redditAdapter` type doc comment, and `Fetch`'s own doc comment. All three are rewritten for the new two-tier shape, keeping the never-falls-through-to-browser guarantee they also state.
  `plugins/prowler/fetch.go`'s file doc comment is deliberately **not** on this list: it describes the generic static-fetch cascade and enumerates no Reddit tiers, so it does not go stale.
- Rationale: the README currently tells the reader that credentials are a prerequisite and that Reddit fetches fail without them — both become false. The other four are file-level doc comments describing code being deleted, and this module's convention (see every file's opening comment) is that those describe the file as it actually is. Naming the sites explicitly beats a conditional instruction that an implementer can satisfy by looking and concluding "nothing to do".
- Rejected: deferring the doc update (the repo's task-completion rule requires docs in the same commit).

## Technical context

### Files in play

- `plugins/prowler/reddit.go` — `redditAdapter` (`Matches`, `Fetch`), `redditHostPattern`, `redditHostReplace`, `toOldRedditURL`, `maxTopComments = 20`. `Fetch` always returns `handled=true` so a Reddit URL never reaches the generic browser cascade; **this guarantee must survive**, and the existing tests enforce it by installing a `t.Fatal`-ing `f.browser`.
- `plugins/prowler/redditoauth.go` — credentials, token cache, `formatRedditThread`, `redditOAuthURL`, `fetchRedditOAuthThread`, `redditAPIUserAgent`, `var timeNow = time.Now`.
- `plugins/prowler/fetch.go` — `fetchOldRedditHTML` (to be deleted), plus `decodeContentEncoding`, `stripToBodyText`, `minUsableTextLen`, `errorResult`, `defaultHeaders` (all shared, all stay).
- `plugins/prowler/blockdetect.go` — `looksLikeBlockPage`, called by the new tier.
- `plugins/prowler/htmltext.go` — `htmlToText(fragment string) string`; strips script/style/noscript and normalises whitespace. The Hacker News adapter already uses it for HTML-bodied comments. **It is built on `goquery`'s `.Text()`, so it discards every `<a href>` and every block boundary** — verified during this discussion. That is why the RSS tier wraps it rather than calling it directly (see `rss-body-html-to-markdown`), and why this file must not be modified.
- `plugins/prowler/fetcher.go` — the `fetcher` injection seam: `do`, `doNoRedirect`, `browser`, `adapters`. Its doc comment states that no field has a nil fallback, so an unset field is a wiring bug — which is why `doNoRedirect` is deleted rather than left orphaned (see `drop-old-reddit-html-tier`). After this task the seam is `do`, `browser`, `adapters`.
- `plugins/prowler/headers.go` — `httpClient`, `noRedirectHTTPClient` (deleted by this task), `defaultHeaders`, `browserUA`.
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
- **The post entry's content has a trailer after `<!-- SC_ON -->`**: `&#32; submitted by &#32; <a href="…/user/x"> /u/x </a> <br/> <span><a href="…">[link]</a></span> &#32; <span><a href="…">[comments]</a></span>`. Comment entries have no trailer. Extracting the span between `<!-- SC_OFF -->` and `<!-- SC_ON -->` handles the self-post case.
- **Link posts carry no markers at all.** Confirmed from the capture: 21 of the 25 entries in `.scratch/reddit-rss-capture/subreddit.rss` contain `SC_OFF`; the other 4 are link posts whose entire `<content>` is a `<table>` thumbnail plus the `submitted by … [link] … [comments]` trailer, with no body. See `rss-marker-absent-body` below for how that case is handled — falling back to "use the whole content" would render the trailer as the post body.
- **The trailer's `[link]` anchor is the external URL.** In the captured link-post entry it is `https://konradreiche.com/blog/…`, while the sibling `[comments]` anchor is the reddit permalink. That is the source for `redditPost.URL`.
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

- **`redditRSSURL`** — table-driven over: bare/`www.`/`old.` hosts; `http` and `https` schemes; trailing slash present and absent; a query string and a fragment (both dropped); a path already ending in `.rss`, both with and without a trailing slash, asserting the result is unchanged rather than `/.rss/.rss`; feeding the function's own output back into it (idempotence); a subreddit path; empty path (error); unparseable URL (error).
- **The Atom parser** — against `testdata/reddit-thread.rss`: post title, subreddit, author with the `/u/` prefix stripped, selftext extracted from between the `SC_OFF`/`SC_ON` markers with the `submitted by … [link] … [comments]` trailer excluded, comment count, comment authors and bodies, `Score` nil throughout, `Replies` nil throughout, `Flat` true. Against `testdata/reddit-listing.rss`: every entry `t3_`, titles and links present. Against `testdata/reddit-rss-notfound.rss`: zero entries → error.
- **Marker-absent link post** — trim one of the four marker-less link-post entries from `.scratch/reddit-rss-capture/subreddit.rss` into the listing fixture (or its own small fixture) and assert: `Selftext` is empty, `URL` is the external `[link]` href and not the reddit permalink, and the rendered output contains a `Link:` line and none of the `submitted by` / `[comments]` trailer text. This is the case a whole-content fallback would get wrong.
- **Self-post `URL` suppression** — a post whose trailer has only a `[comments]` anchor pointing at its own permalink leaves `URL` empty, so no `Link:` line is emitted alongside its selftext.
- **Wait logging** — a stubbed wait of `redditRSSLogWaitThreshold` exactly emits nothing to stderr; a wait one second longer emits exactly one line naming the seconds and the URL; nothing is ever written to stdout.
- **The limiter** — with the clock and wait both stubbed, completing in milliseconds:
  - First request proceeds immediately; a second waits until `nextAllowed`.
  - The wait duration equals the parsed `x-ratelimit-reset` **verbatim**, asserted with a value *below* `redditRSSMinSpacing` (e.g. `3`) — this is the case that fails if anyone reintroduces a `max(reset, 60s)` clamp.
  - A missing header, an empty header, a non-numeric header, and a negative value each fall back to `redditRSSMinSpacing`.
  - A cancelled context aborts both the token acquisition and the pacing wait, returning `ctx.Err()`.
  - The `redditRSSMaxWait` deadline is enforced across the whole call, not per step: a caller that spends most of the budget acquiring the token and would then need a further pacing wait past the deadline fails at the deadline rather than after it.
  - Concurrent callers serialise: drive N goroutines through a stubbed clock and assert exactly one in-flight request at a time.
  - A caller that times out or is cancelled while waiting **returns the token** (or never took it), so a later caller is not deadlocked — assert by running a timing-out caller and then a successful one.
- **429 retry budget** — a stubbed transport returning 429 twice then 200 succeeds; three 429s produce a tier failure whose message names the reset seconds; the retries observe the same single call deadline.
- **Token held across retries** — while one caller is in its 429-retry sequence, a concurrent second caller must not issue a request; assert the second caller's first request lands only after the first caller's sequence finishes.
- **`redditHTMLToMarkdown`** — table-driven: an absolute-href anchor becomes `[text](href)`; a relative `/r/golang` href is absolutized to `https://www.reddit.com/r/golang`; an anchor whose text equals its href renders as the bare URL, not `[url](url)`; `</p>` and `<br>` produce blank-line breaks; `<li>` items render as `- ` bullets; and — the regression that matters — a real comment body from `testdata/reddit-thread.rss` containing an external link keeps that URL in the output. Also assert `htmlToText`'s own behaviour is unchanged, so the shared helper is provably untouched.

Other required coverage:

- **Formatter regression (highest value).** Before refactoring, capture `formatRedditThread`'s exact current output for `testdata/reddit-thread.json` as a golden string, then assert the refactored `[]redditListing → redditPost → markdown` path reproduces it byte-for-byte. This is what proves the neutral-representation refactor changed nothing on the OAuth path.
- **Cap placement** — a synthetic `redditPost` with more than `maxTopComments` comments, and a comment with more than `maxTopComments` replies, are each truncated by the formatter; the OAuth mapping applied to an over-cap listing hands over every comment untruncated. The real fixture has fewer than 20 comments and cannot detect a misplaced cap on its own.
- **Evaluation order** — a `/comments/` URL whose feed's first entry is `t1_` fails the tier (rule 4) instead of falling through to the listing rendering; a non-`/comments/` URL with the same feed renders as a listing.
- **Wait-log seam** — assertions capture `redditRSSLogOut` into a `bytes.Buffer`; nothing is ever written to the process's stdout.
- **Nil-score rendering** — a `redditPost` with `Score == nil` at post and comment level omits the points segment; `Score` pointing at `0` still renders `0 points`.
- **Heading discriminator** — `Flat == false` renders `## Top Comments`, `Flat == true` renders `## Comments`; crucially, a `Flat == false` post whose comments all have nil `Replies` still renders `## Top Comments`, which is the assertion that fails if anyone reintroduces inference.
- **`redditAdapter.Fetch` tier wiring**, via `stubResponses` with a `t.Fatal`-ing `f.browser` on every subtest: credentials present and OAuth succeeds → RSS never requested; credentials present, OAuth fails, RSS succeeds; credentials absent → OAuth skipped with no request issued, RSS succeeds; both tiers fail → `errorResult` naming both, with `handled == true`; `old.reddit.com` is never requested on any path.
- **Block-page detection on the RSS path** — an HTML wall body (reuse `testdata/reddit-block-page.html`) returned with a 200 from the `.rss` URL reports as a wall, not as an XML parse error.
- **Listing rendering** — a non-`/comments/` Reddit URL renders the link-list shape and not the thread shape.
- **Integration (`//go:build integration`)** — the two-request self-discovering live test from the `rss-integration-test-target` decision. It neither requires nor tolerates credentials: it blanks both credential variables with `t.Setenv` and resets `redditTokens`, so it exercises the RSS tier on a credentialed machine as well as a bare one. It must still install a `t.Fatal`-ing `f.browser`.
- **`Source:` line provenance** — the RSS tier's rendered output carries the caller's original URL, not the `.rss` URL; assert on both the thread branch and the listing branch.
- **Deletions** — remove `TestToOldRedditURL` and the `old.reddit.com` subtests, and drop the `doNoRedirect` field from every `fetcher` literal in `reddit_test.go` and `fetch_test.go`. Confirm `go vet ./...` and `go build ./...` are clean in `plugins/prowler` and that no dangling reference to `fetchOldRedditHTML`, `toOldRedditURL`, `redditHostReplace`, `doNoRedirect`, or `noRedirectHTTPClient` remains anywhere in the module — a grep for each of the five is the check.

## Q&A log

- **Q:** Tier ordering — RSS first unconditionally, or keep OAuth first when credentials are present? **A:** [auto-pick] OAuth tier 1 when credentialed, RSS tier 2 unconditionally. **Why:** ordering only matters when credentials exist, and there OAuth is strictly richer (scores, nested replies, `limit=100&depth=2`, 100 QPM vs one-per-60 s); when credentials are absent tier 1 is skipped without a request, so the zero-setup path pays nothing.
- **Q:** Keep the `old.reddit.com` HTML tier? **A:** [auto-pick] Delete it. **Why:** measured always-failing (login-gated for anonymous readers), and each attempt spends a request against the IP standing this whole task is built around.
- **Q:** Keep the OAuth tier at all, given the approval gate? **A:** [auto-pick] Keep it. **Why:** the gate makes OAuth hard to *obtain*, not unreliable to *use*; it costs zero requests when credentials are absent.
- **Q:** How should RSS reuse `formatRedditThread`'s markdown shape? **A:** [auto-pick] Extract a tier-neutral `redditPost`/`redditComment` intermediate and re-signature the one formatter onto it. **Why:** guarantees the two tiers match by construction; synthesizing `[]redditListing` would render a fabricated "0 points" on every RSS line.
- **Q:** RSS carries no scores — how to render the points segment? **A:** [auto-pick] Omit it (`Score *int`, nil ⇒ segment absent). **Why:** an omitted field reads as unavailable; `0 points` reads as data and is false.
- **Q:** Can reply nesting be reconstructed from the feed? **A:** [auto-pick] No — render a flat `## Comments` list. **Why:** confirmed live, every comment is a feed-level sibling with a `t1_` id and no parent reference; `## Top Comments` would falsely claim they are top-level.
- **Q:** Pacing mechanism for the rate limit? **A:** [auto-pick] A process-wide 1-token channel guarding a `nextAllowed` timestamp, driven by `x-ratelimit-reset`, ctx-cancellable, with injectable clock and wait. **Why:** the window is not constant (observed 3–59 s), so only the header is accurate; `x-ratelimit-remaining` reads `0.0` even after a 200 and is unusable. (This answer originally said "process-wide mutex"; round 2 established that a held mutex cannot honour the cap or ctx, and the two r2 entries below record that change.)
- **Q:** How long may a fetch wait, and how many 429 retries? **A:** [auto-pick] 5-minute queue-wait cap plus at most 3 attempts per turn. **Why:** `main` uses `context.Background()`, so an unbounded queue wait can hang the process; 5 minutes clears a realistic ~5-thread burst and fails the rest with a stated reason.
- **Q:** Show waiting progress to the operator? **A:** [auto-pick] One stderr line per non-trivial wait. **Why:** stdout is reserved for the single output-path line the skill wrapper captures; a silent 5-minute wait is indistinguishable from a hang.
- **Q:** Handle non-thread Reddit URLs (subreddit/user feeds)? **A:** [auto-pick] Yes — same fetch and parser, rendered as a link list. **Why:** `Matches` already claims those URLs and they currently fail confusingly; `.rss` serves them with an identical feed shape, so it is one rendering branch over parsing already needed.
- **Q:** User-Agent and headers for the RSS request? **A:** [auto-pick] `redditAPIUserAgent()` + `Accept: application/atom+xml`, no `defaultHeaders()`, no `Accept-Encoding`. **Why:** Reddit throttles generic/impersonating User-Agents harder, and rate budget is the scarce resource.
- **Q:** What counts as an RSS tier failure? **A:** [auto-pick] Non-2xx (429 named with its reset seconds), XML decode failure (block-page-checked first), zero entries, and a missing `t3_` post on a `/comments/` URL. **Why:** `Fetch` aggregates per-tier reasons, and Reddit serves HTML walls with 200 statuses from non-HTML endpoints.
- **Q:** Fixture source — capture live or hand-author? **A:** [auto-pick] Use the live captures already taken during this discussion, trimmed into `testdata/`. **Why:** real captures are the only proof the parser survives Reddit's actual escaping and `SC_OFF`/`SC_ON` wrappers, and re-capturing would burn scarce rate budget for nothing.
- **Q:** What should the live integration test target? **A:** [auto-pick] Two paced requests — discover a thread from `r/golang/.rss`, then fetch it. **Why:** hard-coded threads rot (the existing OAuth test's thread already 404s over `.rss`), and the two-request form is the only test that exercises the limiter for real.
- **Q:** Should the OAuth tier share the RSS limiter? **A:** [auto-pick] No, RSS-only. **Why:** the authenticated budget is ~100 QPM; throttling it to one-per-60 s would make the credentialed path far worse for no reason.
- **Q:** (r2 gap) What primitive serialises RSS requests, given the stated 5-minute cap and ctx-cancellability? **A:** [auto-pick] A 1-token buffered channel `select`ed against `ctx.Done()` and a single call-wide deadline — not a held `sync.Mutex`. **Why:** `Mutex.Lock()` is uncancellable and deadline-free, so a queued goroutine could observe neither bound and the stated cap was unimplementable as originally written.
- **Q:** (r2 gap) Does `redditRSSMaxWait` bound queue time only, or the whole call? **A:** [auto-pick] The whole call — one deadline computed on entry, covering token acquisition, the pacing wait, and every 429-retry wait. **Why:** per-step budgets would let the steps stack to ~8 minutes while each stayed individually "within bounds"; the sum is what the operator experiences as a hang.
- **Q:** (r3 gap) How does one shared formatter choose between `## Top Comments` and `## Comments`? **A:** [auto-pick] An explicit `Flat bool` on `redditPost`, set to `false` by the OAuth mapping and `true` by the RSS mapping. **Why:** inferring it from all-nil `Replies` would flip a genuinely reply-less OAuth thread to the wrong heading and break the byte-identical OAuth regression test.
- **Q:** (r3 gap) What is a link post's body and URL on the RSS path, given it carries no `SC_OFF`/`SC_ON` markers? **A:** [auto-pick] `Selftext` is empty (never the whole content), and `URL` comes from the trailer's `[link]` anchor when it differs from the thread permalink. **Why:** 4 of 25 captured entries are marker-less link posts whose entire content is thumbnail plus `submitted by … [link] … [comments]` scaffolding; a whole-content fallback would render that scaffolding as the post body.
- **Q:** (r3 gap) How does the live test guarantee it exercised the RSS tier rather than OAuth? **A:** [auto-pick] Blank both credential variables with `t.Setenv` and reset `redditTokens` before calling `Fetch`. **Why:** tier 1 is OAuth whenever credentials are set, so on the maintainer's own machine the assertions would otherwise pass without the new code running at all.
- **Q:** (r4 gap) Does a retrying caller hold the limiter token across its 429 waits, or release and re-queue? **A:** [auto-pick] Hold it. **Why:** releasing lets a sibling overtake into a still-exhausted window and earn another 429 — the storm the single token exists to prevent; `redditRSSMaxWait` already bounds how long siblings can be blocked.
- **Q:** (r4 gap) RSS bodies are HTML and `htmlToText` drops every `href` — accept that, or restore links? **A:** [auto-pick] Restore them, via a Reddit-specific `redditHTMLToMarkdown` wrapper that linkifies anchors and restores block boundaries before delegating to the untouched `htmlToText`. **Why:** the OAuth tier emits real markdown with links intact, and links are the substance of "Claude reads a thread as a research source" — silently dropping them defeats the feature while still passing every structural test.
- **Q:** (r4) Where do the `maxTopComments` caps live after the refactor? **A:** [auto-pick] In the formatter, both of them; mappings do kind-filtering only and never truncate. **Why:** one site means both tiers cap identically, and it keeps the OAuth golden regression meaningful.
- **Q:** (r2) Is the no-redirect transport seam kept after the `old.reddit.com` tier is deleted? **A:** [auto-pick] Deleted with the tier — field, wiring, client, and doc comments. **Why:** `fetch.go:186` is its only production caller, and `fetcher`'s own contract says an unset field is a wiring bug that must fail loudly, which an orphaned field silently inverts.
