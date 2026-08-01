# Batch: scoutengine-scout-tag

```yaml
task: Formalize the Tier 1/2 substrate rule and re-tier mis-tagged tests
batch: scoutengine-scout-tag
number: 2
cards: 8
verify: go build -tags scout ./cmd/lyx/... ./internal/scoutengine/... && go vet -tags scout ./cmd/lyx/... ./internal/scoutengine/... && go test ./cmd/lyx/... ./internal/scoutengine/... -count=1 && go test -tags integration ./cmd/lyx/... ./internal/scoutengine/... -count=1
depends-on: [1]
```

## Batch Scope

This batch introduces the `scout` build tag for real: it retags the four scoutengine `*_integration_test.go` files that exercise a real `gopls` process (`ensureserver_integration_test.go`, `refs_integration_test.go`, `supervised_integration_test.go`, `toolchain_integration_test.go`) from `integration` to `scout`, splits `supervised_test.go`'s two gopls-gated subtests out into a new `//go:build scout`-tagged file, and updates the two places whose prose describes these files' old tag in the same commit sequence as the retagging (per this repo's Documentation Lifecycle rule): `cmd/lyx/sandbox_coverage_test.go`'s `excludedModules["scout"]` reason string and CONSTRAINTS.md's Sandbox Suite Coverage bullet's matching reason string. `internal/scoutengine`'s other test files (`daemonstate_test.go`, `refs_test.go`, `ensureserver_test.go`, `definition_test.go`, `lspclient_test.go`, and the remainder of `supervised_test.go`) are untouched — they are substrate-free decision-logic tests per this task's discussion and stay exactly where they are. This batch depends on batch 1 because `cmd/lyx/tierpurity_test.go`'s `isTierTagged()` must already recognize `scout` (via the known-tags list) before the guard's classification of these files is conceptually correct, and because this batch's Card 11 edits the same `allowedSpawners` map batch 1 leaves in place.

`gopls` is not installed in this environment (confirmed via `which gopls` returning nothing), so this batch's `verify:` deliberately does not execute `go test -tags scout` — see `## Batch Tests` below and the overview's "gopls-gated verification step" Decision.

## Cards

### Card 6: Retag `ensureserver_integration_test.go` to `scout`

- **Context:** none
- **Edits:**
  - `internal/scoutengine/ensureserver_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change line 1's `//go:build integration` to `//go:build scout`. In the file-header comment, change `toolchain_integration_test.go's //go:build integration-tagged,` (line 5) to `toolchain_integration_test.go's //go:build scout-tagged,` and change the phrase "separately with `-tags integration`. Even though ensureNative itself" (line 8) to "separately with `-tags scout`. Even though ensureNative itself". In `TestEnsureServer_Integration_SupervisedDispatch`'s body comment, change `isolated from any other integration test's own supervised daemon,` (line 162) to `isolated from any other scout test's own supervised daemon,`. Do not change any filename cross-reference in this file (`refs_integration_test.go`, `toolchain_integration_test.go`, `supervised_integration_test.go` all keep their existing names — see the overview's "filenames are never changed by the retag" Decision) and do not change the function name `TestEnsureServer_Integration_SupervisedDispatch` itself.
- **Commit:** `test(scoutengine): retag ensureserver_integration_test.go from integration to scout`

### Card 7: Retag `refs_integration_test.go` to `scout`

- **Context:** none
- **Edits:**
  - `internal/scoutengine/refs_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change line 1's `//go:build integration` to `//go:build scout`. In the file-header comment, change `language server. It is //go:build integration-tagged and therefore` (line 5) to `language server. It is //go:build scout-tagged and therefore` and change the phrase "Invariant); it is run separately with `-tags integration` on a machine" (line 7) to "Invariant); it is run separately with `-tags scout` on a machine". Do not change the filename cross-references to `ensureserver_integration_test.go`/`supervised_integration_test.go` near the end of the file.
- **Commit:** `test(scoutengine): retag refs_integration_test.go from integration to scout`

### Card 8: Retag `supervised_integration_test.go` to `scout`

- **Context:** none
- **Edits:**
  - `internal/scoutengine/supervised_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change line 1's `//go:build integration` to `//go:build scout`. In the file-header comment, change `end to end through ensureServer itself. //go:build integration-tagged` (line 12) to `end to end through ensureServer itself. //go:build scout-tagged` and change the phrase "Purity Invariant); it is run separately with `-tags integration` on a" (line 14) to "Purity Invariant); it is run separately with `-tags scout` on a". Do not change the filename cross-references to `ensureserver_integration_test.go`/`refs_integration_test.go`.
- **Commit:** `test(scoutengine): retag supervised_integration_test.go from integration to scout`

### Card 9: Retag `toolchain_integration_test.go` to `scout`

- **Context:** none
- **Edits:**
  - `internal/scoutengine/toolchain_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change line 1's `//go:build integration` to `//go:build scout`. In the file-header comment, change `it is //go:build integration-tagged and therefore excluded from the plain` (line 5) to `it is //go:build scout-tagged and therefore excluded from the plain` and change the phrase "with `-tags integration`. Unlike refs_integration_test.go's gopls-presence" (line 7) to "with `-tags scout`. Unlike refs_integration_test.go's gopls-presence", and change the phrase "`go test -tags integration` can even run — so it runs unconditionally" (line 10) to "`go test -tags scout` can even run — so it runs unconditionally". Do not change the filename cross-reference to `refs_integration_test.go`.
- **Commit:** `test(scoutengine): retag toolchain_integration_test.go from integration to scout`

### Card 10: Split `supervised_test.go`'s two gopls-gated subtests into a new `scout`-tagged file

- **Context:**
  - `cmd/lyx/tierpurity_test.go`
- **Edits:**
  - `internal/scoutengine/supervised_test.go`
- **Creates:**
  - `internal/scoutengine/supervised_scout_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/scoutengine/supervised_test.go`: delete `TestEnsureSupervised_StaleSocketCleanupAllowsRebind` and `TestEnsureSupervised_DaemonLogsToOwnFileNotCallersStderr` in their entirety (both currently gated by `if _, err := exec.LookPath("gopls"); err != nil { t.Skip(builtins()["go"].InstallHint) }`). Rewrite the file-header comment (currently: "supervised_test.go covers ensureSupervised's state-file/lock/retry-exhaustion decision logic, mostly offline (no gopls spawn) — the readDaemonState+daemonStale combination itself is already covered by daemonstate_test.go and is not duplicated here. Its second sub-test is the exception: it needs a real bind attempt to prove the stale-socket cleanup step actually avoids EADDRINUSE, not just in a mocked scenario, so it is skip-gated on exec.LookPath(\"gopls\") via t.Skip rather than build-tag-gated like supervised_integration_test.go — it still runs as part of a plain `go test` whenever gopls happens to be on $PATH. Both sub-tests below spawn a real, short-lived child process...") to instead state: this file now covers exactly `ensureSupervised`'s state-file/lock/retry-exhaustion decision logic across its three remaining subtests — `TestEnsureSupervised_RetryExhaustionReturnsErrServerSpawnTimeout`, `TestEnsureSupervised_UncontendedLockWithUndialableHealthyStateReturnsErrServerSpawnTimeout`, `TestEnsureSupervised_WedgedEscalationReuseReleasesLock` — entirely offline (no gopls spawn); the two previously gopls-gated subtests (`TestEnsureSupervised_StaleSocketCleanupAllowsRebind`, `TestEnsureSupervised_DaemonLogsToOwnFileNotCallersStderr`) have moved to the new `//go:build scout`-tagged `supervised_scout_test.go` in this same package. Each remaining subtest spawns one real short-lived child process via `spawnAndHoldSubprocess` only as a PID-liveness fixture (allowlisted in `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map), never gopls itself. Leave `spawnAndHoldSubprocess` and the three remaining test functions otherwise unchanged (do not rename or restructure them).

  Create `internal/scoutengine/supervised_scout_test.go`, `package scoutengine`, `//go:build scout` as its first line, containing verbatim the two deleted test functions from above, but each with its `if _, err := exec.LookPath("gopls"); err != nil { t.Skip(builtins()["go"].InstallHint) }` skip-gate removed entirely (dropped, not replaced — the build tag is now the only gate, matching `toolchain_integration_test.go`'s established no-runtime-skip precedent). Since removing the skip-gate also removes the only use of `os/exec` and `builtins()` in these two functions, the new file's import block needs only `context`, `os`, `path/filepath`, `testing`, `time`, and `github.com/Knatte18/loomyard/internal/hubgeometry` (verify no other import is unused after the skip-gate removal). Give the new file a short header comment stating: this file holds this package's two subtests that need a real, already-installed `gopls` on `$PATH` to prove a real bind/log-file behavior — split out of `supervised_test.go` because, unlike that file's other three subtests, these two cannot run offline; manual invocation only via `go test -tags scout ./...`, no runtime skip-gate (the build tag is the only gate).
- **Commit:** `test(scoutengine): split supervised_test.go's two gopls-gated subtests into scout-tagged supervised_scout_test.go`

### Card 11: Update `tierpurity_test.go`'s `allowedSpawners` entry for `supervised_test.go`

- **Context:** none
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `allowedSpawners` map, change the `"internal/scoutengine/supervised_test.go"` entry's value from `"spawns short-lived test subprocesses for the retry-exhaustion PID-liveness fixture and the stale-socket-cleanup bind proof"` to `"spawns a short-lived test subprocess via spawnAndHoldSubprocess for its three remaining subtests' PID-liveness fixture (retry-exhaustion, uncontended-lock, and wedged-escalation-reuse)"`, dropping the now-inapplicable stale-socket-cleanup clause since that subtest moved to `supervised_scout_test.go` in Card 10, and broadening the description to all three subtests that still call `spawnAndHoldSubprocess` after the split (not just the retry-exhaustion one). Do not change the `"internal/scoutengine/daemonstate_test.go"` entry — it is unaffected by this split.
- **Commit:** `test(cmd/lyx): drop the stale-socket-cleanup clause from supervised_test.go's allowedSpawners reason after the scout split`

### Card 12: Update `sandbox_coverage_test.go`'s `scout` reason string

- **Context:** none
- **Edits:**
  - `cmd/lyx/sandbox_coverage_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `excludedModules` map (line 31), change the `"scout"` entry's value from `"requires an external language-server binary (gopls/pyright/csharp-ls) on $PATH; exercised by //go:build integration tests, not the black-box sandbox suite"` to `"requires an external language-server binary (gopls/pyright/csharp-ls) on $PATH; exercised by //go:build scout tests, not the black-box sandbox suite"` (the one-word `integration` → `scout` change, per this task's retagging).
- **Commit:** `test(cmd/lyx): update sandbox_coverage_test.go's scout exclusion reason for the integration-to-scout retag`

### Card 13: Update CONSTRAINTS.md's Sandbox Suite Coverage `scout` reason string

- **Context:** none
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `## Sandbox Suite Coverage` section's **Allowlist** bullet, change `` `scout` (requires an external language-server binary (gopls/pyright/csharp-ls) on $PATH; exercised by //go:build integration tests, not the black-box sandbox suite). `` to `` `scout` (requires an external language-server binary (gopls/pyright/csharp-ls) on $PATH; exercised by //go:build scout tests, not the black-box sandbox suite). `` — the same one-word change as Card 12's code edit, in the same commit-sequence as the retagging per this repo's Documentation Lifecycle rule.
- **Commit:** `docs(CONSTRAINTS): update Sandbox Suite Coverage's scout reason for the integration-to-scout retag`

## Batch Tests

`verify:` confirms the `scout` tag compiles and vets cleanly (`go build -tags scout`/`go vet -tags scout`, scoped to `cmd/lyx` and `internal/scoutengine`) and confirms the four retagged files no longer execute under either `go test ./...` (Tier 1) or `go test -tags integration ./...` (Tier 2), scoped to the same two packages. It deliberately does not run `go test -tags scout ./internal/scoutengine/...` — `gopls` is not on `$PATH` in this environment, and Card 10 removes the two moved subtests' runtime skip-gate, so that run would hard-fail here rather than skip (see the overview's Decision on this). An operator on a machine with `gopls` installed should separately run `go test -tags scout ./internal/scoutengine/... -count=1` to confirm the four retagged files and the two split subtests actually pass under the new tag — this is a manual confirmation step, not part of this batch's machine-enforced gate.
