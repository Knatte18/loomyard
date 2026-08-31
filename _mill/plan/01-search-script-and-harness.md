# Batch: search-script-and-harness

```yaml
task: Add cross-repo code search to prowler
batch: search-script-and-harness
number: 1
cards: 5
verify: bash plugins/prowler/scripts/github-code-search-selftest.sh
depends-on: []
```

## Batch Scope

This batch delivers the whole runnable capability: the offline fixture tree, the stub `gh` that serves it, the selftest harness that asserts the contract, and `github-code-search.sh` itself.
It is one batch because none of the four pieces is independently verifiable — a fixture without a stub is inert, a harness without a script fails by construction, and a script without a harness has no proof of contract at all.
The card order follows the discussion's stated workable order (capture fixtures → write the stub → write the harness scenarios → write the script against them);
the batch-level `verify:` runs only once at the end of the batch, so the intermediate cards legitimately land with the harness still failing.

The external interface batch 2 consumes is the script's command line and output contract: `github-code-search.sh <query> <owner/repo> [<owner/repo>...]`, one `<owner>/<repo>\t<path>\t<snippet>` record per matching file on stdout, diagnostics on stderr, exit 2 for a usage-shape error and exit 1 for every other rejection.

Batch-local decisions that differ from `## Shared Decisions` in the overview:

- **The new stub keys fixtures on a request key, not on the endpoint alone.**
  `testdata/github-tree/bin/gh` keys `map.tsv` on the bare endpoint, which works there because every call in a tree walk hits a distinct endpoint.
  Every search call in a multi-repo sweep hits the identical `search/code` endpoint, so an endpoint-keyed map cannot express a single one of the per-repo scenarios this harness needs.
  The `q` value is what already distinguishes them, and each repo's search call carries a distinct `q` by construction.
- **A separate stub and fixture tree, not a generalised shared one.**
  The existing tree stub hard-asserts a four-argument invocation and rejects anything else with exit 98, deliberately, so a re-added preflight call in `github-tree.sh` would fail loudly.
  Generalising it to accept both shapes would destroy exactly that property.

## Cards

### Card 1: offline fixture bodies for the code-search harness

- **Context:**
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-401.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-403.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-404.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-422.json`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/small-root-rec.json`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/preflight-ok.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/error-401.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/error-403.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/error-404.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/error-422.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-alpha.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-beta.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-gamma.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-multi.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-zero.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-truncate.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-no-textmatches.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/incomplete.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/capped.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/badpath-path.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/badpath-fullname.json`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the fixture body tree for the new harness.
  Every file is a JSON document reproducing a real GitHub REST response shape.
  Search responses carry top-level `total_count` and `incomplete_results` plus an `items` array;
  each item carries `repository.full_name`, `path`, and (usually) a `text_matches` array whose entries carry `fragment`, `property`, and `indices`.
  Error bodies reproduce the four-field shape the sibling tree fixtures already use (a `message`, a `documentation_url`, and a `status` string) — read those four sibling error fixtures and copy their shape exactly, changing only the message where a code-search-specific message reads better.
  Fixture bodies are pretty-printed with two-space indentation, matching the sibling tree fixture tree.

  The preflight fixture is shared by every repo in every scenario — the script never reads a field out of it, only the call's success or failure, so one body serves all preflight calls:

  ```json
  {
    "full_name": "acme/preflight-ok"
  }
  ```

  The three ordering fixtures each hold one repo's hits and are used to prove multi-repo output ordering and per-repo call identity:

  ```json
  {
    "total_count": 2,
    "incomplete_results": false,
    "items": [
      {
        "path": "src/alpha1.go",
        "repository": { "full_name": "acme/alpha" },
        "text_matches": [
          { "property": "content", "fragment": "alpha one", "indices": [0, 5] }
        ]
      },
      {
        "path": "src/alpha2.go",
        "repository": { "full_name": "acme/alpha" },
        "text_matches": [
          { "property": "content", "fragment": "alpha two", "indices": [0, 5] }
        ]
      }
    ]
  }
  ```

  hits-beta.json holds one item with path "beta.md", full_name "acme/beta", fragment "beta hit", and a total_count of 1.
  hits-gamma.json holds one item with path "gamma.rs", full_name "acme/gamma", fragment "gamma hit", and a total_count of 1.

  hits-multi.json holds three items for full_name "acme/multi" with a total_count of 3, and is the fixture that proves fragment sanitation.
  Its first item has path "docs/guide.md" and a fragment containing a real newline, written in JSON as "use tree-sitter\nfor parsing".
  Its second item has path "src/parser.rs" and a fragment containing a real tab, written in JSON as "let ts =\ttree_sitter::Parser".
  Its third item has path "notes.txt" and the plain fragment "bump tree-sitter".

  hits-zero.json is the empty result set: a total_count of 0, incomplete_results false, and an empty items array.

  hits-truncate.json holds two items for full_name "acme/trunc" with a total_count of 2.
  Its first item has path "long.txt" and a fragment that is exactly 250 repetitions of the character x, so the 200-character truncation boundary is observable.
  Its second item has path "two.txt" and a text_matches array with exactly two entries, whose fragments are "first fragment" and "second fragment", so "only the first fragment is emitted" is observable.

  hits-no-textmatches.json holds two items for full_name "acme/nomatch" with a total_count of 2.
  Its first item has path "absent.txt" and no text_matches key at all.
  Its second item has path "empty.txt" and a text_matches key whose value is an empty array.

  incomplete.json is the partial-result-set body: a total_count of 0, incomplete_results **true**, and an empty items array.
  This is the exact shape a malformed qualifier returns live, with HTTP 200 and exit 0.

  capped.json is the more-hits-than-returned body: a total_count of 250, incomplete_results false, and exactly two items for full_name "acme/capped" with paths "c1.txt" and "c2.txt" and fragments "cap one" and "cap two".

  badpath-path.json holds one item for full_name "acme/badpath" whose path contains a real tab, written in JSON as "src/a\tb.txt", with a fragment "bad path" and a total_count of 1.
  badpath-fullname.json holds one item whose repository full_name contains a real newline, written in JSON as "acme/bad\nname", with path "ok.txt", fragment "bad name", and a total_count of 1.
- **Commit:** `test(prowler): add offline fixture bodies for github-code-search`

### Card 2: stub `gh` keyed on a request key

- **Context:**
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/preflight-ok.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-alpha.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/error-403.json`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/testdata/github-code-search/bin/gh`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the offline stand-in for the real GitHub CLI that drives `github-code-search.sh` with no network access.
  Read the sibling tree stub first and copy its structure: `set -u`, the up-front loop requiring `GH_STUB_MAP`, `GH_STUB_BODIES`, and `GH_STUB_LOG` (exit 99 when any is unset), the `printf '%s\n' "$*" >> "$GH_STUB_LOG"` log line placed **before** any shape validation, the tab-splitting `while IFS= read -r line || [ -n "$line" ]` map loop with its `|| [ -n "$line" ]` last-line guard and its three `field1`/`field2`/`field3` extractions, the exit 97 no-fixture path, the `$GH_STUB_BODIES/$matched_body` body resolution, the failure emulation (`cat` the body to **stdout**, nothing to stderr, `--jq` never applied, exit 1), and the success path `jq -r "$jq_expr" "$body_path"`.
  Keep the file extensionless and executable (mode 755), matching the sibling stub.

  Two deliberate divergences from the sibling stub, both to be argued in the stub's own header comment:

  1. **It accepts exactly two invocation shapes, not one.**
     The preflight shape is the sibling's four-argument `api <endpoint> --jq <expr>`.
     The search shape is `api -X GET <endpoint> -f <key>=<value> ... -H <header> --jq <expr>`.
     Parse the argument vector rather than positionally indexing it: require the first argument to be the literal `api`, then walk the remainder collecting the `-X` value, every `-f` key=value pair, every `-H` value, the `--jq` value, and the single non-flag positional as the endpoint.
     Any argument vector that does not resolve to one of the two shapes is rejected with the sibling's `gh-stub: unsupported invocation: $*` message on stderr and exit 98.
  2. **`map.tsv`'s first field is matched against a request key, not against the bare endpoint.**
     The request key is the endpoint string when the invocation carries no `-f q=` parameter (every preflight call, whose endpoint is already unique per repo), and `<endpoint>?q=<the full q parameter value>` when it does (every search call).
     `map.tsv` keeps its existing three-field `key<TAB>body<TAB>status` shape and the existing tab-split-by-parameter-expansion loop;
     only the value compared against `field1` changes.
     The key is unique within a scenario because preflight endpoints are one per distinct repo and search `q` values are one per distinct repo — duplicate refs never reach the network at all, having been deduped during the script's argument handling.

  The stub asserts the search shape's mandatory parts rather than tolerating them, because a stub that ignores them lets a live-only failure through: a search invocation missing `-X GET`, missing an `-H` argument whose value is exactly `Accept: application/vnd.github.text-match+json`, or missing `--jq` is rejected with the same exit 98 unsupported-invocation path.
  A search invocation must also carry a `-f` parameter whose key is `q`;
  without one there is no request key to look up, so it too is an unsupported invocation.
  The `-f per_page=100` parameter is accepted and ignored — the fixtures encode the response, so page size has no effect offline.

  On the failure path the stub reproduces the real `gh api` failure shape exactly as the sibling does, because that is what lets the script under test parse the HTTP status out of the raw JSON error body itself.
  This holds for both shapes: a search call with a status column in `map.tsv` writes its body to stdout with `--jq` unapplied and exits 1, identically to a preflight call.
- **Commit:** `test(prowler): add request-key-driven stub gh for github-code-search`

### Card 3: selftest harness — structure and success-path scenarios

- **Context:**
  - `plugins/prowler/scripts/github-tree-selftest.sh`
  - `plugins/prowler/scripts/testdata/github-code-search/bin/gh`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/preflight-ok.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-alpha.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-beta.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-gamma.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-multi.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-zero.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-truncate.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-no-textmatches.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/capped.json`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/github-code-search-selftest.sh`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the offline harness, mirroring the sibling tree harness's structure exactly.
  Read that sibling first and copy: the header comment (including its honest not-covered list and its jq-versus-gojq seam paragraph and its portability-envelope paragraph), `set -u`, the `SCRIPT_DIR`/`PLUGIN_ROOT` self-location, the `BASH_BIN="$(command -v bash)"` capture with its comment explaining why it is resolved before the PATH-stripping scenario, the `STUB_BIN`/`BODIES_DIR` bindings pointed at the new fixture tree, the `SCRATCH="$PLUGIN_ROOT/../../.scratch/github-code-search-selftest"` scratch root, the `failures` counter with its `fail` and `pass` helpers, the `JQ_BIN` indirection reading an environment override at call time inside `require_jq`, the `rm -rf`/`mkdir -p` scratch reset at the start and the `rm -rf` at the end, and the final pass/fail summary block with its exit 0 / exit 1 split.
  Name the harness's own jq override environment variable after this harness, not after the tree one.

  Provide the same three scenario helpers, adapted:

  - `run_scenario <name> <map-content> [script-args...]` writes the map content to that scenario's `map.tsv`, truncates that scenario's `calls.log`, runs the script under test with the stub prepended to `PATH` and the three `GH_STUB_*` variables exported, and captures stdout, stderr, and the exit status into the variables `out`, `err`, and `status`.
    Capturing stdout and stderr into separate variables is mandatory — a great many assertions turn on stdout being byte-empty while stderr is not.
  - `calls <name>` prints that scenario's call log verbatim.
  - `call_line_count <name>` prints its line count.

  Replace the sibling's `call_count_for_endpoint` with two request-key-aware helpers, because the search calls all share one endpoint and are only distinguishable by their `q` value:

  - `preflight_call_count <scenario> <owner/repo>` counts logged calls whose logged argument vector is the four-argument preflight shape naming exactly that repo's endpoint.
  - `search_call_count <scenario> <q-substring>` counts logged calls that are search-shaped and whose logged argument vector contains the given substring.
    Match by shell `case`/`[[ == *…* ]]` globbing rather than grep, so a `q` value's own characters are never read as regex metacharacters.

  Then write the success-path scenarios.
  Each asserts the exit status, the exact stdout bytes where stdout is expected to be non-empty, and the call log where call count or call identity is the point of the scenario.

  1. **Single repo, several hits, fragment sanitation.**
     One repo whose search fixture is the three-item multi fixture.
     Assert exit 0 and exact stdout of three tab-separated records in the fixture's own item order, whose first fields are the API's full_name, and whose third fields are the fragments with the embedded newline and the embedded tab each collapsed to a single space.
     This is the scenario that proves a fragment never breaks the one-record-per-line discipline.
  2. **Single repo, zero hits.**
     Assert exit 0, byte-empty stdout, and that the preflight call was still made (exactly one preflight call and exactly one search call).
  3. **Multiple repos: ordering, call count, and call identity.**
     Three distinct repos mapped to the three ordering fixtures, supplied in an argument order that differs from alphabetical so the assertion is meaningful.
     Assert exit 0;
     assert exact stdout with the repos' record blocks in the repo-argument order and, within each block, the fixture's own item order;
     assert the call log has exactly six lines (three preflight, three search);
     and assert call identity by checking that each repo's own search call is present, i.e. that a search call was logged carrying the shared caller query with that repo's own `repo:` qualifier appended.
  4. **Every search invocation carries `-X GET`, the text-match `Accept` header, and `--jq`.**
     Re-run the three-repo scenario's map and assert against the logged argument vectors directly that every search-shaped line contains `-X GET`, contains the literal `Accept: application/vnd.github.text-match+json`, and contains `--jq`.
     Assert this against the log rather than trusting the stub's own rejection, so the assertion survives a future loosening of the stub.
  5. **Fragment truncation and multiple text_matches.**
     One repo mapped to the truncation fixture.
     Assert exit 0 and that the first record's third field is exactly 200 characters long, and that the second record's third field is the first of the item's two fragments and not the second.
  6. **An item whose text_matches array is absent or empty.**
     One repo mapped to the no-text-matches fixture.
     Assert exit 0 and exact stdout of two records, each ending with a trailing tab and an empty third field, proving the record shape stays invariant rather than the record being dropped or the script crashing.
  7. **total_count greater than the returned item count.**
     One repo mapped to the capped fixture.
     Assert exit 0, the full two-record stdout, and one stderr note naming both the repo and the true total.
  8. **Duplicate repo refs are deduped, preserving first-occurrence order.**
     The same repo ref passed twice plus a third distinct repo.
     Assert exactly two preflight calls and two search calls, that the duplicated repo's records appear exactly once, and that stdout is ordered by first occurrence.
  9. **Duplicate repo refs are deduped case-insensitively.**
     The same repo spelled in two different cases plus a third distinct repo.
     Assert exactly two preflight calls and two search calls, that the duplicated repo's records appear exactly once, and that the emitted first field carries the API's own full_name rather than either caller spelling.
  10. **Dedup happens before the cap.**
      Eleven refs of which two are duplicates of each other, so ten are distinct.
      Assert the sweep runs normally — exit 0, ten preflight calls and ten search calls — rather than being rejected by the ten-repo cap.
      Map every one of those ten repos' search calls to the empty-result fixture so the scenario's map stays legible.
- **Commit:** `test(prowler): add github-code-search selftest harness and success scenarios`

### Card 4: selftest harness — failure, rejection, and edge scenarios

- **Context:**
  - `plugins/prowler/scripts/github-tree-selftest.sh`
  - `plugins/prowler/scripts/testdata/github-code-search/bin/gh`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/preflight-ok.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-alpha.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-beta.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/hits-gamma.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/error-401.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/error-403.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/error-404.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/error-422.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/incomplete.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/badpath-path.json`
  - `plugins/prowler/scripts/testdata/github-code-search/bodies/badpath-fullname.json`
- **Edits:**
  - `plugins/prowler/scripts/github-code-search-selftest.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Append the failure, argument-rejection, and edge scenarios to the harness, using the helpers card 3 established and keeping the existing numbered-comment banner style.
  Every one of these scenarios asserts byte-empty stdout unless stated otherwise, and every failure scenario asserts a distinguishing stderr substring so two different failures can never be conflated by an assertion that merely checks for a non-zero exit.

  1. **incomplete_results true on repo 2 of several.**
     Three repos, the second mapped to the partial-result fixture.
     Assert a non-zero exit, byte-empty stdout, and stderr naming the offending repo.
     Byte-empty stdout here despite repo 1 having succeeded is itself the proof that output is buffered, not streamed.
  2. **Preflight 404.**
     Assert a non-zero exit, byte-empty stdout, stderr distinguishing "not found or not accessible with this token", and — asserted against the call log — that **no** search call was made at all.
  3. **Preflight 401.**
     Same shape, with stderr distinguishing the not-authenticated case and naming the `gh auth login` remedy, and the same no-search-call assertion.
  4. **Preflight 403.**
     Same shape, with stderr distinguishing the access-denied-or-rate-limited case, and the same no-search-call assertion.
  5. **All preflights run before any search call.**
     Three repos where the **second** repo's preflight fails 404.
     Assert zero search calls in the log, and preflight calls for repos 1 and 2 only.
     This scenario is what pins the global ordering: a per-repo interleaving would have issued repo 1's search before ever reaching repo 2's preflight, and the three preceding preflight-failure scenarios all fail on repo 1, so none of them can distinguish the two orderings.
  6. **Search 403 mid-sweep, repo 2 of 3.**
     All three preflights succeed;
     repo 1's search succeeds and repo 2's search returns 403.
     Assert a non-zero exit, byte-empty stdout despite repo 1 having produced records, stderr naming both the offending repo and the 403 status, and that repo 3's search call was never made.
  7. **Search 422.**
     One repo whose search call returns 422.
     Assert a non-zero exit, byte-empty stdout, and stderr carrying the status.
  8. **A returned path containing a tab.**
     One repo mapped to the bad-path fixture.
     Assert a non-zero exit, byte-empty stdout, and stderr identifying the refusal as the one-record-per-line output being unable to represent the value.
  9. **A returned full_name containing a newline.**
     One repo mapped to the bad-full-name fixture, asserting the same three properties.
     This is the assertion that pins the sentinel covering the repository field and not only the path field.
  10. **Argument rejection, all before any network call.**
      Each of the following asserts byte-empty stdout, an empty call log, its own exit code, and its own distinguishing stderr substring:
      no arguments at all exits 2 with the bare usage synopsis;
      a query with no repo ref exits 2 with the same synopsis;
      an invalid `<owner>/<repo>` ref exits 1 naming the offending ref;
      eleven distinct repo refs exits 1 naming the ten-repo cap and the rate-limit bucket that motivates it;
      a caller query containing the substring `repo:` exits 1 pointing at the positional repo arguments and at the raw `gh api` escape hatch;
      an empty-string query exits 1 naming the query argument;
      a whitespace-only query exits 1 naming the query argument.
  11. **`gh` missing from PATH.**
      Invoke the script through the absolute `BASH_BIN` path with `PATH` emptied, exactly as the sibling harness's own equivalent scenario does, so the script's own prerequisite guard is what fires rather than the harness failing to find bash.
      Assert a non-zero exit, byte-empty stdout, and stderr mentioning gh.
  12. **The harness's own `require_jq` guard.**
      In a subshell, override the jq binary name to a name that cannot exist and call `require_jq` directly, asserting a non-zero return, the missing binary's name in the message, and the install hint.
      Bound it in a subshell so nothing recurses back into this script.
  13. **The stub's own rejection path.**
      Invoke the stub directly with an invocation shape it must refuse (an `auth status` call, which this script never makes), asserting exit 98, the unsupported-invocation message on stderr, and that the call was still appended to the log before being rejected.
      Add a second direct invocation asserting that a search-shaped call **missing** `-X GET` is refused with the same exit 98, since that is the assertion that keeps the live-only POST-by-default failure mode out of reach.
  14. **stdout cleanliness.**
      Re-run two of the success scenarios and assert that no emitted line begins with the `#` sentinel prefix and no emitted line is empty, mirroring the sibling harness's own equivalent scenario.
- **Commit:** `test(prowler): add github-code-search failure and rejection scenarios`

### Card 5: github-code-search.sh

- **Context:**
  - `plugins/prowler/scripts/github-tree.sh`
  - `plugins/prowler/scripts/github-code-search-selftest.sh`
  - `plugins/prowler/scripts/testdata/github-code-search/bin/gh`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/github-code-search.sh`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the script the harness asserts against.
  Read the sibling tree script first and mirror its shape: the `set -u`, the `die()` helper that prints one stderr line and exits 1 (with its comment noting the one case that exits 2 instead), the explicit prerequisite-then-argument-then-network section banner, and a header comment that argues the design the way the sibling's does rather than restating the code.
  The header comment must carry the same no-retries-no-backoff argument the sibling makes — every non-2xx response aborts immediately with one stderr line and it is the caller's job to decide whether to re-invoke, because a transient 403 (secondary rate limit) and a permanent one (token lacks access) are indistinguishable from inside the script — and must state the buffer-until-complete rule and the ten-repo cap's rate-limit rationale.
  Mark the file executable (mode 755), matching the sibling.

  The synopsis is `github-code-search.sh <query> <owner/repo> [<owner/repo>...]`.
  Execute the steps below in exactly this order, so that every rejection is reached before any network call and two simultaneously-invalid arguments always produce the same message:

  1. `command -v gh` prerequisite check;
     on failure `die` with the install-and-authenticate hint, matching the sibling's wording.
  2. Argument-count check: fewer than two arguments prints the bare `usage:` synopsis to stderr and exits **2**.
     This is the only exit-2 path in the script.
  3. Bind the query to the first argument and shift;
     the remaining arguments are the repo refs.
  4. Reject a query that is empty or contains only whitespace, via `die`, naming the query argument.
     Arity alone does not catch this, and live behaviour makes it dangerous: a query carrying only a repo qualifier returns hits, so an empty query silently degrades into "the first 100 indexed files in this repo" — a tree listing wearing a search's output shape, burning a request from the scarcest bucket in the plugin to produce something the sibling tree script already provides for free.
  5. Reject a query containing the substring `repo:`, via `die`.
     The script appends its own repo qualifier last and the last one wins, so the caller's would be silently discarded with no diagnostic.
     The guard is a plain substring test on the whole query, not a qualifier-position test, so it also rejects a search for the literal token as file content — an accepted, documented limitation rather than a bug to engineer around, since distinguishing the two means reimplementing GitHub's query tokenizer in bash.
     The message must therefore both name the limitation and point at a raw `gh api -X GET search/code -f q=...` call as the escape hatch.
  6. Validate every repo ref against the predicate named in the overview's Shared Decisions, aborting via `die` on the first non-matching ref and naming it.
  7. Deduplicate the refs, keeping first-occurrence order, comparing on the **lowercased** ref rather than the raw string, and keeping the caller's original spelling of the surviving first occurrence as the ref forwarded into the qualifier.
     Case-folding is what makes the cap's guarantee hold: GitHub refs are case-insensitive, so an exact-string comparison would let two spellings of one repo both survive, burn two of the ten requests on it, and emit its records twice under one identical name.
  8. Reject more than ten **distinct** refs via `die`, naming the cap and the ten-requests-per-minute `code_search` bucket that motivates it.
     Deduplication before the cap is what makes the cap exact — ten distinct refs is ten calls, never eleven.

  Only then does the script reach the network, in two separate loops rather than one interleaved loop:

  9. **Preflight loop.** For every deduplicated ref in order, run `gh api "repos/<ref>" --jq '.full_name'` with stderr discarded, capture stdout, and check the exit status.
     On failure, recover the HTTP status from the captured body with the same status-extraction regex the sibling's `fetch()` uses, then branch: 401 dies naming the not-authenticated case and the `gh auth login` remedy;
     403 dies naming access-denied-or-rate-limited;
     404 dies naming not-found-or-not-accessible-with-this-token;
     anything else falls back to a body-quoting `die` that collapses the body to one physical line first, so the one-stderr-line promise is kept literally rather than by assumption.
     The preflight exists because a nonexistent repo's search returns HTTP 200 with a zero total_count, byte-identical to a real repo with no matches — without it a typo'd or private repo silently reads as a confident negative answer, the worst available failure mode in the reconnaissance use case this script serves.
     It costs nothing scarce: it hits the 5000-per-hour core bucket, not the ten-per-minute search bucket.
     Running the whole loop to completion before any search call is deliberate, not incidental — a per-repo interleaving would spend search requests on early repos before discovering that a later ref is unusable.
  10. **Search loop.** For every deduplicated ref in order, run one call of the exact shape
      `gh api -X GET "search/code" -f "q=<query> repo:<ref>" -f "per_page=100" -H "Accept: application/vnd.github.text-match+json" --jq "$JQ_EXPR"`, capturing stdout and checking the exit status.
      `-X GET` is mandatory and is the single subtlest thing in this script: `gh api` defaults to POST as soon as any `-f` parameter is present, which would send the query in a request body the search endpoint ignores.
      Say so in a comment at the call site — it is the kind of thing that "works" against a stub that does not check it and fails only live.
      On a non-2xx response, extract the status the same way the preflight does and `die` with one line naming the repo and the status.

  The extraction expression is a single jq program, applied identically by the real gh's embedded gojq at run time and by system jq inside the harness, emitting one header line followed by one line per item:

  ```
  "#meta\t" + (.total_count|tostring) + "\t" + (.incomplete_results|tostring) + "\t" + ((.items|length)|tostring), (.items[] | if ((.repository.full_name | test("[\t\n\r]")) or (.path | test("[\t\n\r]"))) then "#badpath\t" + ((.repository.full_name + "/" + .path) | @json) else .repository.full_name + "\t" + .path + "\t" + ((.text_matches // []) | (if length > 0 then (.[0].fragment // "") else "" end) | gsub("[\t\r\n]+"; " ") | .[0:200]) end)
  ```

  Sanitation happens inside the expression, not in shell, so each record is already one physical line by the time bash sees it.
  The header line is what carries the two response-level fields the records cannot: reading it is how the script learns the total_count and the incomplete-results flag.
  The `#badpath` sentinel carries over from the sibling and is extended to cover the repository name field as well as the path field, because either one containing a tab or newline would break the record shape.
  A `property` of "path" rather than "content" means the fragment is the path rather than file content;
  note that in a comment rather than leaving it as a surprise, but do not branch on it.

  Consume each search response's lines in the main shell via a here-string rather than a pipeline, exactly as the sibling does, so that an abort inside the loop body actually aborts the run instead of only exiting a subshell.
  For each repo:

  - Parse the header line's three fields.
  - If the incomplete-results field is the string true, `die` naming that repo.
    The flag means GitHub returned a partial result set — the search timed out or the query was rejected in a way that still yields 200 — and treating that as "no matches" is the same silent-partial failure the sibling refuses for truncated trees.
  - If a line carries the `#badpath` sentinel, `die` with the escaped value, naming the one-record-per-line output as the reason for refusing.
  - Append every remaining line to the single output buffer shared across all repos.
  - If the total_count exceeds the number of returned items, write exactly one note to stderr naming the repo and the true total, and continue.
    Exit status is unaffected: the returned records are all genuine and complete-as-far-as-they-go, so failing the run would destroy usable output, while staying silent would make a capped listing indistinguishable from a complete one.
    The note goes to stderr specifically so it never pollutes the record stream a caller greps.

  Print the buffer only after the last repo has succeeded, guarding the emission so it is a genuine no-op under `set -u` when the buffer is empty, then exit 0.
  A sweep with zero matches across every repo is a success: exit 0 with byte-empty stdout.
- **Commit:** `feat(prowler): add github-code-search.sh cross-repo code search`

## Batch Tests

`verify:` runs the new harness, `github-code-search-selftest.sh`, and nothing else.
That is the correct scope: this batch's `Creates:`/`Edits:` are the new script, the new harness, the new stub, and the new fixture tree, and the harness is the only runnable surface covering any of them.
It is fully offline — every call goes through the stub `gh` prepended to `PATH`, so the suite makes no network request and consumes nothing from either rate-limit bucket.
The sibling tree harness is deliberately **not** in the verify scope: this batch touches no file under the tree script's own fixture tree and does not edit `github-tree.sh` or its harness, so re-running it would only add wall-clock.

The harness's own header comment must document, as not-asserted, the live checks this offline suite structurally cannot cover, matching the honesty the sibling harness practises about its own gaps:

- One live sweep across three real repos, confirming the record shape and that snippets arrive.
- One live run against a repo with more than 100 matches, confirming the cap note fires.
- One live run against a deliberately misspelled repo, confirming the preflight catches what the search API would otherwise have reported as zero hits.
- A spot-check that the extraction expression behaves identically under gojq (production, via gh) and jq (the harness) — the same acknowledged seam the sibling harness documents.

Those live checks must be paced against the ten-requests-per-minute `code_search` bucket;
they belong in a manual verification pass, never in an automated loop.
This batch adds no Go code, so the prowler module's own `go test ./...` is unaffected and is not part of `verify:`.
