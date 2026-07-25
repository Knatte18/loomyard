# Batch: devbin-and-deploy

```yaml
task: dev/test lyx.exe separated from production deploy
batch: devbin-and-deploy
number: 1
cards: 4
verify: go test ./tools/internal/devbin/ ./tools/deploy/
depends-on: []
```

## Batch Scope

Establishes the single derived source of truth for the dev binary location and wires the
`deploy` tool to it. Creates the `tools/internal/devbin` package (repo-root derivation +
`.dev-bin` path convention) and adds a `-dev` flag to `tools/deploy` that installs into that
derived directory. This batch's `devbin` package is the external interface every later batch
consumes: `tools/sandbox` (batches 2–3) resolves the dev binary through it, and the launchers
(batch 4) invoke `deploy -dev`. No hardcoded paths are introduced.

Batch-local decision: `devbin` exposes `RepoRoot() (string, error)`, `Dir() (string, error)`
(= `<repoRoot>/.dev-bin`), and `BinPath() (string, error)` (= `<repoRoot>/.dev-bin/lyx`, with
`.exe` on Windows). `tools/deploy` drops its own `repoRoot()` and sources the build root from
`devbin.RepoRoot()` so there is exactly one `runtime.Caller` derivation in the codebase.

## Cards

### Card 1: Create the devbin package

- **Context:**
  - `tools/deploy/main.go`
  - `go.mod`
- **Edits:** none
- **Creates:**
  - `tools/internal/devbin/devbin.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `devbin` at `tools/internal/devbin/devbin.go` (module path
  `github.com/Knatte18/loomyard/tools/internal/devbin`, per `go.mod`). Expose three functions:
  `RepoRoot() (string, error)` — derives the repository root from this source file's location
  via `runtime.Caller(0)` and `filepath.Join(dir, "..", "..", "..")` (the file sits at
  `tools/internal/devbin/devbin.go`, three levels below the root); mirror the error-handling
  shape of `tools/deploy/main.go`'s existing `repoRoot()` (return an error when
  `runtime.Caller` reports `!ok`). `Dir() (string, error)` — returns `filepath.Join(root,
  ".dev-bin")` using `RepoRoot()`. `BinPath() (string, error)` — returns `filepath.Join(dir,
  name)` where `name` is `lyx`, or `lyx.exe` when `runtime.GOOS == "windows"` (mirror the
  binary-name logic in `tools/deploy/main.go`'s `run()`). Do not hardcode any absolute path.
- **Commit:** `feat(devbin): add derived .dev-bin path helper`

### Card 2: Unit-test the devbin package

- **Context:**
  - `tools/sandbox/suite_test.go`
- **Edits:** none
- **Creates:**
  - `tools/internal/devbin/devbin_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `package devbin` tests (Go-native, Tier-1 pure — no spawns). Assert:
  `RepoRoot()` returns a path ending in the repo directory name and that
  `tools/internal/devbin` exists beneath it (sanity that the depth is right); `Dir()` returns
  `RepoRoot()` joined with `.dev-bin`; `BinPath()` returns `Dir()` joined with `lyx` on
  non-Windows and `lyx.exe` on Windows — gate the extension assertion on `runtime.GOOS` so the
  test is correct on every platform. Use `filepath` comparisons, not string literals with
  hardcoded separators. Follow the temp-dir / table-test style of
  `tools/sandbox/suite_test.go`.
- **Commit:** `test(devbin): cover RepoRoot/Dir/BinPath derivation`

### Card 3: Add `-dev` flag to the deploy tool

- **Context:**
  - `go.mod`
- **Edits:**
  - `tools/deploy/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Import `github.com/Knatte18/loomyard/tools/internal/devbin`. Add a
  `-dev` bool flag (`flag.Bool("dev", false, …)`). Replace the existing `repoRoot()` function
  and its single call site in `run()` with `devbin.RepoRoot()` (delete the local `repoRoot()`
  definition entirely — there must be no second `runtime.Caller` derivation). Introduce a
  destination resolver `resolveDest(dev bool, dest string) (string, error)`: return an error
  `"-dev and -dest are mutually exclusive"` when both `dev` is true and `dest != ""`; when
  `dev` is true return `devbin.Dir()`; when `dest != ""` return `dest`; otherwise fall back to
  the existing `goBinDir()`. Rewire `run()` to take the `dev` flag and use `resolveDest` in
  place of the current inline `destDir == "" → goBinDir()` block. Keep the binary-name and
  build logic otherwise unchanged. Update `main()` to parse and thread the `-dev` flag.
- **Commit:** `feat(deploy): add -dev flag targeting derived .dev-bin`

### Card 4: Test deploy destination resolution

- **Context:**
  - `tools/deploy/main.go`
- **Edits:** none
- **Creates:**
  - `tools/deploy/main_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `package main` tests (Tier-1 pure — no `go build`/`go env` spawn).
  Cover `resolveDest`: `resolveDest(true, "/x")` returns an error (mutual exclusion);
  `resolveDest(true, "")` returns `devbin.Dir()` (compare against `devbin.Dir()` directly, not
  a hardcoded path); `resolveDest(false, "/some/dir")` returns `/some/dir`. Do NOT test the
  `goBinDir()` fallback path (it shells out to `go env` — leave it uncovered to keep the test
  hermetic).
- **Commit:** `test(deploy): cover resolveDest -dev/-dest handling`

## Batch Tests

`verify: go test ./tools/internal/devbin/ ./tools/deploy/` compiles and runs both new test
files. `devbin_test.go` covers the derivation helpers; `main_test.go` covers `resolveDest`.
Both are Tier-1 pure (no process spawns). The `goBinDir()` `go env` path is intentionally
uncovered to keep tests hermetic.
