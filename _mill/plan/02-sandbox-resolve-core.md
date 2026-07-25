# Batch: sandbox-resolve-core

```yaml
task: dev/test lyx.exe separated from production deploy
batch: sandbox-resolve-core
number: 2
cards: 2
verify: go test ./tools/sandbox/
depends-on: [1]
```

## Batch Scope

Adds the sandbox-side resolution foundation in a new, self-contained `tools/sandbox/resolve.go`
without yet wiring it into the existing call sites (that is batch 3). This isolates the two new
pure/seamed primitives — `resolveLyx` (derived-dev-first, PATH-fallback binary resolution) and
`prependPath` (child-PATH construction) — so they can be unit-tested before the riskier
rewiring of `suite.go`/`report.go`/`main.go`. `resolve.go` is the ONLY file in `tools/sandbox`
permitted to contain a bare-PATH `lyx` lookup; batch 3 adds the guard test that enforces this.

Batch-local decision: `resolve.go` introduces a `devBinPath` seam (`var devBinPath =
devbin.BinPath`) and reuses the existing `lookPath` seam (declared in `suite.go`) so both the
dev-path derivation and the PATH fallback are injectable in tests without touching the real
`<repoRoot>/.dev-bin`.

## Cards

### Card 5: Create resolve.go with resolveLyx + prependPath

- **Context:**
  - `tools/sandbox/suite.go`
  - `tools/internal/devbin/devbin.go`
  - `go.mod`
- **Edits:** none
- **Creates:**
  - `tools/sandbox/resolve.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `tools/sandbox/resolve.go` in `package main`. Import
  `github.com/Knatte18/loomyard/tools/internal/devbin`. Declare source constants `sourceDev =
  "dev"` and `sourceProd = "prod"`. Declare a seam `var devBinPath = devbin.BinPath` so tests
  can inject the derived dev-binary path. Add `resolveLyx() (path string, source string, err
  error)`: call `devBinPath()`; if it succeeds AND `os.Stat(devPath)` shows the file exists,
  return `(devPath, sourceDev, nil)`; otherwise fall back to the existing package-level
  `lookPath("lyx")` seam (declared in `suite.go`) and return `(prodPath, sourceProd, nil)`,
  propagating a non-nil `lookPath` error unchanged (wrapped with the same "lyx not found …
  deploy the binary" guidance style used in `suite.go`/`report.go` today, extended to mention
  `deploy-dev`). A `devBinPath()` error is non-fatal — treat it as "no dev binary" and fall
  through to the PATH branch. Add `prependPath(dir string, environ []string) []string`: when
  `dir == ""` return `environ` unchanged; otherwise return a copy of `environ` where the
  `PATH=` entry has `dir + string(os.PathListSeparator)` prepended to its value (add a fresh
  `PATH=` entry if none exists); leave every non-PATH entry untouched and preserve order.
- **Commit:** `feat(sandbox): add resolveLyx + prependPath resolution core`

### Card 6: Unit-test resolveLyx and prependPath

- **Context:**
  - `tools/sandbox/resolve.go`
  - `tools/sandbox/suite.go`
  - `tools/sandbox/suite_test.go`
- **Edits:** none
- **Creates:**
  - `tools/sandbox/resolve_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `package main` tests (Tier-1 pure — no spawns; `os.Stat`/`os.WriteFile`
  on `t.TempDir()` only). For `resolveLyx`, stub the `devBinPath` and `lookPath` seams
  (save/restore via `defer`): (a) `devBinPath` returns a temp path that exists → assert
  `source == sourceDev` and `path` is that temp path; (b) `devBinPath` returns a
  non-existent temp path, `lookPath` returns a fake prod path → assert `source == sourceProd`
  and `path` is the fake prod path; (c) same as (b) but `lookPath` returns an error → assert
  the error propagates. For `prependPath`: assert `dir` is the first `PATH` segment with prior
  segments preserved after it; assert a non-`PATH` env var (e.g. `HOME=/x`) is untouched;
  assert `dir == ""` returns the input environ unchanged (equal slice contents).
- **Commit:** `test(sandbox): cover resolveLyx source selection and prependPath`

## Batch Tests

`verify: go test ./tools/sandbox/` runs the full sandbox package tests, including the new
`resolve_test.go` and all pre-existing suite/report/main tests (which remain green — `resolve.go`
is additive and not yet wired in). Tier-1 pure: seams are stubbed and only temp files are
touched.
