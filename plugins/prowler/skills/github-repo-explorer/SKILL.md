---
name: github-repo-explorer
description: Browse a GitHub repo's file tree and read files via the gh CLI, without cloning
argument-hint: "<owner/repo> [path] [question]"
---

This skill browses a public or accessible-to-you GitHub repository's file tree and reads individual files through the `gh` CLI, without ever cloning the repository.

**Hard prerequisite:** `gh` must be installed and authenticated (`gh auth status`).
There is no fallback path — if `gh` is missing or unauthenticated, report that and stop.

**List the repository's file tree with one script call, instead of composing the `gh api` walk by hand:**

1. Resolve the script's absolute path now, while `${CLAUDE_SKILL_DIR}` is still set — a dispatched subagent won't have it: `TREE_SH="$(cd "${CLAUDE_SKILL_DIR}/../../scripts" && pwd)/github-tree.sh"`.
2. Run it: `paths=$(bash "$TREE_SH" <owner/repo>)` for a whole-repo listing, or `paths=$(bash "$TREE_SH" <owner/repo> <path>)` to scope it to one repo-relative directory.
   Scoping is worth reaching for whenever the caller already knows which subtree it needs.
   The script handles GitHub's recursive-tree truncation cap internally, so the listing it returns is never silently partial.

**Check the exit code, always:** on failure the script prints exactly one line to stderr, emits nothing on stdout, and exits non-zero — an empty `$paths` on success (a genuinely empty repository) and an empty `$paths` on failure look identical unless the exit code is checked, so the exit code must be checked and any failure reported rather than read as an empty repository.

**Deciding whether the skill's second argument is a `path` or the `question`:** this skill's own argument hint is `<owner/repo> [path] [question]`, and a second token is forwarded to the script as `<path>` only when it looks like a repo-relative directory path — matching the accepted set `^[A-Za-z0-9._/-]+$` and containing no whitespace.
Anything else, a natural-language question in particular, is the question, and is never passed to the script.

**Read a file's content:** `gh api "repos/{owner}/{repo}/contents/{path}" --jq .content | base64 -d`.

**Lighter alternative for public files:** `https://raw.githubusercontent.com/{owner}/{repo}/HEAD/{path}` avoids the base64 decode step.

**Before reading many files, load the `distill-subagent` skill by name** (`prowler:distill-subagent`) via the Skill tool and apply its rule, so a broad repo browse does not bloat the caller's context.
