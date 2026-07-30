# Plan: prowler: site-adapter mechanism + github-repo-explorer skill (Claude reading the web)

```yaml
task: 'prowler: site-adapter mechanism + github-repo-explorer skill (Claude reading the web)'
slug: prowler-web-reading
approved: true
started: 20260730-175552
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: site-adapters
    file: 01-site-adapters.md
    depends-on: []
    verify: go -C plugins/prowler test ./...
  - number: 2
    name: skills-and-docs
    file: 02-skills-and-docs.md
    depends-on: [1]
    verify: null
```

## Shared Decisions

### Decision: adapter interface + registry shape

- **Decision:** A `siteAdapter` interface — `Matches(url string) bool` and `Fetch(ctx context.Context, f fetcher, url string) (out string, handled bool)` — lives in a new `plugins/prowler/adapter.go`, together with `defaultAdapters() []siteAdapter` returning `[]siteAdapter{redditAdapter{}, hackerNewsAdapter{}}` in that order. Adapters are held as an ordered `adapters []siteAdapter` field on the `fetcher` struct, wired in `newFetcher()` via `adapters: defaultAdapters()`. `fetchPage` iterates `f.adapters`; the first adapter whose `Matches` returns true has its `Fetch` called, and on `handled == true` its output is returned; on `handled == false` control falls through to the generic HTML cascade (it does **not** try a second adapter).
- **Rationale:** Faithful generalization of today's hardcoded `if isRedditUrl(url) { fetchReddit(...) }` block, keeping the existing injection seam (`do`/`browser` are already injected fields — `adapters` joins them) so tests inject a custom adapter set with zero package-level globals.
- **Applies to:** site-adapters (skills-and-docs documents it only).

### Decision: Reddit uses old.reddit.com only; JSON path deleted

- **Decision:** `redditAdapter.Fetch` delegates entirely to the retained `fetchOldRedditHTML` (kept in `fetch.go`): on its success return `(out, true)`; on its failure return `("", false)` so the generic cascade + browser fallback still get a chance. All Reddit JSON code is deleted (`toRedditJsonUrl`, `fetchReddit`, `formatRedditPost`, `formatRedditSubreddit`, `selftextSnippet`, `selftextSnippetLen`, the `redditThing`/`redditData` structs, `isRedditUrl`, and the `prowler-reader/1.0` UA). `redditHostPattern`, `redditHostReplace`, `toOldRedditURL`, and `maxTopComments` are kept (`maxTopComments` becomes the HN adapter's comment bound).
- **Rationale:** The Reddit JSON API is bot-blocked in practice (403 verified; operator experience). Deleting it makes Reddit the honest "alternate-HTML-host" adapter and removes a wasted round-trip. Returning `handled=false` on failure is a behavior improvement over today's hard-error terminal.
- **Applies to:** site-adapters.

### Decision: Hacker News adapter via Algolia API

- **Decision:** `hackerNewsAdapter` matches only `news.ycombinator.com/item?id=N` URLs (any scheme/host-prefix variant), extracts the numeric `id`, fetches `https://hn.algolia.com/api/v1/items/{id}`, and formats a markdown post (title/points/author header, post text or link, and up to `maxTopComments` top-level comments from the returned nested tree). A missing/invalid id, non-2xx response, or unparseable body returns `handled=false`. It uses `f.do` (the injected transport) so it is unit-testable with stubs, and sets no special UA (default Go client UA is fine for the public Algolia API).
- **Rationale:** HN's clean JSON API genuinely works, giving a second adapter with a *different* strategy from Reddit — proving the interface generalizes beyond one trick.
- **Applies to:** site-adapters.

### Decision: comment cleanup lands with the code

- **Decision:** In the same batch, rewrite every retained comment that still describes the deleted JSON path: `fetch.go`'s file header + `fetchPage` docstring, `fetchOldRedditHTML`'s doc, `errorResult`'s "shared by fetchPage and fetchReddit", and `reddit.go`'s file header + `redditHostPattern`/`redditHostReplace` comments.
- **Rationale:** Repo docs-in-same-commit rule; stale comments misdescribe behavior once JSON is gone.
- **Applies to:** site-adapters.

### Decision: distill-subagent guidance skill, loaded by name

- **Decision:** The Haiku-wrapper judgment rule is extracted into a codeless guidance skill `plugins/prowler/skills/distill-subagent/SKILL.md`. Both `prowler` and `github-repo-explorer` instruct the orchestrating Claude to invoke it **by name via the Skill tool** (`prowler:distill-subagent`) before deciding how to run a fetch/read. Haiku is documented as the default cheap tier ("cheap-tier subagent, currently Haiku"), not baked into the skill's identity. The existing `prowler/SKILL.md` is refactored to load it and to soften its hardcoded "Haiku subagent" / `model: haiku` wording to the same "cheap tier, currently Haiku" phrasing (the concrete `model: haiku` stays in the run step).
- **Rationale:** Single source of truth for the wrap rule; provider-agnostic identity per the repo's provider-invariance value; matches the mill plugin's guidance-skill-loaded-by-other-skills convention.
- **Applies to:** skills-and-docs.

### Decision: Markdown one-line-per-paragraph

- **Decision:** All `.md` files written or edited (both `SKILL.md` files, `INDEX.md`, `README.md`) use one continuous line per paragraph/list-item — no hard-wrap at a fixed column.
- **Rationale:** Repo CLAUDE.md markdown rule.
- **Applies to:** skills-and-docs.

## All Files Touched

- `.claude-plugin/marketplace.json`
- `plugins/prowler/.claude-plugin/plugin.json`
- `plugins/prowler/README.md`
- `plugins/prowler/adapter.go`
- `plugins/prowler/adapter_test.go`
- `plugins/prowler/fetch.go`
- `plugins/prowler/fetch_test.go`
- `plugins/prowler/fetcher.go`
- `plugins/prowler/hackernews.go`
- `plugins/prowler/hackernews_test.go`
- `plugins/prowler/main.go`
- `plugins/prowler/reddit.go`
- `plugins/prowler/reddit_test.go`
- `plugins/prowler/skills/INDEX.md`
- `plugins/prowler/skills/distill-subagent/SKILL.md`
- `plugins/prowler/skills/github-repo-explorer/SKILL.md`
- `plugins/prowler/skills/prowler/SKILL.md`
