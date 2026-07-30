# Discussion: prowler: site-adapter mechanism + github-repo-explorer skill (Claude reading the web)

```yaml
task: 'prowler: site-adapter mechanism + github-repo-explorer skill (Claude reading the web)'
slug: prowler-web-reading
status: discussing
parent: main
```

## Problem

prowler needs to read things on the web that Claude can't read out of the box. Two concrete gaps, both instances of the same underlying problem — Claude being unable to read something on the web without help — so they land as one task.

**(1) Site adapters.** prowler's fetch cascade (`plugins/prowler/fetch.go`) hardcodes a single site special-case: `fetchPage` begins with `if isRedditUrl(url) { fetchReddit(...) }` before falling through to the generic HTML-extraction cascade. Every additional site that needs special handling would mean another hardcoded `if` branch surgically inserted into `fetchPage`. We want to generalize that one special-case into a pluggable **site-adapter** mechanism (interface + registry), so adding a site means dropping in an adapter file, not editing the cascade.

**(2) github-repo-explorer skill.** Claude cannot browse a GitHub repo's file tree or read arbitrary files without cloning. A codeless `SKILL.md` wrapping the `gh` CLI closes that gap: list a repo's full tree in one call, read individual files, resolve the default branch — no clone, no build step.

**Why now / a correction that reshaped the task.** The brief framed the Reddit special-case as "pull structured data from Reddit's own JSON API instead of scraping HTML." During discussion the operator flagged that the JSON approach **does not actually work on Reddit anymore** — the request must go to `old.reddit.com` instead. Verified empirically from this environment: `https://www.reddit.com/r/golang.json` returns **HTTP 403** (a ~190 KB bot-block HTML page, not JSON) under both the `prowler-reader/1.0` UA and a browser UA; `https://old.reddit.com/r/golang` returns **HTTP 200** with ~135 KB of real HTML (after a trailing-slash 301 that Go's HTTP client follows automatically). So today every Reddit fetch pays a wasted 403 round-trip on the JSON API before the existing `fetchOldRedditHTML` fallback (`fetch.go:152`) actually returns content. This means the thing worth generalizing was never "the JSON trick" — it is the more general notion of a **per-site override strategy**, of which Reddit's *working* strategy (rewrite to `old.reddit.com`, crude body-text strip that keeps comments) is the first instance. The generalization is still the right call; the honest version of it fixes Reddit to use only its working path instead of enshrining the dead JSON path as the framework's flagship example.

## Scope

**In:**

- A `siteAdapter` interface + an ordered registry, replacing the hardcoded `if isRedditUrl(url)` branch in `fetchPage`.
- Migrate Reddit to the first registered adapter, using **only** its working strategy (`old.reddit.com` HTML + crude body-text strip). Delete the now-dead JSON path (`toRedditJsonUrl`, `formatRedditPost`, `formatRedditSubreddit`, `selftextSnippet`, the `redditThing`/`redditData` structs, the JSON-related regex/UA, and the content-type/array-vs-object dispatch in `fetchReddit`).
- A **second** adapter as proof of concept: **Hacker News**, using the Algolia API (`https://hn.algolia.com/api/v1/items/{id}`), matching only HN item pages (`news.ycombinator.com/item?id=N`).
- A new codeless skill **`github-repo-explorer`** (`plugins/prowler/skills/github-repo-explorer/SKILL.md`) wrapping `gh`: resolve default branch, list recursive tree, read files.
- A new guidance skill **`distill-subagent`** (`plugins/prowler/skills/distill-subagent/SKILL.md`) holding the provider-agnostic "offload a noisy/bulky read to a cheap subagent that returns only a distilled answer" judgment rule + subagent contract.
- Refactor the existing `plugins/prowler/skills/prowler/SKILL.md` to **load `distill-subagent`** instead of inlining the Haiku-wrapper rule, and to soften its hardcoded "Haiku subagent" / `model: haiku` wording to "cheap tier, currently Haiku."
- Docs in the same commit: add both new skills to `plugins/prowler/skills/INDEX.md`; note the site-adapter mechanism + adapter list in `plugins/prowler/README.md`; bump `plugins/prowler/.claude-plugin/plugin.json` version `1.0.0` → `1.1.0`.
- Tests: unit tests for the HN adapter and the registry/fall-through, and updates to `reddit_test.go` (drop JSON-path tests, keep/retarget the `old.reddit` behavior).

**Out:**

- No third adapter, no "try each matching adapter in order" chaining (first-match-only; YAGNI — no site overlaps today).
- No unauthenticated `curl` fallback in `github-repo-explorer` — `gh` is a required, authenticated prerequisite.
- No Go code or build step for either skill — both are `SKILL.md` only.
- No reuse of lyx's `internal/githubclient` — it is a lyx-internal Go module; the skill is codeless and prowler is a separate `go.mod` outside lyx, so neither can import it.
- No change to the generic HTML cascade, the browser fallback, `decodeContentEncoding`, `writeOutput`, `runAll`, or `run.sh`/`selftest.sh` (selftest only exercises the build; it does no live Reddit fetch, so it is unaffected).
- No handling of HN front page / `/newest` / user pages, or Reddit search pages, as adapters — these fall through to the generic cascade.
- Reddit's JSON API is not merely demoted or reordered — it is removed.

## Decisions

### adapter-interface

- Decision: `type siteAdapter interface { Matches(url string) bool; Fetch(ctx context.Context, f fetcher, url string) (out string, handled bool) }`. `Matches` is the cheap URL pre-filter (today's `isRedditUrl`); `Fetch` fully owns the site-specific strategy and returns `(out, handled)` exactly as `fetchReddit` does today — `handled=false` means "matched the URL but produced nothing usable, fall through to the generic cascade."
- Rationale: faithful, minimal migration of the existing `isRedditUrl` / `fetchReddit` split. The Reddit correction reinforces it: an adapter must own an *arbitrary* multi-step strategy (host rewrite, alternate API, internal fallback chain), not just a URL-suffix trick — a single interface method that hands the adapter the injectable `fetcher` seam gives it exactly that freedom while staying unit-testable with stubs.
- Rejected: a single `Fetch` method where a non-match also returns `handled=false` (loses the cheap pre-filter, conflates "didn't match" with "matched but unparseable"); a struct-of-funcs like `fetcher` (adapters are stateless behavior — an interface fits better than function fields).

### adapter-registry

- Decision: adapters live as an ordered `adapters []siteAdapter` field on the `fetcher` struct, populated in `newFetcher()`. `fetchPage` iterates `f.adapters`, calling `Matches` then `Fetch` on the first match.
- Rationale: stays true to the existing injection seam. `do` and `browser` are already injected fields on `fetcher`; adapters join them, so tests construct a `fetcher` with a custom adapter set and zero package-level global state — consistent with the codebase's deliberate avoidance of globals/`init()` for testability.
- Rejected: a package-level `var adapters []siteAdapter` registered via each file's `init()` (idiomatic Go "registry," but reintroduces global mutable state + init-ordering the codebase avoids, and is harder to isolate in tests); a `defaultAdapters()` slice threaded as a new parameter through `fetchPage`/`runAll` (no globals, but adds a parameter to the call chain the `fetcher` field avoids).

### adapter-fallthrough

- Decision: only the **first** matching adapter runs. If its `Fetch` returns `handled=false`, `fetchPage` falls straight through to the generic HTML cascade (static GET → Readability → body-text strip → browser) — it does **not** try a next adapter.
- Rationale: preserves today's exact behavior (a Reddit search page that returns non-JSON falls to the static path, not to another adapter). No two adapters overlap on any URL, so multi-adapter chaining is speculative.
- Rejected: try each matching adapter until one returns `handled=true` (YAGNI — no overlap exists).

### reddit-strategy

- Decision: the Reddit adapter uses **only** the `old.reddit.com` HTML strategy: rewrite the host to `old.reddit.com` (existing `toOldRedditURL`), fetch with the browser-impersonation `defaultHeaders()`, strip to body text with `stripToBodyText` (crude strip, keeps comments — Readability is deliberately skipped because it discards the lower-scoring comment block). The JSON API path is deleted entirely.
- Rationale: the JSON API is bot-blocked in practice (operator's real-world experience; corroborated by the 403 observed here). Keeping a reliably-403ing path as the framework's flagship adapter is misleading and pays a wasted round-trip on every Reddit fetch. Deleting it makes Reddit the honest "alternate-HTML-host" exemplar, contrasting with HN's "JSON API" exemplar — two genuinely different strategies, which is what proves the interface generalizes.
- Rejected: keep JSON-first with `old.reddit` fallback (current behavior — flagship example never fires, wasted latency); flip to `old.reddit`-first with JSON as secondary (retains all JSON code + two paths for a path that rarely fires; the operator explicitly reported JSON doesn't work). Caveat acknowledged and accepted: this environment is a datacenter IP that Reddit 403s more aggressively than a residential IP, so the JSON API may work from some networks — but the operator's direct experience plus the block observed here make "unreliable in practice" the correct planning assumption, and the generic cascade + browser fallback remain available if `old.reddit` itself ever fails.

### hackernews-adapter

- Decision: the second adapter is Hacker News. `Matches` accepts only `news.ycombinator.com/item?id=N` URLs (any host/scheme variant). `Fetch` extracts the numeric `id` from the query string, calls the Algolia API `https://hn.algolia.com/api/v1/items/{id}` (verified reachable: HTTP 200 `application/json`), and formats the result into markdown — title/points/author header, the post text/URL, and a bounded set of top-level comments from the returned comment tree (bound it the way Reddit bounds top comments via `maxTopComments`). A missing/invalid id, a non-2xx API response, or an unparseable body returns `handled=false` so the URL falls through to the generic cascade. Non-item HN URLs (front page, `/newest`, `/user`) do not match.
- Rationale: strongest POC — HN's clean JSON API genuinely works (unlike Reddit's), so the two adapters demonstrate two *different* working strategies rather than two copies of one trick. Matching only item pages mirrors Reddit's focused post/subreddit handling without over-reaching into listings the generic cascade already renders acceptably.
- Rejected: Lobsters `.json` (a near-identical suffix trick to old Reddit — proves less about generalization); Wikipedia REST API (the HTML cascade already reads Wikipedia fine); handling HN listings/front page (YAGNI — cascade suffices).

### github-repo-explorer-skill

- Decision: a codeless `SKILL.md` at `plugins/prowler/skills/github-repo-explorer/SKILL.md` wrapping `gh`, documenting exactly the operations Claude needs to explore a repo without cloning: (a) resolve default branch — `gh api repos/{owner}/{repo} --jq .default_branch`; (b) list the full recursive tree in one call — `gh api "repos/{owner}/{repo}/git/trees/{branch}?recursive=1"`; (c) read a file — `gh api repos/{owner}/{repo}/contents/{path} --jq .content | base64 -d`, with `https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}` as a lighter alternative for public files. `gh` must be installed and authenticated (stated prerequisite; no code fallback). The skill loads `distill-subagent` for the context-bloat decision.
- Rationale: the operation set is need-driven — precisely what's required to browse a tree and read files. Requiring authenticated `gh` mirrors how prowler's README states its Go/Chrome prerequisites and adds no new dependency: lyx already leans on `gh` (`internal/githubclient/token.go:81` shells out to `gh auth token`), so it is a de-facto given in this environment. The raw-URL alternative avoids the `gh api` + base64 dance for public repos.
- Rejected: reusing lyx's `internal/githubclient` (impossible — codeless skill + prowler's separate module); an unauthenticated `curl` fallback for public repos (YAGNI, added complexity the brief didn't ask for); dropping default-branch resolution (the brief lists it, and file/tree reads need a branch).

### distill-subagent-skill

- Decision: extract the "wrap the read in a cheap subagent" pattern into its own **guidance skill** `plugins/prowler/skills/distill-subagent/SKILL.md`, invoked by both `prowler` and `github-repo-explorer` ("load the `distill-subagent` skill and apply its rule before deciding how to run this"). It holds only the provider/command-agnostic parts: *when* to wrap (small isolated worker → read inline yourself; general/long-lived thread or expensive tier → wrap); batch all related sources into **one** dispatch; parallelize independent dispatches (never a sequential loop); the subagent returns **only** distilled answers, never raw content, and the caller never dumps raw content to the user or into its own context. Each consuming skill keeps its own command-specific step (`run.sh <url>` for prowler; `gh api …` for github-repo-explorer). The existing `prowler/SKILL.md` is refactored to load this skill rather than inlining the rule.
- Rationale: single source of truth for the wrap rule — matches this repo's own convention, where mill's `conversation`/`testing`/`code-quality`/`linting`/`markdown` are pure guidance skills that other skills "load." A guidance skill invoked by name avoids the `${CLAUDE_SKILL_DIR}/../shared/...` path indirection a shared markdown fragment would need.
- Rejected: inlining the rule in both skills (duplication that can drift — the operator explicitly wants to avoid writing the same instructions twice); a shared markdown fragment referenced by resolved path (keeps it out of the skill list but uses less-idiomatic path indirection); accepting the duplication.

### distill-subagent-naming

- Decision: name the skill `distill-subagent` (function-named), with **Haiku documented as the default cheap tier inside** the rule and in the `model: haiku` run step — worded "dispatch a cheap-tier subagent (currently Haiku)" rather than baking the model into the skill's identity.
- Rationale: cheap, bounded "read something noisy and return a distilled answer" is exactly Haiku's intended job, so Haiku is the correct default — not a smell. But naming the skill `haiku-wrapper` would marry a reusable, model-agnostic pattern to one model's identity, contradicting the repo's stated value that the agent output contract is provider-invariant (CLAUDE.md: "provider-agnostic via engines," "the verdict/output contract is provider-invariant"). Function-naming means a future change of cheap tier is a one-line edit inside the skill — no rename, no caller churn. The "small isolated worker → read inline yourself" escape hatch already covers the rare case where a strong model faces a genuinely hard synthesis and should not delegate.
- Rejected: `haiku-wrapper` (binds the contract to one model); other function names considered and set aside — `offload-read`, `cheap-reader`, `read-and-distill`, `read-delegation` (operator chose `distill-subagent`).

### plugin-versioning-and-docs

- Decision: in the same commit as the code — add `github-repo-explorer` and `distill-subagent` rows to `plugins/prowler/skills/INDEX.md`; document the site-adapter mechanism and the registered adapter list (Reddit, Hacker News) in `plugins/prowler/README.md`; bump `plugins/prowler/.claude-plugin/plugin.json` `version` `1.0.0` → `1.1.0`.
- Rationale: repo rule — a change adding a module/skill or observable behavior lands its docs in the same commit; a new user-facing skill plus a new structural feature is a minor version bump.
- Rejected: INDEX.md-only with no README/version change (under-documents a structural change and two new user-facing skills).

## Technical context

Everything below is in `plugins/prowler/` — a self-contained nested Go module (`go.mod`) that is **not** part of the lyx module, so the main lyx `CONSTRAINTS.md` invariants (hubgeometry, Cobra/CLI, lyxtest) do **not** apply here. `package main` throughout.

Key files and what mill-plan needs to know:

- `fetch.go` — `fetchPage(ctx, f fetcher, url string) string` is the cascade entry point. Line 65's `if isRedditUrl(url) { if out, handled := fetchReddit(...); handled { return out } }` is the exact site-hook being generalized: replace with a loop over `f.adapters` (`for _, a := range f.adapters { if a.Matches(url) { if out, handled := a.Fetch(ctx, f, url); handled { return out }; break } }`), then fall through to the existing static/Readability/body-text/browser cascade unchanged. `fetch.go` also owns `fetchOldRedditHTML` (the old.reddit strategy — moves into / stays callable by the Reddit adapter), `errorResult`, `decodeContentEncoding`, `minUsableTextLen`, `scriptStyleNoscriptBlock`. `stripToBodyText`, `defaultHeaders`, and `httpClient` live in sibling files (`htmltext.go`, `headers.go`, `fetcher`/`main` wiring).
- `fetcher.go` — the `fetcher` struct (injection seam: `do func(*http.Request)(*http.Response,error)`, `browser func(ctx, url)(string,bool)`). Add the `adapters []siteAdapter` field here.
- `main.go` — `newFetcher()` wires `do: httpClient.Do`, `browser: fetchWithBrowser`; add `adapters: defaultAdapters()` (or an inline ordered slice `[]siteAdapter{redditAdapter{}, hackerNewsAdapter{}}`). `runAll` fans out `fetchPage` per URL concurrently and joins with `resultJoiner`; unchanged.
- `reddit.go` — currently holds `isRedditUrl`, `toRedditJsonUrl`, `toOldRedditURL`, `fetchReddit`, the JSON formatters, and the `redditThing`/`redditData` structs. After migration: a `redditAdapter` type with `Matches` (wrapping the existing `redditHostPattern`) and `Fetch` (delegating to the `old.reddit` HTML strategy). Delete `toRedditJsonUrl`, `formatRedditPost`, `formatRedditSubreddit`, `selftextSnippet`, `maxTopComments`/`selftextSnippetLen` if unused after HN reuses its own bound, the two structs, the `prowler-reader/1.0` UA, and the JSON content-type/probe logic. Keep `toOldRedditURL`, `redditHostPattern`.
- Suggested new files: `adapter.go` (the `siteAdapter` interface + `defaultAdapters()`), `hackernews.go` (`hackerNewsAdapter` + Algolia structs + formatter), mirroring the one-file-per-concern layout.
- Skills live under `plugins/prowler/skills/<name>/SKILL.md`; a subdirectory without a `SKILL.md` is not registered as a skill. `INDEX.md` at `skills/` root is the human index. The existing `skills/prowler/SKILL.md` resolves `RUN_SH` from `${CLAUDE_SKILL_DIR}` *before* dispatching a subagent because a dispatched subagent won't have that env var — the same "resolve in the orchestrator, hand the subagent only the distilled task" discipline applies to loading `distill-subagent`.

Empirical findings from discussion (so the plan doesn't re-litigate them):

- `reddit.com/*.json` → HTTP 403 (bot-block HTML, both UAs). `old.reddit.com/r/golang` → 301 (trailing slash) → 200 (real HTML). Go's default client follows the 301.
- `hn.algolia.com/api/v1/items/{id}` → HTTP 200 `application/json` (structured post + nested comment tree). `hacker-news.firebaseio.com/v0/item/{id}.json` also 200 (Algolia preferred — returns the whole thread in one call vs. Firebase's per-id fan-out).
- `news.ycombinator.com/item?id=N` HTML is small and server-rendered but its comments are nested tables Readability mangles — the API path is genuinely higher-fidelity.

## Constraints

- prowler ships **no compiled binary** (LoomYard `.gitignore` bans committed binaries); `scripts/run.sh` builds `bin/prowler` on first run. New adapter files must compile cleanly into that build; no new third-party dependency is needed (HN uses `net/http` + `encoding/json`, already in use).
- The binary's stdout discipline is load-bearing: `main` prints **only** the output file's absolute path to stdout; every diagnostic goes to stderr (`run.sh` captures stdout as `path=$(...)`). Adapters must not print to stdout.
- Markdown files in this repo: one line per paragraph, no hard-wrap (repo CLAUDE.md rule) — applies to both new `SKILL.md` files, `INDEX.md`, and `README.md` edits.
- Docs land in the same commit as the code (repo rule); `manifest/roadmap.md` is not touched (this is not a planned-roadmap item being completed/added).
- `github-repo-explorer` requires an authenticated `gh`; the skill states this as a hard prerequisite (no runtime fallback).

## Testing

Go tests run via the nested module (`go test ./...` inside `plugins/prowler`, or `scripts/selftest.sh` for the build path). Follow the existing table-driven, stub-transport style in `reddit_test.go` / `fetch_test.go` (`stubResponses`, `newTestResponse`, `htmlResponse`). No network in tests.

- **Registry / fall-through (`adapter` or `fetch` test):** with a `fetcher` carrying a stub adapter, assert `fetchPage` (a) routes a matching URL to the adapter and returns its output when `handled=true`; (b) falls through to the generic cascade when the matched adapter returns `handled=false`; (c) runs only the first matching adapter; (d) skips adapters entirely for a non-matching URL. TDD candidate — the routing contract is the core new behavior.
- **Reddit adapter (`reddit_test.go`, updated):** keep `TestIsRedditUrl` (retargeted to `redditAdapter.Matches`) and `TestToOldRedditURL`. Keep the `old.reddit` fallback-success and both-fail behaviors (retarget from `fetchReddit` to the adapter's `Fetch`). **Delete** `TestToRedditJsonUrl`, `TestFormatRedditPost`, `TestFormatRedditSubreddit`, and the JSON-content-type / unparseable-body / empty-payload `fetchReddit` subtests — those cover deleted code. Add: a Reddit URL where `old.reddit` yields usable HTML → `handled=true` with that body; where `old.reddit` fails (non-2xx / too-short) → `handled=false` so `fetchPage` falls through.
- **Hacker News adapter (`hackernews_test.go`, new):** TDD candidate. Cover: `Matches` accepts `item?id=N` across host/scheme variants and rejects non-item HN URLs and non-HN URLs; id extraction from the query string (including a malformed/missing id → `handled=false` without calling `do`); formatting an Algolia fixture (title/points/author header + bounded top-level comments, with the comment bound enforced); non-2xx API response and unparseable body → `handled=false`. Use in-memory JSON fixtures + stubbed `fetcher.do` keyed by the Algolia URL, mirroring `reddit_test.go`.
- **Skills:** `github-repo-explorer` and `distill-subagent` are codeless `SKILL.md` files — no Go tests. Verification is a manual read-through for correctness of the documented `gh` commands and the cross-skill "load `distill-subagent`" reference resolving to a real skill.

## Q&A log

- **Q:** Does the Reddit JSON-API special-case actually work, given the brief describes generalizing it? **A:** No — verified HTTP 403 on `reddit.com/*.json` (both UAs) vs. 200 on `old.reddit.com`. Only the `old.reddit` HTML fallback returns content today.
- **Q:** So what does this task become — generalizing a Reddit method that doesn't work on Reddit? **A:** The generalizable thing is a *per-site override*, not the JSON trick. Reddit's *working* strategy (old.reddit HTML) becomes adapter #1; HN (working JSON) becomes adapter #2. The correction improves the task by replacing a broken flagship example with an honest one. Proceed with the full generalization (chosen over "skip the registry, just add a second hardcoded `if`" and "drop the adapter half entirely").
- **Q:** Adapter interface shape? **A:** `interface { Matches(url) bool; Fetch(ctx, f, url) (out, handled) }` — faithful split of `isRedditUrl`/`fetchReddit`; adapter owns arbitrary multi-step strategy.
- **Q:** Registry mechanism? **A:** `adapters []siteAdapter` field on `fetcher`, set in `newFetcher()` — matches the existing injection seam, no globals/`init()`.
- **Q:** Multiple-match policy? **A:** First matching adapter only; on `handled=false` fall to the generic cascade (not to a next adapter).
- **Q:** Second adapter POC? **A:** Hacker News via the Algolia API (verified 200 JSON), matching only `item?id=N` pages — contrasts strategy with Reddit.
- **Q:** What to do with Reddit's dead JSON path? **A:** Delete it; Reddit adapter uses `old.reddit` HTML only. (Caveat: sandbox datacenter IP may be 403'd more than residential, but operator experience + observed block make "unreliable in practice" the planning assumption.)
- **Q:** github-repo-explorer operation set? **A:** Exactly what's needed to explore a repo without cloning — resolve default branch, recursive tree, read file (`gh api contents` + base64, or `raw.githubusercontent.com` for public files).
- **Q:** Reuse lyx's `internal/githubclient`? **A:** No — codeless skill + prowler's separate module can't import it; use `gh` (already a de-facto dependency; lyx's githubclient itself shells `gh auth token`).
- **Q:** gh availability? **A:** Require `gh` installed + authenticated as a stated prerequisite; no code fallback.
- **Q:** How to avoid writing the Haiku-wrapper rule in two skills? **A:** Extract it into its own guidance skill both skills load — matches the repo's mill guidance-skill convention. Refactor the existing `prowler/SKILL.md` to load it too.
- **Q:** Doesn't naming it `haiku-wrapper` bind the pattern to one model? Isn't this what Haiku is for? **A:** Haiku *is* the correct default (that's its job), but name the skill for the function to keep the contract provider-agnostic (per CLAUDE.md's provider-invariance value); document Haiku as the default cheap tier inside. Chosen name: `distill-subagent`.
- **Q:** Docs/versioning? **A:** INDEX.md rows for both skills, README note on the adapter mechanism + adapter list, `plugin.json` bump 1.0.0 → 1.1.0 — all in the same commit.
