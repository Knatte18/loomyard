MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
verdict: APPROVE
reviewer_model: fable
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [NOTE] Cache file schema left implicit
**Section:** § token-resolution-and-cache
**Issue:** Path, TTL, permissions, and atomic-write mechanics are fully specified, but the `credentials.json` field shape (token + resolved-at timestamp, presumably) is never stated.
**Fix:** Let the plan writer pin the two-field schema explicitly; no discussion change required.

### [NOTE] 401 replay with an env-var token re-resolves to the same value
**Section:** § token-resolution-and-cache
**Issue:** When the token came from `GH_TOKEN`/`GITHUB_TOKEN`, a 401 still invalidates the cache and replays with the identical env value — harmless and bounded (second 401 returns unchanged), but the interaction of "env always wins" with the invalidation rule is unstated.
**Fix:** Plan may either accept the one redundant replay or skip replay when the source was an env var; either is fine, just say which.

## Verdict

APPROVE
Every source citation verified accurate; scope, boundaries, failure modes, and testing are fully specified with no blocking gaps.
MILL_REVIEW_END
