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

Turn count, not tool latency, is the dominant driver of agent wall-clock time in this project (see `docs/benchmarks/`), so a skill that makes an LLM re-execute a branching, potentially-recursive algorithm one tool call at a time is exactly the pattern worth collapsing.
The fix is a deterministic script that does the whole walk internally and prints one complete path list, turning N+3 turns into exactly 1 regardless of repo size.

## Scope

**In:**

- New `plugins/prowler/scripts/github-tree.sh` — resolves the default branch, fetches the recursive tree once, falls back internally when truncated, prints one newline-separated list of file paths to stdout.
- New offline shell harness `plugins/prowler/scripts/github-tree-selftest.sh` plus fixtures under `plugins/prowler/scripts/testdata/github-tree/`.
- Rewrite of the tree-listing portion of `plugins/prowler/skills/github-repo-explorer/SKILL.md` to a single script invocation.
- A `github-tree.sh` section in `plugins/prowler/README.md`, beside the existing "Build-on-first-run" section that documents `run.sh`.

**Out:**

- The `read-file` verb in `SKILL.md` — already a single `gh api .../contents/{path}` call with no branching. Untouched, including the `raw.githubusercontent.com` lighter-alternative note and the `distill-subagent` pointer.
- The Go module (`plugins/prowler/*.go`), `bin/prowler`, `scripts/run.sh`, and `scripts/selftest.sh`. This task adds a sibling script; it does not modify the build-on-first-run wrapper or its harness.
- Any MCP server. Explicitly rejected in the source issue (#217) and not reopened here.
- Reading file *content* in bulk, caching, or any persistence. The script lists paths and exits.
- `plugins/prowler/skills/INDEX.md` — the skill's `description` frontmatter does not change, so the generated index row does not either.
- `manifest/roadmap.md` — this is a plugin-local enhancement sourced from a GitHub issue, not a planned roadmap item.

## Decisions

### Bash script, not a Go subcommand

- Decision: implement as `plugins/prowler/scripts/github-tree.sh`, a standalone bash script invoked as `bash "$TREE_SH" <owner/repo> [path]`.
- Rationale: `gh` is already a documented hard prerequisite of this skill, so bash adds no new dependency. It mirrors the existing `scripts/run.sh` convention (self-locating from `$0`, strict stdout discipline). A Go subcommand would drag an unrelated concern — GitHub API tree-walking — into a binary otherwise scoped to headless web fetching, and would put the listing behind a Go toolchain and a first-run compile that this script does not need.
- Rejected: a new subcommand on `bin/prowler` (better JSON handling and Go test conventions, but net-new code either way, plus a scope violation of the binary's purpose); an MCP server (rejected in the issue — fixed protocol/process/wiring cost unjustified for a two-argument, occasionally-used verb).

### `gh api --jq` only — no system `jq` dependency

- Decision: every JSON field is extracted with `gh api --jq '<expr>'`. The script never pipes to a standalone `jq` binary, and `jq` is not added to the skill's prerequisites.
- Rationale: `gh` embeds a jq implementation (gojq); `--jq` accepts full jq expressions including multi-output ones. Verified on `gh version 2.98.0`: `gh api repos/cli/cli --jq '"branch\t" + .default_branch'` prints `branch<TAB>trunk`. Using it keeps the dependency set at exactly `gh`, matching the skill's existing "hard prerequisite" wording, and avoids the issue sketch's cost of spawning three `jq` processes per tree entry.
- Rejected: the issue sketch's `jq -r ... <<<"$resp"` shape (adds a real second dependency and is process-per-entry expensive on large trees).

### One API response, both fields — combined `--jq` stream

- Decision: each tree fetch is a single `gh api` call whose `--jq` expression emits a header line carrying the truncation flag followed by one tab-separated `type<TAB>sha<TAB>path` line per entry, e.g.

  ```
  --jq '"#trunc\t" + (.truncated|tostring), (.tree[] | .type + "\t" + .sha + "\t" + .path)'
  ```

  The script reads the header, then the entries, from that one response.
- Rationale: this is the direct fix for the issue's "redundant duplicate call against the `recursive=1` endpoint" — both `.truncated` and `.tree` come from one request. Tab-separated output is safe because git tree paths cannot contain a tab (git rejects it) and cannot contain a newline in practice; a single `read -r` loop parses it with no per-entry subprocess. Verified shape against `cli/cli`.
- Rejected: two calls to the same endpoint (today's behaviour, the thing being removed); JSON-per-line output requiring a parser.

### Truncated fallback walks with `recursive=1` per subtree, descending only into subtrees still truncated

- Decision: when the root recursive fetch reports `truncated: true`, list the root tree non-recursively, then fetch each subtree with `?recursive=1`. Emit that subtree's blobs when it comes back untruncated; push its own subtrees onto the worklist only when it is itself truncated. Repeat until the worklist drains.
- Rationale: strictly better than the issue's per-directory non-recursive walk, at no extra complexity — the same loop, with `?recursive=1` appended. Verified against `torvalds/linux`: the root recursive fetch truncates at 71638 entries, but all 24 top-level subtrees come back untruncated under `recursive=1` (largest: `drivers` at 40492 entries). Total: 1 branch resolve + 1 root recursive + 1 root non-recursive + 24 subtree calls = 27 API calls, all inside one script invocation. The naive per-directory walk would be several thousand calls. Because the whole walk is internal, this is one agent turn either way — but it also keeps the run well inside GitHub's 5000/hr authenticated rate limit, which the naive walk would not.
- Rejected: per-directory non-recursive descent (the issue's sketch — correct but pathologically call-heavy); giving up and returning a partial list with a warning (the SKILL's existing text explicitly promises "never silently partial").

### Iterative worklist, not shell recursion

- Decision: the fallback walk uses an explicit worklist (a bash array used as a stack of `sha<TAB>prefix` pairs) driven by a `while` loop, not a recursive shell function.
- Rationale: the issue sketch's `jq -c '.tree[]' <<<"$(gh api ...)" | while read` puts the loop body — including the recursive call — in a pipeline subshell, where a failed `gh api` cannot abort the script and no state escapes. An explicit worklist keeps every iteration in the main shell, so a single failed API call fails the whole run cleanly, and there is no recursion depth to reason about.
- Rejected: the sketch's recursive `walk()` function (subshell-scoped, error-swallowing); process substitution around a recursive function (fixes the subshell but keeps the depth question for no gain).

### Blob paths only, in both branches

- Decision: output is file paths only — `type == "blob"` entries. `tree` entries are never emitted, and `commit` entries (submodule gitlinks) are skipped silently. This holds identically on the fast path and the truncated fallback.
- Rationale: the issue's own sketch is inconsistent here — its fast path prints `.tree[].path` (directories included) while its fallback emits blobs only, so the same repo would list differently depending on its size. The skill's purpose is "browse the file tree and read files": directory paths are derivable from file paths, and a submodule path is not readable through this repo's contents API, so emitting it would only invite a failing read. Symlinks are ordinary blobs and are included.
- Rejected: blobs plus trees (inconsistent-by-size unless both branches change, and adds unreadable entries); a type-annotated two-column output (breaks the plain `paths=$(...)` contract the SKILL will document).

### Optional `[path]` argument is a subtree scope, applied before the walk

- Decision: `github-tree.sh <owner/repo> [path]` accepts an optional repo-relative directory path. On the fast path it filters the recursive listing to entries under that prefix. On the truncated path it resolves that directory to its tree SHA and starts the walk there, so untouched subtrees are never fetched. An empty or omitted `path` means the whole repo. A `path` that does not resolve to a directory is an error, not an empty listing.
- Rationale: the skill's `argument-hint` already advertises `<owner/repo> [path]`, and the issue's sketch header documents it while the body ignores it. On a truncated repo, scoping to one subtree is the difference between 27 calls and 2. Making a bad path a hard error rather than empty output prevents a silent "this directory has no files" answer to a typo.
- Rejected: dropping the argument (contradicts the advertised interface and forgoes the largest saving on exactly the repos this task targets); treating a bad path as an empty result (silently wrong).

### No explicit ref/branch argument

- Decision: the script always resolves and uses the repository's default branch. There is no third argument, flag, or environment variable for a ref.
- Rationale: YAGNI. The skill has only ever browsed the default branch, the `argument-hint` has no ref slot, and no caller has asked for one. Adding it now would widen the tested surface for a hypothetical.
- Rejected: an optional `[ref]` third argument (cheap to add, but unused, and it collides with the `[question]` slot in the skill's own argument hint).

### Strict stdout discipline and fail-fast errors

- Decision: stdout carries the path list and nothing else. Every diagnostic goes to stderr. On any failure the script prints one clear line to stderr, emits nothing on stdout, and exits non-zero. Distinguished failures: `gh` not on `PATH`; `gh` unauthenticated; missing or malformed `<owner/repo>` argument; repo or branch not found (404); `path` not found or not a directory; any other `gh api` failure (including rate-limit exhaustion), which is surfaced with `gh`'s own message rather than masked.
- Rationale: mirrors `run.sh`'s documented contract exactly — "only the executed binary's stdout may reach this script's stdout" — so `paths=$(bash "$TREE_SH" owner/repo)` always captures a clean list or nothing. A partial list on a mid-walk failure would be indistinguishable from a small repo, which is the failure mode the truncation check exists to prevent.
- Rejected: best-effort output with a stderr warning on partial failure (reintroduces silent partiality); swallowing `gh`'s error text behind a generic message (makes rate-limit and auth failures indistinguishable to the caller).

### Traversal-order output, no post-hoc sort

- Decision: paths are printed in traversal order — the API's own order on the fast path, DFS over each response's order on the fallback. No `sort` pass.
- Rationale: the GitHub tree API returns entries in git's sorted tree order, and a deterministic worklist traversal over deterministic responses is itself deterministic, so tests can assert exact output without a sort. Skipping the sort also avoids buffering the whole listing.
- Rejected: `LC_ALL=C sort` at the end (would make fast-path and fallback output identical for the same repo, but costs full buffering for a property neither the skill nor its tests need — the two branches never both run for one repo).

### SKILL.md replaces the walk prose entirely

- Decision: the four tree-listing paragraphs in `SKILL.md` (branch resolve, recursive list, truncation check, fallback walk) collapse to a two-step block: resolve the script's absolute path, then call it. The raw `gh api` tree commands are removed, not kept as a documented fallback.
- Rationale: leaving the manual recipe in place invites the model to execute it and re-incur exactly the turn cost this task removes. Path resolution copies the `prowler` skill's step 1 verbatim in shape — `TREE_SH="$(cd "${CLAUDE_SKILL_DIR}/../../scripts" && pwd)/github-tree.sh"`, resolved while `${CLAUDE_SKILL_DIR}` is still set, because a dispatched subagent will not have it.
- Rejected: keeping the prose as a documented manual fallback (defeats the purpose); a `--help`-only reference in the skill (the skill must show the exact invocation).

### Offline shell harness with a stub `gh`, mirroring `selftest.sh`

- Decision: add `plugins/prowler/scripts/github-tree-selftest.sh` — an offline harness that prepends a fixture directory containing a stub `gh` executable to `PATH`, so the script under test makes no network calls. Fixtures live under `plugins/prowler/scripts/testdata/github-tree/`. Same conventions as the existing `selftest.sh`: numbered assertions, `pass`/`fail` helpers that keep going, PASS/FAIL summary, non-zero exit on any failure. Not a `*_test.go` file.
- Rationale: this repo has no CI workflows and `selftest.sh` is already the established pattern for exercising a plugin shell script deterministically and offline. A stub `gh` that dispatches on its argument string and cats a canned JSON file makes both the fast path and the multi-level truncated fallback fully assertable, including the branches a live API would almost never produce on demand.
- Rejected: Go tests via `os/exec` (would pull shell-script testing into the nested Go module, which is scoped to the fetcher); live-network integration tests (non-deterministic, rate-limited, and cannot force the truncated branch on command); manual-only verification (the fallback walk is the subtlest logic here and is exactly what a harness should pin).

## Technical context

- `plugins/prowler/scripts/run.sh` is the convention to mirror: self-locates `SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"` and `PLUGIN_ROOT="$SCRIPT_DIR/.."` with no dependency on `CLAUDE_PLUGIN_ROOT`; keeps stdout clean; sends every diagnostic to stderr. `github-tree.sh` needs the same self-location idiom (for locating nothing today, but for convention and future fixture use) and the same stdout rule. It does **not** need `run.sh`'s build lock, temp-file rename, or `PROWLER_BUILD_ONLY` machinery — there is nothing to build.
- `plugins/prowler/scripts/selftest.sh` is the harness convention: `SCRIPT_DIR`/`PLUGIN_ROOT` self-location, `failures` counter, `fail()`/`pass()` helpers, `--- Test N: ... ---` section comments, a `=====` separator and a final PASS/FAIL summary line. Reuse this structure; do not invent a new one.
- `plugins/prowler/skills/prowler/SKILL.md` step 1 is the exact model for skill-side script path resolution, including the reason (`${CLAUDE_SKILL_DIR}` is unset in a dispatched subagent).
- `plugins/prowler/skills/github-repo-explorer/SKILL.md` is the file being edited. Its `gh` hard-prerequisite paragraph, the read-file paragraph, the `raw.githubusercontent.com` note, and the `distill-subagent` pointer all stay; only the four tree-listing paragraphs are replaced.
- `plugins/prowler/README.md` documents `run.sh` under "Build-on-first-run". A short sibling section documenting `github-tree.sh` (what it does, why it exists, its one-call-per-browse property, its `gh`-only dependency) belongs there.
- GitHub API facts verified during exploration (`gh` 2.98.0):
  - `gh api --jq` accepts full jq expressions including comma-separated multi-output — no system `jq` needed.
  - `repos/torvalds/linux/git/trees/master?recursive=1` returns `truncated: true` with 71638 entries.
  - Every one of linux's 24 top-level subtrees returns untruncated under `?recursive=1` (`drivers` = 40492, `arch` = 19618, `Documentation` = 12178, `tools` = 10670).
  - Tree entry `type` values seen: `blob`, `tree`. `commit` (submodule) is possible and must be handled.
- Repo has **no** `.github/workflows/` and no shellcheck configuration; `selftest.sh` is not referenced by any automation. Verification is by running the harness directly.
- The `prowler` plugin has no module doc under `manifest/designs/` and no entry in `docs/overview.md`'s module table — it is a plugin, not an `internal/` module. Documentation for this change is `README.md` + `SKILL.md` only.

## Constraints

From `CONSTRAINTS.md`: every listed invariant (Cwd Resolution, Told-Geometry, Lyxdirs Single-Declarer, Durable-vs-Ephemeral State, Hub Containment, gitkit Leaf, hubforge Fabric-Fixture, Modelspec Leaf, Treadle Runner-Seam, Shed Producer-Seam, Shed Recipe Registry, CLI/Cobra, Documentation Lifecycle) governs Go code under `internal/` and the `lyx` CLI. This task touches only `plugins/prowler/` — shell scripts, a skill markdown file, and a plugin README — so none of them apply to the code being written, and no new cross-cutting invariant is introduced. `CONSTRAINTS.md` is therefore not edited.

From `CLAUDE.md` and the loaded `mill:conversation` rules, which do apply:

- **Markdown: semantic line breaks.** `SKILL.md` and `README.md` edits use one sentence per line, plain newlines, no fixed-column hard wrap.
- **No `sed`.** Neither the scripts nor any implementation step may use `sed`; use `Edit`/`Write`, or `awk`/`grep`/`cat`.
- **Never write to `/tmp`.** The selftest harness creates its temp state under `.scratch/` or under its own fixture directory, never a system temp dir.
- **Worktree isolation.** All work stays inside this worktree.
- **Docs land in the same commit** as the behaviour change (`SKILL.md` and `README.md` alongside the script).
- **No binaries committed** (repo `.gitignore` bans them) — irrelevant here, but it is why `run.sh` exists and why this script deliberately has no build step.

## Testing

TDD candidate: **`github-tree.sh`'s walk logic**, driven by `github-tree-selftest.sh`. Write the harness and its fixtures first — the stub `gh` and its canned responses fully define the script's contract, and the truncated fallback is not observable any other way.

The stub `gh`: an executable shell script on `PATH` ahead of the real one, dispatching on its joined arguments to `cat` a canned JSON file from the fixture set, and exiting non-zero with a canned stderr message for the error fixtures. It must also answer `auth status`. Record each invocation to a log file so tests can assert *call counts*, not only output — call count is the entire point of this task.

Scenarios the harness must cover:

- **Fast path, untruncated:** small repo fixture. Asserts the exact blob-path list, that `tree` entries are absent, and that the `git/trees/...?recursive=1` endpoint is hit exactly **once** (the regression guard against the duplicate truncation-check call).
- **Truncated fallback, one level:** root `recursive=1` returns `truncated: true`; root non-recursive lists subtrees; each subtree `recursive=1` returns untruncated. Asserts the union of all subtree blobs is emitted, in traversal order, with no duplicates and nothing dropped.
- **Truncated fallback, two levels:** one subtree is itself truncated, forcing a second descent. Asserts the deeper blobs appear and that untruncated sibling subtrees are *not* re-fetched.
- **Path prefix on the fast path:** `<repo> src` emits only paths under `src/`, and does not emit `src` itself.
- **Path prefix on the truncated path:** the walk starts at that subtree's SHA; asserts sibling subtrees are never fetched (call-log assertion).
- **Entry types:** a fixture containing a `commit` (submodule) entry and a symlink blob. Asserts the submodule path is absent and the symlink path is present.
- **Errors, each asserting non-zero exit and empty stdout:** `gh` missing from `PATH`; `gh auth status` failing; missing/malformed `<owner/repo>`; 404 on the repo; `path` that resolves to a blob rather than a directory; `path` that does not exist; a mid-walk `gh api` failure on a subtree (must fail the whole run, not emit a partial list). The mid-walk case is the most important — it is the assertion that pins the "never silently partial" promise.
- **stdout cleanliness:** on the success paths, assert stdout contains only path lines — no `#trunc` header, no diagnostics.

Manual, documented-not-asserted (mirroring `selftest.sh`'s own "NOT covered here" note): one live run against a small public repo and one against `torvalds/linux`, confirming the real truncated fallback completes in a single invocation and produces a plausible full listing.

## Q&A log

- **Q:** Bash script or a new Go subcommand on `bin/prowler`? **A:** [auto-pick] Bash `github-tree.sh`. **Why:** `gh` is already a hard prerequisite so bash adds no dependency; a Go subcommand would mix GitHub-tree-walking into a binary scoped to headless web fetching, and there is no existing Go code to build on either way.
- **Q:** Depend on system `jq` as the issue sketch does, or extract everything through `gh api --jq`? **A:** [auto-pick] `gh api --jq` only. **Why:** `gh` embeds jq (verified on 2.98.0 with a full expression), keeping the dependency set at exactly `gh` and avoiding three `jq` processes per tree entry.
- **Q:** Keep the issue's per-directory non-recursive fallback walk, or fetch each subtree with `recursive=1` and descend only where still truncated? **A:** [auto-pick] Recursive-per-subtree. **Why:** Same loop, one query-string change; verified to cover `torvalds/linux` in 27 total API calls versus several thousand for the naive walk, keeping the run inside the authenticated rate limit.
- **Q:** Implement the fallback as a recursive shell function (per the sketch) or an explicit worklist? **A:** [auto-pick] Explicit worklist. **Why:** The sketch's `... | while read` puts the recursion in a pipeline subshell where a failed `gh api` cannot abort the script — an explicit stack keeps every iteration in the main shell so errors fail the run cleanly.
- **Q:** Emit directory (`tree`) paths alongside files, as the sketch's fast path does? **A:** [auto-pick] Blob paths only, in both branches. **Why:** The sketch is inconsistent — the same repo would list differently depending on whether it truncated; directories are derivable from file paths, and submodule (`commit`) entries are not readable via this repo's contents API.
- **Q:** Implement the advertised `[path]` argument, or drop it? **A:** [auto-pick] Implement it as a subtree scope. **Why:** The skill's `argument-hint` already advertises it, and on a truncated repo it cuts the walk from ~27 calls to 2. A `path` that is not a directory is a hard error, so a typo can never read as "this directory is empty".
- **Q:** Add an optional `[ref]` argument for browsing a non-default branch or tag? **A:** [auto-pick] No ref argument. **Why:** YAGNI — the skill has only ever browsed the default branch, nothing has asked for a ref, and a third positional would collide with the `[question]` slot in the skill's own argument hint.
- **Q:** On a mid-walk API failure, emit what was collected with a warning, or fail the whole run? **A:** [auto-pick] Fail the whole run: empty stdout, one stderr line, non-zero exit. **Why:** A partial list is indistinguishable from a small repo, which is precisely the silent-partiality the truncation check exists to prevent; `run.sh`'s stdout contract is the same.
- **Q:** Sort the output for cross-branch determinism? **A:** [auto-pick] No sort, traversal order. **Why:** The API returns git-sorted entries and the worklist traversal is deterministic, so tests can assert exact output without buffering the whole listing; the two branches never both run for one repo.
- **Q:** Keep the raw `gh api` tree commands in `SKILL.md` as a documented manual fallback? **A:** [auto-pick] Remove them entirely. **Why:** Leaving the manual recipe invites the model to execute it and re-incur exactly the per-subdirectory turn cost this task exists to remove.
- **Q:** How is a bash script tested in a repo with no CI? **A:** [auto-pick] An offline shell harness with a stub `gh` on `PATH`, mirroring `scripts/selftest.sh`. **Why:** `selftest.sh` is the established pattern here; a stub `gh` makes the truncated fallback — the subtlest logic, and one a live API will not produce on demand — fully assertable, including call counts, which are the metric this task is about.
- **Q:** Does `CONSTRAINTS.md` need a new invariant, or `manifest/roadmap.md` a move? **A:** [auto-pick] Neither. **Why:** Every current invariant governs `internal/` Go code and the `lyx` CLI; this task touches only `plugins/prowler/`, introduces no cross-cutting rule, and is an issue-sourced plugin enhancement rather than a planned roadmap item. Docs for it are `SKILL.md` + `README.md`.
