MILL_REVIEW_BEGIN
# Review: Add RSS-based Reddit read tier

```yaml
duration_s: 187.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] Token ownership across 429 retries undecided
**Section:** `rss-rate-limiter` + `rss-wait-bounds`
**Issue:** The limiter says "issue the request, record the new `nextAllowed`, release", while the retry rule adds up to two further paced waits (~2 min) per call; whether the retrying caller keeps the single process token across those waits or releases and re-queues is never stated, and the two choices differ observably — one blocks every sibling `runAll` goroutine for the whole retry sequence, the other lets a fresh caller overtake.
**Fix:** State explicitly whether the token is held across the 429 retry waits or released and re-acquired per attempt, and add the corresponding concurrency assertion to the Testing section.

### [BLOCKING:design] RSS bodies lose links; "same markdown shape" premise unstated
**Section:** `rss-marker-absent-body` / `neutral-thread-representation`
**Issue:** `formatRedditThread`'s doc comment states OAuth bodies are Reddit markdown emitted unchanged, whereas the RSS path runs `<content>` HTML through `htmlToText`, whose `goquery` `.Text()` drops every `<a href>` — a comment written `[text](url)` renders as bare `text` with the URL gone (confirmed against `plugins/prowler/htmltext.go:23-32` and the anchors in `.scratch/reddit-rss-capture/thread.rss`), so the two tiers' bodies are not the same shape and the loss has no stated disposition.
**Fix:** Decide explicitly — accept the flattening (citing the Hacker News adapter precedent) or convert anchors back to `[text](href)` before emitting — and record the rejected alternative.

### [NIT:design] `maxTopComments` cap placement unstated
**Section:** `neutral-thread-representation`, Testing (formatter regression)
**Issue:** Today the cap is applied inside the formatter, interleaved with the `Kind == "t1"` filter, and also caps replies per comment; after the refactor the mapping does the kind filtering, and nothing says where the cap now lives — the golden regression only catches a wrong choice if the fixture exceeds 20.
**Fix:** State that the formatter retains both caps (post-level and per-comment replies) and that mappings do not truncate.

### [NIT:scope] Live test's discovery step: seam and file unnamed
**Section:** `rss-integration-test-target`, `file-layout`
**Issue:** Step 1 is described as parsing `<link href>` from `r/golang/.rss`, which cannot come from `Fetch`'s rendered markdown, yet neither the function it calls nor whether that call goes through the limiter is named; `file-layout` also never says whether the test joins `reddit_integration_test.go` or a new file.
**Fix:** Name the internal fetch/parse function the discovery step calls, confirm it is limiter-paced, and name the test file.

### [NIT:design] No seam named for the stderr wait log
**Section:** `rss-progress-visibility`, Testing (wait logging)
**Issue:** Every other side effect gets an explicit injectable seam (`timeNow`, `redditRSSWait`), but the wait line is specified as going to stderr with no named writer var, while the test asserts exact line content and that stdout stays clean.
**Fix:** Name a package-level writer seam (e.g. `var redditRSSLogOut io.Writer = os.Stderr`) as the log target.

### [NIT:consistency] Listing fall-through overlaps failure rule 4
**Section:** `non-thread-reddit-urls` vs `rss-failure-detection`
**Issue:** The discriminator sends any feed whose first entry is not `t3_` to the listing branch ("Otherwise render a listing"), while failure rule 4 makes exactly that case a tier failure on a `/comments/` URL — the two read as contradictory for the same input.
**Fix:** State the evaluation order — failure detection runs before the thread/listing discriminator.

## Verdict

REQUEST_CHANGES
Two design decisions are unstated: limiter token ownership under retry, and RSS body fidelity.
MILL_REVIEW_END
