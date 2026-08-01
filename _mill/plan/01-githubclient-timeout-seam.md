# Batch: githubclient-timeout-seam

```yaml
task: Audit and overhaul engine test suites
batch: githubclient-timeout-seam
number: 1
cards: 1
verify: go test ./internal/githubclient/...
depends-on: []
```

## Batch Scope

Shrinks `TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout` from a real 5-second wait to a millisecond-scale wait by turning `ghAuthTokenTimeout` (`internal/githubclient/token.go`) into a package-level `var` and adding a save/override/restore test seam, mirroring the save/override/restore shape the same file already uses for its `runGHAuthToken` function-var seam (`withFakeGHAuthToken`, `internal/githubclient/githubclient_test.go:81-90`). No product-code behavior change: `ghAuthTokenTimeout`'s value is still `5 * time.Second` in production; only its declaration kind changes (`const` → `var`), which does not touch `resolveToken()`'s logic, the GitHub Auth Invariant's leaf-import allowlist, or the Test Tier Purity Invariant (the test already reaches the timeout only via the existing `runGHAuthToken` fake seam, never a real `gh` process, so its tier classification is unaffected). This batch is independent of `webstercli-await-wait-window` (batch 2) — different package, no shared file — so both are root batches with no `depends-on`.

**Batch-local decision (beyond `_mill/discussion.md`):** Card 1 additionally shrinks the test's own `const slack = 5 * time.Second` tolerance to `200 * time.Millisecond`. `_mill/discussion.md`'s "githubclient seam override value" Decision only addresses the `10ms` override value itself and observes the existing 5s slack "comfortably accommodates 10ms" — it does not flag that leaving `slack` at 5s would make the test's upper-bound assertion (`elapsed > ghAuthTokenTimeout+slack`) nearly vacuous once the timeout itself drops to 10ms (it would then only fail on a >5s hang, no longer meaningfully proving the seam engaged). This plan-level addition is not itself a discussion.md Decision — it's a direct mechanical consequence of shrinking the timeout, made at planning time rather than looped back through discussion.

## Cards

### Card 1: turn `ghAuthTokenTimeout` into a var with a save/override/restore test seam

- **Context:**
  - `internal/githubclient/token.go`
- **Edits:**
  - `internal/githubclient/token.go`
  - `internal/githubclient/githubclient_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/githubclient/token.go`, change the declaration of `ghAuthTokenTimeout` (currently `const ghAuthTokenTimeout = 5 * time.Second`) from `const` to `var`, keeping the same value and the same doc comment above it (the comment currently reads "ghAuthTokenTimeout bounds the `gh auth token` shell-out so a hung or unexpectedly prompting `gh` process can never block an autonomous lyx run indefinitely."). Append one sentence to that comment noting it is a `var` specifically so tests can shrink it via a save/override/restore seam, matching the file's existing `runGHAuthToken` seam pattern. Do not change any other line in this file — `resolveToken()`'s `context.WithTimeout(context.Background(), ghAuthTokenTimeout)` call keeps reading the same identifier, unmodified.
  - In `internal/githubclient/githubclient_test.go`, inside `TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout` (currently at lines 524-551), add a save/override/restore of `ghAuthTokenTimeout` at the top of the test function body, before the `withFakeGHAuthToken(...)` call: save the original value into a local (e.g. `origTimeout := ghAuthTokenTimeout`), set the package var to `10 * time.Millisecond`, and register a `t.Cleanup` that restores the original value — the identical inline shape `withFakeGHAuthToken` uses for `runGHAuthToken` (save into a local, assign the override, `t.Cleanup` to restore), just applied directly in this test function rather than through a shared helper (a single call site does not need its own named helper).
  - In the same test function, shrink the `const slack = 5 * time.Second` tolerance to `const slack = 200 * time.Millisecond`. Rationale to preserve in a code comment or leave implicit: the original 5s slack existed to absorb scheduling jitter around a 5-second real timeout; left unchanged against a new 10ms timeout, the upper-bound assertion (`elapsed > ghAuthTokenTimeout+slack`) would only fail if `resolveToken()` took over 5 seconds, which no longer meaningfully proves the seam is engaged — 200ms keeps the assertion tight (20x the new timeout, still generous for scheduling noise) while still catching a regression where the override silently fails to apply (which would make the real call take ~5s and trip this check).
  - Do not change the test's other assertions (`errors.Is(err, ErrTokenUnresolvable)`, the lower-bound `elapsed < ghAuthTokenTimeout` check, the `withFakeGHAuthToken` fake itself, or the `setCacheDir`/`t.Setenv` setup) — they stay correct unmodified since `ghAuthTokenTimeout` is read by reference everywhere in the test, so the override is picked up automatically.
- **Commit:** `test(githubclient): shrink the 5s auth-token timeout test to milliseconds via a var seam`

## Batch Tests

`go test ./internal/githubclient/...` covers the whole package, including `TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout` itself and every other test in the file (`TestResolveToken_Chain`, the `TestAuthRT_*` cluster, `TestReadCachedToken_MalformedFileIsMiss`, etc.) as a regression check that turning `ghAuthTokenTimeout` into a `var` didn't break anything else reading that identifier. This is the package's entire untagged (Tier 1) suite — small enough (one package, ~15 test functions) that scoping further would add no real speed benefit while risking missing a sibling regression, so the batch verify intentionally covers the full package rather than a `-run` filter.

Manually confirm timing after implementing: `go test -run TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout -v ./internal/githubclient/...` should report the test's own elapsed time on the order of 10ms (comfortably under the new 200ms slack ceiling), not ~5s.
