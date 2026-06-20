# Discussion: Optimise and slim the test suite

```yaml
task: Optimise and slim the test suite
slug: optimize-test-suite
status: discussing
parent: main
```

## Problem

The non-test integration suites under `internal/worktree`, `internal/weft`, and
`internal/paths` are real-subprocess, git-backed tests that run **serially** on
Windows and are unacceptably slow: a cold run measured **worktree 234s, weft 94s,
paths 33s** (~6 min for three packages). On Windows each subprocess spawn is
~50–200 ms (`cmd /c` worse), and the suite spawns thousands of them.

Two costs compound, roughly 50/50:

1. **Per-test setup.** Each test rebuilds a full git fixture from scratch —
   `newTestRepo`/`newTestWeftRepo` = 5 spawns (`init` + 2×`config` + `add` +
   `commit`), `addRemote`/`addWeftRemote` = 2–3 more, plus a weft-prime sibling =
   5 more. A full "paired Add" fixture is **~12 git spawns before the test body
   even runs**, and that identical setup is repeated ~20× in worktree, ~12× in
   weft, ~8× in paths.
2. **Production code under test.** `Add` + its weft path spawn ~10–12 git calls
   (`worktree add`, `branch`, junction via `cmd /c mklink /J`, …); `Remove` ~5;
   `Sync`/`Status` 3–6. These cannot be avoided — the tests exist to exercise
   real git/junction behaviour.

**Why now:** the everyday `go test ./...` loop should be seconds (like a
well-built C# suite with hundreds of tests), not minutes. The goal is to make the
default loop instant and the full git suite fast, without losing meaningful
coverage of real git/junction behaviour.

## Scope

**In:**

- A new shared test-support package **`internal/lyxtest`** that owns the
  git-fixture machinery: build the heavy template repos **once per test binary**,
  and hand each test an **isolated filesystem copy** (no git spawns per test).
- Migrate `internal/worktree`, `internal/weft`, `internal/paths` test fixtures to
  `lyxtest`, removing the duplicated `mustRun`/`newTestRepo`/`addRemote` helpers
  and the dead `addWeftRemote` helper.
- **Parallelism**: enable `t.Parallel()` on the git/subprocess tests. This
  requires removing the process-global blockers (`t.Setenv`, `t.Chdir`) from
  parallelizable tests via a **layered env→param refactor** of the in-process
  weft sync functions (see Decisions).
- **Build-tag gating**: gate the git/subprocess tests behind `//go:build
  integration` (following the `internal/board/boardtest` precedent) so default
  `go test ./...` is a fast, offline, pure-unit loop and the git suite runs with
  `-tags integration`.
- **Conservative pruning**: consolidate obviously-overlapping fixtures into
  table-driven tests and remove dead/duplicated helpers. Keep all *distinct*
  behaviour coverage.
- Capture before/after wall-clock timings for the three packages (PR description).

**Out:**

- **`internal/fslink` / junction syscall (proposal item #5) and cross-OS/Linux
  support.** The `cmd /c mklink /J` junction code in `junction_windows.go` is left
  **untouched**. This was split into a dedicated backlog task **`extract-fslink`**
  (a complete extraction migrating all call sites + a direct reparse-point
  syscall is substantial standalone work; a partial one would leave detection
  logic hand-rolled in two places).
- `internal/board` / `boardtest` — already follows the target pattern; not
  modified beyond serving as the precedent.
- Changing what git operations are tested, or replacing real git with a library
  (`go-git`). We keep real-subprocess git behaviour; we only amortise and
  parallelise it.
- No new runtime feature; no change to the public CLI behaviour. The env vars
  (`WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`) remain valid at the process boundary.

## Decisions

### fixture-amortisation: template-once + per-test copy

- Decision: Build the expensive template repos (host hub, bare remote, weft
  prime, weft bare) **once per test binary**, cached in `internal/lyxtest`
  (`sync.Once`-guarded). Each test gets an isolated working copy via a cheap
  filesystem copy into `t.TempDir()` — **zero git spawns at the per-test level**.
  For tests needing a remote, the bare remote is **copied per test** into the
  test's tempdir and `origin` is repointed at the copy by **rewriting the
  `[remote "origin"] url` line in the copied repo's `.git/config` as a text
  edit** (no git spawn). We explicitly do NOT use `git remote set-url`, which
  would reintroduce a per-test spawn and undercut the zero-spawn goal.
- Rationale: ~half the runtime is identical repeated `init`/`config`/`commit`
  setup. Paying it once and copying directory trees (milliseconds, no subprocess)
  removes that half entirely while keeping each test fully isolated and
  parallelizable. Filesystem copy preserves real git repo state.
- Rejected:
  - *One shared long-lived repo + subtests by slug* (the operator's first
    instinct) — simplest, but serial-only and risks cross-subtest contamination
    when a test mutates shared repo state.
  - *Build once + `git reset --hard`/`git clean` between tests* — brittle on
    Windows (junctions/portals live outside the repo) and the reset itself spawns
    git.

### parallelism via layered env→param

- Decision: Move the env read **out** of the in-process functions and into an
  explicit option, then push the env→option mapping to the call sites at the edge.
  Concretely:
  - **Functions that lose their `os.Getenv` and gain an explicit option**
    (`skipGit`/`skipPush`, e.g. a small `opts` struct or two bools threaded as
    parameters):
    - `Commit`, `Push`, `Pull` in `internal/weft/sync.go` (reads at ~lines 34,
      83, 120).
    - `pushWeftBranch` in `internal/worktree/weft.go:208` — the Add-path env
      reader. **Note: this is NOT a `Commit`/`Push`/`Pull` function**; it is the
      weft-branch push step invoked during `Add`. Its new signature takes the
      `skipPush`/`skipGit` option explicitly.
  - **Call sites that gain a NEW env→option read** (they have none today — this is
    new code, not a "keep"):
    - `internal/weft/cli.go` — the CLI dispatcher calls `Commit`/`Push`/`Pull`
      (current call sites ~lines 66, 106, 113, 117, 123, 129) with no env read at
      all today. Each gains an `os.Getenv("WEFT_SKIP_GIT")`/`WEFT_SKIP_PUSH`
      read that it maps to the option. This is where the process-boundary env
      contract is honoured for the real CLI path (including the detached child,
      which runs `lyx weft … push` and therefore goes through cli.go).
    - `Add` in `internal/worktree/add.go` — maps env→option when it calls
      `pushWeftBranch`, so the paired-Add tests can pass `skipPush` explicitly
      without `t.Setenv`.
  - Tests call the in-process functions / `Add` with the option passed directly —
    **no `t.Setenv`** — so `t.Parallel()` becomes legal.
  - The detached-spawn early-return check in `spawn_windows.go` (~line 28) /
    `spawn_other.go` (~line 23) **keeps reading the env vars** (it decides at spawn
    time whether to fork the child at all); a function parameter cannot cross the
    `exec` boundary, so env stays the channel there.
- Rationale: `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` are load-bearing across the
  process boundary (the detached `lyx weft … push` child reads them to decide
  whether to skip). They cannot simply be deleted. The layered approach gives a
  clean, parallel-safe in-process API while preserving the detached-push
  architecture. It also addresses the proposal's Principle-4 smell.
- Rejected:
  - *Full env removal* — breaks the detached child, which has no other channel.
  - *Process-global env set once in `TestMain` + serial isolation of env-divergent
    tests* — no production change, but hacky and limits which tests can
    parallelise.

### t.Chdir handling

- Decision: Prefer passing cwd explicitly where the code allows it (e.g.
  `paths.Resolve(cwd)` already takes cwd, so paths tests need no chdir). Tests
  that exercise the CLI entry point, which resolves cwd via `os.Getwd`
  (`worktree/cli.go`, `weft/cli.go`), may legitimately keep `t.Chdir` and stay
  **serial** — they are few. `t.Chdir` is incompatible with `t.Parallel()`, so
  these are explicitly excluded from parallelisation rather than contorted.
- Rationale: avoids over-engineering the CLI seam for a handful of tests while
  still parallelising the bulk (fixture-bearing `Add`/`Remove`/`Status`/`Sync`
  tests).
- Rejected: forcing a cwd-injection seam through the whole CLI router just to
  parallelise 3–4 router tests — not worth the churn.

### build-tag gating

- Decision: Gate git/subprocess-spawning tests behind `//go:build integration`
  (modern form only, blank line before `package`, matching
  `boardtest/integration_test.go`). Pure-unit tests (config parsing, geometry/
  `Layout` computation, `createJunction` logic that doesn't spawn, link bitmask
  logic, prune logic, static guard tests) stay **untagged** and run in the
  default `go test ./...`. Because the white-box tests access unexported symbols
  (`rollbackAdd`, `seedLyxJunction`, `scopedPathspec`, `createJunction`), the
  tagged tests stay **in their own package, split by file** — we do NOT move them
  to a black-box sibling package the way `boardtest` does.
- Rationale: two speed tiers — instant offline default loop, fast on-demand git
  suite. Matches the existing repo convention.
- Rejected: a single untagged suite (default loop always pays git cost, even if
  fast); a black-box sibling package (impossible for the unexported-symbol tests).

### shared lyxtest package

- Decision: One normal (non-`_test`) package `internal/lyxtest` exposing the
  template builders (cached) + a copy helper returning an isolated working copy,
  plus the shared `mustRun` driver. Imported by all three packages' tests.
- Rationale: `mustRun`/`newTestRepo`/`addRemote` are currently duplicated (two
  copies in worktree across white/black-box packages; separate copies in weft and
  paths). One home removes the duplication and centralises the git-fixture logic.
- Rejected: per-package `TestMain` + local helpers — keeps packages independent
  but retains the duplication the operator wants gone.

### conservative pruning

- Decision: Consolidate, don't cut behaviour. Concretely: remove the dead
  `addWeftRemote` (worktree `testhelpers_test.go`); fold the `TestAdd` precondition
  subtests (DirtySource / BranchExists / TargetDirExists / NoRemote / NoWeftRepo)
  and the `TestRemove` dirty-gate variants into table-driven cases built on a
  single shared base fixture + per-case delta; similarly table-drive the weft
  `Status_Junction*` and `Commit_*`/`Push_*` families where they share setup. Keep
  every distinct behavioural assertion.
- Rationale: the speed win comes from mechanics (amortise + parallelise), not from
  deleting coverage. Table-driving removes duplicate *setup*, not *cases*.
- Rejected: aggressive cut to a target count (accepts coverage loss); no pruning
  at all (leaves duplication and dead code).

## Technical context

Key files and facts mill-plan needs:

- **Central git driver:** `internal/git` (`git.RunGit([]string{...}, dir)`) is the
  production wrapper for all git calls. Tests use their own `mustRun` closure
  (`exec.Command` + `CombinedOutput` + `t.Fatalf`).
- **Board precedent:** `internal/board/boardtest/` — `//go:build integration` in
  `integration_test.go` and `bench_git_test.go` (modern tag, blank line before
  `package`, no legacy `// +build`). No `Makefile`/CI in repo; the convention is
  documented in `boardtest/doc.go` and `docs/benchmarks/board-performance.md`
  (e.g. `go test -tags integration … ./internal/board/boardtest`). Note: board
  re-clones per test — it does **not** demonstrate the template-once-shared
  fixture; we are introducing that pattern.
- **worktree fixtures** (`internal/worktree/`): two test packages coexist —
  white-box `package worktree` (`testhelpers_test.go`, `add_test.go`,
  `junction_test.go`, `launchers_test.go`, `links_test.go`, `portals_test.go`,
  `prune_test.go`, `remove_test.go`, `weft_test.go`) and black-box
  `package worktree_test` (`helpers_test.go`, `cli_test.go`, `config_test.go`,
  `list_test.go`). Helpers: `newTestRepo` (5 spawns), `addRemote` (2),
  `newWeftRepo` (5), `addWeftRemote` (2, **unused — delete**). Full paired-Add
  fixture (`newTestRepo`+`addRemote`+`newWeftRepo`+`WEFT_SKIP_PUSH=1`) repeats
  ~20×. `t.Setenv("WEFT_SKIP_PUSH","1")` is pervasive; `t.Chdir` only in
  `cli_test.go`'s `setupCLIRepo` and `remove_test.go`'s `TestRemoveSubpathJunction`.
- **weft fixtures** (`internal/weft/`): helpers in `sync_test.go` —
  `newTestWeftRepo` (5 spawns; writes `_lyx/config.yaml`), `addWeftRemote` (3
  spawns, **includes a real `git push -u`**). Env seams `WEFT_SKIP_GIT` (cli/sync)
  and `WEFT_SKIP_PUSH` (sync). `TestRunCLI_StatusWithMinimalFixture` rolls its own
  2-repo (host + `<base>-weft` sibling) fixture inline (~10 spawns) and uses
  `t.Chdir` — re-express via a "hub+weft pair" lyxtest fixture. The only
  `cmd /c mklink /J` in *test* code is `TestStatus_JunctionOk_Windows` (skippable
  via `SKIP_MKLINK_TEST=1`).
- **paths fixtures** (`internal/paths/`): external `paths_test` uses `newTestRepo`
  (5 spawns); `paths.Resolve(cwd)` itself spawns `git rev-parse --show-toplevel`
  per call (`TestMirroredMethods` triggers ~13). `weft_test.go` and the guard
  tests (`codeguide_guard_test.go`, `enforcement_test.go`) do **no** git/IO — they
  stay untagged. `worktreelist_test.go`'s `BareRepoRejection` needs a bare repo.
- **Production env reads to move out into an option:** `internal/weft/sync.go`
  (`Commit`/`Push`/`Pull`, lines ~34, ~83, ~120) and `internal/worktree/weft.go`
  (`pushWeftBranch`, ~208 — the Add-path push step, not a sync function).
- **Call sites that gain a NEW env→option read** (none today): `internal/weft/cli.go`
  (~lines 66, 106, 113, 117, 123, 129, where it currently calls Commit/Push/Pull
  with no env read) and `internal/worktree/add.go` (`Add`, where it calls
  `pushWeftBranch`). This is new code — the discussion does **not** treat the CLI
  as merely "keeping" an existing read.
- **Keep env unchanged at the spawn boundary:** `internal/weft/spawn_windows.go`
  (~28), `spawn_other.go` (~23) — the spawn-time early-return check. Board's
  `BOARD_SKIP_*` is the analogous in-function pattern — do not touch board.
- **Junctions (do NOT change):** `internal/worktree/junction_windows.go`
  (`cmd /c mklink /J`), `junction_other.go` (`os.Symlink`), callers `portals.go`
  and `weft.go`. Detection in `links.go` and `weft/status.go` (`checkJunction`).
  All deferred to the `extract-fslink` task.

## Constraints

- No `CONSTRAINTS.md` at the hub root.
- **Windows is the primary platform**; junctions require the no-privilege
  `mklink /J` path (kept as-is). Symlink-based tests already skip when symlink
  creation lacks privilege (`SKIP_SYMLINK_TEST`).
- **Go test rules that shape the design:** `t.Parallel()` is forbidden after
  `t.Setenv()` and is incompatible with `t.Chdir()`. Both must be removed from any
  test that opts into parallelism.
- Real git must remain on `PATH` for the integration suite; the default untagged
  loop must be fully offline and subprocess-free.
- Keep the env-var contract (`WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`) intact at the
  process boundary — the detached child depends on it.

## Testing

This task *is* test work; the "testing approach" is the migration strategy plus
the guardrails that prove we didn't lose coverage.

- **Equivalence guardrail:** before refactoring, capture the baseline two ways
  and diff pre/post:
  1. Top-level functions: `go test -tags integration -list '.*'
     ./internal/worktree/... ./internal/weft/... ./internal/paths/...` (snapshot
     the printed test-name list).
  2. Subtest (`t.Run`) leaves, which `-list` does NOT show: capture
     `go test -tags integration -v -run '.*' ./internal/{worktree,weft,paths}/...`
     and grep the `=== RUN`/`--- PASS` lines for the full subtest path set.
  After refactoring, diff both snapshots — every distinct behavioural case (incl.
  the new table-driven `t.Run` leaves) must still be present. No silent coverage
  loss.
- **`internal/lyxtest` (TDD candidate):** the copy helper and template builders
  are themselves testable — assert that a copied repo is a valid, independent git
  repo (HEAD resolves, origin rewritten, mutating one copy doesn't affect
  another). Build this first; the package migrations depend on it.
- **env→param refactor (TDD candidate):** add/keep direct in-process tests for
  `Commit`/`Push`/`Pull` with `skipGit`/`skipPush` passed explicitly (replacing
  the `t.Setenv` variants `TestCommit_SkipGit`, `TestPush_SkipGit`,
  `TestPush_SkipPush`). Add a test that the CLI/spawn boundary still honours the
  env var (maps env → option).
- **Parallelisation:** after isolating fixtures, add `t.Parallel()` to the
  fixture-bearing tests; verify the suite passes under `go test -race
  -tags integration` and under `-count=2` (catches shared-state leaks). Leave
  `t.Chdir`-based CLI router tests serial.
- **Build-tag verification:** `go test ./...` (no tag) must compile and pass with
  **zero git subprocesses** (fast, offline); `go test -tags integration ./...`
  runs the full git suite. Verify both.
- **Coverage scenarios that MUST remain** (representative, not exhaustive): paired
  Add happy path + each precondition failure + rollback-leaves-no-residue; Remove
  paired teardown + dirty gates (host & weft, with/without force) + nested-subpath
  junction removal; weft Status branch/dirty/junction-integrity (missing / plain
  dir / valid symlink / valid junction); weft Commit pathspec scoping / clean-tree
  no-op / skip-git no-op; weft Push/Pull success + skip variants + bare-remote
  integration; paths Resolve from root/subdir + geometry + not-a-git-repo + bare
  rejection + worktree-list parsing.
- **Success bar:** default `go test ./...` < ~5s; full `-tags integration` suite
  < ~45s (from ~360s). Capture before/after wall-clock for worktree/weft/paths in
  the PR description.

## Q&A log

- **Q:** How far to push — setup-once only, or also parallelise? **A:** Full
  package: template-once + per-test copy **and** parallelism, to reach "seconds"
  like a C# suite. Setup-once alone is only ~2× because production code is ~half
  the cost.
- **Q:** Fixture-sharing model? **A:** Template built once + isolated filesystem
  copy per test (not one shared long-lived repo, not git-reset-between).
- **Q:** Parallelism vs the env seams? **A:** Layered — in-process
  `Commit`/`Push`/`Pull` (and Add-path `pushWeftBranch`) take an explicit option;
  the env→option read moves to the edge: `cli.go` and `Add` gain a NEW read
  (they have none today), and the detached-spawn early-return keeps reading env
  (a param can't cross `exec`).
- **Q:** Gate behind `//go:build integration`? **A:** Yes — default loop becomes
  pure-unit/instant; git suite runs with `-tags integration`.
- **Q:** Pruning appetite? **A:** Conservative — consolidate overlapping fixtures
  into table-driven tests, delete dead/duplicated helpers, keep all distinct
  behaviour coverage.
- **Q:** Helper location? **A:** One shared `internal/lyxtest` package, not
  per-package helpers.
- **Q:** Junction syscall / `internal/fslink` / cross-OS in scope? **A:** No —
  deferred to a dedicated `extract-fslink` backlog task (added via mill-add). A
  complete migration is substantial standalone work; a partial one would leave
  detection logic hand-rolled in two places.
- **Q:** Success metric? **A:** default `go test ./...` < ~5s; `-tags integration`
  < ~45s; record before/after timings.
