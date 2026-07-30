MILL_REVIEW_BEGIN
# Review: prowler: site-adapter mechanism + github-repo-explorer skill (Claude reading the web) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-07-30
```

## Findings

### [BLOCKING] Card 3 instructs keeping an import that becomes unused
**Location:** Batch 1 / Card 3 (`reddit.go`)
**Issue:** Requirements say to keep imports `context, regexp, strings` after deleting the JSON path, but every `strings.*` call site in the current file (`toRedditJsonUrl`'s `TrimSuffix`, `formatRedditPost`/`formatRedditSubreddit`'s `Builder`/`TrimRight`, `selftextSnippet`'s `ReplaceAll`) lives in functions this same card deletes; the surviving code (`redditHostPattern`/`redditHostReplace` via `regexp`, `toOldRedditURL`, `redditAdapter`) never calls `strings`. An unused import is a Go compile error, so following this instruction literally breaks the build even after Card 4 closes the intentional non-compiling gap.
**Fix:** Drop `strings` from Card 3's retained-imports list; the final `reddit.go` needs only `context` and `regexp`.

### [NIT] `maxTopComments`'s doc comment references a function this batch deletes
**Location:** Batch 1 / Card 3 (`reddit.go`)
**Issue:** `maxTopComments`'s existing godoc ("bounds how many top-level comments formatRedditPost includes") names `formatRedditPost`, which Card 3 deletes; Decision "comment cleanup lands with the code" and Card 3's Requirements enumerate the file-header and `redditHostPattern`/`redditHostReplace` comments for rewrite but omit this one, so it survives stale (the const's actual new consumer is Card 4's HN adapter).
**Fix:** Add `maxTopComments`'s doc comment to Card 3's (or Card 4's) rewrite list, describing it as shared bound now reused by the HN adapter.

### [NIT] GitHub tree-API truncation not addressed in github-repo-explorer
**Location:** Batch 2 / Card 10 (`skills/github-repo-explorer/SKILL.md`)
**Issue:** The documented recipe `gh api "repos/{owner}/{repo}/git/trees/{branch}?recursive=1"` silently returns `"truncated": true` with a partial tree for very large repos (GitHub's own API limit), and the skill's requirements don't mention detecting or handling that case, so a large-repo browse could look complete when it isn't.
**Fix:** Add a line noting `.truncated` should be checked and, if true, falling back to per-directory (non-recursive) tree calls.

## Verdict

REQUEST_CHANGES
Card 3's retained-import list leaves a dead `strings` import that breaks compilation; fix before merge.
MILL_REVIEW_END
