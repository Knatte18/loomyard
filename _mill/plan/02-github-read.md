# Batch: github-read raw-first script and its offline harness

```yaml
task: "Prefer raw fetch, scope large tree listings"
batch: "github-read raw-first script and its offline harness"
number: 2
cards: 8
verify: bash plugins/prowler/scripts/github-read-selftest.sh
depends-on: []
```

## Batch Scope

This batch delivers `github-read.sh` — a new script that reads one file's content to stdout, trying `raw.githubusercontent.com` first and falling back to `gh api` only on failure — together with the sibling offline harness, stub `curl`, stub `gh`, and fixture bodies that assert it.
It is one batch because the script's error contract and the harness's fixtures are co-designed: the fallback's status-extraction ordering is pinned against a live capture whose output becomes the stub's fixtures, so splitting them would put the premise in one batch and its proof in another.

The batch touches no file batch 1 touches, so the two are independent and can run in either order or concurrently.
The external interface batch 3 consumes is the finished CLI surface `github-read.sh <owner/repo> <path>`, its exit-code contract, and whichever symlink behaviour card 8's live capture actually observes.

Batch-local decision beyond the overview's Shared Decisions: the very first card is a live-capture card.
It runs real `gh` and real `curl` against real repositories and builds every failure fixture from what actually came back, because a stub built from a written description proves only the parser, not the premise.

## Cards

### Card 8: live capture of the real `gh` and raw failure shapes, and the fixture bodies built from it

- **Context:**
  - `plugins/prowler/scripts/github-tree.sh`
  - `plugins/prowler/scripts/testdata/github-tree/bodies/error-404.json`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/testdata/github-read/bodies/plain.txt`
  - `plugins/prowler/scripts/testdata/github-read/bodies/withnul.bin`
  - `plugins/prowler/scripts/testdata/github-read/bodies/zero.txt`
  - `plugins/prowler/scripts/testdata/github-read/bodies/probe-file.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/probe-dir.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/probe-symlink.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/probe-submodule.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-401.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-403.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-404.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-404.stderr`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-nostatus.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-multiline.json`
  - `plugins/prowler/scripts/testdata/github-read/CAPTURE.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run the live captures first, before writing any other card in this batch, and build every failure fixture from what actually came back rather than from a written description.
  The public repository `Knatte18/quarry` is reachable with this box's authenticated `gh`, and `gh repo list --visibility private --limit 20` lists private repositories reachable with the same credentials, so every capture below has a real target;
  pick any one private repository from that listing as the private target.
  Both this card's own prose and the capture record must refer to the private target by a placeholder such as `<private-repo>`, never by its literal owner-and-name string, and must genericize any private path that appears inside a captured command or response body the same way.
  This repository is public and this plugin is distributed as an installable package, so a literal private repository name written here or into tracked testdata would permanently disclose that repository's existence to everyone who installs it.
  The captures' value is the observed response shapes, which the placeholder preserves entirely.
  Three captures are unconditional, because a real target for each is guaranteed reachable: a `gh api` contents call against a missing path with the raw Accept header, to learn the 404 failure body and the `gh` stderr line shape;
  and the same call against a real file and against a real directory, using the type-probe jq expression, to learn what the probe answers for each.
  Three further captures are conditional, and each carries the same disposition: attempt it, and if no suitable target exists in any reachable repository, record in the capture file that it was not observed and derive its fixture from the pinned 404 shape instead, noting in that same section that the fixture is derived rather than observed.
  Those three are: the type probe against a symlink entry;
  the type probe against a submodule entry;
  and a default-media-type contents call against a file above roughly one megabyte, to learn whether the probe fails on such a file and with what status.
  One capture is conditional in a different way and must be attempted on a best-effort basis: a `curl` request against a symlink path on `raw.githubusercontent.com`, recording the HTTP status and the body, since nothing in this plan assumes what raw does with one — if no symlink is reachable, record that the question is unresolved, which is the disposition batch 3's conditional sentence reads.
  Two fixtures are explicitly **not** live-captured, and the card must say so rather than pretending otherwise: a 401 cannot be produced without revoking the very authentication every other capture depends on, and a 403 is a rate-limit or access-denied condition that must not be deliberately triggered.
  Derive both, along with the no-status and multi-line bodies, from the pinned 404 envelope shape the first capture observed — same field set, different `status` value and message — and record in the capture file that they are derived from that shape rather than observed.
  Write every observation into `plugins/prowler/scripts/testdata/github-read/CAPTURE.md` as a short record: the command run with every private identifier replaced by the placeholder, the exit status, and the observed stdout and stderr shapes with the same substitution applied, one section per capture.
  Open that file with a one-line note stating that private repository names are deliberately redacted because this testdata ships with the plugin.
  This file is the audit trail that makes the fixtures reviewable as reality rather than as invention, and card 11's status-extraction ordering must follow it wherever it disagrees with this plan's description.
  If raw answers a symlink with a non-2xx status, record that the symlink limitation is empty in practice;
  if raw answers 200 with the target path, record that the limitation stands, because batch 3 writes a caveat sentence conditional on exactly this observation.
  If no file above roughly one megabyte is reachable, record that the contents-API ceiling was not exercised and that the limitation is carried as described rather than as observed.
  Then create the fixture bodies from the captures.
  `plugins/prowler/scripts/testdata/github-read/bodies/plain.txt` is ordinary multi-line file content whose final byte is deliberately not a newline.
  `plugins/prowler/scripts/testdata/github-read/bodies/withnul.bin` contains at least one NUL byte among ordinary text.
  `plugins/prowler/scripts/testdata/github-read/bodies/zero.txt` is a zero-byte file.
  `plugins/prowler/scripts/testdata/github-read/bodies/probe-file.json` is the observed contents-API object response for a file and `plugins/prowler/scripts/testdata/github-read/bodies/probe-dir.json` the observed array response for a directory.
  `plugins/prowler/scripts/testdata/github-read/bodies/probe-symlink.json` and `plugins/prowler/scripts/testdata/github-read/bodies/probe-submodule.json` are object responses whose `type` field carries those two values;
  each is observed when a target was found and otherwise derived from the observed file response with the `type` value changed, per the disposition above.
  Both are created unconditionally either way, because card 14 asserts a rejection scenario for each.
  `plugins/prowler/scripts/testdata/github-read/bodies/error-404.json` is the observed error body carrying a `status` field, and `plugins/prowler/scripts/testdata/github-read/bodies/error-401.json` and `plugins/prowler/scripts/testdata/github-read/bodies/error-403.json` are the same envelope shape with their own status values and messages, derived rather than observed for the reason stated above.
  `plugins/prowler/scripts/testdata/github-read/bodies/error-404.stderr` is the captured `gh` stderr line for a 404, carrying its `(HTTP 404)` fragment.
  `plugins/prowler/scripts/testdata/github-read/bodies/error-nostatus.json` is an error body with no `status` field at all, on one line.
  `plugins/prowler/scripts/testdata/github-read/bodies/error-multiline.json` is an error body with no `status` field spanning several physical lines, which exists so a later scenario can prove the generic diagnosis still emits exactly one physical stderr line.
- **Commit:** `test(prowler): capture real gh and raw failure shapes as github-read fixtures`

### Card 9: `github-read.sh` prerequisites, argument parsing, validation, and temp-file scaffolding

- **Context:**
  - `plugins/prowler/scripts/github-tree.sh`
  - `plugins/prowler/scripts/run.sh`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/github-read.sh`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `plugins/prowler/scripts/github-read.sh` as an executable bash script using `set -u` and no `set -e`, with explicit error handling throughout, mirroring `plugins/prowler/scripts/github-tree.sh`'s structure and header-comment style.
  Give it a header comment stating what it does, that it prefers raw and falls back to `gh api`, that it deliberately has no retries and no backoff and that its curl timeouts are bounds rather than retries, that it reads no file inside the plugin and therefore self-locates no plugin root, that it reads exactly one file per invocation with stdout carrying the content verbatim and nothing else, and that reads are pinned to `HEAD` with no ref argument.
  Add a `die` helper printing one stderr line and exiting 1, and a `usage` helper printing `github-read: usage: github-read.sh <owner/repo> <path>` to stderr and exiting 2.
  Check `command -v gh` first, before any argument handling, so a rejection never reaches the network, and `die` with a message naming `gh`, the GitHub CLI, and `gh auth login` when it is absent.
  `gh` is the only hard prerequisite;
  `curl` is checked separately in card 10 and its absence is never an error.
  Parse arguments with the same loop shape card 1 gives `plugins/prowler/scripts/github-tree.sh`, minus the two flags this script does not have: a `--` terminator is honoured at any position and consumed, every token after it is a positional unexamined, and before it any token beginning with two dashes is a usage error while every other token is a positional.
  A single-dash token is never a flag at any position.
  The argument count is checked on the post-terminator positional list and must be exactly two, so an invocation supplying a terminator and a doubly-dashed path is a legitimate two-positional call rather than a count error.
  Assign the first positional to `REPO` and the second to `RAW_PATH`.
  Copy `plugins/prowler/scripts/github-tree.sh`'s slug check verbatim, including its bracket-range regex form, and record in a comment that its collation looseness is an accepted property here too, because divergence between the two scripts' slug checks would be the worse outcome.
  Copy the path normalisation — stripping every leading and trailing slash — and the character validation in its glob-substitution form together with the reasoning of the long comment above it, condensed but keeping both the collation point and the byte-indexing point;
  the regex form must not be substituted, because it wrongly accepts accented characters under a UTF-8 locale, which is a reproduced failure.
  Unlike `plugins/prowler/scripts/github-tree.sh`, a path that is empty after normalisation is a usage error rather than a whole-repo listing, since there is no such thing as reading a whole repository as one file.
  Record in a comment that the accepted character set is a subset of URL-safe characters, which is what makes URL-encoding unnecessary when the path is interpolated into the raw URL.
  Finally create two temp files with `mktemp`, one for the response body and one for the fallback's captured stderr, both created unconditionally so neither is unset under `set -u`, and arm a single `trap` on `EXIT` removing both, immediately after both exist so no exit path leaks either.
- **Commit:** `feat(prowler): add github-read.sh argument parsing and validation`

### Card 10: `github-read.sh` raw attempt

- **Context:** none
- **Edits:**
  - `plugins/prowler/scripts/github-read.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/scripts/github-read.sh`, add the raw attempt after the temp-file scaffolding.
  Check `command -v curl` and, when it is absent, fall straight through to the fallback with no error and nothing written to stderr — a missing `curl` costs speed, not capability, and warning on every read would be noise the caller cannot act on.
  When `curl` is present, invoke it with exactly this argument vector and no other flags: `-s`, `-f`, `-L`, `--connect-timeout 5`, `--max-time 30`, `-o` followed by the body temp file, and the URL `https://raw.githubusercontent.com/<owner/repo>/HEAD/<normalised path>` built by direct interpolation with no encoding.
  Record in a comment why each of the four significant flags is load-bearing: `-f` is what turns every response at or above 400 into a non-zero exit with an empty output file, so that curl's exit status alone is a sufficient failure signal and no HTTP status needs capturing or parsing;
  without it a plain request answers a 404 with exit 0 and a `404: Not Found` body that would be emitted as if it were the file's content.
  `-s` rather than `-sS` is required because this script never reports the raw attempt's failure, so curl must write no progress or error text to stderr either.
  `-L` is required because the raw host may answer with a redirect, and an unfollowed 301 would be a spurious fallback.
  The two timeouts bound a hung request so it becomes one clean non-zero exit that hands off to the single `gh api` attempt exactly once;
  they are not retries.
  Failure is defined as curl's exit status being non-zero and nothing else — no HTTP status is captured, parsed, or branched on, and body emptiness is never the signal, because an empty file that read successfully is a valid outcome the harness asserts explicitly.
  On a zero exit, `cat` the body temp file to stdout and exit 0.
  Never stream curl's output directly to stdout and never buffer it through command substitution: streaming would leave a truncated prefix on stdout when a connection dies mid-body after a 200 and the fallback would then append a second copy behind it, while command substitution strips every trailing newline and silently drops NUL bytes, corrupting byte fidelity on every read rather than only on failure.
  Record that reasoning in a comment.
- **Commit:** `feat(prowler): add the raw.githubusercontent.com fast path to github-read.sh`

### Card 11: `github-read.sh` `gh api` fallback — type probe, raw-Accept fetch, and diagnosis

- **Context:**
  - `plugins/prowler/scripts/github-tree.sh`
  - `plugins/prowler/scripts/testdata/github-read/CAPTURE.md`
- **Edits:**
  - `plugins/prowler/scripts/github-read.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/scripts/github-read.sh`, add the two-call `gh api` fallback, reached whenever the raw attempt did not succeed.
  Wherever `plugins/prowler/scripts/testdata/github-read/CAPTURE.md` disagrees with the shapes described below, follow the capture and note the divergence in a comment;
  the capture is the pinned premise and this description is only its expected form.
  The first call is a type probe against the contents endpoint for the repository and normalised path, applying the jq expression that answers the string `dir` when the response is a JSON array and otherwise answers the response's own `type` field, so one expression covers both shapes and no runtime `jq` is ever invoked — every JSON field is extracted through `gh api --jq`, which uses `gh`'s embedded engine.
  Redirect its stdout to the body temp file and its stderr to the stderr temp file, capturing the exit status.
  The probe exists because the contents endpoint answers a directory with HTTP 200 and a JSON listing, which a non-zero-exit trigger does not catch and which the no-body-inspection rule would otherwise write to stdout as file content — the one failure mode where the caller cannot tell anything went wrong, since exit 0 plus plausible-looking bytes is indistinguishable from success.
  Record in a comment that both fallback calls are paid only when the raw attempt has already failed, which is the rare and far slower path, and that the probe's own response carries base64 content that is downloaded and discarded — that waste is accepted for the same reason.
  Record too that the probe is a default-media-type contents call and so cannot inline a blob above roughly one megabyte, meaning such a file fails at the probe even though the fetch behind it could have read it;
  this is parity with what the skill documents today, not a regression, and the raw path has no such ceiling.
  When the probe answers anything other than `file`, `die` — a directory gets a message naming the repository and the path, stating it is a directory rather than a file, and pointing the caller at the children-listing mode of the sibling tree script, and any other type gets the same shape with the observed type named in place of the word directory.
  Only when the probe answers `file`, make the second call: the same contents endpoint with the header requesting the raw media type, whose response body is the file content itself, again redirected to the two temp files with its exit status captured.
  The base64-plus-decode form is not used anywhere, since it inflates the transferred payload by roughly a third and adds a parse and a decode step for the same bytes over the same authenticated path.
  On success, `cat` the body temp file to stdout and exit 0.
  On failure of either call, diagnose whichever one exited non-zero through a single shared helper that derives the HTTP status in this order: first by matching the body temp file's contents against the same `status` pattern `plugins/prowler/scripts/github-tree.sh` already uses, since GitHub answers a non-2xx with a JSON error body whatever media type was requested and `gh` writes that body to stdout;
  then, if that yields nothing, by matching the captured stderr for a parenthesised `HTTP` code, which `gh` prints on a non-2xx;
  and finally, if neither yields a code, by emitting the generic form naming the endpoint, the exit status, and the body, with the body collapsed to one physical line the way the sibling script already collapses it, so the one-stderr-line contract is kept literally.
  Word the 401, 403, and 404 messages as `plugins/prowler/scripts/github-tree.sh` words them, with this script's own prefix and with the path named in the 404 case.
  Record in a comment why the stderr pass is the second source rather than the first: its message text is a CLI presentation string, not an API contract, and is the more likely of the two to change between releases.
  Nothing is ever written to stdout on any failure path, even though the error body was written into the body temp file the success path would have emitted.
- **Commit:** `feat(prowler): add the gh api fallback and failure diagnosis to github-read.sh`

### Card 12: stub `gh` and stub `curl` for the github-read harness

- **Context:**
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
  - `plugins/prowler/scripts/github-read.sh`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/testdata/github-read/bin/gh`
  - `plugins/prowler/scripts/testdata/github-read/bin/curl`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create two extensionless executables with the exec bit set, mirroring the shape of the existing tree-harness stub: each reads its map, bodies directory, and call-log path from environment variables, refuses to run when any is unset, and logs its full invocation before validating its shape, so a "no call was made" assertion elsewhere is asserting something real even for a call the stub goes on to reject.
  `plugins/prowler/scripts/testdata/github-read/bin/gh` accepts exactly two invocation shapes and rejects everything else with a distinct non-zero status and an unsupported-invocation message: a four-argument probe call whose third argument requests a jq expression, and a four-argument content call whose third argument is the header flag.
  Because both shapes address the identical endpoint, its map must key on the shape as well as the endpoint, so a scenario can give the probe and the content fetch different fixtures.
  Each map row carries the stdout body fixture, an optional HTTP status marking the call as a failure, and an optional stderr body fixture.
  On a failure row it writes the stdout fixture to stdout verbatim, writes the stderr fixture to stderr when one is named and nothing otherwise, does not apply the jq expression, and exits non-zero — the real CLI does not apply the expression on that path either, which is why the script under test parses the raw error body itself.
  On a success row it applies the jq expression with system `jq` for the probe shape, and cats the fixture verbatim for the content shape.
  Record in its header that system `jq` is a dependency of the harness alone and never of `plugins/prowler/scripts/github-read.sh`, which relies on the real CLI's embedded engine at run time — the same accepted seam the tree harness documents.
  `plugins/prowler/scripts/testdata/github-read/bin/curl` logs its full argument vector, then reads from its map an exit status to return and an optional body fixture to write.
  It must locate the file named after the `-o` flag by scanning its own arguments rather than assuming a position, and write the named body fixture there when the row names one, so a scenario can make it write a partial body and then exit non-zero.
  When the row's exit status is non-zero and no body fixture is named, it must write nothing at all to the output file, reproducing what the real command does under its fail flag.
  It never writes to stdout or stderr on any path, matching the silent flag the script always passes.
- **Commit:** `test(prowler): add stub gh and stub curl for the github-read harness`

### Card 13: `github-read-selftest.sh` scaffolding and raw-path scenarios

- **Context:**
  - `plugins/prowler/scripts/github-tree-selftest.sh`
  - `plugins/prowler/scripts/github-read.sh`
  - `plugins/prowler/scripts/testdata/github-read/bin/gh`
  - `plugins/prowler/scripts/testdata/github-read/bin/curl`
  - `plugins/prowler/scripts/testdata/github-read/bodies/plain.txt`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-404.json`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/github-read-selftest.sh`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `plugins/prowler/scripts/github-read-selftest.sh`, mirroring `plugins/prowler/scripts/github-tree-selftest.sh`'s shape: a header comment stating that the harness is fully offline, that system `jq` is a dependency of the harness alone, the same Linux-and-macOS portability envelope with its reason, and a "not covered here" note listing the live checks that remain manual — one run against a public repository confirming the raw path is taken, one against a private repository confirming the fallback fires and succeeds, one against a directory path in a private repository, and whatever the capture record shows was not exercised offline.
  Reuse the same helper vocabulary: a `fail` that records one failed assertion and keeps going, a `pass`, a failure counter, a jq prerequisite guard read at call time, a scratch directory under the repository's own `.scratch` rather than any system temp directory, and a scenario runner that writes the scenario's map, truncates both call logs, puts the stub directory on PATH, runs the script under test, and captures stdout, stderr, and exit status into separate variables.
  Capturing stdout and stderr separately is mandatory, because many assertions turn on stdout being byte-empty while stderr is not.
  The runner must also point the environment's temp directory at the scenario's own scratch directory, which is what makes the temp-file-cleanup assertion in card 15 possible.
  Provide per-stub call-log accessors and line counters so a scenario can assert one command was called and the other was not.
  Provide one further helper the curl-absence scenario needs: a function that builds a `curl`-free stub directory at runtime under the harness scratch, containing a copy of the stub `gh` and nothing else, and returns its path.
  The tree harness's own missing-binary trick — emptying PATH entirely and invoking through an absolute interpreter path — cannot serve here, because this scenario needs `gh` to still resolve while `curl` does not, and an emptied PATH hides both.
  Pointing PATH at a directory holding only the stub `gh`, with no other directory behind it, is what makes the absence real rather than assumed;
  the runtime-construction idiom mirrors what card 4 already establishes for generated fixtures in the sibling harness.
  The scenario runner must accept the stub directory to use as a parameter, defaulting to the normal one, so this single scenario can point at the `curl`-free directory while every other scenario is unaffected.
  Then add the raw-path scenarios.
  First and most important, the preference-order proof: a successful raw read whose stdout equals the fixture bytes exactly, exit status 0, exactly one curl call against the raw host URL with the repository, the literal `HEAD`, and the path in it, and zero `gh` calls — this single assertion pins the entire point of the task.
  Second, the argument-vector proof: assert the logged curl invocation is exactly the decided vector, argument by argument, in order.
  The fail flag in particular must be asserted individually, because its absence is what would silently degrade the script into emitting a not-found body as file content.
  Third, that the trigger is the exit status and not a parsed code: run the stub exiting with a timeout status and again with a connection-refused status, and assert the fallback fired in both.
  Fourth, the not-found regression test: run the stub writing nothing to its output file and exiting with the status the fail flag produces, and assert stdout carries no not-found text.
  Fifth, the no-partial-prefix proof: run the stub writing a partial body to its output file and then exiting non-zero with the fallback succeeding, and assert stdout is exactly the fallback's bytes with no partial prefix in front of them — this assertion would fail against a stream-to-stdout implementation and is what pins the temp-file buffering decision.
  Sixth, curl absent from PATH: run this one scenario against the `curl`-free stub directory built by the helper above, so `gh` resolves and `curl` genuinely does not, and assert the script goes straight to `gh api`, that zero curl calls were logged, that stderr says nothing about curl, and that stdout is still correct.
- **Commit:** `test(prowler): add github-read-selftest.sh and its raw-path scenarios`

### Card 14: harness scenarios for the fallback and its failure diagnosis

- **Context:**
  - `plugins/prowler/scripts/github-read.sh`
  - `plugins/prowler/scripts/testdata/github-read/bin/gh`
  - `plugins/prowler/scripts/testdata/github-read/bin/curl`
  - `plugins/prowler/scripts/testdata/github-read/CAPTURE.md`
  - `plugins/prowler/scripts/testdata/github-read/bodies/plain.txt`
  - `plugins/prowler/scripts/testdata/github-read/bodies/probe-file.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/probe-dir.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/probe-symlink.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/probe-submodule.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-401.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-403.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-404.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-404.stderr`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-nostatus.json`
  - `plugins/prowler/scripts/testdata/github-read/bodies/error-multiline.json`
- **Edits:**
  - `plugins/prowler/scripts/github-read-selftest.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append fallback scenarios to `plugins/prowler/scripts/github-read-selftest.sh`.
  The happy fallback: raw fails, the probe answers `file`, and the content fetch succeeds — assert correct stdout, exit status 0, exactly one curl call, then exactly two `gh` calls in that order, with the first carrying the probe's jq expression and the second carrying the raw media-type header, asserting both full argument vectors rather than just their count.
  The directory rejection: raw fails and the probe answers `dir` — assert exit status 1, byte-empty stdout, exactly one stderr line naming the path and pointing at the sibling tree script's children mode, and exactly one `gh` call, which is what proves the content fetch never ran.
  The symlink and submodule rejections: same shape, with the probe answering each of those two types and the message naming the observed type.
  The three authenticated failures: the probe fails with each of 401, 403, and 404 in turn — assert non-zero exit, byte-empty stdout, exactly one stderr line carrying that diagnosis with no mention of the raw attempt at all, and that the second `gh` call never happened.
  Then the three status-extraction sources in isolation.
  One scenario where only the body fixture carries a status field and the stderr fixture is absent, asserting the code was read from the body.
  One where the body fixture carries no status field and the stderr fixture carries the parenthesised code, asserting the stderr pass supplied it.
  One where neither carries a code and the body spans several physical lines, asserting the generic form naming the endpoint and the exit status was used and that stderr is still exactly one physical line.
  Finally, assert across every fallback-failure scenario in this card that stdout is byte-empty, proving the failed fallback's error body never reaches it even though it was written into the temp file the success path would have emitted.
- **Commit:** `test(prowler): assert github-read.sh fallback behaviour and failure diagnosis`

### Card 15: harness scenarios for validation, normalisation, byte fidelity, and cleanup

- **Context:**
  - `plugins/prowler/scripts/github-read.sh`
  - `plugins/prowler/scripts/testdata/github-read/bin/gh`
  - `plugins/prowler/scripts/testdata/github-read/bin/curl`
  - `plugins/prowler/scripts/testdata/github-read/bodies/plain.txt`
  - `plugins/prowler/scripts/testdata/github-read/bodies/withnul.bin`
  - `plugins/prowler/scripts/testdata/github-read/bodies/zero.txt`
- **Edits:**
  - `plugins/prowler/scripts/github-read-selftest.sh`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append the remaining scenarios to `plugins/prowler/scripts/github-read-selftest.sh`.
  Prerequisite: with an emptied PATH and the script invoked by an absolute interpreter path resolved before PATH is stripped — the same idiom the tree harness uses and for the same reason — assert the missing-`gh` message, exit status 1, byte-empty stdout, and that no network call happened.
  Path validation: a path containing an unsupported character is rejected locally with exit status 1 and zero calls to either command.
  Include the accented-character case explicitly, since that is the reproduced failure the glob-substitution technique exists to prevent and a regex rewrite would pass every other validation assertion while failing this one.
  Path normalisation: a bare path, the same path with a leading slash, and the same path with a trailing slash must produce byte-identical call logs.
  An empty path and a slash-only path are usage errors with exit status 2 and no calls.
  Argument count: zero positionals, one positional, and three positionals counted after a terminator has been consumed are each usage errors with exit status 2 and no calls;
  assert alongside them that a terminator plus a doubly-dashed path is two positionals and a legitimate invocation that reaches the API, not a count error.
  A positional beginning with two dashes without a terminator is a usage error with exit status 2 and empty call logs, matching the sibling tree script's behaviour for the same token.
  A malformed owner-and-repository reference is rejected locally with no calls.
  Byte fidelity, which is the group the temp-file decision exists for: a successful read of the ordinary fixture must produce stdout byte-identical to the fixture with no added trailing newline, no banner, and no filename, compared with a byte comparison against the fixture file rather than a trimmed string comparison;
  the same assertion must be made for the fixture containing a NUL byte;
  and the zero-byte fixture must produce exit status 0 with byte-empty stdout, which is success distinguished from failure only by the exit code and is exactly why the skill tells the caller to check it.
  Cleanup: after a successful read and again after a read where both backends failed, assert the scenario's scratch temp directory is empty, proving the trap removed both temp files on every exit path.
  Close the file with the same accumulated-failure summary the tree harness prints, exiting non-zero when any assertion failed, and remove the scratch directory before printing it.
- **Commit:** `test(prowler): assert github-read.sh validation, byte fidelity, and cleanup`

## Batch Tests

`verify:` runs `bash plugins/prowler/scripts/github-read-selftest.sh`, the new harness this batch creates.
It is the complete offline test surface for `github-read.sh`, driving it through a stub `curl` and a stub `gh` on PATH with no network access, and asserting exact stdout bytes, exact argument vectors for both commands, exact call counts and ordering, and distinguishing stderr substrings.
Its one dependency beyond bash is system `jq`, which it checks for up front and which is present in this environment.

Scoping is exact: this batch creates `github-read.sh` and its harness, stubs, and fixtures, and touches nothing else in the repository.
No other test exercises them, and they exercise nothing else, so no wider suite is implicated.
The harness is not registered with CI or any Go test, matching the existing tree harness, and this batch does not change that.

The single most load-bearing assertion is the preference-order proof in card 13 — a successful raw read makes zero `gh` calls — because that is the measured ten-to-fifteen-fold win the whole task exists to capture, and it is the one assertion that fails if the fallback order is ever inverted.
Card 8's live capture is what keeps the failure fixtures honest: a stub built from a written description would prove the parser while leaving the premise untested.
Not covered offline, and documented as manual checks in the harness header: one live run against a public repository confirming the raw path is taken, one against a private repository confirming the fallback fires and succeeds, and one against a directory path in a private repository confirming the probe's real response shape and that the rejection fires.
