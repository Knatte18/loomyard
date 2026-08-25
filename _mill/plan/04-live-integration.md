# Batch: live-integration

```yaml
task: 'Fix prowler: Reddit adapter blocked'
batch: 'live-integration'
number: 4
cards: 2
verify: go -C plugins/prowler test -tags integration -run 'TestRedditOAuthThread_Integration' .
depends-on: [3]
```

## Batch Scope

This batch closes the one risk batches 1-3 cannot close by reasoning: whether the `client_credentials` app-only grant actually works against the endpoints this adapter needs.
`_mill/discussion.md`'s `oauth-credential-shape` decision records that grant as **unverified** — the only probe possible during discussion was anonymous and returned the expected `401`, because no Reddit application existed to test with.

It is one batch because it is one gate with two halves: the test that can prove the grant, and the explicit operator step without which the test can only skip.

**Operator prerequisite — this task cannot reach "done" without it.**
No Reddit application exists yet and none can be created by an implementing agent: registering one requires an authenticated Reddit account and a web form.
The operator must, by hand:

1. Register a "script"-type app at `https://www.reddit.com/prefs/apps`.
2. Export the resulting client id and secret as `PROWLER_REDDIT_CLIENT_ID` and `PROWLER_REDDIT_CLIENT_SECRET` in their own shell — not in CI, since prowler is a locally-built CLI plugin and this repo runs no CI job for the nested `plugins/prowler` module.
3. Run this batch's `verify:` command in that shell and observe it pass rather than skip.

The secrets are never committed, never written to a config file, and never echoed in test output or an error message.

If that live run shows `client_credentials` is insufficient for the endpoints the adapter needs, the in-scope fallback named by the same discussion decision is the installed-client grant (`grant_type=https://oauth.reddit.com/grants/installed_client` with a device id), which needs no additional secret.
Escalating to the password grant is a scope change and must be raised with the operator, not taken unilaterally.

## Cards

### Card 10: Live credentialed smoke test

- **Context:**
  - `plugins/prowler/browser_integration_test.go`
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/blockdetect.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/main.go`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/reddit_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `plugins/prowler/reddit_integration_test.go`, package `main`, whose very first line is `//go:build integration`, followed by a file comment in the same shape as `plugins/prowler/browser_integration_test.go`'s explaining that this test requires network access and real Reddit credentials and is therefore excluded from the fast untagged run.
  Write one test function, `TestRedditOAuthThread_Integration`, which calls `redditCredentials()` and calls `t.Skip` naming the missing variables when any is absent, then calls `redditTokens.reset()` so the run cannot pass on a token cached by another test.
  Build a real `fetcher` via `newFetcher()` but replace its `browser` field with a function calling `t.Fatal`, so the test fails loudly rather than silently succeeding through the browser tier if the adapter's termination guarantee ever regresses.
  Fetch exactly one hard-coded public Reddit thread URL, once — no loop, no retry, no table of URLs — because the discussion records that repeated live Reddit requests degrade this IP's standing.
  Call `redditAdapter{}.Fetch` with that URL and assert: `handled` is `true`;
  the output does not start with `"# Error fetching "`;
  `looksLikeBlockPage` reports the output is not a wall;
  and the output contains the `Source: ` line naming the requested URL.
  Do not assert on any specific comment text or score — a live thread's contents change.
  Add no credential value, no token, and no fixture of a real API response to this file.
- **Commit:** `test(prowler): add live credentialed Reddit OAuth integration test`

### Card 11: Operator gate — run the live test or report the task incomplete

- **Context:**
  - `plugins/prowler/reddit_integration_test.go`
  - `plugins/prowler/README.md`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run `go -C plugins/prowler test -tags integration -run 'TestRedditOAuthThread_Integration' .` from the worktree root and read its output rather than only its exit code, because a skip and a pass both exit 0.
  If the test **ran and passed**, record that outcome — the `oauth-credential-shape` open risk is closed and the task is complete.
  If the test **ran and failed**, report the failure and stop: this is the case the batch exists to catch, and the in-scope remedy is the installed-client grant named in `## Batch Scope`, which is a change to the code, not to this card.
  If the test **skipped** because `PROWLER_REDDIT_CLIENT_ID` and `PROWLER_REDDIT_CLIENT_SECRET` are absent, report to the operator, verbatim and unhedged, that the task is **not complete**: the offline work is finished and reviewed, but the OAuth grant remains unverified, and completion requires the operator to register a "script"-type app at `https://www.reddit.com/prefs/apps`, export the two variables in their own shell, and re-run the command above.
  Do not create the app, do not invent credentials, do not weaken the test so it passes without them, and do not mark the task complete on the strength of the offline suite alone.
  Make no file change of any kind in this card.
- **Commit:** none

## Batch Tests

`verify:` runs `go -C plugins/prowler test -tags integration -run 'TestRedditOAuthThread_Integration' .` from the worktree root.
The `-tags integration` flag is what compiles card 10's file at all;
without it the file is excluded and the `-run` filter matches nothing.
The `-run` filter deliberately excludes `TestFetchWithBrowser_Integration` in `plugins/prowler/browser_integration_test.go`, which is unrelated to this task and needs Chrome — that test must still pass unchanged, but proving so is a separate manual run, not this batch's gate.

This `verify:` is a compile-and-skip check when credentials are absent and a real end-to-end proof when they are present;
it exits 0 in both cases, which is precisely why card 11 exists as a human-readable gate on top of it.
A green batch 4 with no credentials means "the test compiles and correctly skips", never "the OAuth grant works".

Also re-run the module's offline suite (`go -C plugins/prowler test ./...`) and `plugins/prowler/scripts/selftest.sh` once at the end of this batch.
`selftest.sh` is offline and build-focused and nothing in this task touches `run.sh`'s build/lock mechanic, so it is expected to pass unchanged;
it is the module's existing harness and is cheap to re-run.
