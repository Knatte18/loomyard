# Plan: Add RSS-based Reddit read tier

```yaml
task: "Add RSS-based Reddit read tier"
slug: "reddit-rss-tier"
approved: true
started: "20260825-100100"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: neutral-thread-representation
    file: 01-neutral-thread-representation.md
    depends-on: []
    verify: go -C plugins/prowler test .
  - number: 2
    name: rss-parsing-foundation
    file: 02-rss-parsing-foundation.md
    depends-on: [1]
    verify: go -C plugins/prowler test .
  - number: 3
    name: rss-limiter-and-fetch
    file: 03-rss-limiter-and-fetch.md
    depends-on: [2]
    verify: go -C plugins/prowler test .
  - number: 4
    name: tier-rewiring-deletion-and-docs
    file: 04-tier-rewiring-deletion-and-docs.md
    depends-on: [3]
    verify: go -C plugins/prowler test . && go -C plugins/prowler test -tags integration -run '^$' .
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: verify-command-shape

- **Decision:** every batch's `verify:` is `go -C plugins/prowler test .` (batch 4 additionally chains `go -C plugins/prowler test -tags integration -run '^$' .`).
  The single `.` package pattern — not `./...` — is deliberate, and `go -C` is used instead of `cd` so no command in this plan changes the shell's working directory.
- **Rationale:** `plugins/prowler` is a separate Go module (`github.com/Knatte18/loomyard/plugins/prowler`) with exactly one Go package at its root, so `.` and `./...` are the same set of tests;
  `.` states the scope explicitly and keeps the command out of the `verify-full-suite` validator check's `go test ./...` pattern.
  A repo-root `go test ./...` would not compile this module at all.
  Batch 4's second command is a compile-only gate: `-run '^$'` matches no test name, so the `//go:build integration` files are type-checked and linked without issuing a single live Reddit request.
- **Applies to:** all batches

### Decision: no-live-network-in-verify

- **Decision:** no `verify:` command in this plan ever runs a live-network test.
  The `//go:build integration` suite is compiled (batch 4) but never executed by mill-go.
  It is run by a human with `go -C plugins/prowler test -tags integration .` when they choose to spend the requests.
- **Rationale:** Reddit's `.rss` endpoint allows roughly one request per 60 s per IP, and `verify:` re-runs after every implementer and fixer round.
  Executing the live test on each round would burn the one resource this whole task is built around and would make every round take minutes.
- **Applies to:** all batches

### Decision: limiter-stub-is-mandatory-in-offline-tests

- **Decision:** every untagged test that reaches the RSS tier — not only the limiter's own tests — calls `stubRedditRSSLimiter(t)` as its first statement.
  The helper is defined once, in `plugins/prowler/redditrss_test.go`.
- **Rationale:** the limiter is a process-wide singleton, and `stubResponses` builds responses with no `x-ratelimit-reset` header, so the spacing rule falls back to `redditRSSMinSpacing`.
  Under the production `redditRSSWait`, the second unstubbed RSS test in the process would sleep 60 real seconds.
  This is the single most likely way for this task to ship a slow or flaky suite.
- **Applies to:** rss-limiter-and-fetch, tier-rewiring-deletion-and-docs

### Decision: no-new-module-dependency

- **Decision:** no entry is added to `plugins/prowler/go.mod` or `plugins/prowler/go.sum` by any card in this plan.
  Atom parsing uses stdlib `encoding/xml`;
  HTML rewriting uses the already-present `github.com/PuerkitoBio/goquery`.
- **Rationale:** the discussion's Constraints section states this explicitly, and every capability this task needs is already available.
- **Applies to:** all batches

### Decision: htmltext-go-is-untouched

- **Decision:** `plugins/prowler/htmltext.go` appears in `Context:` lists but never in any card's `Edits:`.
  `htmlToText` keeps its current behaviour exactly.
- **Rationale:** the generic fetch cascade and the Hacker News adapter both depend on `htmlToText`'s current link-and-block-dropping behaviour, and neither is in this task's scope.
  The RSS tier wraps it with `redditHTMLToMarkdown` instead of changing it.
- **Applies to:** all batches

### Decision: fetch-never-reaches-browser

- **Decision:** `redditAdapter.Fetch` keeps returning `handled=true` on every path and never calls `f.browser`, directly or indirectly.
  Every `redditAdapter.Fetch` subtest installs a `t.Fatal`-ing `f.browser`.
- **Rationale:** a second headless request against a Reddit challenge has been measured to escalate it into a hard IP-level block.
  This guarantee predates the task and must survive the rewiring;
  the existing tests enforce it and the new ones extend that enforcement.
- **Applies to:** tier-rewiring-deletion-and-docs

### Decision: done-gate-left-unchanged

- **Decision:** `pipeline.done_gate` in `mill-config.yaml` is not modified by this plan, and no card edits any mill config file.
- **Rationale:** the hub's existing `done_gate` runs from the repository toplevel, where the `lyx` module lives.
  `plugins/prowler` is a separate module that a repo-root `go test ./...` never compiles, so the done gate neither covers nor is affected by this task.
  Coverage for this task is complete without it: every batch's `verify:` runs the entire `plugins/prowler` package, which is the whole of the changed code.
- **Applies to:** all batches

### Decision: fixtures-come-from-the-existing-capture

- **Decision:** the `testdata/*.rss` fixtures are trimmed copies of the live captures already sitting in `.scratch/reddit-rss-capture/` in this worktree.
  No card issues a fresh live Reddit request to build a fixture.
- **Rationale:** re-capturing costs scarce rate budget for no gain, and the existing captures are real responses carrying Reddit's actual XML escaping, `SC_OFF`/`SC_ON` wrappers, and `submitted by … [link] … [comments]` trailers.
  `.scratch/` is gitignored, so only the trimmed `testdata/` copies are committed.
- **Applies to:** rss-parsing-foundation

### Decision: go-comment-density

- **Decision:** every new file opens with a file-level doc comment stating its role, and every new declaration — exported or not — carries a doc comment explaining why it exists, not merely what it is.
- **Rationale:** this is the module's existing convention (see the opening comment of `reddit.go`, `redditoauth.go`, `fetcher.go`), its comments are unusually dense, and `mill:code-comments` / `golang:golang-comments` set the same bar.
- **Applies to:** all batches

### Decision: test-style

- **Decision:** tests are table-driven with `t.Run` subtests and use the module's failure format — `got X; want Y`, naming the call, e.g. `t.Errorf("redditRSSURL(%q) = %q; want %q", in, got, want)`.
- **Rationale:** matches `plugins/prowler/reddit_test.go` and `redditoauth_test.go` exactly;
  a different assertion style in new files would read as foreign.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `plugins/prowler/README.md`
- `plugins/prowler/adapter.go`
- `plugins/prowler/fetch.go`
- `plugins/prowler/fetch_test.go`
- `plugins/prowler/fetcher.go`
- `plugins/prowler/headers.go`
- `plugins/prowler/main.go`
- `plugins/prowler/reddit.go`
- `plugins/prowler/reddit_integration_test.go`
- `plugins/prowler/reddit_test.go`
- `plugins/prowler/redditformat.go`
- `plugins/prowler/redditformat_test.go`
- `plugins/prowler/redditoauth.go`
- `plugins/prowler/redditoauth_test.go`
- `plugins/prowler/redditrss.go`
- `plugins/prowler/redditrss_integration_test.go`
- `plugins/prowler/redditrss_test.go`
- `plugins/prowler/skills/prowler/SKILL.md`
- `plugins/prowler/testdata/reddit-listing.rss`
- `plugins/prowler/testdata/reddit-rss-notfound.rss`
- `plugins/prowler/testdata/reddit-thread-golden.md`
- `plugins/prowler/testdata/reddit-thread.rss`
