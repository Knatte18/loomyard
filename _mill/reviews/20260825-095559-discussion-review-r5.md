MILL_REVIEW_BEGIN
# Review: Add RSS-based Reddit read tier

```yaml
duration_s: 163.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] Limiter unstubbed in non-limiter unit tests
**Section:** Testing / Constraints ("No real-time waits in untagged tests")
**Issue:** The no-real-time-wait rule is scoped to "the unit tests for the limiter", but every other offline test that reaches the RSS tier through `Fetch` — tier wiring, failure detection, block-page-on-RSS, listing rendering, `Source:` provenance — also passes through the process-wide `redditRSSToken`/`nextAllowed` singleton, and `stubResponses` (`fetch_test.go:22-35`) returns responses with no `x-ratelimit-reset` header, so the second such test in the process falls back to `redditRSSMinSpacing` = 60 s of *real* time under the default `redditRSSWait`.
**Fix:** State the rule as binding on every untagged test that reaches the RSS tier — a named helper (stub `redditRSSWait` + limiter `reset()` via `t.Cleanup`) that all of them install — rather than on the limiter tests alone.

### [NIT:consistency] docs decision says five sites, lists six
**Demoted-from:** BLOCKING
**Section:** `### docs` and Scope ("In", doc-updates bullet)
**Issue:** The decision opens "five named sites" and then enumerates six (1–6), and Scope's own enumeration names only `README.md`, `SKILL.md`, `adapter.go`, `headers.go` — omitting item 6, `reddit.go`'s three stale comment sites (file doc, `redditAdapter` doc, `Fetch` doc), all three verified stale at `plugins/prowler/reddit.go:1-4,34-39,48-59`.
**Fix:** Correct the count and add `plugins/prowler/reddit.go` to Scope's list so a plan writer reading Scope alone does not drop those three sites.

### [NIT:design] `x-ratelimit-reset` parse strictness unpinned
**Section:** `### rss-rate-limiter` (spacing rule)
**Issue:** "parses as a non-negative number of seconds" does not say integer-vs-float, and the sibling header `x-ratelimit-remaining: 0.0` (verified in `.scratch/reddit-rss-capture/headers-429-and-404.txt`) proves Reddit float-formats this header family, so a `strconv.Atoi` implementation would silently fall back to 60 s the day Reddit emits `53.0`.
**Fix:** Pin float-tolerant parsing (`ParseFloat`) and add a `"53.0"` row to the limiter test table.

### [NIT:consistency] README lede also mis-describes Reddit fetching
**Section:** `### docs`, site 1
**Issue:** Site 1 scopes the README edit to the "Runtime prerequisite" section and the "Site adapters" paragraph, but `plugins/prowler/README.md:3` also states Reddit posts are fetched "by driving a real headless browser plus Mozilla-Readability-style extraction", which this task makes further from true.
**Fix:** Name README line 3 as part of site 1.

## Verdict

REQUEST_CHANGES
Two blockings: unstubbed limiter in non-limiter unit tests, and a self-contradicting docs-site enumeration.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
