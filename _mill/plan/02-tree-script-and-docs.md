# Batch: tree-script-and-docs

```yaml
task: "prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call"
batch: "tree-script-and-docs"
number: 2
cards: 1
verify: bash plugins/prowler/scripts/github-tree-selftest.sh
depends-on: [1]
```

## Batch Scope

This batch delivers `github-tree.sh` together with both documentation changes the behaviour change requires — the `SKILL.md` rewrite that replaces the four prose tree-walking paragraphs with a single script invocation, and the `README.md` section documenting the script beside the existing "Build-on-first-run" section.

It is one card, not three, on purpose.
`CLAUDE.md` requires that a task changing observable behaviour update its docs **in the same commit**, and mill's execution model produces exactly one commit per card;
splitting the script from the docs it makes true would therefore violate that rule, and no cross-card "squash these together" instruction is permitted as a substitute.
The three files are atomic in the sense that matters: the moment `github-tree.sh` lands, `SKILL.md`'s manual `gh api` recipe becomes a stale instruction the model would still execute, and its `raw.githubusercontent.com` note carries a `{branch}` placeholder that nothing defines any more.

Batch-local decision beyond `## Shared Decisions`: none — the script's contract, its error wording, and its call shape are all fixed by batch 1's harness.

## Cards

### Card 5: `github-tree.sh`, the SKILL rewrite, and the README section

- **Context:**
  - `plugins/prowler/scripts/run.sh`
  - `plugins/prowler/scripts/github-tree-selftest.sh`
  - `plugins/prowler/skills/prowler/SKILL.md`
- **Edits:**
  - `plugins/prowler/skills/github-repo-explorer/SKILL.md`
  - `plugins/prowler/README.md`
  - `plugins/prowler/scripts/testdata/github-tree/bin/gh`
- **Creates:**
  - `plugins/prowler/scripts/github-tree.sh`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  **The script.**
  Create `plugins/prowler/scripts/github-tree.sh` as a bash script with shebang `#!/usr/bin/env bash`, `set -u` (never `set -e`, because every `gh` call's exit status is inspected explicitly), and the exec bit set.
  It takes one or two positional arguments — `<owner/repo>` and an optional repo-relative directory `[path]` — and prints one repo-relative file path per line to stdout.
  It does not self-locate a `SCRIPT_DIR` or `PLUGIN_ROOT`, because unlike `run.sh` it reads no file inside the plugin;
  what it does copy from `run.sh` is the strict stdout rule (only the path list ever reaches stdout, every diagnostic goes to stderr) and the `command -v` prerequisite check.

  Write a header comment covering: what the script does and why it exists (collapsing an N-call LLM-driven walk into one deterministic invocation), that stdout carries the path list and nothing else, that the listing is buffered and emitted only after the whole walk succeeds so a failure never leaves a partial prefix behind, and that there are deliberately no retries — every non-2xx aborts the run and the caller re-invokes.
  State that last point explicitly enough that a later maintainer does not add a backoff loop thinking it was an oversight.

  Define a single `die` helper taking a message and writing it to stderr followed by `exit 1`, and use exit 2 for the usage error alone.

  Perform prerequisite and argument handling in this order, so that a rejection never reaches the network.
  First, `command -v gh` — on failure, `github-tree: gh not found on PATH — install the GitHub CLI and authenticate it (gh auth login)`.
  Second, argument count: fewer than one or more than two arguments produces `github-tree: usage: github-tree.sh <owner/repo> [path]` and exit 2.
  Third, the repository reference must match the bash regex `^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`, or `github-tree: '<arg>' is not a valid <owner>/<repo> reference`.
  Fourth, normalize the path: strip every leading and trailing `/`, and treat a result that is empty as "whole repo".
  A non-empty result must match `^[A-Za-z0-9._/-]+$`;
  when it does not, name the offending input by deleting every accepted character from a copy of the path with the bash pattern substitution `"${path//[A-Za-z0-9._\/-]/}"` and reporting whatever remains, then emit `github-tree: path '<path>' contains unsupported character(s) '<offending>' — only [A-Za-z0-9._/-] is accepted`.
  Do not iterate the path character by character looking for the first offender.
  Bash string indexing is byte-oriented under a `C`/POSIX locale, which minimal shells and CI images routinely run with and which this script deliberately does not pin, so indexing would slice a multi-byte UTF-8 character such as `ï` into single invalid bytes and report one of them as "the character".
  The substitution above is locale-independent instead: the accepted set is pure ASCII, and a UTF-8 continuation byte is never in it, so no multi-byte sequence can be split — the remainder always holds each offending character whole.
  Reporting the full remaining set rather than just the first offender is the same choice made for the same reason;
  there is no first-offender extraction that is byte-safe without re-introducing the indexing problem.
  Do no other rewriting — internal `//`, `.`, and `..` segments pass straight through to the API and surface as a loud error there rather than being silently reinterpreted.

  Derive two values from the normalized path: `BASE_REF` is `HEAD` when the path is empty and `HEAD:<path>` otherwise, and `PREFIX` is the empty string when the path is empty and `<path>/` otherwise.

  Define the jq expression once, as a single-quoted shell constant, exactly:

  ```
  '"#trunc\t" + (.truncated|tostring), (.tree[] | if (.path|test("[\t\n]")) then "#badpath\t" + (.path|@json) else .type + "\t" + .sha + "\t" + .path end)'
  ```

  One `gh api` response therefore yields both the truncation flag and the entry list, which is the direct fix for the duplicate call against the recursive endpoint.

  Define `fetch <endpoint> <kind>`, where `<kind>` is `root` or `child`.
  Bind those two values to their call sites explicitly rather than leaving them inferable: `root` is passed by exactly the two fetches that address the walk's seed item — its `rec` fetch of `BASE_REF` and, when that comes back truncated, the `nonrec` re-fetch of the same `BASE_REF` — and `child` is passed by every other fetch the queue drains, all of which address a subtree by the sha its parent listing gave.
  This is what makes the scoped-versus-unscoped 404 branch below well defined: only a `root` fetch can be the one carrying the caller's `path`, so only there is a 404 attributable to that path rather than to a subtree that vanished mid-walk.
  It runs `gh api "<endpoint>" --jq "<expr>"` — exactly four arguments, no other flags, ever — capturing stdout into a local variable and discarding `gh`'s own stderr, since the script prints its own diagnostics.
  On a non-zero exit it extracts the HTTP status from the captured body using a bash regular expression tolerant of whitespace around the colon, matching a `status` key whose value is a three-digit string, and branches:
  401 gives `github-tree: repos/<repo> — not authenticated (HTTP 401); run 'gh auth login'`;
  403 gives `github-tree: repos/<repo> — rate limited or access denied (HTTP 403)`;
  404 or 409 on a `root` fetch with an empty path gives `github-tree: repos/<repo> — not found, not accessible with this token, or has no commits yet (HTTP <code>)`;
  404 on a `root` fetch with a non-empty path gives `github-tree: repos/<repo> — path '<path>' not found (HTTP 404)`;
  422 gives `github-tree: repos/<repo> — path '<path>' is not a directory (HTTP 422)`;
  and anything else, including an unparseable body, gives `github-tree: gh api <endpoint> failed (exit <status>): <body>` so a rate-limit or auth failure is never flattened into a generic message.
  Collapse the body to one physical line before embedding it, by substituting a space for every newline with `"${body//$'\n'/ }"`.
  GitHub's error bodies are compact JSON in practice, but nothing guarantees it, and the rewritten `SKILL.md` promises callers exactly one stderr line on failure — a promise the script should keep literally rather than by assumption.
  Both 404 branches share a code but must produce different text — that difference is asserted, and it is what keeps a mistyped path from ever reading as "this directory has no files".

  On success `fetch` parses the captured stream into two outputs the caller reads: a truncation flag and an array of entry lines.
  Parse with `while IFS= read -r line` fed by a here-string over the captured variable, never a pipeline, so the loop body runs in the main shell and a failure inside it can abort the run.
  Skip empty lines.
  The first non-empty line must be the `#trunc` header;
  split it on the tab and store the boolean.
  Any line beginning with `#badpath` followed by a tab aborts the whole run with `github-tree: repos/<repo> — refusing to list: a path contains a tab or newline (<escaped>), which the one-path-per-line output cannot represent`, where `<escaped>` is the JSON-escaped path the sentinel carried.
  Every other line is split into type, sha, and path by successive parameter expansions on the first and second tab — not by `IFS=$'\t' read`, so a path can never be split further even though the `#badpath` guard already makes that impossible.

  The walk itself uses one explicit FIFO queue in the main shell — an append-only bash array plus an advancing head index consumed by `while [ "$head" -lt "${#queue[@]}" ]` — never a recursive shell function and never a LIFO stack.
  Each queue item is a tab-separated triple of a mode (`rec` or `nonrec`), a ref, and a path prefix.
  Seed the queue with a single `rec` item carrying `BASE_REF` and `PREFIX`.

  Processing a `rec` item fetches `repos/<repo>/git/trees/<ref>?recursive=1` and, if the response reports truncated, appends one `nonrec` item with the same ref and prefix and moves on;
  otherwise it appends every `blob` entry's prefixed path to the output buffer.
  Processing a `nonrec` item fetches `repos/<repo>/git/trees/<ref>` with no query string.
  A `nonrec` response that reports itself truncated is the residual hard error and aborts with `github-tree: repos/<repo> — the non-recursive listing of '<ref>' is itself truncated; this repository has a directory too large for the GitHub tree API`.
  Otherwise it appends every `blob` entry's prefixed path to the output buffer and then appends, in API order, one `rec` item per `tree` entry carrying that entry's sha as the ref and the prefix extended with that entry's path and a trailing `/`.
  Entries of type `commit` are skipped silently in both branches — a submodule path is not readable through this repository's contents API, so emitting it would only invite a failing read.
  The root is re-listed by the ref it was originally addressed by (`HEAD`, or `HEAD:<path>` when scoped), while every descendant is re-listed by the sha its parent listing gave;
  only the addressing differs, and the rule is otherwise identical at every depth.

  This single-queue discipline fixes the output order completely, and the harness asserts it: the root's own blobs first, then each root subtree's blobs in the order the root's non-recursive listing gave them, then any deeper level, strictly in the order work items were appended.
  A subtree discovered to be truncated has its own blobs emitted when its `nonrec` item is reached, which is after all of its already-queued siblings — that is a consequence of the queue, not an accident, and the two-level fixture pins the resulting sequence exactly.

  Nothing is written to stdout until the queue is exhausted with no error.
  At that point, emit the buffer with a single `printf '%s\n'` over the array, guarded by a check that the array is non-empty so the emission is a genuine no-op under `set -u` when the repository legitimately contains no files.
  Zero blobs is a success: exit 0 with empty stdout.
  Empty stdout therefore never means failure on its own, which is why every error path above must exit non-zero.

  Add no retry loop, no backoff, and no partial-progress resume anywhere in the walk.

  **The skill rewrite.**
  In `plugins/prowler/skills/github-repo-explorer/SKILL.md`, leave the frontmatter untouched — `name`, `description`, and `argument-hint` all stay exactly as they are, which is why the generated skills index needs no regeneration.
  Leave the opening sentence, the `gh` hard-prerequisite paragraph, the read-file paragraph, and the closing `distill-subagent` pointer untouched.

  Replace the four consecutive paragraphs that today resolve the default branch, list the recursive tree, re-check truncation, and describe the non-recursive per-directory fallback with a two-step block.
  Step one resolves the script's absolute path while `${CLAUDE_SKILL_DIR}` is still set, copying the shape of step 1 in `plugins/prowler/skills/prowler/SKILL.md` including its reason (a dispatched subagent will not have that variable): assign `TREE_SH` from `"$(cd "${CLAUDE_SKILL_DIR}/../../scripts" && pwd)/github-tree.sh"`.
  Step two shows both invocation forms — `paths=$(bash "$TREE_SH" <owner/repo>)` for a whole-repo listing and `paths=$(bash "$TREE_SH" <owner/repo> <path>)` for a scoped one — with a one-line note that scoping is worth reaching for whenever the caller already knows which subtree it needs, and a statement that the script handles GitHub's recursive-tree truncation cap internally so the listing is never silently partial.
  State the failure contract the caller must honour: on failure the script prints one line to stderr, emits nothing on stdout, and exits non-zero, so the exit code must be checked and a failure reported rather than read as an empty repository.
  Add a short paragraph telling the skill how to decide whether a second token from its own `<owner/repo> [path] [question]` argument hint is a `path` or the `question`: it is forwarded as `<path>` only when it looks like a repo-relative directory path, matching the accepted set `^[A-Za-z0-9._/-]+$` and containing no whitespace;
  anything else, a natural-language question in particular, is the question and is never passed to the script.
  Do not keep the raw `gh api` tree commands anywhere in the file, not even as a documented manual fallback — leaving the recipe in place invites the model to execute it and re-incur exactly the per-subdirectory turn cost this task removes.

  In the same file, the surviving lighter-alternative note about `raw.githubusercontent.com` currently carries a `{branch}` placeholder that only the deleted branch-resolve paragraph defined.
  Change that one placeholder to `HEAD`, leaving the rest of the line as it is;
  `HEAD` is the repository's symbolic ref for its default branch and resolves correctly on that host.

  **The README section.**
  In `plugins/prowler/README.md`, add a new section immediately after the existing "Build-on-first-run" section and before "Runtime prerequisite: Chrome/Chromium".
  Title it after the script and cover, in this order: what it does (lists a GitHub repository's file paths, optionally scoped to one directory, in a single invocation);
  why it exists (the skill previously had the model execute a branching, potentially recursive API walk one call per turn, and the walk contains no decision a model needs to make);
  the one-call property (an untruncated listing costs exactly one `gh api` call, and even a repository large enough to truncate is one agent turn regardless of how many API calls the internal fallback makes);
  that its only runtime dependency is `gh`, already a hard prerequisite of the skill, because every JSON field is extracted through `gh api --jq` and no system `jq` is ever invoked at run time;
  that unlike `run.sh` it has no build step and no lock, since there is nothing to compile;
  and that its offline test harness carries the one extra dependency of system `jq`, which the harness checks for up front.

  Follow the repository's markdown convention throughout both documentation edits: one sentence per line, an additional break at an internal independent-clause boundary in a long sentence, plain newlines only — never a trailing double space, never a backslash, and never a fixed-column hard wrap.
- **Commit:** `feat(prowler): list a GitHub repo tree in one script call`

## Batch Tests

`verify:` for this batch is `bash plugins/prowler/scripts/github-tree-selftest.sh` — the harness batch 1 created, run for real against the script this batch creates.
It covers all three files: the twenty-two assertions exercise `github-tree.sh` end to end through a stub `gh`, and they transitively validate the stub and all 25 JSON fixture bodies from batch 1, since a malformed body or a mis-keyed map surfaces as a failing scenario.
The command is scoped to this task's own harness and does not invoke `plugins/prowler/scripts/selftest.sh`, which exercises the unrelated build-on-first-run wrapper and needs a Go toolchain.

The two documentation files have no runnable surface and are verified by review.
Also verified by review, because the harness cannot assert it, is that the `gh api` recipes are gone from the skill rather than merely supplemented — a leftover manual fallback would pass every test here while defeating the task's purpose.

Manual, documented-not-asserted, mirroring the "NOT covered here" convention of `plugins/prowler/scripts/selftest.sh`: one live run against a small public repository and one against a repository large enough to truncate, confirming the real fallback completes in a single invocation and produces a plausible full listing;
a spot-check that the jq expression behaves identically under `gh`'s embedded gojq and the harness's system `jq`;
and the HTTP 409 commitless-repository alias, which no fixture pins because confirming that GitHub returns 409 rather than 404 from this endpoint would require creating a repository.
