MILL_REVIEW_BEGIN
# Review: prowler: site-adapter mechanism + github-repo-explorer skill (Claude reading the web)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-30
```

## Findings

### [NOTE] Stale JSON-referencing comments on retained code
**Section:** Scope (In) / Technical context (reddit.go, fetch.go)
**Issue:** Scope enumerates deletions but not comment cleanup on kept code — `fetch.go`'s file header + `fetchPage` docstring ("Reddit JSON special-case first"), `fetchOldRedditHTML`'s "JSON API unreachable / falls back to the original JSON-API error" doc, `redditHostPattern`/`redditHostReplace`'s ".json special-case" comments, and `errorResult`'s "shared by both fetchPage and fetchReddit" all misdescribe behavior once the JSON path is gone.
**Fix:** Have the plan explicitly include rewriting these retained comments to the old.reddit-only strategy in the same commit (repo docs-in-same-commit rule).

### [NOTE] maxTopComments reuse-vs-own-bound is ambiguous
**Section:** Decisions (hackernews-adapter) vs Technical context (reddit.go)
**Issue:** The HN decision says bound comments "the way Reddit bounds top comments via `maxTopComments`," while the technical-context says delete `maxTopComments` "if unused after HN reuses its own bound" — unclear whether HN reuses the shared const or defines its own.
**Fix:** State one choice (reuse the existing `maxTopComments` const, or define an HN-local bound and delete `maxTopComments`) so the plan writer isn't guessing.

### [NOTE] Cross-skill "load" convention not verifiable in this worktree
**Section:** Decisions (distill-subagent-skill)
**Issue:** The rationale rests on "this repo's own convention, where mill's conversation/testing/... guidance skills other skills 'load'," but no mill skills exist under this worktree's `plugins/` to verify, and the concrete mechanism by which one prowler `SKILL.md` "loads" another (name-discovery vs path reference) isn't specified.
**Fix:** Confirm the load-by-name mechanism works for a codeless skill and specify how `prowler`/`github-repo-explorer` reference `distill-subagent`.

## Verdict

APPROVE
Thorough, source-grounded discussion; three record-only NOTEs, no blocking gaps.
MILL_REVIEW_END
