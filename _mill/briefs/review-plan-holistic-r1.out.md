MILL_REVIEW_BEGIN
# Review: Give codeintel a persistent, session-long daemon — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] Context omits test-helper files named in Requirements
**Location:** batch 1 card 2; batch 4 card 11
**Issue:** Card 2's Requirements names `spawnAndHoldSubprocess` from `internal/codeintelengine/supervised_test.go` as the fixture pattern to mirror; card 11's Requirements names both `spawnAndHoldSubprocess` and `spawnAndWaitForDeadPID` from `supervised_test.go`/`daemonstate_test.go`. Neither file appears in either card's `Context:` list (card 2: proc_linux.go/proc_windows.go/isalive_test.go only; card 11: proc_linux.go/daemonstate.go/lspclient.go only) — confirmed against the actual `supervised_test.go`/`daemonstate_test.go` source, whose signatures match the plan's description exactly, but which the implementer has no `Context:` license to read.
**Fix:** Add `internal/codeintelengine/supervised_test.go` to card 2's Context, and add `supervised_test.go` + `daemonstate_test.go` to card 11's Context.

### [BLOCKING] Card 11 risks an un-allowlisted spawn in an untagged file
**Location:** batch 4 card 11
**Issue:** Card 11 explicitly anticipates a new `ensureserver_test.go` case needing a real live/dead PID (following the `spawnAndHoldSubprocess`/`spawnAndWaitForDeadPID` precedent) to exercise the "genuinely wedged → KillPID" race. `internal/codeintelengine/ensureserver_test.go` is untagged and, per `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map, only `daemonstate_test.go` and `supervised_test.go` are file-allowlisted in this package — `ensureserver_test.go` is not. A spawn there fails `TestTierPurity_UntaggedTestsSpawnNothing` on the repo-wide `go test ./...` done_gate, which card 11's own `verify: go test ./internal/codeintelengine/...` never runs (different package), so the batch's own gate would pass while the done_gate fails.
**Fix:** Place any spawning fixture case in the already-allowlisted `supervised_test.go` instead of `ensureserver_test.go`, or add an `allowedSpawners` entry for `ensureserver_test.go` to `cmd/lyx/tierpurity_test.go` in the same commit.

### [NIT] Card 10 over-claims which existing test needs the rename
**Location:** batch 4 card 10
**Issue:** Card 10 says both `TestNativeArgv_IncludesExtendedIdleTimeout` and `TestNativeArgv_PreservesBinPathAndExtraArgs` reference `nativeDaemonIdleTimeout` and must be updated for the rename. Confirmed against source: only the former references the constant; `TestNativeArgv_PreservesBinPathAndExtraArgs` asserts argv ordering only and needs no change.
**Fix:** Correct the card to note only `TestNativeArgv_IncludesExtendedIdleTimeout` needs the identifier rename.

### [NIT] Stale `nativeDaemonIdleTimeout` prose mention in doc.go not explicitly assigned
**Location:** batch 4 card 10 vs. card 12
**Issue:** `doc.go`'s "EnsureServer seam" paragraph names `nativeDaemonIdleTimeout` by name in prose ("kept warm via ... — see nativeDaemonIdleTimeout in ensureserver.go"). Card 10 (the rename) doesn't touch doc.go; card 12's doc.go edit is scoped to the dispatch-arm/"Known limitation" reversal, and doesn't explicitly call out this inline name.
**Fix:** Have card 10 or card 12 explicitly name this prose reference as part of the required edit, so the rename isn't accidentally left stale.

### [NIT] Card 11's core helper is unnamed
**Location:** batch 4 card 11
**Issue:** Every other new function across the plan gets a proposed name (`supervisedArgv`, `collectInFileMatches`, `inFileQuery`, `buildOptions`); card 11's safety-critical re-dial/restart decision helper is only described as "a helper that accepts injectable dial and finalize function values," with no proposed name or signature.
**Fix:** Name the helper and sketch its signature (params/return) alongside the already-detailed algorithm description.

### [NIT] Native fallback re-resolves the toolchain
**Location:** batch 4 card 12
**Issue:** `ensureServer` resolves the toolchain once via `resolveGoToolchain` to build `command` for `ensureSupervised`; on any `ensureSupervised` failure it falls back to `ensureNative(ctx, lang, entry, targetDir, timeout)`, which internally calls `resolveGoToolchain` again. Harmless (fast-path `os.Stat`) but redundant.
**Fix:** Note the duplication is accepted, or thread the already-resolved `binPath` into the fallback path.

## Verdict

REQUEST_CHANGES
Two Context-completeness gaps and one Test-Tier-Purity allowlist gap in batches 1 and 4 need fixing before B.
MILL_REVIEW_END
