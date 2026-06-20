# Plan: Optimise and slim the test suite

```yaml
task: "Optimise and slim the test suite"
slug: "optimize-test-suite"
approved: false
started: "20260620-182234"
parent: "main"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: lyxtest-foundation
    file: 01-lyxtest-foundation.md
    depends-on: []
    verify: go test -tags integration ./internal/lyxtest/...
  - number: 2
    name: weft-envparam-and-tests
    file: 02-weft-envparam-and-tests.md
    depends-on: [1]
    verify: go test ./internal/weft/... && go test -tags integration ./internal/weft/...
  - number: 3
    name: worktree-envparam-and-tests
    file: 03-worktree-envparam-and-tests.md
    depends-on: [1]
    verify: go test ./internal/worktree/... && go test -tags integration ./internal/worktree/...
  - number: 4
    name: paths-tests
    file: 04-paths-tests.md
    depends-on: [1]
    verify: go test ./internal/paths/... && go test -tags integration ./internal/paths/...
  - number: 5
    name: docs-and-cross-verification
    file: 05-docs-and-cross-verification.md
    depends-on: [2, 3, 4]
    verify: go test ./... && go test -race -tags integration -count=1 ./internal/lyxtest/... ./internal/weft/... ./internal/worktree/... ./internal/paths/...
```

## Shared Decisions

### Decision: build-tag convention

- **Decision:** Gate every git/`cmd`-subprocess-spawning test behind the modern build constraint `//go:build integration` as the first line, **followed by a blank line, then `package …`** (no legacy `// +build` companion). Pure-unit tests (no subprocess) stay untagged so the default `go test ./...` loop is fully offline. Match `internal/board/boardtest/integration_test.go` exactly.
- **Rationale:** Two speed tiers — instant offline default loop, fast on-demand `-tags integration` git suite. Matches the existing repo convention.
- **Applies to:** all batches (which files are tagged is enumerated per batch; the criterion is "spawns a git/`cmd` subprocess on Windows").

### Decision: lyxtest public API (consumed by batches 2–4)

- **Decision:** `internal/lyxtest` is a normal (non-`_test`) package exposing:
  - `func MustRun(tb testing.TB, dir string, args ...string)` — run a command in `dir`, `tb.Fatalf` on non-zero exit (replaces every per-package `mustRun`).
  - `func CopyHostHub(tb testing.TB) HostFixture` — isolated copy of the host-hub template (a git repo with `origin` bound to a copied bare remote). For worktree `list`/`portals`/`launchers`/`cli` tests.
  - `func CopyPaired(tb testing.TB) PairedFixture` — isolated copy of the full paired-Add fixture: host hub + bare origin + weft-prime sibling (`<base>-weft` with `_lyx/config/placeholder`) + weft bare. For worktree `Add`/`Remove`/`weft` tests.
  - `func CopyWeft(tb testing.TB) WeftFixture` — isolated copy of the weft-only template: a weft worktree carrying `_lyx/config.yaml`, with `origin` bound to a copied bare remote **and upstream tracking established** (`branch.main.remote`/`merge` + `refs/remotes/origin/main`). For weft `sync`/`status`/`integration` tests.
  - Fixture structs expose the relevant absolute paths (e.g. `HostFixture{Hub, Bare}`, `PairedFixture{Container, Hub, Bare, WeftPrime, WeftBare, Layout *paths.Layout}`, `WeftFixture{WeftPath, Bare}`). Exact field set is the implementer's call, but the names above are stable references for cards.
- **Rationale:** `mustRun`/`newTestRepo`/`newWeftRepo`/`addRemote`/`addWeftRemote` are duplicated across worktree (white+black box), weft, and paths. One home removes the duplication and centralises the git-fixture logic. Template-built-once + per-test filesystem copy gives zero per-test git spawns and full isolation (parallel-safe).
- **Applies to:** all batches.

### Decision: template-once + per-test filesystem copy

- **Decision:** Each template (host hub, bare, weft prime, weft bare) is built **once per test binary** via `sync.Once` inside `lyxtest`. Each `Copy*` helper does a recursive filesystem copy of the cached template into `tb.TempDir()` (no git spawns) and, for fixtures with a remote, repoints `origin` by **rewriting the single `url = …` line under `[remote "origin"]` in the copied repo's `.git/config` as a text edit** — never `git remote set-url` (that re-introduces a spawn). The template build runs the one-time `git push -u origin main` so upstream tracking survives the copy intact.
- **Rationale:** ~half the suite runtime is identical repeated `init`/`config`/`commit` setup; paying it once and copying directory trees removes that half. Invariant: each template `.git/config` has exactly one `origin` remote / one `url` line in stable formatting.
- **Applies to:** lyxtest-foundation; consumed by all package batches.

### Decision: layered env→option (parallelism enabler)

- **Decision:** Move `os.Getenv("WEFT_SKIP_GIT"/"WEFT_SKIP_PUSH")` reads **out** of the in-process functions into an explicit option, and push the env→option mapping to the call sites at the edge. weft uses `type SyncOptions struct { SkipGit, SkipPush bool }`; worktree uses `type AddOptions struct { SkipGit, SkipPush bool }` (names are the implementer's call but must be a small struct, not loose bools threaded everywhere). Tests pass the option directly (no `t.Setenv`), making `t.Parallel()` legal. The detached-spawn early-return check in `spawn_windows.go`/`spawn_other.go` **keeps** reading env (a param can't cross `exec`).
- **Rationale:** `WEFT_SKIP_*` are load-bearing across the process boundary (the detached `lyx weft … push` child); they cannot be deleted. The layered split gives a clean parallel-safe in-process API while preserving the detached-push architecture.
- **Applies to:** weft-envparam-and-tests, worktree-envparam-and-tests.

### Decision: t.Parallel without t.Setenv/t.Chdir

- **Decision:** Add `t.Parallel()` to every fixture-bearing test once its fixture is an isolated `lyxtest` copy and it no longer calls `t.Setenv`. Tests that must exercise the CLI cwd-resolution entry point (which reads `os.Getwd`) keep `t.Chdir` and stay **serial** (do not add `t.Parallel`) — these are few (`worktree/cli_test.go` `setupCLIRepo`, `worktree/remove_test.go` `TestRemoveSubpathJunction`, `weft/cli_test.go` `TestRunCLI_StatusWithMinimalFixture`). `t.Parallel()` is forbidden after `t.Setenv()` and incompatible with `t.Chdir()`.
- **Rationale:** Parallelism is the lever that takes the gated suite to seconds; the few cwd-bound CLI tests are not worth a cwd-injection seam.
- **Applies to:** all package batches.

### Decision: equivalence guardrail (no coverage loss)

- **Decision:** Before editing a package, capture its test inventory two ways and diff after: (1) `go test -tags integration -list '.*' ./internal/<pkg>/...`; (2) `go test -tags integration -v -run '.*' ./internal/<pkg>/...` and collect the `=== RUN` subtest paths. Save both to uncommitted scratch files (e.g. `.scratch/baseline-<pkg>.txt`), diff pre/post with a scripted `diff`. The post set must be a **superset** of the pre set, modulo intentionally-folded duplicate cases (each fold recorded in the PR description). Scratch files are removed before handoff.
- **Rationale:** the speed win comes from mechanics, not from dropping cases. Table-driving removes duplicate *setup*, not behavioural *cases*.
- **Applies to:** weft/worktree/paths batches (per-package); summarised in batch 5.

### Decision: junctions and fslink are out of scope

- **Decision:** Do not touch `internal/worktree/junction_windows.go` / `junction_other.go` / the `mklink` path / detection in `links.go` / `weft/status.go` `checkJunction`. The cross-OS `internal/fslink` extraction is a separate backlog task (`extract-fslink`).
- **Rationale:** A complete fslink migration is substantial standalone work; a partial one would leave detection logic hand-rolled in two places.
- **Applies to:** all batches.

## All Files Touched

- `docs/benchmarks/test-suite-timing.md`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/lyxtest.go`
- `internal/lyxtest/lyxtest_test.go`
- `internal/paths/paths_test.go`
- `internal/paths/worktreelist_test.go`
- `internal/weft/cli.go`
- `internal/weft/cli_test.go`
- `internal/weft/status_test.go`
- `internal/weft/sync.go`
- `internal/weft/sync_test.go`
- `internal/weft/weft_integration_test.go`
- `internal/worktree/add.go`
- `internal/worktree/add_test.go`
- `internal/worktree/cli.go`
- `internal/worktree/cli_test.go`
- `internal/worktree/junction_test.go`
- `internal/worktree/launchers_test.go`
- `internal/worktree/list_test.go`
- `internal/worktree/portals_test.go`
- `internal/worktree/remove_test.go`
- `internal/worktree/weft.go`
- `internal/worktree/weft_test.go`
