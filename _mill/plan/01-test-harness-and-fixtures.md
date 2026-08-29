# Batch: test-harness-and-fixtures

```yaml
task: "prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call"
batch: "test-harness-and-fixtures"
number: 1
cards: 4
verify: bash -n plugins/prowler/scripts/testdata/github-tree/bin/gh && bash -n plugins/prowler/scripts/github-tree-selftest.sh
depends-on: []
```

## Batch Scope

This batch delivers the entire offline test apparatus for `github-tree.sh` before any line of that script exists: a stub `gh` executable that applies the real `--jq` expression it was handed, 25 canned GitHub tree-API JSON response bodies, and `github-tree-selftest.sh` — a harness in the shape of the existing `selftest.sh`, carrying twenty-two numbered assertions.
Taken together these fully define the contract batch 2 must satisfy: the exact stdout for every scenario, the exact call count and call identity for every scenario, and a distinguishing stderr substring for every distinguished failure.

The external interface batch 2 consumes is threefold and is fixed here: (a) `github-tree.sh` is invoked as `bash <path>/github-tree.sh <owner/repo> [path]` and nothing else;
(b) it calls `gh` in exactly one shape, `gh api "<endpoint>" --jq "<expr>"`, four arguments in that order;
(c) each error path has one fixed stderr wording whose distinguishing substring is asserted here and restated in full in batch 2, card 5.

Batch-local decision beyond `## Shared Decisions`: the harness generates each scenario's endpoint-to-body map and the stub's call log into `.scratch/github-tree-selftest/`, so the committed fixture surface is the stub plus bodies only.

## Cards

### Card 1: stub `gh` executable

- **Context:**
  - `plugins/prowler/scripts/selftest.sh`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create an extensionless bash executable named `gh` (shebang `#!/usr/bin/env bash`, `set -u`, and the exec bit set via `chmod +x`) whose whole job is to stand in for the real `gh` CLI while `github-tree.sh` runs offline.
  It reads three environment variables, all mandatory, and exits non-zero with a `gh-stub:` stderr line naming the missing one if any is unset or empty: `GH_STUB_MAP` (absolute path to a tab-separated map file), `GH_STUB_BODIES` (absolute path to the directory holding the canned JSON response bodies), and `GH_STUB_LOG` (absolute path to an append-only call log file).
  Its first action, before any validation of the invocation itself, is to append one line to `$GH_STUB_LOG` containing all of its arguments joined by single spaces — the log must record even invocations the stub goes on to reject, so a test asserting "no call was made" is asserting something real.
  It then requires exactly four arguments in exactly this order: `api`, the endpoint, `--jq`, and the jq expression.
  Any other argument count, or a first argument other than `api`, or a third argument other than `--jq`, is refused with `gh-stub: unsupported invocation: <all args>` on stderr and exit 98 — never a guess, never a fallthrough, because tolerating an unexpected shape is exactly how a re-added preflight call would slip through unnoticed.
  It resolves the endpoint by reading `$GH_STUB_MAP` line by line with `while IFS= read -r line`, splitting each line on tab into at most three fields via bash parameter expansion (never `grep` and never `sed`, since an endpoint contains a literal `?` and would be read as a regular expression).
  Field 1 is the endpoint, compared for exact string equality;
  field 2 is a body filename resolved against `$GH_STUB_BODIES`;
  field 3, when present and non-empty, is an HTTP status marker.
  A line whose first field does not match is skipped, blank lines are skipped, and if no line matches the stub writes `gh-stub: no fixture for endpoint: <endpoint>` to stderr and exits 97, so a mis-keyed map fails loudly instead of impersonating a 404.
  On a match with a status marker present, the stub emulates the real `gh api` failure shape verified in the discussion: it writes the body file's contents verbatim to **stdout** with `cat`, writes nothing to stderr, and exits 1 — the `--jq` expression is deliberately not applied on this path, because the real `gh` does not apply it either and the script under test parses the raw JSON error body.
  On a match with no status marker, the stub runs `jq -r "$4" "$GH_STUB_BODIES/<body>"` and exits with jq's own exit status, so the actual expression the script passed is exercised against real JSON rather than a pre-transformed fixture.
  Write a header comment stating what the stub is for, that it deliberately answers no `auth status` call because the script makes none, and that the `jq` dependency here is the harness's alone and never `github-tree.sh`'s.
- **Commit:** `test(prowler): add stub gh executable for offline github-tree fixtures`

### Card 2: fast-path and error-body fixtures

- **Context:**
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/testdata/github-tree/bodies/small-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/scoped-src-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/types-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/noblobs-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/badpath-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-401.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-403.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-404.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-422.json`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Every tree-response body in this card and the next uses the same minimal shape — a JSON object with a `sha` string, a `tree` array, and a `truncated` boolean — and every element of `tree` carries exactly `path`, `mode`, `type`, and `sha`.
  Real GitHub responses also carry `url` and `size`;
  those are dropped deliberately, because the jq expression under test reads only `path`, `type`, and `sha`, and keeping the bodies small keeps them diffable.
  Use mode `100644` for a regular blob, `120000` for a symlink blob, `040000` for a tree, and `160000` for a `commit` (submodule) entry.
  Give every entry a short mnemonic sha such as `bREADME` or `tmmm`, because subtree shas are what the script turns into follow-up endpoints and a mnemonic sha makes the expected call log readable.

  `small-root-rec.json`: `truncated` false, tree entries in this order — blob `intro.md`, tree `src` with sha `tsrc`, blob `src/main.go`, blob `src/util.go`.
  The root blob is named `intro.md` rather than the more obvious readme filename so that no fixture path string collides with a real file in this repository, which would otherwise read as an undeclared dependency to the plan validator and to a bulk-mode reviewer alike.
  This is the untruncated whole-repo fast path;
  the `src` tree entry exists so the harness can assert that directory paths never reach stdout.

  `scoped-src-rec.json`: `truncated` false, tree entries in this order — blob `main.go`, tree `deep` with sha `tdeep`, blob `deep/x.go`.
  Paths are relative to the requested subtree, exactly as the real API returns them, so this body is what proves the script re-applies the `src/` prefix rather than emitting the API's own relative paths.

  `types-root-rec.json`: `truncated` false, tree entries in this order — blob `link` with mode `120000`, entry `vendor/dep` with type `commit` and mode `160000`, blob `real.txt`, tree `dir` with sha `tdir`.
  This pins that a symlink is an ordinary blob and is emitted, while a submodule and a directory are not.

  `noblobs-root-rec.json`: `truncated` false, tree containing only two `commit` entries, `mod/one` and `mod/two`.
  This is the zero-blobs-is-success fixture.

  `badpath-root-rec.json`: `truncated` false, tree entries in this order — blob `ok.txt`, then a blob whose `path` in the JSON source is written `"we\tird.txt"` (a JSON `\t` escape, so the decoded path holds a literal tab character).
  Write the escape sequence in the file;
  do not paste a raw tab into the JSON string.

  The four error bodies reproduce GitHub's own error-response shape, which is what the script parses its status code out of.
  Each is a single JSON object with `message`, `documentation_url`, and a `status` field whose value is the code **as a string**.
  Use message `Bad credentials` with status `"401"`;
  `API rate limit exceeded` with status `"403"`;
  `Not Found` with status `"404"`;
  and `Invalid object requested. SHA must identify a commit or a tree.` with status `"422"`.
  Set `documentation_url` to `https://docs.github.com/rest` in all four.
- **Commit:** `test(prowler): add fast-path and error-body github-tree fixtures`

### Card 3: truncated-fallback fixtures

- **Context:**
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-root-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-mmm-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-aaa-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-bbb-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-root-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-a-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-b-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-b-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-bx-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-by-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/scopedtrunc-src-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/scopedtrunc-src-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/scopedtrunc-lib-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/nonrectrunc-root-nonrec.json`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Same body shape and mode conventions as card 2.
  The defining property of this set is that every truncated response is *deliberately incomplete relative to its non-recursive counterpart* — an implementation that enumerates children out of a truncated response instead of re-listing must produce a demonstrably shorter output, not merely a differently-ordered one.

  The `trunc1` family is the one-level fallback and doubles as the sibling-order fixture.
  `trunc1-root-rec.json`: `truncated` **true**, tree holding only blob `zzz.txt` and tree `mmm` with sha `tmmm` — missing `Makefile`, `aaa`, and `bbb` entirely.
  `trunc1-root-nonrec.json`: `truncated` false, tree entries in exactly this order — blob `zzz.txt`, blob `Makefile`, tree `mmm` sha `tmmm`, tree `aaa` sha `taaa`, tree `bbb` sha `tbbb`.
  This order is chosen so that the correct output matches neither git-sorted order nor the reverse-sibling order a LIFO stack would produce.
  `trunc1-mmm-rec.json`: `truncated` false, entries in order — blob `m1.txt`, tree `sub` sha `tsub`, blob `sub/m2.txt`.
  `trunc1-aaa-rec.json`: `truncated` false, single blob `a1.txt`.
  `trunc1-bbb-rec.json`: `truncated` false, single blob `b1.txt`.

  The `trunc2` family is the two-level fallback.
  `trunc2-root-rec.json`: `truncated` **true**, tree holding only blob `r.txt` and tree `a` sha `ta`.
  `trunc2-root-nonrec.json`: `truncated` false, entries in order — blob `r.txt`, tree `a` sha `ta`, tree `b` sha `tb`.
  `trunc2-a-rec.json`: `truncated` false, single blob `a1.txt`.
  `trunc2-b-rec.json`: `truncated` **true**, tree holding only tree `x` sha `tbx` and no blobs at all — strictly fewer subtrees than its non-recursive counterpart below, which is what makes the "children come from the non-recursive re-fetch" rule provable rather than merely asserted.
  `trunc2-b-nonrec.json`: `truncated` false, entries in order — blob `bown.txt`, tree `x` sha `tbx`, tree `y` sha `tby`.
  `trunc2-bx-rec.json`: `truncated` false, single blob `x1.txt`.
  `trunc2-by-rec.json`: `truncated` false, single blob `y1.txt`.

  The `scopedtrunc` family is a scoped listing whose subtree truncates, proving prefixes stay repo-relative at depth.
  `scopedtrunc-src-rec.json`: `truncated` **true**, tree holding only blob `s.txt`.
  `scopedtrunc-src-nonrec.json`: `truncated` false, entries in order — blob `s.txt`, tree `lib` sha `tlib`.
  `scopedtrunc-lib-rec.json`: `truncated` false, single blob `l1.txt`.

  `nonrectrunc-root-nonrec.json` is the residual hard-error case: `truncated` **true**, tree holding a single blob `only.txt`.
  A non-recursive listing that reports itself truncated has no complete counterpart to fall back to, so this fixture exists to prove the script stops rather than emitting a partial list.
- **Commit:** `test(prowler): add truncated-fallback github-tree fixtures`

### Card 4: `github-tree-selftest.sh` harness

- **Context:**
  - `plugins/prowler/scripts/selftest.sh`
  - `plugins/prowler/scripts/run.sh`
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/small-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/scoped-src-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/types-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/noblobs-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/badpath-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-401.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-403.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-404.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-422.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-root-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-mmm-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-aaa-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc1-bbb-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-root-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-root-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-a-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-b-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-b-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-bx-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/trunc2-by-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/scopedtrunc-src-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/scopedtrunc-src-nonrec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/scopedtrunc-lib-rec.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/nonrectrunc-root-nonrec.json`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/github-tree-selftest.sh`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Mirror the structure of `plugins/prowler/scripts/selftest.sh` exactly rather than inventing a new one: `set -u`, `SCRIPT_DIR`/`PLUGIN_ROOT` self-location from `$0`, a `failures` counter, `fail()` and `pass()` helpers that record and keep going, `# --- Test N: ... ---` section comments, a `====` separator, and a final PASS/FAIL summary line with a non-zero exit when `failures` is non-zero.

  Write a header comment in the same spirit as `selftest.sh`'s own, stating: that this is an offline harness driving `github-tree.sh` through a stub `gh` on `PATH` with no network;
  that system `jq` is a dependency of this harness alone and never of `github-tree.sh`, which uses `gh`'s embedded gojq at run time;
  that the harness therefore validates the jq expression under jq while production runs it under gojq, an accepted seam that a future expression using a jq/gojq-divergent construct would fall through;
  and the portability envelope — the stub-on-`PATH` mechanism needs an extensionless executable with the exec bit set, so Linux and macOS are the asserted platforms, Windows Git Bash is expected to work but is not claimed, and `cmd`/PowerShell cannot run it at all.
  Mirror `selftest.sh`'s "NOT covered here" note for this harness's own manual-only cases: one live run against a small public repo, one live run against `torvalds/linux` confirming the real truncated fallback completes in a single invocation, a spot-check that the jq expression behaves identically under gojq and jq, and the HTTP 409 commitless-repository alias, which no fixture pins because it was never observed live.

  Define `JQ_BIN="${GITHUB_TREE_SELFTEST_JQ:-jq}"` and a `require_jq` function that returns 1 with the message `github-tree-selftest: '<JQ_BIN>' not found on PATH — install jq to run this harness` on stderr when `command -v "$JQ_BIN"` fails, and 0 otherwise.
  Call `require_jq || exit 1` up front, before any test runs, so a missing `jq` produces that line rather than an obscure downstream failure.
  Reading `$JQ_BIN` at call time rather than capturing it at definition time is what lets test 20 exercise the guard in a subshell without recursion.

  Set up run-time state under `SCRATCH="$PLUGIN_ROOT/../../.scratch/github-tree-selftest"`, created fresh at the start of the run and used for every generated map file and for the stub's call log.
  Never write to a system temporary directory.
  Prepend the stub's directory to `PATH` for each invocation of the script under test, and export `GH_STUB_MAP`, `GH_STUB_BODIES`, and `GH_STUB_LOG` per scenario.

  Provide two helpers used by every test.
  `run_scenario <scenario-name> <map-content> <args...>` writes `<map-content>` to `$SCRATCH/<scenario-name>/map.tsv`, truncates `$SCRATCH/<scenario-name>/calls.log`, then runs `github-tree.sh` with the stub on `PATH`, capturing stdout, stderr, and the exit status into the shell variables `out`, `err`, and `status` — capturing stdout and stderr into separate variables is mandatory, since a great many assertions here turn on stdout being byte-empty while stderr is not.
  `calls <scenario-name>` prints the scenario's call log.
  Map content is tab-separated: endpoint, body filename, and an optional HTTP status marker;
  build it with `printf` and explicit `\t` escapes so the tabs are unambiguous in the source.

  Then write these twenty-two tests, in this order.
  Every one of tests 9 through 18 additionally asserts that stdout is byte-empty and that the exit status is non-zero, because that pair *is* the all-or-nothing contract and asserting it once per error path is the point.

  Test 1, fast path untruncated: scenario `small`, repo `acme/small`, no path argument, map routing `repos/acme/small/git/trees/HEAD?recursive=1` to `small-root-rec.json`.
  Assert stdout is exactly the three lines `intro.md`, `src/main.go`, `src/util.go` in that order;
  assert the bare directory path `src` does not appear as a line of its own;
  and assert the call log holds exactly one line.
  That one-line assertion is the regression guard covering all three removed calls at once — the duplicate truncation check, the branch resolve, and the `gh auth status` preflight.

  Test 2, path scoping on the fast path: scenario `scoped`, repo `acme/scoped`, path argument `src`, map routing `repos/acme/scoped/git/trees/HEAD:src?recursive=1` to `scoped-src-rec.json`.
  Assert stdout is exactly `src/main.go` then `src/deep/x.go`, proving the prefix is re-applied to the API's subtree-relative paths;
  assert the call log holds exactly one line, and that its endpoint is the `HEAD:src` form.

  Test 3, path normalization: run the `scoped` scenario three times with path arguments `src`, `/src`, and `src/`.
  Assert all three produce byte-identical stdout and byte-identical call logs.

  Test 4, one-level truncated fallback and sibling order: scenario `trunc1`, repo `acme/big`, no path argument, map routing `repos/acme/big/git/trees/HEAD?recursive=1` to `trunc1-root-rec.json`, `repos/acme/big/git/trees/HEAD` to `trunc1-root-nonrec.json`, and `repos/acme/big/git/trees/tmmm?recursive=1`, `.../taaa?recursive=1`, `.../tbbb?recursive=1` to the three matching subtree bodies.
  Assert stdout is exactly these six lines in this order: `zzz.txt`, `Makefile`, `mmm/m1.txt`, `mmm/sub/m2.txt`, `aaa/a1.txt`, `bbb/b1.txt`.
  Three separate properties ride on that one assertion and each deserves its own `fail` message if the harness can cheaply separate them: `Makefile` appears only in the non-recursive listing, so its presence proves the root's own blobs are collected at the re-list step;
  the order is neither git-sorted nor sibling-reversed, so it proves a FIFO queue and the absence of a `sort` pass;
  and `aaa` and `bbb` appear in no truncated response, so their presence proves subtrees are enqueued from the non-recursive listing.
  Assert the call log holds exactly five lines.

  Test 5, two-level truncated fallback: scenario `trunc2`, repo `acme/deep`, no path argument, map routing `.../HEAD?recursive=1` to `trunc2-root-rec.json`, `.../HEAD` to `trunc2-root-nonrec.json`, `.../ta?recursive=1` to `trunc2-a-rec.json`, `.../tb?recursive=1` to `trunc2-b-rec.json`, `.../tb` to `trunc2-b-nonrec.json`, `.../tbx?recursive=1` to `trunc2-bx-rec.json`, and `.../tby?recursive=1` to `trunc2-by-rec.json`.
  Assert stdout is exactly these five lines in this order: `r.txt`, `a/a1.txt`, `b/bown.txt`, `b/x/x1.txt`, `b/y/y1.txt`.
  Assert specifically that `b/y/y1.txt` is present — `y` exists only in the non-recursive re-fetch of `b`, so an implementation reading children from the truncated response loses it.
  Assert that `b/bown.txt` is present, since `b`'s own blob likewise exists only in that re-fetch.
  Assert the call log contains exactly one line for `ta`, and no line requesting `ta` without `?recursive=1` — an untruncated sibling must never be re-listed.
  Assert the call log holds exactly seven lines.

  Test 6, scoped listing whose subtree truncates: scenario `scopedtrunc`, repo `acme/scopedbig`, path argument `src`, map routing `.../HEAD:src?recursive=1` to `scopedtrunc-src-rec.json`, `.../HEAD:src` to `scopedtrunc-src-nonrec.json`, and `.../tlib?recursive=1` to `scopedtrunc-lib-rec.json`.
  Assert stdout is exactly `src/s.txt` then `src/lib/l1.txt`, proving prefixes stay repo-relative below the scoping point.
  Assert the call log holds exactly three lines and that none of them requests `git/trees/HEAD` or `git/trees/HEAD?recursive=1` — no sibling of the scoped directory may ever be fetched.

  Test 7, entry types: scenario `types`, repo `acme/types`, map routing the recursive root endpoint to `types-root-rec.json`.
  Assert stdout is exactly `link` then `real.txt`;
  assert `vendor/dep` is absent, since a submodule is not readable through the contents API;
  assert `dir` is absent.

  Test 8, zero blobs is success: scenario `noblobs`, repo `acme/subsonly`, map routing the recursive root endpoint to `noblobs-root-rec.json`.
  Assert the exit status is exactly 0 and stdout is byte-empty.
  This is the test that makes "empty stdout" ambiguous by design, which is why every error test also asserts a non-zero status.

  Test 9, a returned path containing a tab: scenario `badpath`, repo `acme/badpath`, map routing the recursive root endpoint to `badpath-root-rec.json`.
  Assert stderr contains the two-character sequence backslash-t, so the offending path is reported JSON-escaped rather than mangling the terminal.

  Test 10, a non-recursive listing that is itself truncated: scenario `nonrectrunc`, repo `acme/nonrectrunc`, map routing `.../HEAD?recursive=1` to `trunc1-root-rec.json` and `.../HEAD` to `nonrectrunc-root-nonrec.json`.
  Assert stderr contains `truncated`.

  Test 11, mid-walk failure and the buffering proof: scenario `midwalk`, repo `acme/midwalk`, map routing `.../HEAD?recursive=1` to `trunc1-root-rec.json`, `.../HEAD` to `trunc1-root-nonrec.json`, `.../tmmm?recursive=1` to `trunc1-mmm-rec.json`, and `.../taaa?recursive=1` to `error-403.json` with status marker `403`.
  By the time `taaa` fails, the root's own two blobs and the whole of `mmm` have been collected, so asserting stdout is byte-empty here is what proves the listing is buffered rather than streamed — a streaming implementation passes every other test in this harness and fails only this one.
  Also assert the call log contains no line requesting `tbbb`, proving the walk aborts on the first failure rather than continuing.

  Test 12, HTTP 401: scenario `err401`, repo `acme/e401`, map routing the recursive root endpoint to `error-401.json` with status marker `401`.
  Assert stderr contains the literal substring `not authenticated`, matching card 5's fixed wording rather than paraphrasing it.

  Test 13, HTTP 403: scenario `err403`, repo `acme/e403`, map routing the recursive root endpoint to `error-403.json` with status marker `403`.
  Assert stderr contains the literal substring `rate limited`, matching card 5's fixed wording rather than paraphrasing it — every assertion in this harness that inspects an error message greps for a substring the fixed wording literally contains, never a reworded form of it.

  Test 14, HTTP 404 on an unscoped fetch: scenario `err404`, repo `acme/e404`, no path argument, map routing `.../HEAD?recursive=1` to `error-404.json` with status marker `404`.
  Assert stderr names all three causes — that the repository was not found, that it may not be accessible with this token, and that it may have no commits yet — since the script deliberately does not spend a second call telling them apart.

  Test 15, scoped 404 versus 422 as distinct messages: run repo `acme/e404` with path argument `nope` against a map routing `.../HEAD:nope?recursive=1` to `error-404.json` with status marker `404`, and repo `acme/e422` with path argument `notadir` against a map routing `.../HEAD:notadir?recursive=1` to `error-422.json` with status marker `422`.
  Assert both exit non-zero with byte-empty stdout, that the 404 message says the path was not found, that the 422 message says the path is not a directory, and — the actual point of the test — that the two stderr strings are not equal to each other.

  Test 16, missing or malformed repository argument: invoke with no arguments at all, and separately with the single argument `notaslug`.
  Assert both exit non-zero with byte-empty stdout, and that the call log is empty in both cases.

  Test 17, a path needing URL encoding: invoke repo `acme/small` with each of the path arguments `src dir`, `a#b`, and `naïve`.
  Assert each exits non-zero with byte-empty stdout, that the call log is empty every time — the rejection must happen before any `gh` call — and that stderr contains the offending character or characters: a space for the first, `#` for the second, and `ï` for the third.
  The `naïve` case is the one that matters most: it must report `ï` whole, not a lone invalid byte, so this assertion is what pins the locale-independent rejection mechanism specified in batch 2, card 5, rather than a byte-indexed character walk that behaves differently under a `C` locale than a UTF-8 one.

  Test 18, `gh` missing from `PATH`: invoke `github-tree.sh` with `PATH` set to the empty string and a valid repository argument.
  Assert a non-zero exit, byte-empty stdout, and stderr mentioning `gh`.

  Test 19, stdout cleanliness: re-run the `small` and `trunc1` scenarios and assert that no line of stdout begins with `#`, so neither the `#trunc` header nor a `#badpath` sentinel can leak into the path list, and that every stdout line is non-empty.

  Test 20, the harness's own prerequisite guard: in a subshell, set `JQ_BIN` to `definitely-not-jq` and call `require_jq`, capturing its stderr and status.
  Assert it returns non-zero and that its message contains both `definitely-not-jq` and `install jq`.
  Running the guard directly in a subshell rather than re-invoking the harness is deliberate — it exercises the same function the up-front check calls, with no recursion to bound.

  Test 21, too many arguments: invoke `github-tree.sh` with three arguments, `acme/small`, `src`, and `extra`.
  Assert exit status 2 specifically — not merely non-zero, since 2 is the usage code the script reserves and distinguishing it from the general failure code is the only thing that makes it worth reserving — byte-empty stdout, an empty call log, and stderr containing the literal `usage:`.
  Test 16 covers the argument-count check's lower bound;
  this covers its upper bound, which is a separate branch of the same condition and was otherwise unexercised.

  Test 22, the stub's own rejection path: invoke the stub `gh` directly, bypassing `github-tree.sh` entirely, with a deliberately wrong shape — `auth status` — and the three `GH_STUB_*` variables exported as usual.
  Assert exit status 98 and stderr containing `unsupported invocation`, then assert that the call log nonetheless gained a line, since the stub logs before it validates and a test asserting "no call was made" elsewhere in this harness depends on that ordering holding.
  Every other test in this harness confirms the one-call property by counting log lines, which catches a re-added `gh auth status` preflight by arithmetic alone;
  this one confirms the mechanism that is supposed to make such a call fail loudly rather than be silently absorbed, and it is the only test that exercises the stub as the thing under test rather than as scaffolding.

  Delete the `$SCRATCH` tree at the end of a successful run, in the same spirit as `selftest.sh`'s closing `clean_bin` call.
- **Commit:** `test(prowler): add offline github-tree-selftest harness`

## Batch Tests

`verify:` for this batch is `bash -n` over the two shell files it creates — the stub `gh` and `github-tree-selftest.sh`.
It is a syntax gate rather than a behavioural one because the harness this batch writes exercises `github-tree.sh`, which does not exist until batch 2;
running it here would fail by construction.
The JSON fixture bodies have no runnable surface of their own and are validated transitively: the stub feeds each of them through real `jq`, so a malformed body fails batch 2's harness run loudly at the scenario that uses it.
The behavioural gate for everything this batch creates is batch 2's `verify:`, which runs the harness for real.
