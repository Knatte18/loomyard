# Batch: github-tree flag parsing, children mode, entry-count guard

```yaml
task: "Prefer raw fetch, scope large tree listings"
batch: "github-tree flag parsing, children mode, entry-count guard"
number: 1
cards: 7
verify: bash plugins/prowler/scripts/github-tree-selftest.sh
depends-on: []
```

## Batch Scope

This batch delivers every change to `github-tree.sh` and its offline harness: a leading-flag parser with a `--` terminator, a `--children` non-recursive listing mode, and a uniform incremental entry-count guard defaulting to 1000 entries with a `--max-entries <N>` override.
It is one batch because all three features land in the same 255-line script and are asserted by the same harness — the flag parser exists to carry the other two flags, and the guard's mode-aware error wording depends on knowing whether `--children` is in effect.

The external interface batch 3 consumes is the finished CLI surface: `github-tree.sh [--children] [--max-entries N] <owner/repo> [path]`, the trailing-`/` directory marker in `--children` output, and the guard abort's exit-1/one-stderr-line shape.

Batch-local decision beyond the overview's Shared Decisions: the stub `gh` gains absolute-path fixture support so the two large-listing scenarios can generate their fixture bodies at harness runtime into the scenario's own scratch directory rather than committing several hundred KB of mechanically-identical entries under `testdata/`.

Backwards compatibility is the binding constraint here: the existing 22 tests must keep passing without modification, and every invocation whose path does not begin with two dashes must behave byte-identically to today in both stdout and `gh` call identity and count, for every listing under the ceiling.
The one accepted behavioural deviation is a positional argument beginning with two dashes, which now exits 2 instead of reaching the API.

## Cards

### Card 1: `github-tree.sh` argument parsing — flags, terminator, usage line

- **Context:**
  - `plugins/prowler/scripts/run.sh`
- **Edits:**
  - `plugins/prowler/scripts/github-tree.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the fixed two-argument check in `plugins/prowler/scripts/github-tree.sh` (today the `[ "$#" -lt 1 ] || [ "$#" -gt 2 ]` block, which prints the usage line and exits 2) with a parse loop, placed after the existing `command -v gh` prerequisite check and before the `REPO`/`RAW_PATH` assignment, so the prerequisite-before-argument-handling-before-network ordering the file's own comment states is preserved.
  Introduce a `usage` shell function printing `github-tree: usage: github-tree.sh [--children] [--max-entries N] <owner/repo> [path]` to stderr and exiting 2;
  it is deliberately not routed through `die`, which exits 1.
  Introduce two globals with their defaults: `CHILDREN=0` and `MAX_ENTRIES=1000`.
  The loop collects positionals into an `args` array and behaves as follows.
  While a `--` terminator has already been seen, every remaining token is appended to `args` unexamined.
  Otherwise a bare `--` token sets the terminator flag and is consumed;
  a `--children` token sets `CHILDREN=1` and is consumed, but only while `args` is still empty — otherwise it is a usage error;
  a `--max-entries` token consumes the following token into `MAX_ENTRIES`, but only while `args` is still empty and only when a following token exists — otherwise it is a usage error;
  any other token beginning with two dashes is a usage error;
  every other token is appended to `args`.
  A single-dash token is never a flag at any position and always reaches `args`.
  After the loop, validate `MAX_ENTRIES` with the byte-safe glob form `case "$MAX_ENTRIES" in ''|*[!0-9]*) usage ;; esac` rather than a regex bracket-range test, for the same collation reason the adjacent path-validation comment already gives — a negative or non-integer value is therefore a usage error.
  Then reject an `args` length outside 1..2 via `usage`, and assign `REPO="${args[0]}"` and `RAW_PATH="${args[1]:-}"`.
  Everything downstream of that assignment — the slug regex, the path normalisation and its long explanatory comment, `BASE_REF`, `PREFIX`, `JQ_EXPR`, and `fetch` — stays exactly as it is.
  Extend the script's header comment with a short paragraph recording that flags are recognised before the first positional only, that a token beginning with two dashes in positional position is a usage error rather than a path, and that a `--` terminator makes such a path reachable.
- **Commit:** `feat(prowler): parse leading flags and a -- terminator in github-tree.sh`

### Card 2: `github-tree.sh` incremental entry-count guard

- **Context:** none
- **Edits:**
  - `plugins/prowler/scripts/github-tree.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an `emit` shell function to `plugins/prowler/scripts/github-tree.sh`, declared after the `output=()` array is initialised, that appends its single argument to `output` and then, when `MAX_ENTRIES` is not `0` and the resulting `${#output[@]}` is strictly greater than `MAX_ENTRIES`, aborts through the existing `die` helper.
  Checking strictly greater than after the append is what makes a listing of exactly `MAX_ENTRIES` entries succeed while `MAX_ENTRIES` plus one aborts.
  The abort message is mode-aware, selected on `CHILDREN`.
  When `CHILDREN` is `0` the message names the repository, the ceiling number, and all three remedies — scoping to a subdirectory, `--children`, and raising `--max-entries`.
  When `CHILDREN` is `1` the message names the repository, the ceiling number, and only two remedies — scoping to a subdirectory and raising `--max-entries` — and must not contain the substring `--children` anywhere, because that mode is already in effect and suggesting it back to the caller is noise.
  Both messages go through `die`, so both are exactly one stderr line with exit status 1;
  exit 2 stays reserved for usage errors.
  Replace both `output+=(...)` sites inside the walk loop — the recursive branch's blob append and the non-recursive branch's blob append — with `emit` calls, so the ceiling is enforced as entries are appended rather than once at the end.
  Aborting on the crossing append is what keeps the truncated-fallback walk from burning its whole multi-call budget to produce a rejection already determined by the first few hundred entries.
  Leave the end-of-script `[ "${#output[@]}" -gt 0 ]` guarded emission untouched: output is still printed only after the whole walk succeeds, so a guard abort leaves stdout completely empty exactly like every other failure.
  Extend the script's header comment with a sentence recording the default ceiling, the `--max-entries` override including that `0` means unlimited, and that the check is incremental.
- **Commit:** `feat(prowler): add an incremental entry-count guard to github-tree.sh`

### Card 3: `github-tree.sh` `--children` non-recursive listing mode

- **Context:** none
- **Edits:**
  - `plugins/prowler/scripts/github-tree.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/scripts/github-tree.sh`, branch on `CHILDREN` immediately before the existing walk's `queue`/`head` initialisation, so that a `--children` run bypasses the FIFO queue entirely and the recursive walk is reached only when `CHILDREN` is `0`.
  The `--children` branch performs exactly one `fetch "repos/$REPO/git/trees/$BASE_REF" "root"` — reusing the existing `fetch` helper, `BASE_REF`, `PREFIX`, and `JQ_EXPR` unchanged, so a `--children` run makes exactly one `gh` call against the non-recursive endpoint with no `?recursive=1` suffix.
  Restate the non-recursive truncation abort on this branch rather than assuming it is inherited: when `FETCH_TRUNCATED` is `true`, `die` with the same message text the walk loop's non-recursive branch already uses, naming `BASE_REF` as the ref.
  It cannot be inherited because that check lives inside the walk loop's `else` branch, which `--children` never enters.
  Then iterate `FETCH_ENTRIES`, guarding the loop with `[ "${#FETCH_ENTRIES[@]}" -gt 0 ]` so an empty directory is a genuine no-op under `set -u` rather than an unbound-variable error, and for each entry parse the same tab-separated `type`, `sha`, `path` triple the walk already parses.
  A `blob` entry is emitted through `emit` as `$PREFIX$epath`;
  a `tree` entry is emitted through `emit` as `$PREFIX$epath/` with a single trailing slash;
  a `commit` entry is skipped silently, consistent with the recursive modes.
  Routing both through `emit` is what applies the entry-count guard uniformly to this mode.
  The recursive modes' output contract is unchanged — they still emit blob paths only and never a trailing-slash entry.
  An empty directory is a success: exit 0 with byte-empty stdout.
  Extend the script's header comment with a paragraph recording that `--children` lists one path's direct children without recursing, that a directory is marked by a trailing slash while a file is not, and that a trailing slash cannot collide with a file name because a blob path never ends in one.
- **Commit:** `feat(prowler): add a --children non-recursive listing mode to github-tree.sh`

### Card 4: stub `gh` absolute-fixture support, new fixtures, and a runtime body generator

- **Context:**
  - `plugins/prowler/scripts/testdata/github-tree/bodies/small-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-root-nonrec.json`
- **Edits:**
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
  - `plugins/prowler/scripts/github-tree-selftest.sh`
- **Creates:**
  - `plugins/prowler/scripts/testdata/github-tree/bodies/children-src-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/children-empty-nonrec.json`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/scripts/testdata/github-tree/bin/gh`, change the single `body_path` assignment so that a mapped fixture name beginning with a slash is used verbatim as an absolute path, while every other value keeps resolving under `GH_STUB_BODIES` exactly as today.
  This is the whole mechanism that lets a scenario point at a fixture generated into its own scratch directory;
  it changes nothing for the 22 existing scenarios, which all use bare filenames.
  Record the reason in the stub's header comment.
  Create `plugins/prowler/scripts/testdata/github-tree/bodies/children-src-nonrec.json` as a non-recursive tree response with `truncated` false and a `tree` array holding, in this order, one `blob`, one `tree`, one `commit`, and a second `blob`, using subtree-relative single-segment paths and distinct sha values in the style of the existing fixtures.
  Create `plugins/prowler/scripts/testdata/github-tree/bodies/children-empty-nonrec.json` as a non-recursive tree response with `truncated` false and an empty `tree` array.
  In `plugins/prowler/scripts/github-tree-selftest.sh`, add a `gen_tree_body <outfile> <count>` helper near the other helpers that writes a syntactically valid tree response to `<outfile>` with `truncated` false and exactly `<count>` `blob` entries whose paths and shas are mechanically derived from the loop index.
  Record in a comment above it that these bodies are generated rather than checked in because a thousand mechanically-identical entries have no shape worth reviewing and committing them would bury the fixtures that do, and that reading the same generator with different counts is what keeps the at-ceiling and one-over scenarios provably one entry apart.
  Extend the harness's header comment to note the absolute-fixture mechanism and the generator.
  Add no new scenarios in this card;
  cards 5 through 7 consume what it builds.
- **Commit:** `test(prowler): add generated-fixture support to the github-tree stub gh and harness`

### Card 5: harness scenarios for `--children`

- **Context:**
  - `plugins/prowler/scripts/github-tree.sh`
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/children-src-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/children-empty-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-root-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/nonrectrunc-root-nonrec.json`
- **Edits:**
  - `plugins/prowler/scripts/github-tree-selftest.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append new numbered test sections to `plugins/prowler/scripts/github-tree-selftest.sh`, after the existing test 22 and before the scratch teardown, each driven through the existing `run_scenario` helper and asserted with the existing `calls`, `call_line_count`, and `call_count_for_endpoint` helpers.
  Cover six scenarios.
  First, `--children` on a path: map the non-recursive scoped endpoint to `children-src-nonrec.json` and assert the exact stdout has blob entries unmarked and the directory entry carrying one trailing slash, that the submodule entry is absent, that exit status is 0, that exactly one `gh` call was made, and that the called endpoint is the `HEAD:<path>` form with no `?recursive=1` suffix.
  Second, `--children` with no path: map the bare `HEAD` non-recursive endpoint to `trunc1-root-nonrec.json` and assert the exact stdout is the two root blobs unmarked followed by the three directories each carrying a trailing slash, and exactly one `gh` call.
  Third, proof of non-recursion: reuse the second scenario's fixture, whose listing contains `tree` entries, and assert the call count stays at 1 and that no descendant path (a stdout line containing a slash anywhere other than as its final character) ever appears.
  Fourth, the submodule skip: reuse the first scenario and assert the `commit` entry's name never appears on stdout, marked or unmarked.
  Fifth, an empty directory: map the scoped non-recursive endpoint to `children-empty-nonrec.json` and assert exit status 0 with byte-empty stdout.
  Sixth, a `--children` listing that is itself truncated: map the endpoint to `nonrectrunc-root-nonrec.json` and assert byte-empty stdout, non-zero exit, and a stderr line containing the word `truncated` — this is the assertion that the abort was restated on the `--children` path rather than assumed inherited from the walk loop.
- **Commit:** `test(prowler): assert github-tree.sh --children listing behaviour`

### Card 6: harness scenarios for the entry-count guard

- **Context:**
  - `plugins/prowler/scripts/github-tree.sh`
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/small-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-root-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-mmm-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-aaa-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-bbb-rec.json`
- **Edits:**
  - `plugins/prowler/scripts/github-tree-selftest.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append new numbered test sections to `plugins/prowler/scripts/github-tree-selftest.sh` covering seven guard scenarios, using `gen_tree_body` for the two that need large listings and pointing their map's fixture column at the generated file's absolute path.
  First, the guard fires on the recursive fast path: run the three-blob `small-root-rec.json` scenario with a ceiling of 2 and assert byte-empty stdout, exit status 1, and a stderr line containing the ceiling number, the substring `--children`, and the substring `--max-entries`, asserted by substring rather than by literal-text equality so a wording improvement is not a test edit.
  Second, the guard fires in `--children` mode, proving uniform application: run a `--children` scenario with a ceiling below its entry count and assert byte-empty stdout, exit status 1, and a stderr line containing the ceiling number and `--max-entries` but not containing `--children`.
  Third, the guard fires incrementally: run the existing five-call truncated-fallback map with a low ceiling, run the identical map again with the ceiling disabled, and assert the guarded run's `gh` call count is strictly lower than the unguarded run's — this is the one scenario that would silently pass under an end-of-walk implementation, so it is the assertion that distinguishes the two.
  Fourth, the boundary: assert that a ceiling exactly equal to the listing's entry count succeeds and prints the full listing, and that a ceiling one lower aborts.
  Fifth, the default ceiling is 1000: generate a body with 1001 entries, run with no `--max-entries` argument at all, and assert the abort fires;
  generate a body with exactly 1000 entries, run the same way, and assert it succeeds.
  Sixth, `--max-entries 0` disables the ceiling: run the 1001-entry body with `--max-entries 0` and assert exit status 0 and a stdout line count of 1001.
  Seventh, the buffering guarantee under the new failure mode: on any aborting scenario whose walk buffered many entries before crossing the ceiling, assert stdout is byte-empty, proving the guard abort leaks no partial prefix.
- **Commit:** `test(prowler): assert github-tree.sh entry-count guard behaviour`

### Card 7: harness scenarios for flag parsing and usage errors

- **Context:**
  - `plugins/prowler/scripts/github-tree.sh`
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/scoped-src-rec.json`
- **Edits:**
  - `plugins/prowler/scripts/github-tree-selftest.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append new numbered test sections to `plugins/prowler/scripts/github-tree-selftest.sh` covering the flag parser.
  Every usage-error assertion in this card must assert `status -eq 2` specifically, the way the existing too-many-arguments test does, not merely `status -ne 0` — the exit-code distinction is the thing under test.
  Each must also assert byte-empty stdout and an empty call log, because the parser runs before any network call.
  Cover: `--max-entries` with a non-integer value;
  `--max-entries` with a negative value;
  `--max-entries` with no following value at all;
  an unrecognised token beginning with two dashes in leading position;
  a token beginning with two dashes appearing after the positionals, which is the plan's one deliberate behavioural deviation from today and is pinned here so it reads as a decision rather than a regression;
  and a recognised flag appearing after the positionals, which is likewise a usage error.
  Then cover the two accepting cases.
  A `--` terminator followed by a path beginning with a dash must be accepted as a path and reach the API — map the endpoint the resulting `BASE_REF` produces and assert a `gh` call was made.
  A single-dash token in path position, with no terminator, must still reach path validation and the API exactly as it does today, proving the flag test is on a leading double dash only.
  Finally, assert that combining both flags with both positionals parses — `--children` and `--max-entries` together ahead of an owner/repo and a path — by mapping the scoped non-recursive endpoint and asserting a successful listing.
- **Commit:** `test(prowler): assert github-tree.sh flag parsing and usage-error exit codes`

## Batch Tests

`verify:` runs `bash plugins/prowler/scripts/github-tree-selftest.sh`, the harness this batch extends.
It is the complete test surface for `github-tree.sh`: fully offline, driving the script through a stub `gh` on PATH with no network access, asserting exact stdout, exact `gh` call identity and count, and distinguishing stderr substrings.
Its one dependency beyond bash is system `jq`, which it checks for up front and which is present in this environment.
The harness is not registered with CI or any Go test and this batch does not change that, so invoking it directly is the only way it runs.

Scoping is exact: this batch touches `github-tree.sh`, its stub `gh`, and its fixtures, and this harness is the only thing that exercises any of them.
Nothing else in the repository imports or invokes them, so no wider suite is implicated.

The existing 22 assertions passing unmodified is itself the backwards-compatibility test the discussion requires — every invocation whose path does not begin with two dashes must behave byte-identically to today.
Not covered offline, and documented as manual checks in the same style as the harness's existing header note: one live `--children` run against a real repository, and one live guard trip against a large public repository.
