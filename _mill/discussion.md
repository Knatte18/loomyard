# Discussion: Prefer raw fetch, scope large tree listings

```yaml
task: Prefer raw fetch, scope large tree listings
slug: raw-fetch-tree-scoping
status: discussing
parent: main
```

## Problem

Commit `63916b1e2` (prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call) replaced an N-call, model-driven `gh api` tree walk with a single deterministic `github-tree.sh` invocation.
Hands-on testing of that commit against a real repository (`Knatte18/quarry`) surfaced two follow-up gaps that the commit did not address — one measured, one architectural.

The measured gap: `plugins/prowler/skills/github-repo-explorer/SKILL.md` tells the agent to read files with `gh api "repos/{owner}/{repo}/contents/{path}" --jq .content | base64 -d`, and mentions `raw.githubusercontent.com` only as a "lighter alternative for public files".
A controlled benchmark (6 files spanning 1KB-43KB, 3 trials each) measured `raw.githubusercontent.com` at a flat ~28-32ms per file once the connection is warm, against ~360-530ms for `gh api .../contents` — and the `gh api` figure *grows* with file size, because base64 inflates the payload by ~33% on top of the JSON parse.
That is a 10-15x gap on the operation the skill performs most often, and the skill currently recommends the slow side of it.

The architectural gap: `github-tree.sh` has exactly two behaviours — "whole repo, recursive" and "one path, still recursive under it".
Against a repository with tens of thousands of files, an unscoped call prints every path to stdout in one shot, and a caller that reads that into context swallows the whole listing — potentially tens of thousands of tokens — with no warning that it was about to happen.
The script has no top-down mode an agent could use to explore a directory at a time, and no ceiling that would force scoping rather than silently returning an enormous listing.

**Why now:** both gaps were found by using the just-merged commit against a real repository, while the design is still fresh and the script has exactly one consumer.
The read-order flip is a measured, uncontested win being left on the table on every file read; the listing ceiling is a latent context blow-up that has not fired yet only because the repos exercised so far have been small.

## Scope

**In:**

- A new `plugins/prowler/scripts/github-read.sh` that reads one file's content to stdout, trying `raw.githubusercontent.com` first and falling back to `gh api` only on failure.
- A new `--children` mode on `plugins/prowler/scripts/github-tree.sh` that lists one path's direct children (files and directories) without recursing.
- A hard entry-count guard on `github-tree.sh`, default 1000, overridable with `--max-entries <N>` (`0` = unlimited), applied uniformly to every mode.
- New offline test coverage: additional scenarios in `plugins/prowler/scripts/github-tree-selftest.sh` (plus any fixture bodies they need under `plugins/prowler/scripts/testdata/github-tree/bodies/`), and a new sibling harness `plugins/prowler/scripts/github-read-selftest.sh` with its own stub `curl` and stub `gh`.
- `plugins/prowler/skills/github-repo-explorer/SKILL.md`: flip the documented file-read order to `github-read.sh`, and add guidance on choosing between `--children`, scoped-recursive, and whole-repo listing.
- `plugins/prowler/README.md`: a section for `github-read.sh`, and the existing `github-tree.sh` section extended for the new mode and the guard.

**Out:**

- Partial or range reads of huge files. Explicitly out of scope per the task body.
- Binary-file handling. Explicitly out of scope per the task body. `github-read.sh` writes whatever bytes it receives to stdout and makes no attempt to detect or refuse binary content.
- A file-size guard on `github-read.sh`. Considered and deliberately deferred — see the Decision "No file-size guard on github-read.sh".
- Cross-repo code search via `gh api search/code`. Raised in the same discussion and already split into its own separate task.
- Any change to `plugins/prowler`'s Go code (`fetch.go`, `adapter.go`, the browser/Reddit paths, etc.). This task touches shell scripts, skill markdown, and the plugin README only.
- Any change to `plugins/prowler/skills/INDEX.md`. It indexes *skills*, and this task adds a script, not a skill.
- Any change to `plugins/prowler/skills/prowler/SKILL.md` or `distill-subagent/SKILL.md`.
- Retries and backoff anywhere in either script. `github-tree.sh`'s header states the no-retry policy as a deliberate design decision, not an omission; `github-read.sh` adopts the same policy.
- A `manifest/designs/` module doc. No prowler design doc exists today and this change does not warrant creating one.

## Decisions

### Raw-first file reads ship as a script, not as SKILL.md prose

- Decision: add `plugins/prowler/scripts/github-read.sh <owner/repo> <path>`, which writes the file's content to stdout.
  SKILL.md's file-read instruction becomes "call this script", not "try raw, then fall back".
- Rationale: prose puts the fallback on the agent's discipline — it must remember to try raw first on every single read, detect the failure, and compose the fallback command correctly, each time, under context pressure.
  A script makes the preference order structural.
  It also mirrors the exact reasoning that produced `github-tree.sh` in commit `63916b1e2`: the raw-then-fallback sequence contains no decision a model actually needs to make.
- Rejected: SKILL.md prose only — cheaper to build and zero new test surface, but the measured win is then realised only when the agent happens to comply.
  Rejected: script *plus* prose fallback commands — two sources of truth for the same operation, and the prose copy rots.

### The `gh api` fallback requests raw content, never base64

- Decision: the fallback is `gh api "repos/{owner}/{repo}/contents/{path}" -H "Accept: application/vnd.github.raw"`, whose response body is the file content itself.
  The `--jq .content | base64 -d` form currently documented in SKILL.md is not used anywhere.
- Rationale: base64 inflates the transferred payload by ~33% and adds a JSON parse plus a decode step, for no benefit — the Accept header gets the same bytes over the same authenticated path with none of that overhead.
  This is strictly better even in the fallback position, which is the only position it occupies.
- Rejected: keeping `--jq .content | base64 -d` because that is what the benchmark measured.
  The benchmark measured it as the *baseline being replaced*; there is no reason to preserve its inefficiency in the fallback.

### Non-recursive listing is a `--children` flag on `github-tree.sh`, not a second script

- Decision: `github-tree.sh [--children] [--max-entries N] <owner/repo> [path]`.
  With `--children`, the script performs exactly one non-recursive tree fetch of the addressed path and emits its direct children only.
- Rationale: one script, one prerequisite check, one error vocabulary, one harness, and one thing for SKILL.md to explain.
  The non-recursive fetch is already implemented inside `github-tree.sh` as the truncation-fallback path, so `--children` reuses the existing `fetch()` helper rather than duplicating it.
- Rejected: a separate `github-children.sh` — leaves `github-tree.sh` untouched, but at the cost of a second script, a second harness, and a third listing concept in SKILL.md.

### `--children` emits directories with a trailing `/`

- Decision: in `--children` mode, a `blob` entry is emitted as `<prefix><name>` and a `tree` entry as `<prefix><name>/`.
  `commit` entries (submodules) continue to be skipped silently, as in the recursive modes.
  The recursive modes' output contract is unchanged: they still emit blob paths only, with no trailing-slash entries ever.
- Rationale: the entire purpose of a top-down mode is telling the caller what it can descend into.
  A children listing that hides directories cannot be walked; a children listing that shows them unmarked forces a second call per entry to find out which is which.
  A trailing `/` is the conventional, zero-ambiguity marker, and it cannot collide with a file name because a blob path never ends in `/`.
- Rejected: emitting directories unmarked — indistinguishable from files.
  Rejected: emitting a `type\tpath` two-column format in `--children` mode — breaks the one-path-per-line stdout discipline the whole script is built around, and forces every caller to parse.

### Entry-count guard: default 1000, `--max-entries <N>`, `0` disables

- Decision: `github-tree.sh` aborts when the number of entries it is about to emit would exceed the ceiling.
  Default ceiling is 1000. `--max-entries <N>` overrides it; `--max-entries 0` means unlimited.
  A non-integer or negative `<N>` is a usage error (exit 2).
- Rationale: 1000 paths at a realistic ~40 characters each is roughly 40KB, on the order of 10k tokens — already more than a caller should absorb without having asked for it.
  Setting the default low and making the override explicit means "yes, I really do want the whole dump" is a deliberate, visible act in the caller's transcript rather than an accident.
  The flag also keeps the number easy to bump without editing the script, which the task body asked for.
- Rejected: 2000 as the default — less likely to fire spuriously, but a 2000-path listing is already ~20k tokens, which is the failure this guard exists to prevent.
  Rejected: a hard-coded ceiling with no override — a caller with a legitimate need would have to edit the script.

### The guard applies to every mode, uniformly

- Decision: whole-repo recursive, scoped recursive, and `--children` all enforce the same ceiling by the same rule: count of entries about to be emitted.
- Rationale: a single directory can hold thousands of entries, so `--children` is not inherently bounded and exempting it would leave a real hole.
  A uniform rule is also one sentence in SKILL.md rather than a mode-conditional carve-out that a caller has to reason about.
- Rejected: exempting `--children` on the theory that one directory is always small — false for generated, vendored, or data directories.

### The guard fires incrementally, during the walk

- Decision: the ceiling is checked as entries are appended to the output buffer, and the run aborts on the first append that crosses it — not once at the end.
- Rationale: on a repository large enough to trigger GitHub's truncation cap, the fallback walk makes many `gh api` calls.
  Checking only at the end burns that entire call budget to produce a rejection that was determined by the first few hundred entries.
  Aborting early costs nothing and saves the rest.
- Rejected: a single check just before printing — trivially simpler, but pays the full multi-call walk to say "too big".
- Note: this does not weaken the existing buffering guarantee. Output is still printed only after the whole walk succeeds, so a guard abort leaves stdout completely empty, exactly like every other failure.

### Guard abort is a normal `die`: one stderr line, exit 1, empty stdout

- Decision: the guard uses the existing `die` helper. One stderr line naming the ceiling and both remedies (scope to a subdirectory, or use `--children`), exit status 1, nothing on stdout.
- Rationale: `github-tree.sh`'s contract is "on failure, exactly one stderr line, nothing on stdout, non-zero exit", and SKILL.md already instructs the caller to check the exit code on every call.
  A guard abort that fits that contract needs no new caller-side handling.
  Exit 2 stays reserved for usage errors — a listing that is too large is not a malformed invocation.
- Rejected: a distinct exit code (e.g. 3) for "too large" — no current caller branches on exit code beyond zero/non-zero, so a third code would be an untested distinction serving nobody.

### `github-read.sh` skips raw silently when `curl` is absent

- Decision: `github-read.sh` checks `command -v curl`.
  If `curl` is missing it goes directly to the `gh api` fallback, with no error and no stderr output.
  Only `gh` is a hard prerequisite; its absence is a `die`.
- Rationale: SKILL.md's existing prerequisite statement is about `gh` alone ("There is no fallback path — if `gh` is missing or unauthenticated, report that and stop").
  Introducing a second hard prerequisite for what is purely an optimisation would make the skill fail on boxes where it works fine today.
  A missing `curl` costs speed, not capability.
- Rejected: making `curl` a hard prerequisite — turns an optimisation into a breakage.
  Rejected: warning on stderr when `curl` is absent — it would fire on every single read on such a box, and the caller can do nothing about it mid-run.

### `github-read.sh` reports the `gh api` failure, not the raw failure

- Decision: when raw fails and the `gh api` fallback also fails, exactly one stderr line is printed, carrying the `gh api` diagnosis (401 / 403 / 404 / other, in the same style as `github-tree.sh`).
  The raw attempt's failure is never reported.
- Rationale: raw returns an indistinguishable 404 for a private repository, a missing file, and a typo'd path — it cannot tell the caller which.
  `gh api` is authenticated and returns the authoritative answer.
  Reporting both would break the one-line contract and lead with the less informative half.
- Rejected: reporting both attempts — two lines, and the first is noise.

### `github-read.sh` takes no ref argument; reads are pinned to `HEAD`

- Decision: the signature is `github-read.sh <owner/repo> <path>`, exactly two required arguments.
  The raw URL is `https://raw.githubusercontent.com/{owner}/{repo}/HEAD/{path}` and the `gh api` fallback addresses the default branch's content.
- Rationale: prowler is cross-repo reconnaissance on current state ("does repo X have architecture trait Y"), matching `github-tree.sh`, which is likewise `HEAD`-only.
  A ref argument would have to be threaded through two different URL/endpoint constructions and validated separately, for a capability no current caller needs.
  Deep single-repo work at a specific ref is served by cloning the repo locally, which the task body names as the intended path for that.
- Rejected: an optional third `<ref>` argument — YAGNI, and asymmetric with `github-tree.sh` unless that is changed too.

### `github-read.sh` reads exactly one file per invocation

- Decision: one path per call. stdout is the file's content verbatim and nothing else — no header, no filename banner, no trailing delimiter.
- Rationale: it preserves the strict stdout discipline shared by `run.sh` and `github-tree.sh` — stdout is the payload, stderr is diagnostics.
  Multi-file batching would require inventing a framing format, which every caller would then have to parse, and would make partial failure ambiguous.
  A caller reading many files already has the right tool: the `distill-subagent` skill, which SKILL.md already directs it to load before a broad browse.
- Rejected: accepting multiple paths and delimiting them on stdout — a new format to specify, parse, and test, and unclear semantics when file 3 of 5 fails.

### Path validation is duplicated in `github-read.sh`, not extracted to a shared file

- Decision: `github-read.sh` carries its own copy of the path-normalisation and character-validation logic (strip leading/trailing `/`, reject anything outside `[A-Za-z0-9._/-]` via the glob-substitution technique).
  No shared sourced library is introduced.
- Rationale: `github-tree.sh`'s header documents as a deliberate property that, unlike `run.sh`, it reads no file inside the plugin and therefore does not self-locate a `SCRIPT_DIR`/`PLUGIN_ROOT`.
  Introducing a sourced common file would destroy that property for both scripts and add a failure mode (missing or unreadable library) to a script that currently has none.
  Two copies of roughly fifteen lines, across two scripts with no third on the horizon, is the cheaper trade.
- Rationale (correctness): the same validation is what makes URL-encoding unnecessary in `github-read.sh` — the accepted character set is a subset of URL-safe characters, so the path can be interpolated into the raw URL directly.
- Rejected: a sourced `_github-common.sh` — see above.
  Note for the implementer: the copied validation must keep the glob-substitution form (`offending="${path//[A-Za-z0-9._\/-]/}"`) and its accompanying comment's reasoning, not be rewritten as a regex test — the regex form wrongly accepts accented characters under a UTF-8 locale, which is a reproduced failure, not a hypothetical.

### `github-read.sh` requires a non-empty path

- Decision: unlike `github-tree.sh`, where an omitted path means "whole repo", `github-read.sh`'s path argument is required and must be non-empty after normalisation.
  An empty or slash-only path is a usage error (exit 2).
- Rationale: there is no such thing as reading "the whole repo" as one file. An empty path would address a directory, which both backends answer with something the caller cannot use.
- Rejected: silently treating an empty path as an error at the API layer — a loud local rejection is cheaper and clearer than a remote 404.

### No file-size guard on `github-read.sh`

- Decision: `github-read.sh` ships with no maximum-size ceiling. It writes whatever the backend returns.
- Rationale: the task scopes the guard to tree *listings* — the title is "scope large tree listings", and item 3 of the task body is explicitly about listing entry counts.
  The operator has already demonstrated a preference for splitting adjacent ideas into their own tasks rather than folding them in (the `gh api search/code` companion idea was split out of this very discussion).
  Recording the deferral here makes it a considered decision rather than an oversight, and a follow-up task can add it if reading a large file into context turns out to bite in practice.
- Rejected: adding a `--max-bytes` ceiling by symmetry with `--max-entries` — defensible on principle, but unrequested scope on a task whose author is demonstrably scope-conscious.

### Flag parsing accepts flags before positionals only

- Decision: `github-tree.sh` parses leading flags (`--children`, `--max-entries <N>`) in a loop, then treats the remaining 1-2 arguments as `<owner/repo> [path]`.
  A `--`-style terminator is supported so a path can never be mistaken for a flag.
  An unrecognised leading `--`-prefixed token is a usage error (exit 2), not a path.
  Flags appearing after the positionals are a usage error.
- Rationale: it preserves every existing invocation verbatim — `github-tree.sh <owner/repo>` and `github-tree.sh <owner/repo> <path>` keep working unchanged, which matters because SKILL.md documents exactly those two forms and the existing harness asserts them.
  Rejecting an unknown flag rather than silently treating it as a path prevents a typo'd `--childern` from becoming a confusing 404.
- Rejected: allowing flags anywhere among the positionals — more parsing surface, no benefit, and it makes "is this token a path or a typo'd flag" ambiguous.

### The usage line and its exit code

- Decision: the usage message becomes `github-tree: usage: github-tree.sh [--children] [--max-entries N] <owner/repo> [path]`, still on stderr, still exit 2.
- Rationale: exit 2 for usage versus exit 1 for every operational failure is an existing distinction the harness already asserts (test 16, test 21); the new flags extend the usage line without disturbing it.

## Technical context

**`plugins/prowler/scripts/github-tree.sh`** (255 lines) is the file being extended. Its existing structure, which the implementation must work with rather than around:

- `die()` — one stderr line, exit 1. Used for every operational failure. The usage error is the sole exit-2 path and does not go through `die`.
- Prerequisite check (`command -v gh`) runs *before* argument handling, deliberately, so no rejection reaches the network.
  New flag parsing must slot in after the `gh` check and before path normalisation, preserving that ordering.
- Path normalisation strips leading/trailing `/` and validates via `offending="${path//[A-Za-z0-9._\/-]/}"`.
  The long comment above it explains why a regex bracket-range test is wrong here (collation ordering makes `[[ "naïve" =~ ^[A-Za-z0-9._/-]+$ ]]` match under a UTF-8 locale — a reproduced failure) and why byte indexing is wrong too (byte-oriented under C/POSIX locale, would split a multi-byte character).
  Do not rewrite this; `github-read.sh` copies it as-is.
- `BASE_REF` is `HEAD` for an empty path and `HEAD:<path>` for a scoped one; `PREFIX` is `""` or `<path>/` correspondingly.
  `--children` reuses both unchanged.
- `JQ_EXPR` emits a `#trunc\t<bool>` header line followed by `type\tsha\tpath` lines, with a `#badpath\t<json>` sentinel for any path containing a tab or newline.
  The same expression serves both the recursive and non-recursive endpoints, so `--children` needs no new expression.
- `fetch <endpoint> <kind>` runs exactly `gh api "<endpoint>" --jq "<expr>"` — four arguments, no other flags, ever — and leaves results in the globals `FETCH_TRUNCATED` and `FETCH_ENTRIES`.
  `<kind>` is `"root"` or `"child"`; only a `root` fetch can carry the caller's path, which is what makes a 404 attributable to that path.
  A `--children` run makes exactly one `fetch "repos/$REPO/git/trees/$BASE_REF" "root"`.
  The `while ... <<<"$captured"` here-string (never a pipeline) is what lets the badpath abort actually abort the run.
- The walk is one explicit FIFO queue in the main shell with `head` index, never recursion and never a LIFO stack, so output order is fixed.
  Queue items are tab-separated 4-tuples: `mode`, `ref`, `prefix`, `kind`.
  `--children` bypasses the queue entirely — it is a single fetch, not a walk.
- The `output=()` array is printed only at the end, guarded by `[ "${#output[@]}" -gt 0 ]` so the emission is a no-op under `set -u` for a zero-blob repo.
  Zero blobs is success: exit 0, empty stdout.
  The entry-count guard checks against `${#output[@]}` as entries are appended.
- The script uses `set -u` (not `set -e`); error handling is explicit throughout.
- The non-recursive path already has its own truncation abort (`the non-recursive listing of '<ref>' is itself truncated`), which `--children` inherits for free.

**`plugins/prowler/scripts/github-tree-selftest.sh`** (463 lines, 22 tests) is the harness to extend:

- `run_scenario <name> <map-content> <args...>` writes a TSV endpoint→fixture map, truncates a call log, puts the stub `gh` on PATH, runs `github-tree.sh`, and leaves `$out`, `$err`, `$status` set.
- `calls <name>`, `call_line_count <name>`, `call_count_for_endpoint <name> <endpoint>` assert call identity and count. `call_count_for_endpoint` splits log lines with `read`, not `grep`, because endpoints contain literal `?` and `&`.
- Fixture bodies live in `plugins/prowler/scripts/testdata/github-tree/bodies/*.json` and are raw GitHub tree-API response JSON. A failure fixture gets a third TSV column carrying the HTTP status.
- The stub `gh` (`testdata/github-tree/bin/gh`) logs every invocation *before* validating its shape, rejects anything that is not exactly `api <endpoint> --jq <expr>` with exit 98, and has no fixture for an unmapped endpoint (exit 97).
  It applies the `--jq` expression using system `jq`, while production uses `gh`'s embedded gojq — a documented, accepted seam.
- Scratch lives at `<repo>/.scratch/github-tree-selftest`, never a system temp directory.
- `$failures` accumulates; the harness prints `PASS:`/`FAIL:` per assertion and exits non-zero if any failed.
- Neither harness is wired into CI or any Go test; both are invoked manually. There is no runner to register a new harness with.
- Portability envelope is stated in the header: Linux and macOS asserted (the stub-on-PATH mechanism needs an extensionless executable with the exec bit set); Windows Git Bash expected to work but not claimed.

**`plugins/prowler/skills/github-repo-explorer/SKILL.md`** is short and dense. Sections that change:

- The tree-listing block (numbered steps 1-2) resolves `TREE_SH` from `${CLAUDE_SKILL_DIR}` while it is still set, because a dispatched subagent will not have it.
  `github-read.sh` must be resolved by the same idiom, in the same place, for the same reason.
- The "Check the exit code, always" paragraph — its reasoning (empty stdout on success and on failure are indistinguishable) applies verbatim to `github-read.sh` and to the new guard abort.
- The path-versus-question disambiguation paragraph pins the accepted set `^[A-Za-z0-9._/-]+$`; it is unaffected by this task but must not drift out of sync with the scripts' validation.
- The **Read a file's content** and **Lighter alternative for public files** lines are the two being replaced by a single `github-read.sh` instruction.
- The closing `distill-subagent` pointer stays; it is the answer to "how do I read many files", which is why `github-read.sh` needs no batch mode.

**`plugins/prowler/README.md`** has a `## github-tree.sh: one-call repo tree listing` section (around lines 28-38) stating the one-call cost property and the "only runtime dependency is `gh`" claim.
Both need updating: the new script's optional `curl` dependency, the new mode, and the guard.

**Constraint check:** `CONSTRAINTS.md`'s **GitHub Auth Invariant** ("all GitHub authentication goes through `internal/githubclient`; no other production package shells out to `gh`") binds Go production packages.
These are bash plugin scripts, outside that invariant's scope — `github-tree.sh` already shells out to `gh` and predates nothing.
`github-read.sh` sits in exactly the same position and introduces no new violation.
No new cross-cutting invariant arises from this task, so `CONSTRAINTS.md` is not modified.

## Constraints

- **GitHub Auth Invariant** (`CONSTRAINTS.md`): scoped to Go production packages; `plugins/prowler/scripts/*.sh` are outside it. No conflict, no amendment needed. Do not add a Go path for either script.
- **Documentation Lifecycle** (`CONSTRAINTS.md` → `docs/overview.md`): SKILL.md and `plugins/prowler/README.md` are updated in the same commit as the code, per the project's task-completion rule.
- **`manifest/roadmap.md` is not touched** — this is hardening of an already-merged change, which the project's CLAUDE.md explicitly excludes from roadmap movement.
- **Markdown: semantic line breaks** (project CLAUDE.md): one sentence per line, with breaks at internal independent-clause boundaries. No fixed-column hard-wrap, no trailing double-space or backslash breaks. Applies to every `.md` file touched, including lines being edited in place.
- **Never write to a system temp directory** (`mill:conversation`): the new harness scratches under `<repo>/.scratch/`, mirroring `github-tree-selftest.sh`'s `SCRATCH="$PLUGIN_ROOT/../../.scratch/github-tree-selftest"`.
- **Backwards compatibility is required, not optional**: `github-tree.sh <owner/repo>` and `github-tree.sh <owner/repo> <path>` must behave byte-identically to today, in both stdout and `gh` call identity/count, for every listing under the ceiling. The existing 22 tests assert this and must keep passing unmodified.
- **Strict stdout discipline** (established by `run.sh` and `github-tree.sh`): stdout carries the payload and nothing else; every diagnostic goes to stderr; a failure never leaves a partial prefix on stdout.
- **No retries, no backoff** in either script — an existing, documented design decision, not an omission to fix.
- **Runtime `jq` is never invoked** by the production scripts; all JSON extraction goes through `gh api --jq`, which uses `gh`'s embedded gojq. System `jq` is a harness-only dependency.

## Testing

Both harnesses are fully offline and manually invoked. Neither is registered with CI, and this task does not change that.

**`github-tree-selftest.sh` — extend in place.**
The existing 22 tests must pass unchanged; that is itself the backwards-compatibility assertion.
New scenarios to cover, each using the existing `run_scenario` / `call_count_for_endpoint` helpers and new fixture bodies where needed:

- `--children` on a path: exact stdout with blobs unmarked and directories carrying a trailing `/`, and exactly one `gh` call against the non-recursive `HEAD:<path>` endpoint (no `?recursive=1`).
- `--children` with no path: same, against the bare `HEAD` endpoint.
- `--children` proves it does not recurse: a fixture whose listing contains a `tree` entry, asserting the call count stays at 1 and no descendant path appears on stdout.
- `--children` on a directory containing a `commit` (submodule) entry: skipped silently, consistent with the recursive modes.
- `--children` on an empty directory: exit 0, empty stdout.
- Guard fires on the recursive fast path: a fixture with more entries than a low `--max-entries`, asserting empty stdout, exit 1, and a stderr line naming both remedies.
- Guard fires in `--children` mode, proving uniform application.
- Guard fires incrementally: a truncated-fallback scenario where the ceiling is crossed early, asserting the `gh` call count is strictly lower than the same scenario run with the guard disabled — this is the assertion that distinguishes an incremental check from an end-of-walk one, and it is the one scenario that would silently pass under a wrong implementation.
- Guard boundary: exactly-at-ceiling succeeds and prints, ceiling-plus-one aborts.
- `--max-entries 0` disables the ceiling on a listing that would otherwise trip the default.
- Default ceiling is 1000 when `--max-entries` is not passed.
- `--max-entries` with a non-integer, a negative value, or a missing value is a usage error: exit 2, and no `gh` call was made at all (assert the call log is empty — the prerequisite-and-argument-before-network ordering is a stated property worth pinning).
- An unrecognised `--`-prefixed flag is a usage error, not a path.
- `--` terminator: a path that begins with `-` is accepted as a path.
- Flags after the positionals are a usage error.
- Guard abort leaves stdout completely empty even though many entries were buffered — the buffering guarantee under the new failure mode.

**`github-read-selftest.sh` — new sibling harness.**
Mirror `github-tree-selftest.sh`'s shape: a `testdata/github-read/bin/` holding stub `curl` and stub `gh`, a `bodies/` directory of fixture payloads, per-scenario call logs under `<repo>/.scratch/github-read-selftest`, `fail`/`pass` accumulation, non-zero exit on any failure.
Both stubs must log every invocation before validating its shape, so "no call was made" assertions are real.
Scenarios:

- Raw succeeds: exact stdout equals the fixture bytes, exit 0, exactly one `curl` call against the `https://raw.githubusercontent.com/{owner}/{repo}/HEAD/{path}` URL, and **zero** `gh` calls — the assertion that proves the preference order.
- Raw 404, `gh api` succeeds: correct stdout, exit 0, one `curl` call then one `gh` call, and the `gh` call carries `-H Accept: application/vnd.github.raw` (assert the exact argument vector, mirroring the tree harness's four-argument shape assertion).
- Raw fails, `gh api` fails with 401 / 403 / 404: exit non-zero, empty stdout, exactly one stderr line, and that line carries the `gh` diagnosis with no mention of the raw attempt.
- `curl` absent from PATH: goes straight to `gh api`, zero `curl` calls, nothing on stderr about `curl`, correct stdout.
- `gh` absent from PATH: `die` with the missing-`gh` message, exit 1, before any network call.
- Path validation: a path with an unsupported character is rejected locally — exit 1, and zero `curl` and zero `gh` calls.
  Include the UTF-8 case (`naïve`) explicitly, since that is the reproduced failure the glob-substitution technique exists to prevent, and a regex rewrite would pass every other validation test while failing this one.
- Path normalisation: `src/x.go`, `/src/x.go`, and a path with a trailing slash stripped produce identical call logs.
- Empty or slash-only path: usage error, exit 2, no calls.
- Wrong argument count (zero, one, three): usage error, exit 2, no calls.
- Malformed `<owner/repo>`: rejected locally, no calls.
- Empty file: exit 0, empty stdout — success, distinguished from failure only by the exit code, which is exactly why SKILL.md tells the caller to check it.
- stdout cleanliness: on a successful read, stdout is byte-identical to the fixture with no added trailing newline, banner, or filename — assert against the fixture bytes, not a trimmed comparison.

**Not covered offline, documented as manual checks** (mirroring the existing harness's "NOT covered here" header note): one live `github-read.sh` run against a public repo confirming the raw path is taken; one live run against a private repo confirming the `gh api` fallback fires and succeeds; one live `--children` run; and one live guard trip against a large public repo.

**TDD candidates:** the entry-count guard and the flag parser are the two clean TDD targets — both are pure input/output with no network shape to discover, and the guard's incremental-abort behaviour is defined by a call-count assertion that is easy to write first and impossible to satisfy accidentally.
`github-read.sh`'s preference-order assertion (raw succeeds ⇒ zero `gh` calls) is the third: it is the single test that pins the entire point of the task.

## Q&A log

- **Q:** Does the raw-first read become a script, or stay SKILL.md prose? **A:** [auto-pick] A new `github-read.sh` script. **Why:** prose makes the fallback depend on agent discipline on every single read; a script makes the preference order structural, exactly as `github-tree.sh` did for the tree walk.
- **Q:** What is the fallback when raw fails (private repo / 404)? **A:** [auto-pick] `gh api` with `-H "Accept: application/vnd.github.raw"`. **Why:** no base64 inflation and no decode step, for the same bytes over the same authenticated path.
- **Q:** CLI shape for the non-recursive mode, and how directories appear on stdout? **A:** [auto-pick] A `--children` flag on `github-tree.sh`, directories emitted with a trailing `/`. **Why:** one script, one harness, one error vocabulary; the trailing `/` is required because a top-down mode whose whole purpose is descending must say what can be descended into.
- **Q:** Entry-count guard threshold and override? **A:** [auto-pick] Default 1000, `--max-entries <N>`, `0` = unlimited. **Why:** 1000 paths is already ~10k tokens; an explicit flag makes the full dump a deliberate, auditable act, and keeps the number easy to bump.
- **Q:** When does the guard fire? **A:** [auto-pick] Incrementally during the walk. **Why:** an end-of-walk check burns the entire multi-call fallback budget to produce a rejection already determined by the first few hundred entries.
- **Q:** Which modes does the entry-count guard cover? **A:** [auto-pick] All modes uniformly, counting entries about to be emitted. **Why:** a single directory can hold thousands of entries, so `--children` is not inherently bounded; a uniform rule is also one sentence in SKILL.md instead of a carve-out.
- **Q:** What if `curl` is missing? **A:** [auto-pick] Skip raw silently and go straight to the `gh api` fallback. **Why:** `gh` is the skill's only stated hard prerequisite; a missing `curl` costs speed, not capability, and warning on every read would be noise the caller cannot act on.
- **Q:** Does `github-read.sh` take a ref argument? **A:** [auto-pick] No — `<owner/repo> <path>` only, pinned to `HEAD`. **Why:** YAGNI, symmetric with `github-tree.sh`, and deep single-repo work at a specific ref is served by cloning locally, which the task body names as the intended path.
- **Q:** Error contract when both raw and `gh api` fail? **A:** [auto-pick] Exactly one stderr line, carrying the `gh api` failure. **Why:** raw's 404 cannot distinguish private repo from missing file from typo; `gh api` is authoritative, and two lines would break the one-line contract.
- **Q:** Test approach? **A:** [auto-pick] Extend `github-tree-selftest.sh` with new fixtures, and add a sibling `github-read-selftest.sh` with stub `curl` and stub `gh`. **Why:** mirrors the established shape and keeps both harnesses fully offline.
- **Q:** Add a file-size guard on `github-read.sh`, mirroring the tree guard? **A:** [auto-pick] No — deliberately deferred and recorded here. **Why:** the task scopes the guard to tree listings, and the operator has already split adjacent ideas into their own tasks rather than folding them in; a follow-up can add it if large reads bite in practice.
- **Q:** Which docs are updated in the same commit? **A:** [auto-pick] `github-repo-explorer/SKILL.md` and `plugins/prowler/README.md`. **Why:** SKILL.md carries the read-order and listing-mode guidance; README carries the script-level dependency and cost claims, one of which ("only runtime dependency is `gh`") becomes inaccurate otherwise. `skills/INDEX.md` indexes skills, not scripts, so it is unchanged.
- **Q:** Does `github-read.sh` accept multiple paths per invocation? **A:** [auto-pick] No, one file per call, stdout is the content verbatim. **Why:** batching would require inventing a framing format with ambiguous partial-failure semantics; the `distill-subagent` skill is already the documented answer for reading many files.
- **Q:** Extract the shared path validation into a sourced common file? **A:** [auto-pick] No — duplicate the ~15 lines in `github-read.sh`. **Why:** `github-tree.sh` documents as a deliberate property that it reads no file inside the plugin and self-locates no `SCRIPT_DIR`; a sourced library would destroy that for both scripts and add a new failure mode, for two copies with no third caller on the horizon.
