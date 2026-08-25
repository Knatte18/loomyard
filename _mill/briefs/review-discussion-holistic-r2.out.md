MILL_REVIEW_BEGIN
# Review: Add RSS-based Reddit read tier

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:consistency] max(header, 60s) makes the header dead
**Section:** `### rss-rate-limiter` + Testing ("The limiter")
**Issue:** `nextAllowed = now + max(reset, redditRSSMinSpacing=60s)` contradicts the same bullet's "floor used when the header is missing or unparseable" and the rationale that the header is more accurate than a fixed delay — every observed reset in `.scratch/reddit-rss-capture/ratelimit-probe.log` and `headers-429-and-404.txt` is 3–59 s, i.e. always < 60, so `max` always picks the constant and the design degenerates into exactly the "fixed sleep, ignoring the headers" alternative it rejects.
**Fix:** State one rule: use the parsed `x-ratelimit-reset` when present and parseable, and `redditRSSMinSpacing` only as the missing/garbage-header fallback (or justify the floor and drop the header-accuracy rationale and the "wait duration comes from `x-ratelimit-reset`" test, which is unsatisfiable under `max`).

### [BLOCKING:design] Mutex-held-across-request defeats the 5-min cap
**Section:** `### rss-rate-limiter` / `### rss-wait-bounds`
**Issue:** The limiter acquires a `sync.Mutex` and holds it across the wait *and* the request, but `sync.Mutex.Lock()` is not cancellable and carries no deadline — a goroutine queued behind others cannot observe `redditRSSMaxWait` or `ctx.Done()` at all, so the stated 5-minute queue cap and ctx-cancellability are unimplementable as specified (worse with the 429 retry, up to ~3 more minutes under the same lock).
**Fix:** Specify a cancellable acquisition primitive (buffered-channel semaphore or `chan struct{}` token, `select`ed against `ctx.Done()` and the `redditRSSMaxWait` timer) and say whether `redditRSSMaxWait` measures queue time only or queue+turn+retry.

### [BLOCKING:decision] `doNoRedirect` seam left with no disposition
**Section:** `### drop-old-reddit-html-tier` ("Note for mill-plan")
**Issue:** `fetch.go:186` is the only production caller of `f.doNoRedirect`; deleting `fetchOldRedditHTML` orphans `fetcher.doNoRedirect` (`fetcher.go:26`), its `newFetcher` wiring (`main.go:22`), and `noRedirectHTTPClient` (`headers.go:37-41`, whose doc comment cites old.reddit's login redirect as its reason to exist) — the note enumerates `redditHostReplace` and the fixtures but is silent on all four.
**Fix:** State whether the no-redirect seam is removed with the tier or deliberately retained for future adapters, and say so in the same note.

### [NIT:consistency] `.rss` idempotence vs the stated algorithm
**Section:** `### rss-url-construction`
**Issue:** "ensure exactly one trailing `/` on the path, append `.rss`" applied to a path already ending in `.rss` yields `/.rss/.rss`, which the same bullet's idempotence clause forbids.
**Fix:** Spell out the strip-then-normalise order (drop a trailing `.rss` before the slash normalisation).

### [NIT:design] `sourceURL` for the RSS path unspecified
**Section:** `### neutral-thread-representation` / `### rss-integration-test-target`
**Issue:** The formatter takes `(redditPost, sourceURL)` but nothing says whether the RSS tier passes the caller's original URL or the derived `.rss` URL; the integration test asserts a `Source:` line "for the resolved URL", and the OAuth path passes the raw URL (`redditoauth.go:401`).
**Fix:** State that the RSS tier passes the original (non-`.rss`) URL as `sourceURL`, matching the OAuth tier.

### [NIT:scope] Stale old.reddit claims outside the named doc list
**Section:** `### docs` / Scope ("In")
**Issue:** The doc list names only `README.md` and (conditionally) `SKILL.md`, but `adapter.go:4` ("Reddit's old.reddit.com HTML"), `headers.go:38`, and `skills/prowler/SKILL.md:10` ("a fetched Reddit page especially mixes nav/sidebar chrome with the real content") all become false, and the repo convention requires accurate file-level doc comments.
**Fix:** Name those three sites explicitly in the docs decision rather than leaving SKILL.md conditional and the two Go file-docs unlisted.

## Verdict

REQUEST_CHANGES
Limiter spacing rule self-contradicts, lock model blocks the stated cap, orphaned seam undecided.
MILL_REVIEW_END
