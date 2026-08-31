# prowler

prowler is a Go-native replacement for Millhouse's `weblens` skill: it fetches pages the built-in `WebFetch` tool cannot read — bot-blocked sites, paywalls, JS-rendered content, and Reddit posts — and returns the result as readable markdown.
The generic cascade drives a real headless browser plus Mozilla-Readability-style extraction;
Reddit is read from structured sources instead — see "Site adapters" below.
It keeps the same any-repo/any-session reach as weblens because it ships as an installable Claude Code plugin, not a repo-scoped project skill.

## Install

1. `/plugin marketplace add <this LoomYard repo>`
2. `/plugin install prowler@loomyard`

Once installed, the `prowler` skill is available in every Claude Code session, the same way weblens is today.

## Build-on-first-run

prowler ships no compiled binary — LoomYard's `.gitignore` bans committing binaries.
Instead, `scripts/run.sh` builds the nested Go module into `bin/prowler` (or `bin/prowler.exe` on Windows) the first time it is invoked, under a lock, and reuses that binary on every later call.
This means:

- A Go toolchain must be on `PATH` at first use.
  The operator's Linux and Windows machines are Go dev boxes, so this is normally already satisfied.
- The first invocation is slower (it compiles chromedp, go-readability, and goquery);
  every later invocation just execs the cached binary.
- If a build is ever interrupted in a way that leaves a stuck lock (e.g. a hard-killed process), the wrapper reclaims a lock older than ~300 seconds automatically.
  If you ever need to clear one manually: `rm -rf plugins/prowler/bin/.build.lock`.

## `github-tree.sh`: one-call repo tree listing

`github-tree.sh` lists a GitHub repository's file paths, optionally scoped to one directory, in a single invocation.
It exists because the `github-repo-explorer` skill previously had the model execute a branching, potentially recursive `gh api` walk one call per turn — resolve the default branch, list the recursive tree, check truncation, then fall back to non-recursive per-directory calls — and that walk contains no decision a model actually needs to make.

An untruncated listing costs exactly one `gh api` call,
and even a repository large enough to trigger GitHub's recursive-tree truncation cap is one agent turn regardless of how many API calls the internal fallback makes.
Its only runtime dependency is `gh`, already a hard prerequisite of the skill: every JSON field is extracted through `gh api --jq`,
and no system `jq` is ever invoked at run time.
Unlike `run.sh`, it has no build step and no lock, since there is nothing to compile.
Its offline test harness (`github-tree-selftest.sh`) carries the one extra dependency of system `jq`, which the harness checks for up front.

## `github-code-search.sh`: one-call cross-repo code search

`github-code-search.sh` runs one GitHub code search query across one or more repositories in a single invocation.
It replaces an N-call LLM-driven "search each repo, one at a time, retyping the query each time" loop with a single script call the model no longer has to compose turn by turn.

The contract: a query followed by one or more `<owner>/<repo>` refs, and one tab-separated record per matching file on stdout — `<owner>/<repo>\t<path>\t<snippet>`.
Every record across every repo is buffered in memory and printed only once the whole sweep has succeeded, so a failure partway through never leaves a partial prefix on stdout for a caller to mistake for a complete (if short) result set.

The rate-limit budget is what fixes the repo cap: each repo costs one preflight call against the 5000-per-hour core bucket, plus one search call against the 10-per-minute search bucket, and it is the ten-per-minute search bucket that caps a single invocation at 10 distinct repos.

Three GitHub API quirks shape the contract:

- Repeated `repo:` qualifiers do not combine — the last one wins and the earlier ones are silently discarded — which is why the script issues one search call per repo rather than one call for the whole sweep.
  For the same reason, a caller-supplied query containing `repo:` is refused outright;
  use a raw `gh api -X GET search/code -f q=...` call instead if an explicit qualifier is genuinely needed.
- A nonexistent repo answers 200 with a zero total, indistinguishable from a real repo with no matches, which is why the script runs a preflight call against every repo before running any search call.
- A partial result set arrives as a 200 carrying an incomplete-results flag, which the script treats as a hard failure rather than a silent partial success.

Results are capped at one page per repo here, and at 1000 by the API regardless;
a repo with more matches than fit on one page still exits 0, with a stderr note naming the true total so a capped listing is never mistaken for a complete one.

As with `github-tree.sh`, its only runtime dependency is `gh`: every field is extracted through `gh api --jq` (gh's embedded gojq), and no system `jq` is ever invoked at run time.
Its offline test harness (`github-code-search-selftest.sh`) carries the one extra dependency of system `jq`, which the harness checks for up front.

## Runtime prerequisite: Chrome/Chromium

The headless-browser fallback (used when a page is bot-blocked or JS-rendered and static extraction alone isn't enough) needs a local Chrome or Chromium install. prowler discovers it via the `CHROME_PATH` environment variable first, then a platform-specific candidate list (matching weblens' own discovery).
If no browser is found, the browser fallback is simply skipped — the run still returns whatever the static-extraction path produced, with a note;
it is never a hard failure of the whole invocation.

## Reddit credentials: optional, upgrade the read

Reddit needs no setup at all to read: with no credentials configured, prowler reads Reddit's unauthenticated `.rss` feed, which needs no app registration and works for every reader out of the box.
That zero-setup path exists because Reddit's November 2025 Responsible Builder Policy puts new app registrations behind a manual review that routinely rejects small personal projects.
The `.rss` feed is paced at roughly one request per 60 seconds per IP, so a burst of several Reddit URLs takes minutes rather than seconds.

Configuring credentials upgrades Reddit reads to the richer, authenticated OAuth API instead — scores, one level of nested replies, a fuller comment page, and a 100-requests-per-minute budget instead of one per 60 seconds.
To use it, register a "script"-type app at `https://www.reddit.com/prefs/apps` and export `PROWLER_REDDIT_CLIENT_ID` and `PROWLER_REDDIT_CLIENT_SECRET`.
The credentials are read from the environment only and are never written to a config file.
`PROWLER_REDDIT_USER_AGENT` optionally overrides the descriptive `prowler/1.0` API User-Agent prowler sends by default.

## Site adapters

prowler routes each fetch through an ordered registry of site adapters before falling back to the generic static-fetch/Readability/browser cascade.
Each adapter matches a URL family and provides a higher-fidelity strategy for that site, falling through to the generic cascade when it cannot handle the page.
Two adapters are registered today.
Reddit tries, in order, the authenticated OAuth API when credentials are configured, then the unauthenticated `.rss` feed, then reports a definitive error naming why each attempted tier failed — a Reddit URL never reaches the generic cascade's headless-browser fallback, since a second headless request against a solvable-looking Reddit challenge has been measured to escalate it into a hard IP-level block rather than recover it.
Hacker News matches `item?id=N` discussion pages and reads them from the community-run Algolia JSON API instead of scraping HN's own HTML.

## License

Apache-2.0, per the `license` field in `.claude-plugin/plugin.json`.
See the repository-root `LICENSE` file, which covers this plugin along with the rest of LoomYard.
