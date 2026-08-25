# Plan: Fix prowler: Reddit adapter blocked

```yaml
task: 'Fix prowler: Reddit adapter blocked'
slug: 'prowler-fix-reddit-block'
approved: false
started: '20260825-063810'
parent: 'main'
root: ""
verify: go -C plugins/prowler build ./... && go -C plugins/prowler vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: block-detection
    file: 01-block-detection.md
    depends-on: []
    verify: go -C plugins/prowler test -run 'TestLooksLikeBlockPage|TestFetchPage|TestBrowserFallback' .
  - number: 2
    name: reddit-oauth-client
    file: 02-reddit-oauth-client.md
    depends-on: [1]
    verify: go -C plugins/prowler test -run 'TestRedditCredentials|TestRedditAPIUserAgent|TestRedditToken|TestFormatRedditThread|TestRedditOAuthURL|TestFetchRedditOAuthThread' .
  - number: 3
    name: tiered-adapter
    file: 03-tiered-adapter.md
    depends-on: [1, 2]
    verify: go -C plugins/prowler test -run 'TestReddit|TestFetchOldRedditHTML|TestFetchPage|TestRunAll|TestFormatRedditThread|TestFetchRedditOAuthThread' .
  - number: 4
    name: live-integration
    file: 04-live-integration.md
    depends-on: [3]
    verify: go -C plugins/prowler test -tags integration -run 'TestRedditOAuthThread_Integration' .
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: nested-module-boundary

- **Decision:** every file this plan touches lives under `plugins/prowler/`, which is its own Go module (`github.com/Knatte18/loomyard/plugins/prowler`).
  All build/test/vet commands are written as `go -C plugins/prowler <verb>` so they run inside that module while cwd stays at the worktree root.
  No file in the repo-root `github.com/Knatte18/loomyard` module is edited.
- **Rationale:** `go test ./...` from the worktree root does not descend into a nested module, so a repo-root-relative command silently verifies nothing.
  The hub's own `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) runs from the repo root and therefore does **not** cover this module — the per-batch `verify:` commands are the only real gate for this task's code.
- **Applies to:** all batches

### Decision: injection-seam-discipline

- **Decision:** all new network code is written against the `fetcher` struct in `plugins/prowler/fetcher.go` — never against the package-level `httpClient` directly, and never by constructing an `http.Client` inside a fetch path.
  When a fetch path needs transport behaviour the shared client does not provide (redirect suppression), the capability is added as a new field on `fetcher` and wired in `newFetcher()` in `plugins/prowler/main.go`, so tests keep full control via stubs.
- **Rationale:** `fetcher` is the module's documented single injection seam;
  every existing test stubs it. A bypass would make the new code untestable offline and break the module's Tier-1 offline-test convention.
- **Applies to:** all batches

### Decision: no-secrets-anywhere-but-env

- **Decision:** Reddit credentials are read from `PROWLER_REDDIT_CLIENT_ID` and `PROWLER_REDDIT_CLIENT_SECRET` only, resolved in exactly one place (the new `plugins/prowler/redditoauth.go`).
  No `os.Getenv` for these two variables appears in any other file.
  Credential *values* are never logged, never included in an error string, and never written to a fixture, a test table, or an output file;
  error text names the *variables* only.
  No credential is ever committed.
- **Rationale:** mirrors the GitHub Auth Invariant's shape in `CONSTRAINTS.md` — credential resolution belongs in one place — and keeps the secret out of prowler's markdown output, which is written to a scratch file and read back by a Claude session.
- **Applies to:** all batches

### Decision: never-return-a-wall-as-content

- **Decision:** no code path in this module returns a bot wall, login wall, or challenge interstitial as successful content.
  Every place that currently decides "this text is good enough" — the generic cascade in `plugins/prowler/fetch.go`, the browser fallback's result, and each Reddit tier — routes its candidate text through the shared detector first.
- **Rationale:** the reproduction in `_mill/discussion.md` shows prowler writing `# Reddit - Prove your humanity` as a successful result;
  a calling session cannot distinguish that from real content, which is strictly worse than an honest error.
- **Applies to:** all batches

### Decision: reddit-terminates-in-adapter

- **Decision:** `redditAdapter.Fetch` always reports `handled=true`.
  When every tier fails it returns an `errorResult`-formatted markdown error naming each attempted tier and its reason, rather than reporting `handled=false` and letting `fetchPage` fall through to the generic cascade and its headless-Chrome tier.
- **Rationale:** measured in `_mill/discussion.md` — a second headless request from the same Chrome profile escalated a solvable-looking challenge into a hard network-security block on a residential IP.
  The browser tier has no success path against this challenge and its failure mode is cumulative and externally visible.
- **Applies to:** batch 3, batch 4

### Decision: offline-tier-purity

- **Decision:** every test file added by this plan is untagged and offline **except** `plugins/prowler/reddit_integration_test.go`, which carries `//go:build integration` as its first line, matching `plugins/prowler/browser_integration_test.go`.
  Untagged tests make no network call and spawn no process;
  they drive stubbed `fetcher` fields only.
- **Rationale:** the Test Tier Purity Invariant in `CONSTRAINTS.md` is mechanically enforced only over the repo-root module, but this nested module already honours its spirit and the plan keeps it that way.
- **Applies to:** all batches

### Decision: no-new-dependencies

- **Decision:** `plugins/prowler/go.mod` is not modified.
  Token acquisition uses `net/http` + `encoding/base64` and JSON decoding uses `encoding/json`, all stdlib.
- **Rationale:** confirmed in `_mill/discussion.md`'s technical context;
  nothing this task needs is outside the standard library on top of the existing module graph.
- **Applies to:** all batches

### Decision: operator-prerequisite-gates-completion

- **Decision:** the task is not complete when batches 1-3 are green.
  Batch 4 carries an explicit operator step — register a Reddit "script" app at `https://www.reddit.com/prefs/apps` and export the two credential env vars — that must happen before the live smoke test can prove the `client_credentials` grant actually works.
  Batch 4's verification-only card states this to the operator when the credentials are absent.
- **Rationale:** `_mill/discussion.md`'s `oauth-credential-shape` decision carries an explicitly unclosed risk: the grant was never verified end-to-end because no credentials existed.
  A plan that only reasons about the grant working is insufficient.
- **Applies to:** batch 4

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `plugins/prowler/README.md`
- `plugins/prowler/blockdetect.go`
- `plugins/prowler/blockdetect_test.go`
- `plugins/prowler/fetch.go`
- `plugins/prowler/fetch_test.go`
- `plugins/prowler/fetcher.go`
- `plugins/prowler/headers.go`
- `plugins/prowler/main.go`
- `plugins/prowler/reddit.go`
- `plugins/prowler/reddit_integration_test.go`
- `plugins/prowler/reddit_test.go`
- `plugins/prowler/redditoauth.go`
- `plugins/prowler/redditoauth_test.go`
- `plugins/prowler/testdata/good-article.html`
- `plugins/prowler/testdata/reddit-block-page.html`
- `plugins/prowler/testdata/reddit-login-page.html`
- `plugins/prowler/testdata/reddit-thread.json`
- `plugins/prowler/testdata/reddit-www-interstitial.html`
