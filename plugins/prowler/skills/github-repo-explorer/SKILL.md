---
name: github-repo-explorer
description: Browse a GitHub repo's file tree and read files via the gh CLI, without cloning
argument-hint: "<owner/repo> [path] [question]"
---

This skill browses a public or accessible-to-you GitHub repository's file tree and reads individual files through the `gh` CLI, without ever cloning the repository.

**Hard prerequisite:** `gh` must be installed and authenticated (`gh auth status`). There is no fallback path — if `gh` is missing or unauthenticated, report that and stop.

**Resolve the default branch first:** `gh api repos/{owner}/{repo} --jq .default_branch`.

**List the full recursive tree in one call:** `gh api "repos/{owner}/{repo}/git/trees/{branch}?recursive=1" --jq '.tree[].path'`.

**Check for truncation before trusting the recursive listing:** very large repos hit GitHub's API cap on the recursive tree, which the response marks with `"truncated": true` — check it explicitly with `gh api "repos/{owner}/{repo}/git/trees/{branch}?recursive=1" --jq .truncated`.

**When truncated is true, fall back to non-recursive per-directory calls:** `gh api "repos/{owner}/{repo}/git/trees/{branch}"` for the root, then descend into each subdirectory's tree SHA the same way, so a large-repo browse is never silently partial.

**Read a file's content:** `gh api "repos/{owner}/{repo}/contents/{path}" --jq .content | base64 -d`.

**Lighter alternative for public files:** `https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}` avoids the base64 decode step.

**Before reading many files, load the `distill-subagent` skill by name** (`prowler:distill-subagent`) via the Skill tool and apply its rule, so a broad repo browse does not bloat the caller's context.
