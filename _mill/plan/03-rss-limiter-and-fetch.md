# Batch: rss-limiter-and-fetch

```yaml
task: "Add RSS-based Reddit read tier"
batch: "rss-limiter-and-fetch"
number: 3
cards: 3
verify: go -C plugins/prowler test .
depends-on: [2]
```

## Batch Scope

This batch adds everything in the RSS tier that has side effects: the process-wide per-IP rate limiter with its injectable clock, wait, and log seams;
the fetch-and-parse helper that acquires the limiter, issues the request, retries a 429, and detects every failure mode;
and the tier entry point that decides between the thread branch and the listing branch and renders the result.
It is one batch because the three are inseparable — the limiter has no observable behaviour without a request going through it, and the failure-detection rules are the limiter's own error surface.

The batch also ships `stubRedditRSSLimiter(t)`, the helper that every later offline test touching this tier must call.
Card 8 delivers it first, deliberately, so no card in this batch or batch 4 can be written against an unstubbed limiter.

The external interface batch 4 consumes is `fetchRedditRSS`, used as `redditAdapter.Fetch`'s tier 2, and `fetchRedditRSSFeed`, used by the live integration test.

Batch-local decision: serialisation uses a one-token buffered channel rather than a `sync.Mutex`, even though the neighbouring `redditTokenCache` in `redditoauth.go` uses a mutex.
`Mutex.Lock()` is uncancellable and takes no deadline, so a goroutine queued behind others could observe neither context cancellation nor `redditRSSMaxWait`, and the bounds this tier promises would be unimplementable.
A `select` over a channel receive, `ctx.Done()`, and a deadline timer honours all three.

## Cards

### Card 8: The process-wide per-IP limiter and its test seams

- **Context:**
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/main.go`
  - `plugins/prowler/reddit.go`
- **Edits:**
  - `plugins/prowler/redditrss.go`
  - `plugins/prowler/redditrss_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add the rate limiter to `plugins/prowler/redditrss.go`.
  Reddit's `.rss` endpoint allows roughly one request per 60 seconds per IP and reports the remaining window on every response — 200, 404, and 429 alike — in an `x-ratelimit-reset` header.
  `runAll` in `main.go` fans out one goroutine per URL, so this state must be concurrency-safe, and `main` builds its fetcher with `context.Background()`, so it must also be bounded independently of the context.

  Constants, each with a doc comment giving the measured reason for its value:
  - `redditRSSMinSpacing = 60 * time.Second` — the fallback spacing used **only** when the header is absent, empty, or unparseable.
  - `redditRSSMaxWait = 5 * time.Minute` — the single deadline covering one whole tier call.
  - `redditRSSLogWaitThreshold = 2 * time.Second` — waits at or below this log nothing.
  - `redditRSSMaxAttempts = 3` — one initial attempt plus at most two 429 retries.

  Seams, all package-level vars so tests can replace them, following the existing `var timeNow = time.Now` seam in `redditoauth.go`:
  - `var redditRSSWait = func(ctx context.Context, d time.Duration) error` — the production implementation is a `select` over a `time.NewTimer(d)` channel and `ctx.Done()`, returning `ctx.Err()` on cancellation and nil on elapse, stopping the timer in either case.
  - `var redditRSSLogOut io.Writer = os.Stderr` — never `os.Stdout`.
    `main` prints exactly one line to stdout, the output file path, and the invoking skill wrapper captures that single line, so stdout is off limits for this tier.

  The limiter itself, structurally mirroring `redditTokenCache`'s singleton-plus-`reset()` shape:
  - `type redditRSSLimiter struct { token chan struct{}; nextAllowed time.Time }`.
  - `func newRedditRSSLimiter() *redditRSSLimiter` — allocates a buffered channel of capacity 1 and pre-fills it with one token.
  - `var redditRSSLimit = newRedditRSSLimiter()` — one token for the whole process, because Reddit's budget is per-IP, not per-URL.
  - `func (l *redditRSSLimiter) reset()` — drains the channel non-blockingly, refills the single token, and zeroes `nextAllowed`.
    Tests only;
    document that it is never called concurrently with a fetch.
  - `func (l *redditRSSLimiter) acquire(ctx context.Context, deadline time.Time) error` — a `select` over receiving from `l.token`, `ctx.Done()`, and a timer set to `deadline.Sub(timeNow())`.
    On timer fire it returns an error naming how long the call waited and that it was waiting for the RSS request slot.
    On cancellation it returns `ctx.Err()`.
  - `func (l *redditRSSLimiter) release()` — returns the token, non-blockingly.
    Always called from a `defer` in the acquiring function so a cancelled or timed-out caller can never deadlock a later one.
  - `func (l *redditRSSLimiter) pace(ctx context.Context, deadline time.Time, rawURL string) error` — called only by the token holder.
    Computes `d = l.nextAllowed.Sub(timeNow())`;
    returns nil immediately when `d <= 0`.
    When `timeNow().Add(d)` is after `deadline`, returns an error naming the wait it would have needed and the deadline it would have crossed, without waiting.
    When `d > redditRSSLogWaitThreshold`, writes exactly one line to `redditRSSLogOut` of the form `prowler: reddit rss rate limit, waiting <n>s before fetching <rawURL>` before waiting.
    Then calls `redditRSSWait(ctx, d)` and returns its error unchanged.
  - `func (l *redditRSSLimiter) record(h http.Header)` — called by the token holder after every response, success or failure alike, setting `l.nextAllowed = timeNow().Add(redditRSSResetDelay(h))`.
  - `func redditRSSResetDelay(h http.Header) time.Duration` — reads `x-ratelimit-reset`, parses it with `strconv.ParseFloat`, and on a successful parse of a non-negative value returns that many seconds rounded **up** to the nearest whole second.
    On an absent, empty, unparseable, or negative value it returns `redditRSSMinSpacing`.
    Use `strconv.ParseFloat`, not `strconv.Atoi`: Reddit float-formats this header family — the captured `x-ratelimit-remaining: 0.0` proves it — so an integer-only parser would silently degrade to the 60-second fallback the day Reddit starts emitting `53.0`.
    Document that `redditRSSMinSpacing` is a missing-header fallback and never a floor applied on top of a parsed value.
    A clamp such as `max(reset, redditRSSMinSpacing)` would make the header dead code, because every reset value ever observed from this endpoint was under 60 seconds.

  Document on `redditRSSLimiter` that `nextAllowed` is owned by whichever goroutine currently holds the token, so the channel already provides mutual exclusion for it and no separate mutex is needed.
  Document on `acquire` why a `sync.Mutex` is deliberately not used here.

  Add `stubRedditRSSLimiter(t *testing.T)` to `plugins/prowler/redditrss_test.go`.
  It replaces `redditRSSWait` with a no-op that records the durations it was asked to wait and returns nil, points `timeNow` at a controllable fake clock the test can advance, redirects `redditRSSLogOut` at a `bytes.Buffer` the test can read, calls `redditRSSLimit.reset()`, and registers a `t.Cleanup` restoring all four so no test leaks state into another.
  Give it a doc comment stating that every untagged test reaching the RSS tier must call it as its first statement, and why: the limiter is a process-wide singleton and `stubResponses` builds responses with no `x-ratelimit-reset` header, so the second unstubbed RSS test in the process would sleep 60 real seconds under the production wait.
  Return whatever handle the tests need — the recorded waits, the clock, and the log buffer — rather than making tests reach for package vars directly.

  Add limiter tests to `plugins/prowler/redditrss_test.go`, all completing in milliseconds because both the clock and the wait are stubbed:
  - The first acquisition proceeds with no wait;
    a second, after a `record` that set `nextAllowed`, waits until `nextAllowed`.
  - The wait duration equals the parsed `x-ratelimit-reset` verbatim, asserted with a value below `redditRSSMinSpacing` such as `3` — this is the case that fails if anyone reintroduces a `max(reset, 60s)` clamp.
  - A float-formatted header value `"53.0"` parses to 53 seconds — the row that fails if anyone reaches for `strconv.Atoi`.
  - A missing header, an empty header, a non-numeric header, and a negative value each fall back to `redditRSSMinSpacing`.
  - A fractional value such as `"12.3"` rounds up to 13 seconds.
  - A cancelled context aborts both `acquire` and `pace`, returning `ctx.Err()`.
  - `redditRSSMaxWait` is enforced across the whole call, not per step: a caller that has already spent most of the budget acquiring the token and would then need a further pacing wait past the deadline fails at the deadline rather than after it.
  - Concurrent callers serialise: drive several goroutines through the stubbed clock and assert exactly one is inside the token at a time.
  - A caller that times out or is cancelled while waiting returns the token, so a later caller is not deadlocked — assert by running a timing-out caller and then a successful one.
  - Wait logging: a wait of exactly `redditRSSLogWaitThreshold` writes nothing to the log buffer, a wait one second longer writes exactly one line naming the seconds and the URL, and nothing is ever written to the process's stdout.
- **Commit:** `feat(prowler): add the process-wide reddit rss rate limiter`

### Card 9: Paced fetch, 429 retry, and failure detection

- **Context:**
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/blockdetect.go`
  - `plugins/prowler/fetch_test.go`
  - `plugins/prowler/blockdetect_test.go`
  - `plugins/prowler/testdata/reddit-thread.rss`
  - `plugins/prowler/testdata/reddit-rss-notfound.rss`
  - `plugins/prowler/testdata/reddit-block-page.html`
- **Edits:**
  - `plugins/prowler/redditrss.go`
  - `plugins/prowler/redditrss_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func fetchRedditRSSFeed(ctx context.Context, f fetcher, rawURL string) (redditAtomFeed, error)` to `plugins/prowler/redditrss.go`.
  It is the one function that issues a Reddit `.rss` request: it computes the call's deadline, acquires the limiter, paces, requests, retries a 429, records the window, and returns the parsed feed.
  It sits one level below markdown rendering, which is what lets the live integration test in batch 4 read a discovered thread link out of a subreddit feed.

  Behaviour, in order:

  1. Compute `deadline := timeNow().Add(redditRSSMaxWait)` once, on entry.
     That single value bounds every blocking step of this call — token acquisition, the pacing wait before the first request, and the pacing wait before each retry.
     A per-step budget would let queue time and retry time stack to roughly eight minutes while every individual step stayed "within bounds", and the sum is what an operator experiences as a hang.
     Document that on the declaration.
  2. Build the feed URL with `redditRSSURL(rawURL)`, wrapping its error as `build reddit rss URL: %w`.
  3. `redditRSSLimit.acquire(ctx, deadline)`;
     on success `defer redditRSSLimit.release()` immediately, so every exit path returns the token.
     The token is held across the whole call including its 429 retries, never released and re-acquired per attempt — releasing between attempts would let a sibling `runAll` goroutine overtake into a window that is still exhausted, earning another 429 and making the storm worse, which is the exact outcome one process-wide token exists to prevent.
     Document that trade-off: siblings queue behind a retrying call for up to its remaining deadline, which `redditRSSMaxWait` already bounds.
  4. Loop at most `redditRSSMaxAttempts` times.
     Each attempt calls `redditRSSLimit.pace(ctx, deadline, rawURL)`, returning its error unchanged on failure, then builds and issues the request.
  5. Request shape: `http.NewRequestWithContext` with `http.MethodGet`, the `User-Agent` header set to `redditAPIUserAgent()`, and `Accept` set to `application/atom+xml`.
     Send it with `f.do` — the redirect-following transport.
     Do not call `defaultHeaders()`, do not set `Accept-Encoding`, and do not call `decodeContentEncoding`: leaving the encoding header unset lets Go's transport handle compression transparently, exactly as `fetchRedditOAuthThread` already does.
     `browserUA` and the browser header set belong to the HTML cascade this task removes from the Reddit path, and Reddit's API rules penalise generic or impersonating User-Agents with harsher rate limiting — the one resource this tier cannot afford to lose.
  6. After every response, success or failure, read the body fully, close it, and call `redditRSSLimit.record(resp.Header)` before branching.
  7. A `429` status: when attempts remain, continue the loop — `record` has already re-armed the pacing from this response's own `x-ratelimit-reset`, so the next iteration's `pace` waits out the real window.
     When no attempts remain, return an error naming the status and the reset seconds parsed from the header.
     A 429 body is empty (`content-length: 0`), so the status code is the only signal to key on.

  Failure detection — the tier reports a failure, never a partial or empty success, on each of:

  - A transport error, or a non-2xx status.
    A `429` is named as such together with its reset seconds.
  - An XML decode failure.
    Run the body through `looksLikeBlockPage` **first**, so an HTML wall served in place of the feed reports as a wall rather than as an XML syntax error, mirroring `fetchRedditOAuthThread`'s own handling.
    Reddit has a history of serving HTML walls with 200 statuses from non-HTML endpoints.
    Apply the same block-page check to a non-2xx body before reporting the bare status.
  - A well-formed feed with zero `<entry>` elements.
    Reddit's not-found response is a valid, entry-less Atom feed;
    it arrives with a 404 so the status rule catches it too, but the entry-count check must stand on its own because a genuinely empty feed is a failed read either way.

  Every error is wrapped in the module's `fmt.Errorf("<action>: %w", err)` style and must be distinguishable in prose, because `redditAdapter.Fetch` aggregates per-tier reasons into one `errorResult`.

  Extend `plugins/prowler/redditrss_test.go`, every subtest calling `stubRedditRSSLimiter(t)` first:
  - A stubbed 200 carrying `plugins/prowler/testdata/reddit-thread.rss` returns a feed with 5 entries, and the request the stub observed carried `User-Agent: prowler/1.0`, `Accept: application/atom+xml`, no `Accept-Encoding` header, and a URL equal to `redditRSSURL(rawURL)`.
  - A transport error, a 500, and a 404 each return an error naming the cause.
  - A 200 whose body is `plugins/prowler/testdata/reddit-block-page.html` returns an error naming the wall reason, not an XML syntax error.
  - A 200 whose body is `plugins/prowler/testdata/reddit-rss-notfound.rss` returns an error for zero entries.
  - 429 retry budget: a stub returning 429 twice then 200 succeeds and issued exactly three requests;
    a stub returning 429 three times returns an error whose message names the reset seconds and issued exactly three requests.
  - The retries observe the same single call deadline: with the clock advanced past `redditRSSMaxWait` between attempts, the call fails at the deadline rather than exhausting the retry budget.
  - Token held across retries: while one caller is in its 429-retry sequence, a concurrent second caller issues no request until the first caller's sequence finishes.
  - A cancelled context returns `ctx.Err()` without issuing a request.
- **Commit:** `feat(prowler): add the paced reddit rss fetch with 429 retry and failure detection`

### Card 10: The RSS tier entry point and its rendering branches

- **Context:**
  - `plugins/prowler/redditformat.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/fetch_test.go`
  - `plugins/prowler/testdata/reddit-thread.rss`
  - `plugins/prowler/testdata/reddit-listing.rss`
- **Edits:**
  - `plugins/prowler/redditrss.go`
  - `plugins/prowler/redditrss_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func fetchRedditRSS(ctx context.Context, f fetcher, rawURL string) (string, error)` to `plugins/prowler/redditrss.go` — the tier entry point `redditAdapter.Fetch` calls in batch 4.

  It calls `fetchRedditRSSFeed(ctx, f, rawURL)`, returning its error unchanged, then discriminates on the **parsed feed**, not on the URL alone, with a fixed evaluation order:

  1. Transport, status, decode, and zero-entry failures have already been reported by `fetchRedditRSSFeed` before any rendering decision is reached.
  2. When `rawURL`'s parsed path contains `/comments/`, take the thread branch: call `redditPostFromFeed(feed, rawURL)` and return its error unchanged when the first entry is not `t3_`.
     Otherwise return `formatRedditThread(post, rawURL)`.
  3. Only a non-`/comments/` URL reaches the listing branch, which returns `formatRedditListing(feed, rawURL)`.

  Document why the listing branch is never a fall-through for a thread URL: a `/comments/` URL whose feed carries no post is a broken read — most likely a removed thread or a wall — and rendering its comments as an anonymous link list would disguise that as a successful fetch.

  `sourceURL` is `rawURL`, the caller's original URL, on both branches — never the derived `.rss` URL — so the rendered `Source:` line stays a URL a human can open.

  Extend `plugins/prowler/redditrss_test.go`, every subtest calling `stubRedditRSSLimiter(t)` first and using the existing `stubResponses` helper where a canned URL-keyed response fits:
  - A `/comments/` URL served `plugins/prowler/testdata/reddit-thread.rss` renders the thread shape: an H1 with the post title, a metadata line with no points segment, a `## Comments` heading, and the comment authors.
  - A non-`/comments/` subreddit URL served `plugins/prowler/testdata/reddit-listing.rss` renders the link-list shape and not the thread shape — no `## Comments` heading, one bullet per entry.
  - Evaluation order: a `/comments/` URL whose feed's first entry is `t1_` returns an error rather than falling through to the listing rendering, while a non-`/comments/` URL served that same feed renders as a listing.
  - `Source:` provenance on both branches: the rendered output carries the caller's original URL, and contains no `.rss` suffix on the `Source:` line.
- **Commit:** `feat(prowler): add the reddit rss tier entry point with thread and listing branches`

## Batch Tests

`verify: go -C plugins/prowler test .` runs the module's single Go package.
The coverage this batch adds all lives in `plugins/prowler/redditrss_test.go`: the limiter suite and the wait-logging assertions (card 8), the fetch/retry/failure-detection suite (card 9), and the tier-entry rendering and evaluation-order suite (card 10).

Every one of those subtests calls `stubRedditRSSLimiter(t)` as its first statement, so the whole batch's tests complete in milliseconds with no real-time wait anywhere.
That is the batch's principal correctness risk, not an incidental style rule: `stubResponses` builds responses carrying no `x-ratelimit-reset` header, so an unstubbed test falls back to `redditRSSMinSpacing` and the second such test in the process sleeps 60 real seconds under the production `redditRSSWait`.
A reviewer checking this batch should grep `plugins/prowler/redditrss_test.go` for every `t.Run` body that reaches `fetchRedditRSSFeed` or `fetchRedditRSS` and confirm the stub call is present.

Batch 1's and batch 2's suites keep running as regression guards.
The command is fully offline and no card in this batch touches a `//go:build integration` file, so no tag flag is needed here;
batch 4 adds the compile-only integration gate.
