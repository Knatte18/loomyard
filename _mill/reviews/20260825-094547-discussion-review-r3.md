MILL_REVIEW_BEGIN
# Review: Add RSS-based Reddit read tier

```yaml
duration_s: 133.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] Comments heading undetermined in one formatter
**Section:** `neutral-thread-representation` + `flat-comments-from-rss`
**Issue:** RSS must render `## Comments` and OAuth `## Top Comments`, but the shared formatter's only inputs are `(redditPost, sourceURL)` and `redditPost` carries no field distinguishing the two; inferring it from "all `Replies` nil" would flip an OAuth thread whose comments happen to have no replies to `## Comments`, breaking the byte-identical regression the `absent-score-rendering` decision requires.
**Fix:** Name the discriminator explicitly on the neutral representation (e.g. a `Flat bool` / heading field on `redditPost`) and state which value each tier sets.

### [BLOCKING:design] Marker-absent fallback renders the post trailer as body
**Section:** Technical context → "Thread feed shape"
**Issue:** The rule "extract between `SC_OFF`/`SC_ON`, fall back to the whole content when the markers are absent" contradicts the trailer-exclusion rule for exactly the case it names — a link post with no selftext has no markers, so its whole content *is* the `submitted by … [link] … [comments]` trailer; the captured `.scratch/reddit-rss-capture/subreddit.rss` has 21 `SC_OFF` markers across 25 entries, so 4 live entries take this branch. Relatedly, nothing says what `redditPost.URL` is set to on the RSS path, which decides whether the formatter's `Link:` branch fires and with what value.
**Fix:** Decide the marker-absent behaviour (strip the trailer / treat selftext as empty) and state the RSS-path source for `redditPost.URL`.

### [BLOCKING:design] Live test may never exercise the RSS tier
**Section:** `rss-integration-test-target` + Testing
**Issue:** The test fetches the discovered thread "through `redditAdapter.Fetch`", but `Fetch` tier 1 is OAuth whenever `PROWLER_REDDIT_CLIENT_ID`/`_SECRET` are set (verified in `plugins/prowler/reddit.go:66-75`), so on a credentialed machine the assertions pass while the new tier is never called — and "must not require credentials" does not exclude credentials being present.
**Fix:** State that the test forces the RSS path (clear the two env vars via `t.Setenv`, or call the RSS tier function directly).

### [NIT:consistency] Q&A log still names a mutex-based limiter
**Section:** Q&A log, "Pacing mechanism for the rate limit?"
**Issue:** That answer reads "Process-wide mutex + `nextAllowed`", which the `rss-rate-limiter` decision explicitly rejects in favour of a 1-token channel; the later r2 entry supersedes it but the stale line is still quotable as the decision.
**Fix:** Rewrite the earlier Q&A answer to the channel-based mechanism, or mark it superseded.

### [NIT:consistency] Doc site list conditional where it should be named
**Section:** `docs`
**Issue:** The decision forbids "check whether it needs it" wording, then closes with "also re-check `reddit.go`'s and `fetch.go`'s file-level doc comments, both of which enumerate the tier list" — `fetch.go:1-4` enumerates no tiers, while `reddit.go:1-4`, the `redditAdapter` type doc, and `Fetch`'s doc comment all name three tiers and old.reddit.com and are certainly stale.
**Fix:** Promote the three `reddit.go` comment sites to named entries and drop the `fetch.go` claim.

## Verdict

REQUEST_CHANGES
Three decisions underdetermined: comments heading, marker-absent post body, RSS-path live test.
MILL_REVIEW_END
