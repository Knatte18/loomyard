# Discussion: Add cross-repo code search to prowler

```yaml
task: Add cross-repo code search to prowler
slug: cross-repo-code-search
status: discussing
parent: main
```

## Problem

prowler's `github-repo-explorer` skill is built for *trawling*: relevance-mapping a query across many GitHub repos without cloning any of them — "which of these repos combine tree-sitter with an LSP client?".
Deep single-repo work is out of its remit entirely; that goes through a local clone.
But the skill's only two capabilities today are `github-tree.sh` (list a repo's file paths) and a single-file read via `gh api .../contents/...`.
Answering an architecture-trait question with those two tools means guessing which files carry the signal (README, `go.mod`, a doc comment), reading each one by hand, and repeating that per repo.
The cost scales linearly with files read and multiplies across every repo in a survey, and it silently misses repos where the signal sits deeper than the files the agent happened to pick.

Why now: the `github-tree.sh` hardening task (commit `63916b1e2`) just collapsed the tree walk from an N-turn model-driven `gh api` walk into one deterministic script call, and established the script+selftest+README shape this addition slots into.
The code-search idea surfaced during that task's scoping and was deliberately deferred as a genuine new capability rather than folded in as a fix.
GitHub's code search REST API answers exactly the missed-signal problem: one call per repo returns every matching path across the whole repo, indexed content included, with no per-file guessing.

## Scope

**In:**

- A new script `plugins/prowler/scripts/github-code-search.sh`, parallel to `github-tree.sh`: takes a search query plus one or more `<owner>/<repo>` refs, and returns matching file paths with a short match snippet, one record per line.
- Per-repo existence/accessibility preflight, so an inaccessible or misspelled repo is reported as an error instead of silently reading as "this repo has no matches".
- A new offline test harness `plugins/prowler/scripts/github-code-search-selftest.sh` plus its stub `gh` and JSON fixtures under `plugins/prowler/scripts/testdata/github-code-search/`, mirroring `github-tree-selftest.sh`'s shape.
- `plugins/prowler/skills/github-repo-explorer/SKILL.md`: document the new script and the search-vs-tree routing rule ("known term/import/symbol across a whole repo" → search; "understand a repo's structure" → tree + selective reads).
- `plugins/prowler/skills/github-repo-explorer/SKILL.md`'s **frontmatter**, and the matching row in `plugins/prowler/skills/INDEX.md` — see the "Skill dispatch surface is updated too" decision for the exact new strings.
- `plugins/prowler/README.md`: a `## github-code-search.sh` section, mirroring the existing `## github-tree.sh` section, documenting the contract, the rate-limit budget, and the API quirks the contract is built around.

**Out:**

- GraphQL. GitHub's GraphQL `search` connection has no `CODE` type at all — code search is REST-only. This closes the task body's "REST vs GraphQL for snippets" open question outright: snippets come from the REST `text-match` media type, which was verified working (see Technical context).
- Whole-org / whole-user sweeps (`org:`, `user:` qualifiers) as a first-class script argument. The concrete trigger case supplies an explicit repo list. A caller who wants an org-wide sweep runs a raw `gh api` call; the script does not grow a second scope mode.
- Pagination past the first page of results per repo.
- Any retry, backoff, or rate-limit sleep inside the script — same deliberate stance `github-tree.sh` documents at length.
- Changes to `github-tree.sh`, `run.sh`, `selftest.sh`, the Go module under `plugins/prowler/`, the `prowler` skill, or the `distill-subagent` skill.
- `plugins/prowler/.claude-plugin/plugin.json` version bump. The prior comparable feature commit (`63916b1e2`) did not bump it either.
- `CONSTRAINTS.md`, `docs/overview.md`, `manifest/designs/`, `manifest/roadmap.md`. See Constraints for why each is untouched.

## Decisions

### Script, not documented-invocation

- Decision: ship a script `plugins/prowler/scripts/github-code-search.sh`, not a `gh` invocation documented in SKILL.md for the model to compose.
- Rationale: this is exactly the argument `github-tree.sh` already won. The multi-repo loop, the last-wins `repo:` quirk, the preflight, the fragment sanitation, and the `incomplete_results` check contain no decision a model needs to make, and composing them turn by turn costs one agent turn per repo. One script call is one turn regardless of repo count.
- Rejected: documenting `gh api -X GET search/code -f q=...` inline in SKILL.md — cheaper to write, but re-introduces the per-turn walk the sibling task just eliminated, and leaves every quirk below as a trap the model rediscovers each time.

### One API call per repo, not repeated `repo:` qualifiers

- Decision: the script issues one `search/code` call per repo ref, appending its own ` repo:<owner>/<repo>` to the caller's query, and concatenates the results.
- Rationale: verified live — a query carrying two `repo:` qualifiers does **not** OR them. `q='tree-sitter repo:helix-editor/helix repo:zed-industries/zed'` returned 287 hits, all from `zed-industries/zed` (helix alone returns 100), i.e. the last `repo:` wins and the first is silently discarded. A single-call multi-repo sweep is therefore impossible on this API; the loop is not an implementation shortcut, it is the only correct shape.
- Rejected: one call with N `repo:` qualifiers (silently wrong, returns one repo's hits as if they were the sweep's); one call with no repo scoping plus client-side filtering (searches all of GitHub, dominated by irrelevant hits, and still truncated at 1000 results before filtering).

### Hard cap of 10 repos per invocation, rejected up front

- Decision: more than 10 distinct repo refs is rejected before any network call, naming the cap and the reason, exiting 1 via `die` (see "Exit codes: 2 for usage shape, 1 for everything else" — the cap is a rule about argument values, not arity, so it is not an exit-2 usage-shape error).
  The count is taken after deduplication (see "Duplicate repo refs are deduped silently").
- Rationale: the authenticated `code_search` rate-limit bucket is 10 requests per minute (verified: `gh api rate_limit` → `code_search.limit: 10`). An 11-repo sweep is guaranteed to 403 partway through, and — given the buffer-until-complete output discipline below — would produce nothing at all after burning the whole minute's budget. Rejecting up front converts a guaranteed expensive failure into a free, immediate, explanatory one. This matches `github-tree.sh`'s ordering discipline: prerequisite and argument rejection never reach the network.
- Rejected: no cap plus abort-on-403 (burns the budget, returns nothing, and the user learns the limit only by hitting it); no cap plus internal sleeping between calls (the script's whole design stance is no retries, no backoff, no waiting — the caller decides).

### Single output format: repo, path, snippet — always

- Decision: stdout carries exactly one tab-separated record per matching file — `<owner>/<repo>\t<path>\t<snippet>` — and nothing else. The snippet is the first `text_matches` fragment with every tab/CR/LF collapsed to a single space and the result truncated to 200 characters. There is no flag and no second output mode.
- Rationale: the fragment is free — it arrives on the same call, gated only by the `Accept: application/vnd.github.text-match+json` header (verified working alongside `--jq`). It is what separates a real signal from a coincidental mention: a `tree-sitter` hit in `CHANGELOG.md` and one in `crates/syntax/src/parser.rs` are indistinguishable by path alone in some repos. Sanitising to one physical line preserves `github-tree.sh`'s strict one-record-per-line stdout discipline, so the output stays greppable and the caller never has to parse a multi-line record format. One format means one contract to test.
- Rejected: paths only (throws away the free disambiguating signal, and pushes the caller back into per-file reads — the exact cost this task exists to remove); an opt-in `--snippets` flag (two output contracts, two sets of assertions, for a body of output that is small either way); multi-line fragment blocks (breaks the line discipline for marginal extra context).

### Preflight each repo against the core rate-limit bucket

- Decision: before any search call, the script verifies every repo ref with `gh api repos/<owner>/<repo>`, aborting on the first failure with a message distinguishing 404 (not found / not accessible with this token), 401 (not authenticated), and 403 (access denied or rate limited).
- Rationale: verified live — searching a nonexistent repo returns **HTTP 200 with `total_count: 0`**, byte-identical to a real repo with no matches. Without the preflight, a typo'd or private repo silently reads as "this repo does not use tree-sitter" — a confidently wrong answer in precisely the reconnaissance use case this task serves. The preflight calls the `core` bucket (5000/hour), not `code_search` (10/minute), so it costs nothing from the scarce budget; at the 10-repo cap it is at most 10 core calls.
- Rejected: no preflight (silent wrong answers, the worst available failure mode); preflight only on a zero-result repo (saves calls in the common case but makes the call pattern data-dependent and much harder to assert in the offline harness, for a bucket that is not scarce).

### `incomplete_results: true` is a hard failure

- Decision: if any repo's response carries `incomplete_results: true`, the script dies with one stderr line naming that repo, and emits nothing on stdout.
- Rationale: the flag means GitHub returned a partial result set — the search timed out or the query was rejected in a way that still yields 200. Verified live: a malformed qualifier (`q='repo:notavalidref'`) returns `{"total_count": 0, "incomplete_results": true, "items": []}` with exit 0. Treating that as "no matches" is the same silent-partial failure `github-tree.sh` refuses for truncated trees. The caller re-invokes or fixes the query; the script does not guess.
- Rejected: a stderr warning with exit 0 (a warning on a successful-looking run is the thing agents skip, and the result is indistinguishable from a clean zero-hit sweep); silently returning the partial items (indefensible).

### One page per repo, `per_page=100`, cap announced on stderr

- Decision: exactly one `search/code` call per repo with `per_page=100`, no pagination. When a repo's `total_count` exceeds the number of returned items, the script writes one stderr note naming the repo and the true total, and still exits 0 with the full stdout record set.
- Rationale: for relevance-mapping, "does this repo match, and where" is answered by the first 100 hits; the 101st adds nothing to the decision. Paginating would multiply the scarce `code_search` budget by the page count for no decision-relevant gain, and the API caps out at 1000 results regardless (verified: page 11 at `per_page=100` returns HTTP 422 "Cannot access beyond the first 1000 results"). The stderr note exists so a capped listing is never *silently* partial — the distinction from the `incomplete_results` case is that here the returned records are all genuine and complete-as-far-as-they-go, so failing the run would destroy usable output.
- Rejected: full pagination (burns the minute's budget on one repo); no cap note at all (silently partial, the failure mode this codebase consistently refuses); emitting the cap notice on stdout as a sentinel record (pollutes the record stream that callers grep).

### Reject a caller query containing `repo:`

- Decision: if the caller's query string contains `repo:`, exit 2 with a usage error saying repos are supplied as positional arguments.
- Rationale: the script appends its own `repo:` qualifier last, so by the last-wins rule the script's always wins and the caller's is silently discarded — the caller's intent evaporates with no diagnostic. Rejecting is a one-line guard that turns a silent surprise into an explicit message. Every other qualifier (`language:`, `path:`, `in:file`, `extension:`) passes through untouched and is genuinely useful.
- Rejected: silently letting the script's qualifier win (silent intent loss); attempting to merge the caller's `repo:` into the repo list (guessing at intent, and unparseable in general).

### Buffer output, print only on complete success

- Decision: every record is accumulated in memory across all repos and printed only once the last repo has succeeded. Any failure at any point means empty stdout and a non-zero exit.
- Rationale: verbatim the discipline `github-tree.sh` documents — a failure partway through must never leave a partial prefix on stdout that a caller mistakes for a complete, short result. With a multi-repo sweep the risk is worse than for a single tree: a prefix covering repos 1–3 of 8 looks exactly like a sweep where repos 4–8 had no hits.
- Rejected: streaming per repo as it completes (partial results indistinguishable from complete ones — the whole reason the sibling script buffers).

### No retries, no backoff, no sleeping

- Decision: every non-2xx response aborts immediately with one stderr line. The caller decides whether to re-invoke.
- Rationale: inherited verbatim from `github-tree.sh`, whose header comment argues it at length and explicitly flags it as "not an oversight to fix by adding a retry loop". A transient 403 (secondary rate limit) and a permanent one (token lacks access) are indistinguishable from inside the script, and only the caller has the context to tell them apart. Consistency between the two sibling scripts is itself worth preserving — one retrying and one not is a trap.
- Rejected: retry-with-backoff on 403 (would routinely sleep a full minute inside an agent turn, and cannot distinguish the two 403 causes).

### Runtime dependency is `gh` alone; system `jq` is a harness-only dependency

- Decision: all JSON extraction at run time goes through `gh api --jq` (gh's embedded gojq). System `jq` is required only by the offline selftest harness, which checks for it up front.
- Rationale: exactly the split `github-tree.sh` and `github-tree-selftest.sh` already establish and document. `gh` is already a hard prerequisite of the skill; adding a system-`jq` runtime dependency would narrow where the skill works for no gain.
- Rejected: system `jq` at run time (new prerequisite); shell-only JSON parsing (fragile against fragments containing quotes and escapes).

### Separate stub `gh` and fixture tree, not a generalised shared stub

- Decision: a new stub at `plugins/prowler/scripts/testdata/github-code-search/bin/gh` and a new fixture directory beside it, rather than extending the existing `testdata/github-tree/bin/gh`.
- Rationale: the existing stub hard-asserts a four-argument `api <endpoint> --jq <expr>` invocation shape and rejects anything else with exit 98 — deliberately, so that a re-added preflight call in `github-tree.sh` would fail loudly rather than be silently absorbed. The new script's invocations carry `-X GET`, repeated `-f` parameters, and an `-H Accept:` header, and it makes preflight calls the tree script must never make. Generalising the shared stub to accept both shapes would destroy exactly the property that makes the existing assertions meaningful.
- Rejected: one shared stub accepting both shapes (weakens the tree harness's strictness guarantee to save a file).

### The new stub keys fixtures on a request key, not on the endpoint alone

- Decision: the new stub derives a **request key** from each invocation and matches `map.tsv`'s `field1` against that key, not against the bare endpoint.
  The key is the endpoint string when the call carries no `-f q=` parameter (every preflight call — `repos/<owner>/<repo>`, already unique per repo), and `search/code?q=<the full q parameter value>` when it does (every search call).
  `map.tsv` keeps its existing three-field `key<TAB>body<TAB>status` shape and its existing tab-split-by-parameter-expansion loop; only the value compared against `field1` changes.
- Rationale: `testdata/github-tree/bin/gh` keys on `$2` — the endpoint — alone (`field1 = endpoint`, lines 44–62), which works there because every call in a tree walk hits a distinct endpoint.
  Every search call in a multi-repo sweep hits the identical endpoint `search/code`, so an endpoint-keyed map cannot express a single one of the per-repo scenarios this harness needs: multi-repo ordering, `incomplete_results` on repo 2 of several, a 403 on repo 2 of 3, or a per-repo `total_count` note.
  The `q` value is what already distinguishes those calls — the script appends its own ` repo:<owner>/<repo>` to the shared caller query (see "One API call per repo"), so each repo's search call carries a distinct `q` by construction.
  Keying on it therefore needs no new discipline in the script and no synthetic scenario ids in the fixtures.
- The key is unique within a scenario for the same reason: preflight endpoints are one per distinct repo, and search `q` values are one per distinct repo.
  Duplicate repo refs — the one case that could collide two identical keys — never reach the network at all, because they are deduped during argument handling (see "Duplicate repo refs are deduped silently").
- The stub must also accept the new invocation shape rather than the tree stub's rigid four-argument assertion: `-X GET`, repeated `-f` parameters, and an `-H Accept:` header.
  It still rejects an unrecognised shape with a distinct non-zero exit, and still appends every invocation verbatim to `GH_STUB_LOG` before any shape check, so the "no call was made" and call-identity assertions stay meaningful.
- Rejected: a call-sequence index as the key (works, but couples every fixture to the script's internal call ordering, so reordering preflight-then-search would silently re-point every fixture at the wrong scenario);
  keying on the endpoint plus a synthetic `X-Scenario` header injected by the harness (invents a request field the real `gh` never sends, so the harness would stop exercising the script's actual invocation).

### Exit codes: 2 for usage shape, 1 for everything else

- Decision: exit 2 is reserved for a usage-shape error — too few arguments to satisfy the synopsis `github-code-search.sh <query> <owner/repo> [<owner/repo>...]`.
  Every other rejection exits 1 via `die`, with one stderr line.
  Concretely: no arguments → 2; a query but no repo ref → 2; an invalid `<owner>/<repo>` ref → 1; more than 10 distinct repo refs → 1; a caller query containing `repo:` → 1.
- Rationale: this is `github-tree.sh`'s actual convention, which the earlier draft of this file misstated.
  That script exits 2 only for the arg-count check (lines 41–44, printing the bare `usage:` synopsis) and exits 1 via `die` for a semantically invalid ref (lines 49–51) and for every other rejection.
  The split is shape-versus-semantics, not "all argument problems are 2".
  The repo cap in particular is a semantic rule about argument *values*, not arity — 11 refs still satisfies the synopsis's `<owner/repo>...`, so exit 2 would be wrong there.
- Rejected: exit 2 for every pre-network rejection (diverges from the sibling script a caller may already have wrapped, and collapses a distinction that script deliberately draws).

### Duplicate repo refs are deduped silently

- Decision: repeated `<owner>/<repo>` refs are deduped before the 10-repo cap is applied, keeping first-occurrence order.
  The cap counts distinct refs, and each distinct repo produces exactly one preflight call, one search call, and one contiguous block of output records.
- Rationale: a duplicate is unambiguously a caller slip with exactly one sensible reading, so rejecting it would fail a sweep that the script knows how to run correctly.
  Silently accepting it is worse than either alternative: it burns two of the ten `code_search` requests on the same repo and emits every one of that repo's records twice, which a caller counting hits per repo would read as real duplication in the repo.
  Deduping before the cap also makes the cap's guarantee exact — ten distinct refs is ten calls, never eleven.
- Rejected: reject as a usage error (fails a runnable sweep over a typo); accept as-is (doubles the records and the rate-limit spend, silently).

### Skill routing documented as a rule, not a preference

- Decision: SKILL.md gains an explicit routing rule — a known term, import, symbol, or dependency name anywhere in a repo → `github-code-search.sh`; understanding a repo's overall structure or layout → `github-tree.sh` plus selective reads; a repo list to relevance-map against one trait → `github-code-search.sh` over the whole list in one call.
- Rationale: the task body asks for exactly this, and without it the model has two overlapping tools and will default to the one it used last. The routing is also where the "deep single-repo work goes through a local clone instead" boundary gets restated, so the skill's remit stays visible at the point of use.
- Rejected: leaving routing implicit in the two script descriptions.

### Skill dispatch surface is updated too, not just the skill body

- Decision: `skills/github-repo-explorer/SKILL.md`'s frontmatter and the matching `skills/INDEX.md` row are updated in the same commit, to these exact strings:
  - `description: Search code across GitHub repos, browse a repo's file tree, and read files via the gh CLI, without cloning`
  - `argument-hint: "<owner/repo>... [path | search term] [question]"`
  - `INDEX.md`'s `github-repo-explorer` row (line 6) takes the same new `description` text verbatim.
- Rationale: the frontmatter `description` is the skill's dispatch surface — it is what decides whether this skill is reached at all — and today it names only tree-browsing and file-reading (`Browse a GitHub repo's file tree and read files via the gh CLI, without cloning`).
  Leaving it unchanged ships a search capability that a cross-repo search query would never route to, which defeats the task's own trigger case.
  `INDEX.md` line 6 duplicates that description verbatim today, so updating one without the other leaves the two disagreeing.
  The `argument-hint` changes because the skill's input is now plural repos, and the second positional slot is either a path (tree/read) or a search term.
- This is the only edit either file receives; nothing else in `INDEX.md` is touched.
- Rejected: leaving the frontmatter alone (a search capability nothing routes to);
  updating `SKILL.md` only (the two files' descriptions are duplicates by convention, and a silent divergence is the kind of rot the Documentation Lifecycle invariant exists to prevent).

## Technical context

**Where things live.** Everything is under `plugins/prowler/`, a self-contained Claude Code plugin with its own nested Go module (untouched by this task).
The relevant existing pieces:

- `plugins/prowler/scripts/github-tree.sh` — the model to follow. Read its header comment and its `fetch()` function before writing anything: it establishes the stdout/stderr discipline, the `die()` helper, the prerequisite-then-argument-then-network ordering, the `gh api --jq` header-line-plus-records trick, the `#badpath` sentinel for paths containing tab or newline, the HTTP-status extraction from `gh`'s raw error body, and the buffer-until-complete output rule. The new script mirrors all of it.
- `plugins/prowler/scripts/github-tree-selftest.sh` — the harness model. Stub `gh` prepended to `PATH`, a per-scenario `map.tsv` mapping endpoint → fixture body (+ optional HTTP status), a per-scenario `calls.log` the stub appends to *before* validating the invocation shape, and assertions on exact stdout, exact call count and call identity, and a distinguishing stderr substring per failure. Scratch goes to `$PLUGIN_ROOT/../../.scratch/<harness-name>/` — the repo's sanctioned gitignored scratch, never a system temp directory.
- `plugins/prowler/scripts/testdata/github-tree/bin/gh` — the stub model, including the log-before-validate ordering and the failure emulation (raw error body to **stdout**, nothing to stderr, `--jq` not applied, exit 1 — which is what lets the script under test parse the status out of the body itself).
- `plugins/prowler/skills/github-repo-explorer/SKILL.md` — where the new script is documented for the model. Note its existing `${CLAUDE_SKILL_DIR}/../../scripts` resolution idiom and its explicit "resolve the path now, while `${CLAUDE_SKILL_DIR}` is still set — a dispatched subagent won't have it" warning; the new script's documentation follows the same pattern.
- `plugins/prowler/README.md` — has a `## github-tree.sh: one-call repo tree listing` section; the new section sits beside it in the same voice.

**API behaviour, verified live against `gh` during this discussion** (not assumed — every item below was run):

| Observation | Evidence |
| --- | --- |
| `code_search` rate-limit bucket is 10 req/min authenticated; `search` is 30/min; they are distinct buckets | `gh api rate_limit` → `code_search: {limit: 10}` |
| Two `repo:` qualifiers do not OR — the last one wins, the first is silently dropped | `q='tree-sitter repo:helix-editor/helix repo:zed-industries/zed'` → 287 hits, all `zed-industries/zed`; helix alone → 100 |
| Snippets work on REST via the text-match media type, and compose with `--jq` | `-H "Accept: application/vnd.github.text-match+json"` → `.items[].text_matches[] | {fragment, property, indices}`, fragments are multi-line strings |
| A nonexistent repo returns 200 / `total_count: 0`, indistinguishable from a real repo with no hits | `q='foo repo:helix-editor/definitely-not-a-real-repo-xyz'` → `{"total_count": 0}`, exit 0 |
| A malformed qualifier returns 200 / `total_count: 0` / **`incomplete_results: true`** | `q='repo:notavalidref'` → `{"total_count":0,"incomplete_results":true,"items":[]}`, exit 0 |
| A qualifier-only query with no free-text term is accepted (no "at least one term" requirement any more) | `q='repo:helix-editor/helix'` → `total_count: 1744` |
| `org:` scoping does work in a single call and spans that org's repos | `q='tree_sitter org:helix-editor language:rust'` → 25 hits across `helix`, `regex-cursor`, `tree-house` |
| Results are hard-capped at 1000; page 11 at `per_page=100` is a 422 | `-f page=11 -f per_page=100` → `{"message":"Cannot access beyond the first 1000 results","status":"422"}` |

**`gh` invocation shape gotcha.** `gh api` defaults to POST as soon as any `-f` parameter is present, which would send `q` in a request body the search endpoint ignores.
`-X GET` (or `--method GET`) is mandatory and must be asserted by the harness, not left to reviewer vigilance — it is the kind of thing that "works" against a stub that does not check it and fails only live.

**Response fields the script needs:** per item, `.repository.full_name`, `.path`, and `.text_matches[0].fragment`; per response, `.total_count` and `.incomplete_results`.
A single item can carry several `text_matches` entries (confirmed: `grammars.nix` returned two); only the first is emitted.
`text_matches` entries also carry `property` (`"content"` or `"path"`) and `indices` — neither is used, but a `property: "path"` match has a fragment that is the path rather than file content, which is worth a comment rather than a surprise.

**Fragment sanitation** happens inside the `--jq` expression, not in shell, so the record is already one physical line by the time bash sees it: collapse `[\t\r\n]+` to a single space, then truncate.
The `#badpath` guard from `github-tree.sh` carries over for `.path` and additionally for `.repository.full_name`.

## Constraints

From `CONSTRAINTS.md` — the relevant finding is that **none of its invariants bind this change**, and the plan should not manufacture compliance work:

- **GitHub Auth Invariant** ("all GitHub authentication goes through `internal/githubclient`; no other production package shells out to `gh`") scopes to the `lyx` Go module's production packages. `plugins/prowler/` is a separate nested Go module and, for this task, plain bash; `github-tree.sh` already shells out to `gh` and was merged under this invariant. No conflict, and no new exemption needs recording.
- **Test Tier Purity**, **Hermetic Git Test Environment**, **Sandbox Suite Coverage**, and every `internal/*` leaf/seam invariant concern Go packages under `internal/` and the sandbox suite. This task adds no Go code and registers no lyx module.
- **Documentation Lifecycle** applies: docs land in the same commit as the change (see below).

From `CLAUDE.md`:

- **Markdown: semantic line breaks.** Every `.md` file touched — `README.md`, `SKILL.md`, `skills/INDEX.md`, and this file — uses one sentence per line, with additional breaks at internal independent-clause boundaries. Never a fixed-column hard wrap, never trailing double-spaces or a backslash. Table cells stay on one line.
- **Task completion — docs land in the same commit.** `plugins/prowler/` has no entry in `manifest/designs/` and is not named in `docs/overview.md` (verified: `grep -rl prowler docs manifest` returns nothing), so the module-doc and overview obligations have no target here. The docs that must land in the same commit are `README.md`, `SKILL.md` (body and frontmatter), and `plugins/prowler/skills/INDEX.md`. `CONSTRAINTS.md` is untouched — this introduces no cross-cutting invariant. `manifest/roadmap.md` is untouched — this is neither completing nor adding a planned roadmap item.
- **No version bumps pre-publish.** `plugins/prowler/.claude-plugin/plugin.json` stays at `1.1.0`; the comparable prior feature commit `63916b1e2` did not bump it either.
- **Worktree isolation.** All work stays inside `wts/cross-repo-code-search`.

Operational constraints discovered during exploration:

- The `code_search` bucket is 10 req/min. Any live verification during implementation must be paced accordingly — this discussion's own exploration exhausted 9 of 10 in a single minute. Live checks belong in the manual-verification list, not in an automated test loop.
- The offline harness must never make a network call. Every assertion runs against the stub `gh` on `PATH`.

## Testing

TDD is the wrong frame for a bash script whose entire behaviour is API-response-shaped; the fixtures have to exist before the assertions mean anything.
The workable order is: capture fixtures from the live observations above → write the stub → write the harness scenarios → write the script against them.
The harness is the deliverable that proves the contract, and it must be written in the same batch as the script, not after.

**`plugins/prowler/scripts/github-code-search-selftest.sh` — offline, stub-driven, no network.**
Mirrors `github-tree-selftest.sh`'s structure exactly — with the one deliberate divergence that its `map.tsv` is keyed on the request key rather than the bare endpoint (see "The new stub keys fixtures on a request key, not on the endpoint alone"), which is what makes the per-repo scenarios below expressible at all: `run_scenario` writes a per-scenario `map.tsv`, truncates `calls.log`, runs the script with the stub on `PATH`, and captures stdout, stderr, and exit status into separate variables (separate is mandatory — many assertions turn on stdout being byte-empty while stderr is not).
`jq` availability is checked up front with the same `require_jq` guard.
Scratch lives under `.scratch/github-code-search-selftest/`.

Scenarios that must be covered:

- **Single repo, several hits** — exact stdout bytes, including the `repo\tpath\tsnippet` shape and a fragment whose original form contained newlines, proving sanitation.
- **Single repo, zero hits** — exit 0, byte-empty stdout, and the preflight call still made.
- **Multiple repos** — output ordering is the repo-argument order, then per-repo API order; call count is exactly `N` preflight + `N` search calls, and call *identity* is asserted (each search endpoint carries its own repo's `repo:` qualifier appended to the shared query).
- **`-X GET` is present** on every search invocation, and the `Accept: application/vnd.github.text-match+json` header is present. Asserted against the logged invocation, since a stub that ignores them would let a live-only failure through.
- **Fragment truncation** at the 200-character boundary, and an item with multiple `text_matches` emitting only the first.
- **An item whose `text_matches` array is absent or empty** — emits an empty third field rather than crashing or dropping the record.
- **`incomplete_results: true`** on one repo of several — non-zero exit, byte-empty stdout, stderr naming that repo.
- **`total_count` greater than the returned item count** — exit 0, full stdout, one stderr note naming the repo and the total.
- **Preflight failures**, each with its own distinguishing stderr substring and byte-empty stdout: 404 (repo not found / not accessible), 401 (not authenticated), 403 (access denied or rate limited). Each must also assert that **no search call was made** — the call log makes this checkable.
- **Search-call failures**: 403 mid-sweep (repo 2 of 3) proving buffer-until-complete — byte-empty stdout despite repo 1 having succeeded; and 422.
- **Argument rejection, all before any network call** (assert an empty call log), each asserting its own exit code per the "Exit codes" decision: no arguments → 2; a query but no repo ref → 2; an invalid `<owner>/<repo>` ref → 1; more than 10 distinct repo refs → 1; a caller query containing `repo:` → 1.
  Each also asserts byte-empty stdout and its own distinguishing stderr substring.
- **Duplicate repo refs are deduped** — the same `<owner>/<repo>` passed twice, plus a third distinct repo, yields exactly 2 preflight calls and 2 search calls (asserted against the call log), output records for the duplicated repo appearing once, and first-occurrence argument ordering preserved.
- **Dedup happens before the cap** — 11 refs of which 2 are duplicates of each other (10 distinct) runs normally rather than being rejected;
  11 distinct refs is rejected with exit 1 and an empty call log.
- **`gh` missing from `PATH`** — reuses `github-tree-selftest.sh`'s `BASH_BIN` absolute-path trick so the script's own prerequisite guard is what fires, not the harness failing to find bash.
- **A path or `full_name` containing a tab or newline** — the `#badpath` sentinel path: refuse to emit, non-zero exit, byte-empty stdout.

**Manual / live verification, documented in the harness header comment as not-asserted** (the same honesty `github-tree-selftest.sh` practises about its own gaps):

- One live sweep across three real repos, confirming the record shape and that snippets arrive.
- One live run against a repo with more than 100 matches, confirming the cap note fires.
- One live run against a deliberately misspelled repo, confirming the preflight catches what the search API would have reported as zero hits.
- A spot-check that the `--jq` expression behaves identically under gojq (production, via `gh`) and `jq` (the harness) — the same acknowledged seam the tree harness documents.

**No Go tests.** This task adds no Go code, so `go test ./...` for the prowler module is unaffected and is not part of the verify command; the selftest script is.

## Q&A log

- **Q:** REST `search/code` or the GraphQL search API for snippets? **A:** [auto-pick] REST with the `text-match` media type. **Why:** GitHub's GraphQL `search` connection has no `CODE` type at all, so GraphQL is not an option; the REST text-match header was verified live to return fragments and to compose with `--jq`.
- **Q:** Ship a script, or document a `gh` invocation in SKILL.md? **A:** [auto-pick] A script, `github-code-search.sh`, parallel to `github-tree.sh`. **Why:** the multi-repo loop and every quirk below contain no decision a model needs to make; the sibling task just won this exact argument for the tree walk.
- **Q:** Should this generalize to a multi-repo sweep in one invocation (the task body's open question 3)? **A:** [auto-pick] Yes — a repo list is the script's primary argument shape, implemented as one API call per repo. **Why:** it is the concrete trigger case; and repeated `repo:` qualifiers were verified not to OR (last one wins silently), so per-repo calls are the only correct implementation, not a shortcut.
- **Q:** Cap the repo count, or let a large sweep fail on rate limit? **A:** [auto-pick] Hard cap at 10, rejected before any network call. **Why:** the `code_search` bucket is exactly 10/min, so an 11-repo sweep is a guaranteed failure that also burns the minute; rejecting up front is free and explanatory.
- **Q:** Snippets always, opt-in behind a flag, or never? **A:** [auto-pick] Always, as a sanitised single-line third field. **Why:** the fragment is free on the same call and is what distinguishes a real hit from a changelog mention; one format means one contract to test.
- **Q:** Preflight each repo's existence, given it costs an extra call? **A:** [auto-pick] Yes, via `gh api repos/<owner>/<repo>`. **Why:** a nonexistent repo returns 200/`total_count: 0`, identical to a real no-match, so without preflight a typo reads as a confident negative answer; the preflight hits the 5000/hour `core` bucket, not the scarce one.
- **Q:** How should `incomplete_results: true` be handled? **A:** [auto-pick] Hard failure, empty stdout, non-zero exit. **Why:** it means a partial result set returned with a 200; treating it as "no matches" is the silent-partial failure this codebase consistently refuses.
- **Q:** Paginate per repo? **A:** [auto-pick] No — one page, `per_page=100`, with a stderr note when `total_count` exceeds what was returned. **Why:** the first 100 hits answer the relevance question; pagination multiplies the scarce budget, and the API refuses past 1000 anyway (verified 422).
- **Q:** What if the caller's own query string contains `repo:`? **A:** [auto-pick] Reject with exit 2, pointing at the positional repo arguments. **Why:** the script's appended qualifier wins by the last-wins rule, silently discarding the caller's intent with no diagnostic.
- **Q:** Extend the existing `testdata/github-tree/bin/gh` stub, or write a second one? **A:** [auto-pick] A second stub and fixture tree under `testdata/github-code-search/`. **Why:** the existing stub deliberately rejects any invocation that is not exactly `api <endpoint> --jq <expr>`, and that strictness is a load-bearing property of the tree harness; loosening it to fit both shapes would weaken assertions that already exist.
- **Q:** Support `org:` / `user:` whole-org sweeps as a script argument? **A:** [auto-pick] No — out of scope; a raw `gh api` call covers it. **Why:** the trigger case supplies an explicit repo list, and a second scope mode doubles the argument surface and the harness matrix for a case not yet demanded. (Verified that `org:` does work in one call, so the raw-call fallback is real, not hypothetical.)
- **Q:** Bump `plugin.json`'s version for a new capability? **A:** [auto-pick] No, leave it at `1.1.0`. **Why:** the comparable prior feature commit `63916b1e2` did not bump it, and unpublished loomyard plugins are not version-bumped per feature.
- **Q:** Which docs must land in the same commit? **A:** [auto-pick] `README.md` and `SKILL.md` only. **Why:** `grep -rl prowler docs manifest` returns nothing — prowler has no `manifest/designs/` doc and no `docs/overview.md` entry; no new cross-cutting invariant means no `CONSTRAINTS.md` change; this is neither completing nor adding a roadmap item.
