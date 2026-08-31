---
name: github-repo-explorer
description: Search code across GitHub repos, browse a repo's file tree, and read files via the gh CLI, without cloning
argument-hint: "<owner/repo>... [path | search term] [question]"
---

This skill searches code across, and browses the file tree of, one or more public or accessible-to-you GitHub repositories, and reads individual files, through the `gh` CLI, without ever cloning a repository.

**Hard prerequisite:** `gh` must be installed and authenticated (`gh auth status`).
There is no fallback path — if `gh` is missing or unauthenticated, report that and stop.

**List a repository's file tree with one script call, instead of composing the `gh api` walk by hand:**

1. Resolve the script's absolute path now, while `${CLAUDE_SKILL_DIR}` is still set — a dispatched subagent won't have it: `TREE_SH="$(cd "${CLAUDE_SKILL_DIR}/../../scripts" && pwd)/github-tree.sh"`.
2. Run it: `paths=$(bash "$TREE_SH" <owner/repo>)` for a whole-repo listing, or `paths=$(bash "$TREE_SH" <owner/repo> <path>)` to scope it to one repo-relative directory.
   Scoping is worth reaching for whenever the caller already knows which subtree it needs.
   The script handles GitHub's recursive-tree truncation cap internally, so the listing it returns is never silently partial.

**Check the exit code, always:** on failure the script prints exactly one line to stderr, emits nothing on stdout, and exits non-zero — an empty `$paths` on success (a genuinely empty repository) and an empty `$paths` on failure look identical unless the exit code is checked, so the exit code must be checked and any failure reported rather than read as an empty repository.

**Search code across one or more repos with one script call, instead of composing the `gh api` call by hand for each repo:**

1. Resolve the script's absolute path now, while `${CLAUDE_SKILL_DIR}` is still set — a dispatched subagent won't have it: `SEARCH_SH="$(cd "${CLAUDE_SKILL_DIR}/../../scripts" && pwd)/github-code-search.sh"`.
2. Run it: `hits=$(bash "$SEARCH_SH" <query> <owner/repo> [<owner/repo>...])` — a query followed by one or more repo refs, searched in a single invocation.
   Each line of `$hits` is a tab-separated three-field record: `<owner>/<repo>\t<path>\t<snippet>`.
   A hit with no matching fragment still emits a record — the third field is simply empty, never a dropped line.
   At most 10 distinct repo refs are accepted per invocation, because GitHub's code-search API bucket allows only 10 requests per minute, and a larger sweep would burn that bucket for every other caller sharing the token.

**Check the exit code here too, always:** on failure the script prints exactly one line to stderr, emits nothing on stdout, and exits non-zero — an empty `$hits` on success (genuinely no matches) and an empty `$hits` on failure look identical unless the exit code is checked, so the exit code must be checked and any failure reported rather than read as "no matches".

Two rejections a caller is most likely to hit by accident:
- A query containing the `repo:` qualifier is refused: the script appends its own `repo:` qualifier per repo, and GitHub keeps only the last one, so a caller-supplied `repo:` would be silently discarded with no diagnostic if the script let it through.
  If an explicit qualifier is genuinely needed, use a raw `gh api -X GET search/code -f q=...` call instead.
- An empty or whitespace-only query is refused: arity alone doesn't catch it, and live behaviour makes it dangerous — a query carrying only a repo qualifier still returns hits, so an empty query would silently degrade into a worse version of the tree listing `github-tree.sh` already provides for free, while burning a request from the scarcest bucket in the plugin.

A repo with more matches than fit on one page still exits 0, with its returned records intact; the script writes a stderr note naming the true total, so a capped listing is never mistaken for a complete one.

**Routing rule — search versus tree, as a rule rather than a preference:**
- A known term, import, symbol, or dependency name anywhere in a repo routes to the search script.
- Understanding a repo's overall structure or layout routes to the tree script plus selective reads.
- A list of repos to relevance-map against one trait routes to the search script, run once over the whole list.
- Deep single-repo work is out of this skill's remit — that goes through a local clone instead, not through repeated calls to either script.

**Deciding what the trailing arguments mean:** this skill's own argument hint is `<owner/repo>... [path | search term] [question]`.
Working out where the repo list ends, and what the remainder means, is a two-part rule.

Part one decides where the repo list ends.
A repo-shaped token is a leading whitespace-separated token matching `^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$` — the same `<owner>/<repo>` predicate `github-tree.sh` already enforces, reused rather than a second, looser one.
Take the maximal leading run of repo-shaped tokens, then resolve the run by its length.
A run of length one is one repo, and the remainder is the second slot, decided by part two below.
A run of length three or more is always a repo list — nobody writes one repo followed by two paths, and the tree script takes at most one path anyway.
A run of length exactly two is genuinely ambiguous, because a repo-relative path is repo-shaped too;
break it on phrasing, where a sweep marker in the request ("which of these", "any of", "across", "compare", "both", or an explicit enumeration of the repos in prose) makes it a two-repo list, and otherwise the second token is a path against the first repo.
Two explicit overrides settle it for certain: a trailing slash on the second token always forces path, and naming the repos in prose always forces sweep.
The repo list never extends past that run, and no terminator token is introduced.

Part two decides what the remainder is.
With two or more repo refs, the remainder is never a path — a repo-relative path has no meaning across a repo list — so it is the search term or question, and the invocation routes to the search script.
With exactly one repo ref, the remainder is a path for the tree script only when the phrasing asks for structure ("what's in", "list", "show me the tree", a trailing slash), and is a search term otherwise.
When phrasing settles it either way, phrasing wins over token shape.
When neither phrasing nor shape settles the single-repo case, route to the search script: a mis-routed search costs one request and still answers whether the token appears in the repo and where, while a mis-routed tree call returns the whole file list, answers nothing about the token, and costs the caller a second turn to recover.

**Read a file's content:** `gh api "repos/{owner}/{repo}/contents/{path}" --jq .content | base64 -d`.

**Lighter alternative for public files:** `https://raw.githubusercontent.com/{owner}/{repo}/HEAD/{path}` avoids the base64 decode step.

**Before reading many files, load the `distill-subagent` skill by name** (`prowler:distill-subagent`) via the Skill tool and apply its rule, so a broad repo browse does not bloat the caller's context.
