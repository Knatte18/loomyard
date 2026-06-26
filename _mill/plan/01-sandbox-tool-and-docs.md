# Batch: sandbox-tool-and-docs

```yaml
task: "Local lyx sandbox for manual experimentation"
batch: "sandbox-tool-and-docs"
number: 1
cards: 3
verify: go test ./tools/sandbox/... ./internal/paths/...
depends-on: []
```

## Batch Scope

This batch delivers the entire task: a small Go dev tool `tools/sandbox` that builds the
dogfood Hub by driving the deployed `lyx warp clone`, a very thin `sandbox.cmd` launcher
that supplies the machine-specific parent directory, and the durable docs (a new
`docs/dogfood-hub.md` plus a pointer section in `docs/overview.md`). It is one batch because
the pieces are small, tightly related, and share the same context (the `warp clone`
contract, the `deploy.cmd` launcher shape, and the overview doc). There is no external Go
interface a later batch consumes — the tool is a standalone `package main` command. All
Shared Decisions in `00-overview.md` apply unchanged; this batch defines no overrides.

## Cards

### Card 1: sandbox Go tool + unit tests

- **Context:**
  - `internal/warp/clone.go`
  - `tools/deploy/main.go`
  - `CONSTRAINTS.md`
  - `cmd/lyx/main.go`
- **Edits:** none
- **Creates:**
  - `tools/sandbox/main.go`
  - `tools/sandbox/main_test.go`
- **Deletes:** none
- **Requirements:**
  - `tools/sandbox/main.go` is `package main`. It builds the dogfood Hub by invoking the
    on-PATH `lyx` binary as a subprocess: `lyx warp clone <hostURL> <weftURL>` (board
    argument deliberately omitted so `warp clone` derives `<weft>.wiki.git`).
  - Define fixed constants: `hostURL = "https://github.com/Knatte18/lyx-test"`,
    `weftURL = "https://github.com/Knatte18/lyx-test-weft"`, and the Hub directory name
    `hubName = "lyx-test-HUB"` (the host basename + `-HUB`, matching
    `internal/warp/clone.go`'s `deriveHostName(hostURL)+"-HUB"`). No flags to override the
    URLs.
  - Parse two flags via the `flag` package: `-parent` (string, **required** — error with a
    clear message and non-zero exit if empty; no default) and `-reset` (bool, default false).
  - Resolve `-parent` to an absolute path with `filepath.Abs` (relative values resolve
    against the process working directory). Compute `hubPath = filepath.Join(absParent,
    hubName)`. Do **not** call `os.Getwd` or `git rev-parse --show-toplevel` anywhere (the
    enforcement scan in `internal/paths/enforcement_test.go` bans both tokens tree-wide). The
    scan is a plain substring match over the whole file (comments included; only `_test.go`
    files are skipped), so the literal strings `os.Getwd` and `--show-toplevel` must not
    appear in `main.go` even inside comments or doc comments (`os.RemoveAll` in the seam
    comment is fine).
  - Decision logic: if `hubPath` already exists and `-reset` is not set → print a message
    that the Hub already exists and that `-reset` rebuilds it, then exit 0 **without**
    cloning (no-op success). If `hubPath` exists and `-reset` is set → remove the Hub
    directory, then clone. If `hubPath` does not exist → clone. Only ever remove the computed
    `hubPath` (the `lyx-test-HUB` directory) — never the parent directory.
  - Run the clone with `exec.Command("lyx", "warp", "clone", hostURL, weftURL)` and set
    `cmd.Dir = absParent` so `warp clone` creates `<absParent>/lyx-test-HUB`. Wire
    `cmd.Stdout`/`cmd.Stderr` to the process's stdout/stderr (stream verbatim) and propagate
    a non-zero exit (per the `error-surface-verbatim` Shared Decision). Distinguish the two
    failure shapes: when the clone run returns an `*exec.ExitError`, the subprocess already
    streamed its own stderr, so exiting non-zero is enough; when it returns a non-`ExitError`
    (a startup failure such as `lyx` not found on PATH, where there is no subprocess stderr),
    write a clear cause to stderr (e.g. `sandbox: lyx not found on PATH: <err>`) before
    exiting non-zero, so the failure is legible rather than a bare exit code.
  - Provide testability seams as package-level variables so the unit tests can exercise the
    decision logic without network or a real `lyx`: a clone-runner variable (e.g.
    `var cloneRun = func(parentDir string) error { ... }`) and a removal variable (e.g.
    `var removeAll = os.RemoveAll`, the same package-level-var seam pattern used at
    `internal/warp/clone.go:30`). Factor the existence/reset decision into a pure,
    directly-callable function that takes the hub path, the reset flag, and these seams so a
    test can assert which seam was called.
  - `tools/sandbox/main_test.go` covers, with the seams stubbed (no network): (a) hub-path
    computation from `-parent` for both an absolute and a relative input; (b) Hub absent →
    the clone runner is invoked with the parent directory; (c) Hub present + no `-reset` →
    the clone runner is **not** invoked (no-op); (d) Hub present + `-reset` → `removeAll` is
    called on the hub path before the clone runner runs; (e) the clone runner returns an
    error → the decision function surfaces it (non-nil return), covering the
    `error-surface-verbatim` propagation behavior. Use Go's standard `testing` package and
    table-driven subtests; create any "existing Hub" directory under `t.TempDir()`.
  - Follow Go house style as seen in `tools/deploy/main.go` (flag parsing, `exec.Command`,
    `fmt.Fprintln(os.Stderr, ...)` + `os.Exit(1)` on error). Do not copy deploy's purpose or
    its `runtime.Caller`/`git`-tag logic — the sandbox tool neither locates the module root
    nor shells out to git.
- **Commit:** `feat(sandbox): add tools/sandbox Hub builder with unit tests`

### Card 2: sandbox.cmd launcher

- **Context:**
  - `deploy.cmd`
- **Edits:** none
- **Creates:**
  - `sandbox.cmd`
- **Deletes:** none
- **Requirements:**
  - `sandbox.cmd` is a thin Windows launcher at the repo root, structurally like `deploy.cmd`:
    `@echo off`, `pushd "%~dp0"` so `go run` finds `go.mod`, then
    `go run ./tools/sandbox -parent C:\Code %*` (the machine-specific parent dir lives HERE,
    not in the Go source), capture `%ERRORLEVEL%` into a variable, `popd`, and
    `exit /b %EXITCODE%` to preserve the tool's exit code. Forward extra args via `%*` so
    `sandbox.cmd -reset` reaches the tool.
  - Add a short header comment block (REM lines) stating that the machine-specific install
    parent (`C:\Code`) is hardcoded here while the Go tool stays general — mirror the
    explanatory comment style in `deploy.cmd`.
- **Commit:** `feat(sandbox): add sandbox.cmd launcher`

### Card 3: dogfood-hub docs + overview pointer

- **Context:**
  - `internal/warp/clone.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:**
  - `docs/dogfood-hub.md`
- **Deletes:** none
- **Requirements:**
  - Create `docs/dogfood-hub.md` recording the durable convention: the two dedicated repo
    URLs (host `https://github.com/Knatte18/lyx-test`, weft
    `https://github.com/Knatte18/lyx-test-weft`); the board = derived weft wiki
    (`https://github.com/Knatte18/lyx-test-weft.wiki.git`, via `deriveBoardURL` in
    `internal/warp/clone.go`) and the operational precondition that this GitHub wiki must be
    initialized (Wikis enabled + at least one page); the Hub-naming convention (host basename
    → `lyx-test-HUB`, with members `lyx-test/`, `lyx-test-weft/`, `_board/`); the on-disk
    location `C:\Code\lyx-test-HUB` and the explicit rule that it lives **outside**
    `C:\Code\loomyard\` so it is never mistaken for part of loomyard; how to build/rebuild it
    (`sandbox.cmd`, `-reset` to rebuild); the hard precondition that a current `lyx` must be
    on PATH (deployed separately — the sandbox tool does not deploy); the dogfood purpose
    (point lyx's agent-driven flow at `lyx-test`; a break is a LoomYard bug to fix); and that
    the `lyx-test` / `lyx-test-weft` repos are dedicated to this use only.
  - Edit `docs/overview.md` to add a short "Dogfood Hub" section (one paragraph summarizing
    the bench and linking to `dogfood-hub.md`) and add a bullet to the existing "## Other
    docs" list pointing to `docs/dogfood-hub.md`. Keep the markdown style consistent with the
    surrounding document (relative links, same heading depth).
  - Follow the project markdown conventions; this card has no runnable test surface.
- **Commit:** `docs(sandbox): document the lyx-test dogfood Hub`

## Batch Tests

`verify: go test ./tools/sandbox/... ./internal/paths/...` covers two scopes, both cheap:

- `./tools/sandbox/...` runs `tools/sandbox/main_test.go` — the unit tests for hub-path
  computation and the existence/reset decision logic (Card 1), compiling the new package so
  any build error in `main.go` is caught.
- `./internal/paths/...` runs `internal/paths/enforcement_test.go`, the tree-wide scan that
  bans `os.Getwd` / `git rev-parse --show-toplevel`. It is included deliberately: this batch
  adds a new non-test `.go` file (`tools/sandbox/main.go`) that the scan covers, so the
  `path-invariant-compliance` Shared Decision is verified by the same command rather than
  only at a later full-suite run.

Card 2 (`sandbox.cmd`) and Card 3 (docs) have no runnable test surface; the launcher is a
trivial shell wrapper and the docs are prose. They are validated by review, not by `verify:`.
