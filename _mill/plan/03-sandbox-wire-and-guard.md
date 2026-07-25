# Batch: sandbox-wire-and-guard

```yaml
task: dev/test lyx.exe separated from production deploy
batch: sandbox-wire-and-guard
number: 3
cards: 7
verify: go test ./tools/sandbox/ ./cmd/lyx/
depends-on: [2]
```

## Batch Scope

Wires the batch-2 resolution core into every sandbox consumer and locks the invariant with a
guard test. `runSuite`, `runFetch`, and the Hub-build clone path stop resolving `lyx` by bare
PATH lookup and route through `resolveLyx`; the fingerprint gains a dev/prod `Source` marker in
both the stamped suite header and the report JSON; the launched agent gets `.dev-bin` prepended
to its child PATH (via `launchAgent` only); `cloneRun` takes an explicit resolved path. Existing
tests are updated to the new signatures, and a new guard test asserts no bare-PATH `lyx` lookup
survives outside `resolve.go`. After this batch the dev/prod split is fully functional.

Batch-local decisions: `binaryFingerprint` gains a `source string` parameter (stamped into
`binaryInfo.Source`); `launchAgent` gains a trailing `binDir string` parameter; `cloneRun` and
`decideClone` gain a trailing `lyxPath string` parameter. `muxDown` is deliberately unchanged.

## Cards

### Card 7: Wire resolveLyx + Source + agent PATH into suite.go

- **Context:**
  - `tools/sandbox/report.go`
  - `tools/sandbox/resolve.go`
- **Edits:**
  - `tools/sandbox/suite.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `Source string` field to `binaryInfo`. Change `binaryFingerprint(path
  string) (binaryInfo, error)` to `binaryFingerprint(path, source string) (binaryInfo, error)`
  and set `Source: source` on the returned `binaryInfo`. Add a `- Source: %s` line to
  `binaryInfo.header()` that renders the source (the value is `sourceDev`/`sourceProd`, i.e.
  `"dev"`/`"prod"`, from `resolve.go`). In `runSuite`, replace `lyxPath, err :=
  lookPath("lyx")` with `lyxPath, source, err := resolveLyx()`; pass `source` to
  `binaryFingerprint`; compute `binDir := ""` and set `binDir = filepath.Dir(lyxPath)` only
  when `source == sourceDev`; pass `binDir` as the new trailing argument to `launchAgent`.
  Change the `launchAgent` seam signature from `func(hostRepoDir, claudePath, instruction
  string) int` to `func(hostRepoDir, claudePath, instruction, binDir string) int`; inside, when
  `binDir != ""`, set `cmd.Env = prependPath(binDir, os.Environ())` before `cmd.Run()`
  (otherwise leave the environment inherited as today). Leave `muxDown` and its
  `muxDown(hostRepoDir, lyxPath)` call site unchanged.
- **Commit:** `feat(sandbox): resolve dev binary and mark source in runSuite`

### Card 8: Add Source to the report fingerprint

- **Context:**
  - `tools/sandbox/resolve.go`
  - `tools/sandbox/suite.go`
- **Edits:**
  - `tools/sandbox/report.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `Source string` field with json tag `json:"source"` to the
  `reportFingerprint` struct. In `runFetch`, replace `lyxPath, err := lookPath("lyx")` with
  `lyxPath, source, err := resolveLyx()` and call `binaryFingerprint(lyxPath, source)`. In
  `fetchReport`, extend the `reportFingerprint{...}` literal (currently Path/SHA256/Size/
  ModTime) with `Source: info.Source` so the marker reaches `meta.fingerprint` in
  `sandbox-report.json`.
- **Commit:** `feat(sandbox): stamp dev/prod source into report fingerprint`

### Card 9: Thread resolved path through the Hub-build clone

- **Context:**
  - `tools/sandbox/report.go`
  - `tools/sandbox/resolve.go`
- **Edits:**
  - `tools/sandbox/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change the `cloneRun` seam from `func(parentDir string) error` to
  `func(parentDir, lyxPath string) error`; inside, replace `exec.Command("lyx", "warp",
  "clone", hostURL, weftURL)` with `exec.Command(lyxPath, "warp", "clone", hostURL, weftURL)`
  and update the stale startup-error string `"lyx not found on PATH"` (the binary is now
  resolved by the caller — reword to reference the resolved `lyxPath` and point at
  `deploy-dev`). Change `decideClone(hubPath string, reset bool) error` to `decideClone(hubPath
  string, reset bool, lyxPath string) error` and pass `lyxPath` to `cloneRun`. In `run()`'s
  build case (around the `decideClone(hubPath, *reset)` call), resolve the binary first via
  `lyxPath, _, err := resolveLyx()` (handle the error by printing `"sandbox: ..."` to stderr
  and returning 1, matching the surrounding error style) and pass `lyxPath` to `decideClone`.
  After this change no bare `exec.Command("lyx", …)` remains in `main.go`.
- **Commit:** `feat(sandbox): resolve dev binary for Hub build clone`

### Card 10: Update suite tests for new signatures

- **Context:**
  - `tools/sandbox/resolve.go`
  - `tools/sandbox/suite.go`
- **Edits:**
  - `tools/sandbox/suite_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update every `binaryFingerprint(...)` call to pass a `source` argument
  (e.g. `sourceProd`). Assert `binaryInfo.header()` output contains a `Source:` line for both a
  `sourceDev` and a `sourceProd` input. Update the `launchAgent` seam stub to the new
  `(hostRepoDir, claudePath, instruction, binDir string)` signature; add an assertion that the
  captured `binDir` is empty on the prod path and, in a dev-path case, that `launchAgent`
  receives the `.dev-bin` directory (stub `devBinPath` to a temp `.../,dev-bin/lyx` that exists
  and `lookPath` accordingly so `runSuite` resolves `sourceDev`). For existing `runSuite` tests
  that stubbed only `lookPath`, also stub `devBinPath` to return a non-existent path so they
  continue to resolve `sourceProd`. Keep all tests Tier-1 pure (seams + temp dirs only).
- **Commit:** `test(sandbox): update suite tests for source + binDir`

### Card 11: Update report tests for Source field

- **Context:**
  - `tools/sandbox/report.go`
  - `tools/sandbox/resolve.go`
- **Edits:**
  - `tools/sandbox/report_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `runFetch`/`fetchReport` tests for the new resolution: stub
  `devBinPath` (non-existent → `sourceProd`, or existent temp → `sourceDev`) and `lookPath` as
  needed. Assert the emitted `sandbox-report.json` `meta.fingerprint` now carries a `source`
  field equal to `"prod"` (prod path) and `"dev"` (dev path). Update any `binaryFingerprint`
  call to pass a `source` argument. Tier-1 pure.
- **Commit:** `test(sandbox): assert source in report fingerprint JSON`

### Card 12: Update main tests for cloneRun and launchAgent signatures

- **Context:**
  - `tools/sandbox/main.go`
  - `tools/sandbox/suite.go`
  - `tools/sandbox/resolve.go`
- **Edits:**
  - `tools/sandbox/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the `cloneRun` seam stub and every `decideClone(...)` test call to
  the new `lyxPath` parameter; assert `decideClone` forwards the received `lyxPath` into
  `cloneRun` (capture it in the stub). **Also** update every `launchAgent` stub in
  `main_test.go` from the 3-arg `func(dir, claude, instruction string) int` form to the new
  4-arg `func(hostRepoDir, claudePath, instruction, binDir string) int` signature (Card 7's
  change) — there are several such stubs across the run()/suite-routing tests; missing any one
  makes `main_test.go` fail to compile. In suite-routing tests that reach `runSuite`, stub
  `devBinPath` to return a non-existent path so resolution stays `sourceProd` (no `.dev-bin`
  dependency). Tier-1 pure (no real clone, no real agent).
- **Commit:** `test(sandbox): update main tests for cloneRun + launchAgent`

### Card 13: Guard test forbidding bare-PATH lyx lookups

- **Context:**
  - `tools/sandbox/resolve.go`
  - `tools/sandbox/suite.go`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/hermeticenv_test.go`
- **Creates:**
  - `tools/sandbox/pathresolve_guard_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `package main` guard test enforcing the Dev/Prod Binary Separation
  invariant. Scan every non-test `*.go` file in the `tools/sandbox` directory (skip files
  ending `_test.go`, so the guard's own source is excluded) for the banned bare-PATH literals
  `lookPath("lyx")`, `exec.Command("lyx"`, and `exec.CommandContext("lyx"`. Fail if any appears
  in a file other than `resolve.go` (the single allowlisted resolution site), with a message
  naming the offending file and directing the author to route through `resolveLyx`. Mirror the
  allowlist-map + vacuous-scan-protection style of `cmd/lyx/tierpurity_test.go` (fail if fewer
  than 3 non-test `.go` files are scanned, guarding against a misconfigured directory read).
  Locate the sandbox source directory relative to the test file (e.g. via `runtime.Caller` or
  the working directory), not a hardcoded absolute path. Tier-1 pure (file reads only, no
  spawns). **Also** add an `allowedSpawners` (self-exclusion allowlist) entry in
  `cmd/lyx/tierpurity_test.go` for `tools/sandbox/pathresolve_guard_test.go`, keyed by its
  module-relative path with a reason like "contains the banned `exec.Command`/
  `exec.CommandContext` token strings as its own scan data (Dev/Prod Binary Separation guard)"
  — mirror the existing `cmd/lyx/tierpurity_test.go` / `cmd/lyx/hermeticenv_test.go` entries.
  **Likewise** add an `allowedNonHermetic` entry (same module-relative key + reason style) in
  `cmd/lyx/hermeticenv_test.go` for `tools/sandbox/pathresolve_guard_test.go`, because
  `hermeticenv_test.go`'s `gitSpawnTokens` also includes `exec.Command`/`exec.CommandContext`
  and its module-wide walk would otherwise mark the new file git-spawning-without-hermetic.
  Without both allowlist entries the module-wide guards (`go test ./cmd/lyx/`) fail on the new
  guard file's scan literals.
- **Commit:** `test(sandbox): guard against bare-PATH lyx lookups`

## Batch Tests

`verify: go test ./tools/sandbox/ ./cmd/lyx/` compiles the rewired package and runs all sandbox
tests (updated suite/report/main tests, the batch-2 `resolve_test.go`, and the new
`pathresolve_guard_test.go`) AND the `cmd/lyx` module-wide guards. The `./cmd/lyx/` scope is
required because the new guard file carries `exec.Command`/`exec.CommandContext` scan literals
that BOTH module-wide guards inspect — Test Tier Purity (`cmd/lyx/tierpurity_test.go`) and
Hermetic Git Test Environment (`cmd/lyx/hermeticenv_test.go`); this batch adds the matching
`allowedSpawners` and `allowedNonHermetic` self-exclusion entries, and only `go test ./cmd/lyx/`
confirms them. The sandbox guard passes only because cards 7–9 removed every bare-PATH `lyx` lookup outside
`resolve.go`. All tests remain Tier-1 pure (seams, temp dirs, file scans).
