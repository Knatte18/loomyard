# Batch: toolchain-manager

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: toolchain-manager
number: 2
cards: 5
verify: go test -count=1 ./internal/codeintelengine/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

Adds the Go toolchain manager: resolve-or-install a pinned `gopls` binary
into a codeintel-owned, machine-global cache directory, fenced by its own
per-language install lock so two worktrees racing a cold install don't
duplicate the work. This is the batch where `internal/codeintelengine`
first imports `internal/lock`, so it is also where the `CONSTRAINTS.md`
Codeintelengine Leaf Invariant amendment lands — in card 6's own commit,
the same one that introduces the import, per the Documentation Lifecycle
(a card whose commit imports a not-yet-allowlisted package would fail
`TestLeafInvariant_AllowlistOnly` in isolation). Card 7 separately backports
an unrelated, pre-existing Sandbox Suite Coverage documentation gap while
`CONSTRAINTS.md` is already being touched.

The external interface batch 5's `ensureNative` consumes:
`resolveGoToolchain(ctx context.Context, pinnedVersion string) (binPath
string, err error)`. Nothing in this batch wires the result into
`EnsureServer` yet — that happens in batch 5, once the native strategy
exists to consume it.

Batch-local decision: the installer is injected via a package-level
function variable, not a hardcoded `exec.Command` call inside
`resolveGoToolchain` itself, so the unit tests in card 4 can assert
install-vs-skip behavior without ever invoking a real `go install`. This
mirrors this package's existing test-seam convention
(`newLSPClient` vs. the injectable `newLSPClientFromRW` in
`lspclient.go`).

## Cards

### Card 5: Cache-dir and install-lock path helpers

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:**
  - `internal/codeintelengine/toolchain.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the new file, add two unexported helpers:
  `goToolchainCacheDir(version string) string` returning
  `filepath.Join(os.UserCacheDir(), "lyx", "tools", "go", version)`, and
  `goToolchainInstallLock() string` returning
  `filepath.Join(os.UserCacheDir(), "lyx", "tools", "go",
  "install.lock")` — deliberately **not** version-scoped (one lock per
  language, not per version), so two processes installing two different
  pinned versions of the same language still serialize through one lock,
  matching `toolchain-manager-authority`'s "one per language" decision.
  Do not use `hubgeometry` for either path: `os.UserCacheDir()` is
  explicitly outside the Hub Geometry Invariant's scope (machine-global,
  not worktree/hub geometry) per `_mill/discussion.md`'s
  `toolchain-manager-authority` decision, so hand-joining here does not
  violate that invariant. Write the file's header doc comment now (even
  though the file gains more code in card 6): state that this file
  implements the Go-only toolchain manager, that it ignores `$PATH`
  entirely for Go, and that `os.UserCacheDir()` was chosen because it is
  the idiomatic stdlib answer to "OS-appropriate cache root" with no
  platform-specific logic to get wrong.
- **Commit:** `feat(codeintelengine): add Go toolchain cache-dir and install-lock path helpers`

### Card 6: `resolveGoToolchain` — pinned-version resolve/install with double-checked locking, allowlist `internal/lock`

- **Context:**
  - `internal/lock/lock.go`
- **Edits:**
  - `internal/codeintelengine/toolchain.go`
  - `CONSTRAINTS.md`
  - `internal/codeintelengine/leaf_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** **Land the `internal/lock` leaf-invariant allowlist
  amendment in this same card/commit**, not a card later — this is the
  card that actually introduces the `internal/lock` import into
  `toolchain.go` (via `lock.AcquireWriteLock`, below), and a card whose
  own commit imports a not-yet-allowlisted package would fail
  `TestLeafInvariant_AllowlistOnly` in isolation (batch 4's analogous
  `internal/proc` amendment, card 13, already gets this ordering right —
  allowlist *before* the introducing import — this card matches it). In
  `internal/codeintelengine/leaf_enforcement_test.go`, add
  `"github.com/Knatte18/loomyard/internal/lock": true,` to the
  `allowedImports` map (alongside the existing `hubgeometry` and
  `yaml.v3` entries). In `CONSTRAINTS.md`'s "Codeintelengine Leaf
  Invariant" section, update the opening statement's import list from
  "stdlib, `internal/hubgeometry`, and `gopkg.in/yaml.v3`" to add
  `internal/lock`, and add a bullet explaining why: `internal/lock`
  fences the toolchain-install race (this batch) and the daemon
  spawn-race (batch 4) — both genuinely cross-process coordination
  problems the leaf needs to solve itself, and `internal/lock` is
  already the repo's one primitive for exactly that, so allowlisting it
  is reuse rather than a new dependency class. Add
  `type toolchainInstaller func(ctx context.Context, version, destDir
  string) error` and a package-level var
  `var installGoToolchain toolchainInstaller = runGoInstall` (the
  production seam) alongside an unexported
  `func runGoInstall(ctx context.Context, version, destDir string) error`
  that runs `go install golang.org/x/tools/gopls@<version>` with
  `GOBIN` set to `destDir` via the subprocess's `Env` (so the built
  binary lands directly in the cache dir rather than the default
  `$GOPATH/bin`), and `resolveGoToolchain(ctx context.Context,
  pinnedVersion string) (string, error)`: (1) compute `cacheDir :=
  goToolchainCacheDir(pinnedVersion)` and `binPath :=
  filepath.Join(cacheDir, "gopls")` (`.exe` suffix on `runtime.GOOS ==
  "windows"`, matching how the rest of this repo names platform
  binaries); (2) if `binPath` already exists (`os.Stat`, `err == nil`),
  return it immediately with **no lock taken at all** — the common-case
  fast path must not pay a lock round trip; (3) otherwise call
  `lock.AcquireWriteLock(goToolchainInstallLock())` (the **blocking**
  variant — a second installer must wait for the first to finish, there
  is no "loser reconnects to a live server" shortcut for a toolchain
  install), `defer` its `Release()`; (4) **double-check** `binPath`
  again after acquiring the lock — the process that was waiting may find
  the version already installed by whoever held the lock first — and
  return it if now present, skipping the install; (5) otherwise
  `os.MkdirAll(cacheDir, 0o755)` then call `installGoToolchain(ctx,
  pinnedVersion, cacheDir)`, and on success return `binPath`. Wrap every
  error with `fmt.Errorf("codeintelengine: ...: %w", err)` naming the
  failing step, matching this package's existing error-wrapping style
  (see `refs.go`). This function does not touch `entry.Command` at all —
  batch 5's `ensureNative` is the one that substitutes the resolved
  `binPath` for `entry.Command[0]`.
- **Commit:** `feat(codeintelengine): add resolveGoToolchain with double-checked install locking; allowlist internal/lock`

### Card 7: CONSTRAINTS.md — Sandbox Suite Coverage backport

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `cmd/lyx/sandbox_coverage_test.go`'s live
  `excludedModules` map already carries a `"codeintel"` entry with the
  reason `"requires an external language-server binary
  (gopls/pyright/csharp-ls) on $PATH; exercised by //go:build
  integration tests, not the black-box sandbox suite"` — but
  `CONSTRAINTS.md`'s "Sandbox Suite Coverage" section's Allowlist bullet
  only documents `ide` and `selfreport`. Add a third line for
  `codeintel`, reusing that exact reason text verbatim (do not
  paraphrase — the point is the doc and the test agree word-for-word).
  This is pre-existing drift unrelated to this task's own scope, folded
  into this batch only because `CONSTRAINTS.md` is already being touched
  by card 6's Leaf Invariant amendment.
- **Commit:** `docs(constraints): backport the codeintel sandbox-coverage exclusion note`

### Card 8: Unit tests for resolveGoToolchain (mocked installer)

- **Context:**
  - `internal/codeintelengine/toolchain.go`
  - `internal/lock/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/toolchain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Untagged, offline, spawn-free (never calls the real
  `installGoToolchain` var — every sub-test swaps it for a fake before
  running and restores the original after via `t.Cleanup`). Use
  `t.Setenv` or a package-level override of `os.UserCacheDir`'s
  consumers by pointing `goToolchainCacheDir`'s inputs at a `t.TempDir()`
  — since `goToolchainCacheDir` calls `os.UserCacheDir()` directly with
  no injection point, add a minimal package-level var seam for it too
  (`var userCacheDir = os.UserCacheDir`) so tests can redirect it to a
  temp dir without touching the real machine-global cache; wire
  `goToolchainCacheDir`/`goToolchainInstallLock` to call `userCacheDir()`
  instead of `os.UserCacheDir()` directly (a one-line change to card 5's
  helpers — note this here since it only becomes necessary once this
  card needs the seam, but make the edit in `toolchain.go`, not by
  duplicating the helpers in the test file). Cover: (1) pinned version
  already present at `binPath` — `resolveGoToolchain` returns it and the
  fake installer is never invoked (assert via a call counter) and no
  lock file is created; (2) pinned version absent — the fake installer
  is invoked exactly once with the expected `(version, destDir)` pair,
  and `resolveGoToolchain` returns the path the fake installer "wrote"
  (have the fake actually create an empty file at `filepath.Join(destDir,
  "gopls")` so the post-install `os.Stat` succeeds, exercising the real
  return path rather than a hand-faked string); (3) concurrent-install
  serialization — launch two goroutines calling `resolveGoToolchain` for
  the same absent version at once, assert the fake installer is invoked
  exactly once total (the double-check-after-lock branch is what the
  second goroutine must hit) using a counter guarded by its own mutex,
  not the toolchain's own lock, so the assertion doesn't accidentally
  depend on the code under test being correct.
- **Commit:** `test(codeintelengine): cover resolveGoToolchain install/skip/concurrency paths`

### Card 9: Skip-gated integration test for a real `go install`

- **Context:**
  - `internal/codeintelengine/toolchain.go`
  - `internal/codeintelengine/refs_integration_test.go`
  - `internal/codeintelengine/registry.go`
- **Edits:**
  - `cmd/lyx/hermeticenv_test.go`
- **Creates:**
  - `internal/codeintelengine/toolchain_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration`-tagged, mirroring
  `refs_integration_test.go`'s header-comment style. Unlike that file's
  `gopls`-presence skip gate, this test has no natural skip condition —
  it needs only the `go` toolchain itself, which is guaranteed present
  in any environment where `go test -tags integration` can even run — so
  it runs unconditionally under the tag (state this explicitly in the
  test's doc comment so a future reader doesn't go looking for a missing
  `t.Skip`). Point `userCacheDir` (card 8's seam) at `t.TempDir()` for
  the duration of the test via `t.Cleanup` restoration, then call
  `resolveGoToolchain(ctx, builtins()["go"].PinnedVersion)` for real (the
  production `installGoToolchain` var, untouched) with a
  `context.WithTimeout` generous enough for a real module-proxy fetch
  (120 seconds), and assert the returned binary path exists and
  `exec.Command(binPath, "version").Run()` succeeds — proving the
  installed binary is actually a working `gopls`, not just a file that
  happens to exist at the expected path. **This file's own
  `exec.Command(binPath, "version")` call is a direct subprocess spawn in
  test code**, which trips `cmd/lyx/hermeticenv_test.go`'s repo-wide
  git-spawn-token scan (that guard scans every `*_test.go` file
  regardless of build tag, unlike the tier-purity guard) — add
  `"internal/codeintelengine": "spawns gopls and go install for
  EnsureServer/toolchain-manager integration coverage, plus short-lived
  test-only subprocesses for PID-liveness fixtures (batches 4, 6);
  never git",` to `allowedNonHermetic` in `cmd/lyx/hermeticenv_test.go`,
  mirroring the existing `internal/proc` entry's shape and reasoning.
  This is a **package-level** entry (hermeticenv's file-level allowlist
  form is reserved for excluding a guard's own test-data literals, not
  for granting a real exemption — see that map's own doc comment), added
  here because this is the first batch, in DAG order, to introduce a
  codeintelengine test file containing a raw `exec.Command` substring;
  later batches (4, 6) that add their own spawning test files rely on
  this same entry and do not need to touch this guard again.
- **Commit:** `test(codeintelengine): add skip-free integration test for real gopls install; allowlist the package for the hermetic-git guard`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/codeintelengine/...
./cmd/lyx/...` — `cmd/lyx/...` because card 9 edits
`hermeticenv_test.go`'s allowlist, and that guard's own test must stay
green. No `-tags integration`, so card 9's own new test does not run in
this gate — it is exercised manually/in CI's integration tier alongside
`refs_integration_test.go`, per the Test Tier Purity Invariant. Card 6's
`leaf_enforcement_test.go` edit is verified by the same `go test` run,
since `TestLeafInvariant_AllowlistOnly` runs untagged.
</content>
