MILL_REVIEW_BEGIN
# Review: prowler: site-adapter mechanism + github-repo-explorer skill (Claude reading the web) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-30
```

## Findings

### [NIT] maxTopComments doc claims it is shared by redditAdapter, but it isn't
**Location:** `plugins/prowler/reddit.go:23-27`
**Issue:** The comment says the constant is "Shared by redditAdapter and the Hacker News adapter," but `redditAdapter`/`fetchOldRedditHTML` never reference `maxTopComments` — Reddit's strategy keeps all comments via `stripToBodyText`, unbounded; only `hackernews.go` uses the constant.
**Fix:** Reword to something like "used by the Hacker News adapter; declared here alongside Reddit's other retained constants" rather than implying Reddit consumes the same bound.

### [NIT] headers.go's "non-Reddit" comment is now fully inaccurate
**Location:** `plugins/prowler/headers.go:17-19`
**Issue:** `defaultHeaders` is documented as applying "to every static (non-Reddit) fetch request," but `fetchOldRedditHTML` (now Reddit's sole, always-used strategy via `redditAdapter.Fetch`) calls `defaultHeaders()` for every Reddit fetch too — the parenthetical was already loose pre-refactor but is unambiguously wrong now that Reddit's only path runs through it.
**Fix:** Drop the "(non-Reddit)" qualifier, e.g. "applied to every static HTTP fetch request in this package, Reddit's old.reddit.com strategy included."

## Verdict

APPROVE
Plan cards, shared decisions, and cross-batch contracts are all faithfully implemented with strong test coverage; only two comment-accuracy NITs found.
MILL_REVIEW_END
