# Discussion: prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call

```yaml
task: "prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call"
slug: prowler-github-tree-script
status: discussing
parent: main
```

## Problem

`plugins/prowler/skills/github-repo-explorer/SKILL.md` has no backing implementation.
Its tree-listing algorithm is written out as prose steps the calling LLM executes one `gh api` call at a time: resolve the default branch, list the recursive tree, re-call the same endpoint a second time just to read `.truncated`, and — when truncated — descend into every subdirectory's tree SHA one non-recursive call at a time.
Every one of those calls is a full agent turn.

Turn count is what makes this expensive: a skill that has an LLM re-execute a branching, potentially-recursive algorithm one tool call at a time pays a round trip through the model for every step of a walk that has no decisions a model needs to make.
The fix is a deterministic script that does the whole walk internally and prints one complete path list, turning N+3 turns into exactly 1 regardless of repo size.

## Scope

**In:**

- New `plugins/prowler/scripts/github-tree.sh` — takes `<owner/repo> [path]`, fetches the recursive tree once, falls back internally when truncated, prints one newline-separated list of file paths to stdout.
- New offline shell harness `plugins/prowler/scripts/github-tree-selftest.sh` plus fixtures under `plugins/prowler/scripts/testdata/github-tree/`.
- Rewrite of the tree-listing portion of `plugins/prowler/skills/github-repo-explorer/SKILL.md` to a single script invocation, plus the one-word `{branch}` → `HEAD` correction in the `raw.githubusercontent.com` note that the removal of the branch-resolve step makes necessary.
- A `github-tree.sh` section in `plugins/prowler/README.md`, beside the existing "Build-on-first-run" section that documents `run.sh`.

**Out:**

- The `read-file` verb in `SKILL.md` — already a single `gh api .../contents/{path}` call with no branching. Otherwise untouched, including the `distill-subagent` pointer.
- The Go module (`plugins/prowler/*.go`), `bin/prowler`, `scripts/run.sh`, and `scripts/selftest.sh`. This task adds a sibling script; it does not modify the build-on-first-run wrapper or its harness.
- Any MCP server. Explicitly rejected in the source issue (#217) and not reopened here.
- Reading file *content* in bulk, caching, or any persistence. The script lists paths and exits.
- `plugins/prowler/skills/INDEX.md` — the skill's `description` frontmatter does not change, so the generated index row does not either.
- `manifest/roadmap.md` — this is a plugin-local enhancement sourced from a GitHub issue, not a planned roadmap item.

## Decisions

### Bash script, not a Go subcommand

- Decision: implement as `plugins/prowler/scripts/github-tree.sh`, a standalone bash script invoked as `bash "$TREE_SH" <owner/repo> [path]`.
- Rationale: `gh` is already a documented hard prerequisite of this skill, so bash adds no new runtime dependency. It mirrors the existing `scripts/run.sh` convention (self-locating from `$0`, strict stdout discipline). A Go subcommand would drag an unrelated concern — GitHub API tree-walking — into a binary otherwise scoped to headless web fetching, and would put the listing behind a Go toolchain and a first-run compile that this script does not need.
- Rejected: a new subcommand on `bin/prowler` (better JSON handling and Go test conventions, but net-new code either way, plus a scope violation of the binary's purpose); an MCP server (rejected in the issue — fixed protocol/process/wiring cost unjustified for a two-argument, occasionally-used verb).

### `gh api --jq` only — no system `jq` at runtime

- Decision: every JSON field is extracted with `gh api --jq '<expr>'`. The script never pipes to a standalone `jq` binary, and `jq` is not added to the skill's prerequisites. (The *test* harness does use system `jq` — see the stub-`gh` decision below; that is a test-only dependency, never a runtime one.)
- Rationale: `gh` embeds a jq implementation (gojq); `--jq` accepts full jq expressions including multi-output ones. Verified on `gh version 2.98.0`: `gh api repos/cli/cli --jq '"branch\t" + .default_branch'` prints `branch<TAB>trunk`. Using it keeps the runtime dependency set at exactly `gh`, matching the skill's existing "hard prerequisite" wording, and avoids the issue sketch's cost of spawning three `jq` processes per tree entry.
- Rejected: the issue sketch's `jq -r ... <<<"$resp"` shape (adds a real second runtime dependency and is process-per-entry expensive on large trees).

### `HEAD` as the ref — the branch-resolve call disappears

- Decision: the script never calls `repos/{owner}/{repo}` to resolve `.default_branch`. It addresses trees as `repos/{owner}/{repo}/git/trees/HEAD[...]` directly.
- Rationale: GitHub's `git/trees/{tree_sha}` endpoint accepts any commit-ish or tree-ish, and `HEAD` is the repository's symbolic ref for its default branch. Verified: `repos/cli/cli/git/trees/HEAD?recursive=1` returns the same 1824 entries as `.../trees/trunk?recursive=1`, and `repos/torvalds/linux/git/trees/HEAD` returns the same 41 root entries as `.../trees/master`. This removes an entire API call from every invocation — the untruncated, unscoped case is now exactly **one** call.
- Consequence: the branch-resolve paragraph disappears from `SKILL.md`, which leaves the surviving `raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}` note with an undefined `{branch}` placeholder. That note becomes `.../HEAD/{path}` — verified to return HTTP 200 for `cli/cli/HEAD/README.md`. It is a one-word edit, and leaving a dangling placeholder behind would be a defect introduced by this task.
- Rejected: keeping the explicit `default_branch` lookup (one guaranteed extra call per invocation to obtain a value `HEAD` already denotes).

### `[path]` is a tree-ish suffix, not a SHA to resolve

- Decision: `github-tree.sh <owner/repo> [path]` accepts an optional repo-relative directory path, and applies it by extending the ref: the starting fetch is `git/trees/HEAD:<path>` instead of `git/trees/HEAD`. No separate SHA-resolution step, no per-component descent, no `contents` API call. Because the API returns paths relative to the requested tree, the script prefixes every emitted path with `<path>/` so output is always repo-relative. An omitted or empty `path` means the whole repo.
- Rationale: `git/trees/{ref}:{path}` accepts an arbitrarily nested path in a single call — verified: `repos/torvalds/linux/git/trees/master:drivers/net/ethernet` returns that tree directly (106 entries), and `repos/torvalds/linux/git/trees/HEAD:drivers/net?recursive=1` returns 7329 entries untruncated. This is what makes the scoped case cheap and makes the whole `path` feature a two-line change rather than a second walk algorithm. The skill's `argument-hint` already advertises `<owner/repo> [path]`, and the issue's sketch documents it in its header while its body ignores it.
- Error behaviour: a bad `path` is a hard error, never an empty listing. The API distinguishes the two cases for free — a nonexistent path returns HTTP 404 (`Not Found`), and a path naming a *file* returns HTTP 422 (`Invalid object requested. SHA must identify a commit or a tree.`). Both verified live. The script reports each distinctly on stderr and exits non-zero, so a typo can never read as "this directory has no files".
- **Call counts** (corrected; the earlier "27 calls to 2" claim predated the `HEAD` and tree-ish findings and was wrong on both ends):
  - `cli/cli`, unscoped, untruncated: **1** call.
  - `torvalds/linux`, unscoped: 1 root recursive (truncated) + 1 root non-recursive + 24 subtree recursive = **26** calls.
  - `torvalds/linux drivers/net`: **1** call — the scoped tree is untruncated, so the fallback never engages.
  - All of these are one *agent turn*, which is the metric this task exists to move; the call counts matter only for the rate-limit budget.
- Rejected: dropping the argument (contradicts the advertised interface and forgoes the largest saving on exactly the repos this task targets); per-component non-recursive descent to find the tree SHA (one call per path component, for something one tree-ish reference does in zero extra calls); treating a bad path as an empty result (silently wrong).

### One API response, both fields — combined `--jq` stream

- Decision: each tree fetch is a single `gh api` call whose `--jq` expression emits a header line carrying the truncation flag followed by one tab-separated `type<TAB>sha<TAB>path` line per entry, e.g.

  ```
  --jq '"#trunc\t" + (.truncated|tostring), (.tree[] | .type + "\t" + .sha + "\t" + .path)'
  ```

  The script reads the header, then the entries, from that one response.
- Rationale: this is the direct fix for the issue's "redundant duplicate call against the `recursive=1` endpoint" — both `.truncated` and `.tree` come from one request. Tab-separated output is safe because git rejects a tab in a path component and a newline cannot appear in one either, so a single `read -r` loop parses it with no per-entry subprocess. Verified shape against `cli/cli`.
- Rejected: two calls to the same endpoint (today's behaviour, the thing being removed); JSON-per-line output requiring a parser.

### Truncated fallback: re-list non-recursively, then fetch each child with `recursive=1`

- Decision: the fallback is one uniform rule applied at every depth, root included. For any node whose `?recursive=1` response reports `truncated: true`:
  1. **Re-fetch that same node non-recursively** (`git/trees/{sha}`, no `recursive=1`).
  2. **Emit every `blob` entry in that non-recursive listing immediately** — these are the files sitting directly in that directory, and they exist in no other response.
  3. **Enqueue every `tree` entry from that non-recursive listing**, each fetched with `?recursive=1`, with the accumulated path prefix.
  A child that comes back untruncated has all its blobs emitted and is done. A child that comes back truncated re-enters this same rule at step 1.
- Rationale (why steps 1 and 2 are stated explicitly): a `truncated: true` response is *by definition* an incomplete entry list, so both its blobs and its subtrees are unreliable — enumerating children straight out of it silently loses every subtree past the cap. The non-recursive re-fetch is the only listing of that directory guaranteed complete, so it is the one both the emit and the enqueue must read from. Applying the rule uniformly (rather than only at the root) is what keeps the design's "never silently partial" promise at depth. Verified against `torvalds/linux`: the root recursive fetch truncates at 71638 entries, but all 24 top-level subtrees return untruncated under `?recursive=1` (largest: `drivers` at 40492, then `arch` 19618, `Documentation` 12178, `tools` 10670), so in practice the recursion terminates at depth 1 — but the rule does not depend on that.
- Residual case: a *non-recursive* listing of a single directory that is itself truncated would mean one directory with more entries than the API cap. This is treated as a hard error (stderr message, non-zero exit, no output), not a partial list. It is not reachable with any real repository, and failing loudly is the only behaviour consistent with the rest of this design.
- Rejected: per-directory non-recursive descent everywhere (the issue's sketch — correct but pathologically call-heavy: thousands of calls for linux against 26); enqueueing children from the truncated response (loses subtrees past the cap — the defect this decision exists to close); returning a partial list with a warning (the SKILL's existing text explicitly promises "never silently partial").

### FIFO worklist, not shell recursion — and the sibling order fixtures pin

- Decision: the fallback walk uses an explicit **FIFO queue**: an append-only bash array plus an advancing head index, consumed with `while [ $head -lt ${#queue[@]} ]`. Not a recursive shell function, and not a LIFO stack.
- Rationale: the issue sketch's `jq -c '.tree[]' <<<"$(gh api ...)" | while read` puts the loop body — including the recursive call — in a pipeline subshell, where a failed `gh api` cannot abort the script and no state escapes. An explicit worklist keeps every iteration in the main shell, so a single failed API call fails the whole run cleanly, and there is no recursion depth to reason about. FIFO specifically, because a stack pops siblings in reverse push order, which would make output order the *inverse* of the API's git-sorted order for no reason.
- **Order the fixtures assert:** breadth-first, siblings in API order. Concretely, on the truncated path the output is: the root's own blobs in API order, then for each root subtree in API order that subtree's full recursive blob list in API order, and so on level by level. This is not git's global sorted order (a top-level `zzz.txt` precedes `aaa/file.txt`), and the harness asserts this exact sequence rather than a sorted one.
- Rejected: the sketch's recursive `walk()` function (subshell-scoped, error-swallowing); a LIFO stack (reverses sibling order for nothing); process substitution around a recursive function (fixes the subshell but keeps the depth question for no gain).

### Blob paths only, in both branches

- Decision: output is file paths only — `type == "blob"` entries. `tree` entries are never emitted, and `commit` entries (submodule gitlinks) are skipped silently. This holds identically on the fast path and every listing in the truncated fallback, recursive and non-recursive alike.
- Rationale: the issue's own sketch is inconsistent here — its fast path prints `.tree[].path` (directories included) while its fallback emits blobs only, so the same repo would list differently depending on its size. The skill's purpose is "browse the file tree and read files": directory paths are derivable from file paths, and a submodule path is not readable through this repo's contents API, so emitting it would only invite a failing read. Symlinks are ordinary blobs and are included.
- Rejected: blobs plus trees (inconsistent-by-size unless both branches change, and adds unreadable entries); a type-annotated two-column output (breaks the plain `paths=$(...)` contract the SKILL will document).

### Strict stdout discipline and fail-fast errors

- Decision: stdout carries the path list and nothing else. Every diagnostic goes to stderr. On any failure the script prints one clear line to stderr, emits nothing on stdout, and exits non-zero. Distinguished failures: `gh` not on `PATH`; `gh` unauthenticated; missing or malformed `<owner/repo>` argument; repo not found (404 on the unscoped fetch); `path` not found (404) versus `path` naming a file rather than a directory (422); a non-recursive listing that is itself truncated; any other `gh api` failure (including rate-limit exhaustion), which is surfaced with `gh`'s own message rather than masked.
- Rationale: mirrors `run.sh`'s documented contract exactly — "only the executed binary's stdout may reach this script's stdout" — so `paths=$(bash "$TREE_SH" owner/repo)` always captures a clean list or nothing. A partial list on a mid-walk failure would be indistinguishable from a small repo, which is the failure mode the truncation check exists to prevent.
- Rejected: best-effort output with a stderr warning on partial failure (reintroduces silent partiality); swallowing `gh`'s error text behind a generic message (makes rate-limit and auth failures indistinguishable to the caller).

### Traversal-order output, no post-hoc sort

- Decision: paths are printed in traversal order — the API's own order on the fast path, and the breadth-first, siblings-in-API-order sequence specified in the FIFO-worklist decision above on the fallback path. No `sort` pass.
- Rationale: the GitHub tree API returns entries in git's sorted tree order, and a FIFO traversal over deterministic responses is itself deterministic, so tests can assert exact output without a sort. Skipping the sort also avoids buffering the whole listing. Note this means the fast path and the fallback path produce *different orderings* for the same hypothetical repo — acceptable because the two branches are selected by repo size and never both run for one repo, and because both are individually deterministic and assertable.
- Rejected: `LC_ALL=C sort` at the end (would reconcile the two branches' ordering, but costs full buffering for a property neither the skill nor its tests need).

### SKILL.md replaces the walk prose entirely

- Decision: the four tree-listing paragraphs in `SKILL.md` (branch resolve, recursive list, truncation check, fallback walk) collapse to a two-step block: resolve the script's absolute path, then call it. The raw `gh api` tree commands are removed, not kept as a documented fallback.
- Rationale: leaving the manual recipe in place invites the model to execute it and re-incur exactly the turn cost this task removes. Path resolution copies the `prowler` skill's step 1 verbatim in shape — `TREE_SH="$(cd "${CLAUDE_SKILL_DIR}/../../scripts" && pwd)/github-tree.sh"`, resolved while `${CLAUDE_SKILL_DIR}` is still set, because a dispatched subagent will not have it.
- Rejected: keeping the prose as a documented manual fallback (defeats the purpose); a `--help`-only reference in the skill (the skill must show the exact invocation).

### Offline harness with a stub `gh` that applies the real `--jq` expression

- Decision: add `plugins/prowler/scripts/github-tree-selftest.sh` — an offline harness that prepends a directory containing a stub `gh` executable to `PATH`, so the script under test makes no network calls. Fixtures live under `plugins/prowler/scripts/testdata/github-tree/`.
  The stub is **not** a plain `cat` of canned JSON. It parses the `gh api <endpoint> --jq <expr>` invocation it received, maps `<endpoint>` to a canned **JSON** fixture file, and runs the **actual `<expr>` it was passed** against that file using system `jq`, emitting the result. It also answers `gh auth status`, and honours per-endpoint canned failures (404/422/other) by writing a canned stderr line and exiting non-zero.
- Rationale: the script consumes a `--jq`-transformed `#trunc<TAB>…` / `type<TAB>sha<TAB>path` TSV stream, not raw JSON, so a stub that ignored `--jq` and printed JSON would exercise none of the script's actual input handling — and would leave the jq expression itself, the single most typo-prone line in the design, completely untested. Passing the real expression through real `jq` covers both. Fixtures stay JSON, which keeps them readable and lets one fixture serve both the recursive and non-recursive query of the same tree.
- **Consequences recorded deliberately:** (1) system `jq` becomes a dependency **of the test harness only** — never of `github-tree.sh`, which uses `gh`'s embedded gojq at runtime. The harness checks for `jq` up front and exits with a clear "install jq to run this harness" message rather than failing obscurely. (2) The harness therefore validates the expression under **jq**, while production runs it under **gojq**. The expressions used here are elementary (`.tree[]`, string concatenation, `tostring`, `select`) and behave identically in both, but this is a known, accepted seam, and any future expression using a jq/gojq-divergent construct would be validated against the wrong engine. It is called out here so a later change does not rediscover it as a surprise.
- Rejected: fixtures pre-transformed to TSV and keyed by jq expression (removes the network but also removes all coverage of the expression, and makes every fixture unreadable and unreusable across the two query shapes); Go tests via `os/exec` (would pull shell-script testing into the nested Go module, which is scoped to the fetcher); live-network integration tests (non-deterministic, rate-limited, and cannot force the truncated branch on command); manual-only verification (the fallback walk is the subtlest logic here and is exactly what a harness should pin).

## Technical context

- `plugins/prowler/scripts/run.sh` is the convention to mirror: self-locates `SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"` and `PLUGIN_ROOT="$SCRIPT_DIR/.."` with no dependency on `CLAUDE_PLUGIN_ROOT`; keeps stdout clean; sends every diagnostic to stderr. `github-tree.sh` needs the same self-location idiom and the same stdout rule. It does **not** need `run.sh`'s build lock, temp-file rename, or `PROWLER_BUILD_ONLY` machinery — there is nothing to build.
- `plugins/prowler/scripts/selftest.sh` is the harness convention: `SCRIPT_DIR`/`PLUGIN_ROOT` self-location, a `failures` counter, `fail()`/`pass()` helpers that keep going after a failure, `# --- Test N: ... ---` section comments, a `====` separator and a final PASS/FAIL summary line with a non-zero exit. Reuse this structure; do not invent a new one. Its header comment also documents *what is deliberately not asserted*, which `github-tree-selftest.sh` should mirror for its own manual-only cases.
- `plugins/prowler/skills/prowler/SKILL.md` step 1 is the exact model for skill-side script path resolution, including the reason (`${CLAUDE_SKILL_DIR}` is unset in a dispatched subagent).
- `plugins/prowler/skills/github-repo-explorer/SKILL.md` is the file being edited. Its `gh` hard-prerequisite paragraph, the read-file paragraph, and the `distill-subagent` pointer all stay; the four tree-listing paragraphs are replaced, and the `raw.githubusercontent.com` note's `{branch}` becomes `HEAD`.
- `plugins/prowler/README.md` documents `run.sh` under "Build-on-first-run". A short sibling section documenting `github-tree.sh` (what it does, why it exists, its one-call-per-browse property, its `gh`-only runtime dependency, and its test harness's extra `jq` dependency) belongs there.
- GitHub API facts verified live during exploration (`gh` 2.98.0):
  - `gh api --jq` accepts full jq expressions including comma-separated multi-output — no system `jq` needed at runtime.
  - `git/trees/HEAD` resolves to the default branch: identical results to `git/trees/trunk` on `cli/cli` (1824 entries) and to `git/trees/master` on `torvalds/linux` (41 root entries).
  - `git/trees/{ref}:{path}` accepts an arbitrarily nested directory path in one call (`master:drivers/net/ethernet` → 106 entries; `HEAD:drivers/net?recursive=1` → 7329 entries, untruncated).
  - A nonexistent path returns HTTP 404 `Not Found`; a path naming a file returns HTTP 422 `Invalid object requested. SHA must identify a commit or a tree.`
  - `repos/torvalds/linux/git/trees/master?recursive=1` returns `truncated: true` with 71638 entries; all 24 of its top-level subtrees return untruncated under `?recursive=1`.
  - Tree entry `type` values seen: `blob`, `tree`. `commit` (submodule) is possible and must be handled.
  - `https://raw.githubusercontent.com/cli/cli/HEAD/README.md` returns HTTP 200.
- Repo has **no** `.github/workflows/` and no shellcheck configuration; `selftest.sh` is not referenced by any automation. Verification is by running the harness directly.
- The `prowler` plugin has no module doc under `manifest/designs/` and no entry in `docs/overview.md`'s module table — it is a plugin, not an `internal/` module. Documentation for this change is `README.md` + `SKILL.md` only.

## Constraints

From `CONSTRAINTS.md`: every listed invariant (Cwd Resolution, Told-Geometry, Lyxdirs Single-Declarer, Durable-vs-Ephemeral State, Hub Containment, gitkit Leaf, hubforge Fabric-Fixture, Modelspec Leaf, Treadle Runner-Seam, Shed Producer-Seam, Shed Recipe Registry, CLI/Cobra, Documentation Lifecycle) governs Go code under `internal/` and the `lyx` CLI. This task touches only `plugins/prowler/` — shell scripts, a skill markdown file, and a plugin README — so none of them apply to the code being written, and no new cross-cutting invariant is introduced. `CONSTRAINTS.md` is therefore not edited.

From `CLAUDE.md` and the loaded `mill:conversation` rules, which do apply:

- **Markdown: semantic line breaks.** `SKILL.md` and `README.md` edits use one sentence per line, plain newlines, no fixed-column hard wrap.
- **No `sed`.** Neither the scripts nor any implementation step may use `sed`; use `Edit`/`Write`, or `awk`/`grep`/`cat`.
- **Never write to `/tmp`.** The harness creates any temp state under `.scratch/` or inside its own fixture directory, never a system temp dir.
- **Worktree isolation.** All work stays inside this worktree.
- **Docs land in the same commit** as the behaviour change (`SKILL.md` and `README.md` alongside the script).
- **No binaries committed** (repo `.gitignore` bans them) — irrelevant here, but it is why `run.sh` exists and why this script deliberately has no build step.

## Testing

TDD candidate: **`github-tree.sh`'s walk logic**, driven by `github-tree-selftest.sh`. Write the harness, the stub `gh`, and the fixtures first — the stub and its canned responses fully define the script's contract, and the truncated fallback is not observable any other way.

The stub `gh` (per the harness decision above) maps an endpoint to a JSON fixture, applies the `--jq` expression it was actually passed via system `jq`, answers `auth status`, and can be made to fail a named endpoint with a canned HTTP error. It appends every invocation to a log file so tests assert **call counts and call identity**, not only output — call count is the metric this task exists to move.

Scenarios the harness must cover:

- **Fast path, untruncated:** small repo fixture. Asserts the exact blob-path list, that `tree` entries are absent, and that exactly **one** `gh api` call was made — the regression guard against both the duplicate truncation-check call and the removed branch-resolve call.
- **Truncated fallback, one level:** root `?recursive=1` returns `truncated: true`; the root is re-listed non-recursively; each subtree `?recursive=1` returns untruncated. Asserts the root's **own root-level files** (e.g. `README.md`, `Makefile`, present only in the non-recursive listing) appear in the output, alongside the union of all subtree blobs, with no duplicates and nothing dropped.
- **Truncated fallback, two levels:** one subtree is itself truncated. Asserts that subtree is re-listed non-recursively, that its own directly-contained files are emitted, that its children are enqueued **from the non-recursive listing** — the fixture gives the truncated response strictly fewer subtrees than the non-recursive one, so a walk reading children from the truncated response provably loses a directory — and that untruncated sibling subtrees are not re-fetched.
- **Sibling order:** asserts the exact breadth-first, siblings-in-API-order sequence specified above, using a fixture where that order differs from both git-sorted order and LIFO order, so a stack implementation or a stray `sort` fails the test.
- **Path scoping, fast path:** `<repo> src` issues exactly one call, to `git/trees/HEAD:src?recursive=1`, and emits repo-relative `src/...` paths (asserting the prefix is re-applied), not paths relative to `src`.
- **Path scoping, truncated path:** the scoped tree truncates and the fallback engages beneath it; asserts prefixes stay repo-relative at depth and that no sibling of `src` is ever fetched (call-log assertion).
- **Entry types:** a fixture containing a `commit` (submodule) entry and a symlink blob. Asserts the submodule path is absent and the symlink path is present.
- **Errors, each asserting non-zero exit and empty stdout:** `gh` missing from `PATH`; `gh auth status` failing; missing/malformed `<owner/repo>`; 404 on the repo; `path` 404 (not found) and `path` 422 (names a file) reported as *distinct* stderr messages; a non-recursive listing that is itself truncated; and a mid-walk `gh api` failure on one subtree. The mid-walk case is the most important — it pins the "never silently partial" promise.
- **stdout cleanliness:** on the success paths, assert stdout contains only path lines — no `#trunc` header, no diagnostics.
- **Harness prerequisite:** asserts the harness's own up-front `jq`-missing message rather than failing obscurely.

Manual, documented-not-asserted (mirroring `selftest.sh`'s own "NOT covered here" note): one live run against a small public repo and one against `torvalds/linux`, confirming the real truncated fallback completes in a single invocation and produces a plausible full listing; and a spot-check that the jq expression behaves identically under `gh`'s embedded gojq and the harness's system `jq`.

## Q&A log

- **Q:** Bash script or a new Go subcommand on `bin/prowler`? **A:** [auto-pick] Bash `github-tree.sh`. **Why:** `gh` is already a hard prerequisite so bash adds no runtime dependency; a Go subcommand would mix GitHub-tree-walking into a binary scoped to headless web fetching, and there is no existing Go code to build on either way.
- **Q:** Depend on system `jq` at runtime as the issue sketch does, or extract everything through `gh api --jq`? **A:** [auto-pick] `gh api --jq` only. **Why:** `gh` embeds jq (verified on 2.98.0 with a full expression), keeping the runtime dependency set at exactly `gh` and avoiding three `jq` processes per tree entry.
- **Q:** Keep the issue's per-directory non-recursive fallback walk, or fetch each subtree with `recursive=1` and descend only where still truncated? **A:** [auto-pick] Recursive-per-subtree. **Why:** Same loop, one query-string change; verified to cover `torvalds/linux` in 26 total API calls versus several thousand for the naive walk, keeping the run inside the authenticated rate limit.
- **Q:** Implement the fallback as a recursive shell function (per the sketch) or an explicit worklist? **A:** [auto-pick] Explicit worklist. **Why:** The sketch's `... | while read` puts the recursion in a pipeline subshell where a failed `gh api` cannot abort the script — an explicit queue keeps every iteration in the main shell so errors fail the run cleanly.
- **Q:** Emit directory (`tree`) paths alongside files, as the sketch's fast path does? **A:** [auto-pick] Blob paths only, in both branches. **Why:** The sketch is inconsistent — the same repo would list differently depending on whether it truncated; directories are derivable from file paths, and submodule (`commit`) entries are not readable via this repo's contents API.
- **Q:** Implement the advertised `[path]` argument, or drop it? **A:** [auto-pick] Implement it. **Why:** The skill's `argument-hint` already advertises it, and it is nearly free — `git/trees/{ref}:{path}` scopes the walk in a single call at any nesting depth. A `path` that is not a directory is a hard error (the API returns a distinct 422), so a typo can never read as "this directory is empty".
- **Q:** Add an optional `[ref]` argument for browsing a non-default branch or tag? **A:** [auto-pick] No ref argument. **Why:** YAGNI — the skill has only ever browsed the default branch, nothing has asked for a ref, and a third positional would collide with the `[question]` slot in the skill's own argument hint.
- **Q:** On a mid-walk API failure, emit what was collected with a warning, or fail the whole run? **A:** [auto-pick] Fail the whole run: empty stdout, one stderr line, non-zero exit. **Why:** A partial list is indistinguishable from a small repo, which is precisely the silent-partiality the truncation check exists to prevent; `run.sh`'s stdout contract is the same.
- **Q:** Sort the output for cross-branch determinism? **A:** [auto-pick] No sort, traversal order. **Why:** The API returns git-sorted entries and the FIFO traversal is deterministic, so tests can assert exact output without buffering the whole listing; the two branches never both run for one repo.
- **Q:** Keep the raw `gh api` tree commands in `SKILL.md` as a documented manual fallback? **A:** [auto-pick] Remove them entirely. **Why:** Leaving the manual recipe invites the model to execute it and re-incur exactly the per-subdirectory turn cost this task exists to remove.
- **Q:** How is a bash script tested in a repo with no CI? **A:** [auto-pick] An offline shell harness with a stub `gh` on `PATH`, mirroring `scripts/selftest.sh`. **Why:** `selftest.sh` is the established pattern here; a stub `gh` makes the truncated fallback — the subtlest logic, and one a live API will not produce on demand — fully assertable, including call counts.
- **Q:** Does `CONSTRAINTS.md` need a new invariant, or `manifest/roadmap.md` a move? **A:** [auto-pick] Neither. **Why:** Every current invariant governs `internal/` Go code and the `lyx` CLI; this task touches only `plugins/prowler/`, introduces no cross-cutting rule, and is an issue-sourced plugin enhancement rather than a planned roadmap item. Docs for it are `SKILL.md` + `README.md`.
- **Q (r2 gap):** Are blob entries in the fallback's non-recursive listings emitted? **A:** Yes, explicitly — the root's and every re-listed subtree's own files are emitted at that step. **Why:** Those files appear in no other response, so omitting them was a silently-partial listing contradicting the design's own promise. A fixture now asserts a root-level file survives the truncated path.
- **Q (r2 gap):** For a subtree that is itself truncated, where does its child list come from? **A:** From a non-recursive re-fetch of that subtree, never from the truncated response. **Why:** A truncated response is an incomplete entry list by definition, so children past the cap would be lost. The rule is now stated uniformly for root and every depth, and a two-level fixture with strictly fewer subtrees in the truncated response than the non-recursive one proves the difference.
- **Q (r2 gap):** How is `[path]` resolved to a tree, and what does it actually cost? **A:** It is not resolved separately at all — the path becomes a tree-ish suffix on the ref (`git/trees/HEAD:<path>`), verified to work at arbitrary nesting in one call. The earlier "27 calls to 2" claim was wrong and is replaced with measured counts (1 / 26 / 1). **Why:** The reviewer was right that no mechanism was named and that the arithmetic did not match the algorithm; investigating it produced a cheaper mechanism than any of the candidates, and separately showed `HEAD` removes the branch-resolve call entirely.
- **Q (r2 NIT):** How does a stub `gh` that `cat`s JSON exercise a script that reads `--jq` output? **A:** It does not — the stub instead runs the real `--jq` expression it was passed against the JSON fixture using system `jq`. **Why:** Otherwise the jq expression, the most typo-prone line in the design, would be entirely untested. Accepted consequences recorded: `jq` becomes a test-harness-only dependency, and the expression is validated under jq while production runs it under gojq.
- **Q (r2 NIT):** Does `docs/benchmarks/` support the turn-count claim? **A:** No — its four files cover board writes, test-suite timing, fixture copy, and running tests. The citation is dropped and the rationale restated without it. **Why:** The argument stands on its own; a citation pointing at unrelated measurements is worse than none.
- **Q (r2 NIT):** Stack or queue, and which sibling order do the fixtures pin? **A:** FIFO queue; output is breadth-first with siblings in API order, asserted by a fixture where that differs from both git-sorted and LIFO order. **Why:** A stack reverses sibling order for no benefit, and "traversal order" was too vague to write a test against.
