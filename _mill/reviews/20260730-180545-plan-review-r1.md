MILL_REVIEW_BEGIN
# Review: prowler: site-adapter mechanism + github-repo-explorer skill (Claude reading the web) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-07-30
```

## Findings

### [BLOCKING] Card 3 Context omits fetcher.go
**Location:** batch 01-site-adapters, Card 3
**Issue:** Requirements spell out `Fetch(ctx context.Context, f fetcher, url string) (string, bool)`, referencing the `fetcher` type, but Context lists only fetch.go/htmltext.go/headers.go — fetcher.go (where `fetcher` is defined) is absent. Card 4's near-identical signature correctly includes fetcher.go, exposing this as an omission rather than a deliberate choice.
**Fix:** Add `plugins/prowler/fetcher.go` to Card 3's Context.

### [BLOCKING] Card 4 Context omits htmltext.go
**Location:** batch 01-site-adapters, Card 4
**Issue:** Requirements explicitly say "Reuse `htmlToText` from `htmltext.go` for comment/post HTML," naming both the function and its file, but htmltext.go is not in Card 4's Context (adapter.go, reddit.go, fetcher.go, fetch.go only).
**Fix:** Add `plugins/prowler/htmltext.go` to Card 4's Context.

### [BLOCKING] Card 5 Context omits fetch_test.go
**Location:** batch 01-site-adapters, Card 5
**Issue:** Requirements instruct "reusing the `stubResponses`/`htmlResponse` helpers from `fetch_test.go` and the `redditLikeHTMLWithComments`-style fixture pattern" — all three identifiers live in fetch_test.go, which is absent from Card 5's Context (reddit.go, fetch.go only).
**Fix:** Add `plugins/prowler/fetch_test.go` to Card 5's Context.

### [BLOCKING] Card 6 Context omits reddit_test.go
**Location:** batch 01-site-adapters, Card 6
**Issue:** Requirements say "Ensure any helper the deleted assertions used (`newTestResponse`) is either still defined here or in reddit_test.go" — `newTestResponse` is defined in reddit_test.go, which is not in Card 6's Context (reddit.go, adapter.go, fetch.go) or Edits (fetch_test.go only).
**Fix:** Add `plugins/prowler/reddit_test.go` to Card 6's Context.

### [NIT] Batch 1 leaves the package non-compiling for 3 of its 8 card-commits
**Location:** batch 01-site-adapters, Cards 1–4
**Issue:** Card 1's `defaultAdapters()` references `redditAdapter{}`/`hackerNewsAdapter{}` before either type exists (added in Cards 3 and 4), so the package fails to build across the Card 1/2/3 commits. This is called out in Card 1's own text as expected, but it contradicts the "each card small and self-contained" atomicity criterion and complicates bisection.
**Fix:** Either note explicitly that Cards 1–4 must land as one squashed unit before `go build` is expected to succeed, or restructure so `defaultAdapters()` is added last (its own card) after the concrete adapter types exist.

### [NIT] Circular deferred decision on deleting `newTestResponse`
**Location:** batch 01-site-adapters, Cards 5 and 6
**Issue:** Card 5 (executed first) is told to keep `newTestResponse` only "if no longer referenced ... check fetch_test.go card 6 first" — but Card 6 hasn't run yet at that point, and per both cards' own text neither new test actually uses `newTestResponse` (both point to `stubResponses`/`htmlResponse` instead), so it is very likely dead code either way. Harmless for compilation (Go doesn't flag unused funcs) but the instruction is confusing busywork.
**Fix:** State plainly in Card 5 that `newTestResponse` is deleted (both current call sites are removed by Card 5's own `TestFetchReddit` deletion, and neither replacement test needs it).

### [NIT] Card 12 Context is empty but needs the two new skills' descriptions
**Location:** batch 02-skills-and-docs, Card 12
**Issue:** Card 12 adds INDEX.md rows with "its description"/"its helper description" for github-repo-explorer and distill-subagent, but neither new SKILL.md (created in Cards 10 and 9) is listed in Card 12's Context, risking a paraphrase that drifts from the actual frontmatter `description:` text.
**Fix:** Add `plugins/prowler/skills/distill-subagent/SKILL.md` and `plugins/prowler/skills/github-repo-explorer/SKILL.md` to Card 12's Context.

## Verdict

REQUEST_CHANGES
Four Context-completeness gaps (Cards 3, 4, 5, 6) force cold-start exploration; decisions/DAG/sequencing otherwise sound.
MILL_REVIEW_END
