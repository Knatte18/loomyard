# Discussion: Speed up and stabilize the integration test tier

```yaml
task: Speed up and stabilize the integration test tier
slug: optimize-integration-tier
status: discussing
parent: main
```

## Problem

The opt-in integration tier (`go test -tags integration ./...`, "Tier 2") is slow and
run-to-run noisy. The default offline loop (Tier 1, `go test ./...`) is already fast
(~3.5s) and is **out of scope** — this task is only about Tier 2.

Two prior tasks (`optimize-test-suite`, `optimize-remaining-test-suites`, both
2026-06-22) already did most of the historical speedup, so the **task brief is working
from stale numbers** (it cites ~70-86s and a boardtest floor of ~82s). Current measured
reality on this machine (Windows, 14 logical CPUs, warm builds, `-count=1`):

| Package | warm wall-clock | note |
|---|---|---|
| Tier 2 overall | ~42s | bounded by its slowest package |
| `internal/board/boardtest` | ~42s (the floor) | ~26s of that is **local** git tests run serially; the rest is the real-GitHub network test + noise |
| `internal/worktree` | ~32s | already parallel; bound by Windows filesystem I/O, not git logic |
| `internal/weft` | ~15s | already parallel; below the floor |

Two distinct goals: **stabilize** (kill the run-to-run noise, whose source is the one
real-GitHub network round-trip) and **speed up** (move the wall-clock floor down). The
diagnosis below shows the real levers differ from the brief's.

**Why now:** the integration tier is the gate developers run before touching
worktree/weft/board/ide git behaviour; noise and slowness make it painful enough to
skip, which erodes its value.

## Scope

**In:**

- **A. Remove the real-GitHub tests entirely.** Delete `TestIntegrationCommitPush` and
  `TestIntegrationPull` (`internal/board/boardtest/integration_test.go`) and the
  network benchmarks that share their remote (`bench_git_test.go`). After this, the
  whole repo's `-tags integration` run is **all-local git** — no network, no GitHub
  round-trip, deterministic.
- **B. Parallelize boardtest's local git tests.** Replace the process-global
  `BOARD_SKIP_GIT` / `BOARD_SKIP_PUSH` env seams (which force the tests serial, because
  `t.Setenv` forbids `t.Parallel()`) with explicit flags threaded through the `board`
  API, env retained as fallback. Then the local tests run in parallel.
- **C. Trim wasted fixture I/O in worktree tests.** `Add` always pushes the **host**
  branch (add.go:172, unconditional); `SkipPush`/`SkipGit` gate only the **weft** push
  (add.go:182-183 → `pushWeftBranch`). So for `AddOptions{SkipPush:true}` tests the
  host bare is a live push target and must stay, but the **weft-bare** is never reached
  and can be dropped. Add a lean `CopyPaired` variant that copies hub + bare +
  weft-prime but **omits the weft-bare** (cutting one of four copied repos), for the
  SkipPush worktree/weft tests. This is a smaller win than first estimated (~25% of the
  per-test copy, not half) but still removes real Windows file-copy + Defender cost.
- **D. Record a fresh timing block** in `docs/benchmarks/test-suite-timing.md`
  (median of several warm runs).

**Out:**

- Tier 1 (the offline loop) — already fast, not touched.
- **No `network` build tag.** The earlier plan to gate the network test behind
  `integration && network` is rejected in favour of deleting it (see Decision A).
- The serial `RunCLI` tests in `internal/worktree` (`cli_test.go`) and `internal/weft`
  (`weft_integration_test.go`) — they use `t.Chdir` / `t.Setenv` and stay serial by
  design (see Decision E). ~2s total, below the floor.
- `internal/worktree` and `internal/weft` are **already parallelized** at the test
  level (the brief's "secondary lever" of adding `t.Parallel()` to
  `TestRemoveHostJunctionRemoved` / `TestAddRollback` / `TestWeftSpawnPairedWorktrees`
  is already done — those tests already call `t.Parallel()`). The only new lever there
  is the fixture trim (C), not test-level parallelization.
- No hard wall-clock target — "cheap wins, then measure" (Decision F).
- No production behaviour change. `cmd/lyx board sync` and all CLI output are unchanged.

## Decisions

### A. Delete the real-GitHub tests rather than gate them

- Decision: Remove `internal/board/boardtest/integration_test.go` (the two
  `TestIntegration*` tests) and `internal/board/boardtest/bench_git_test.go` (the
  `BenchmarkSyncGit*` benchmarks that clone the same real remote). Delete the
  `testRepoURL` constant. No `network` build tag is introduced.
- Rationale: The two network tests push throwaway commits to a real GitHub repo
  (`Knatte18/loomyard-test`) and are the sole source of Tier 2's run-to-run noise
  (documented: a warm re-run measured *slower* than a cold run — network variance, not
  compute). Their logic is **already covered locally**: `git_test.go:TestCommitPush`
  exercises `board.CommitPush` (commit, push to a local bare, and a non-fast-forward
  rebase-retry subtest) and `git_test.go:TestPull` exercises `board.Pull` — both via
  `lyxtest` local bare fixtures. The network tests add network dependency, not unique
  coverage. The benchmarks `bench_git_test.go` exist specifically to *measure the real
  network push cost*; repointing them to a local bare would make them measure something
  else and be misleading, so they are deleted rather than repurposed. The local
  board-logic benchmarks in `bench_test.go` (which use `BOARD_SKIP_GIT=1`, no network)
  are kept.
- Rejected:
  - *Gate behind `integration && network`* (the brief's plan): keeps a flaky
    real-remote test alive for a "rare check" that duplicates local coverage, and adds a
    build-tag dimension plus a `cmd/testtiming -network` flag for little value.
  - *Repoint benchmarks to a local bare*: keeps a "network push benchmark" that no
    longer touches the network — dishonest naming.

### B. Parallelize boardtest local git tests via explicit skip-flags (env as fallback)

- Decision: Add `SkipGit` and `SkipPush` to `board.Config`; the `Board` facade retains
  them and honours them in `writeOp` (board.go:83) and `Board.Sync()` (board.go:172).
  For the package-level functions that have no `Config` — `board.Sync(boardPath)`
  (sync.go), `board.CommitPush(boardPath, …)` (git.go) — add a functional-options seam
  (e.g. `board.WithSkipPush()`, `board.WithSkipGit()`). At each consumption site
  (board.go:83, sync.go:32, sync.go:103, git.go:69) an **explicit flag takes precedence
  over the env var**: consult env only when the caller passed no explicit value. For the
  package-func options seam use a tri-state (e.g. `*bool` or an option-was-passed marker)
  so `WithSkipPush(false)` genuinely overrides an ambient `BOARD_SKIP_PUSH=1`; for the
  `Board`/`Config` path, production resolves env→`Config` at the entry point
  (`cmd/lyx board sync`) so internal sites read the resolved flag only. This keeps env a
  true *fallback* rather than an ambient override that leaks into parallel tests (see the
  ambient-leakage note in Testing). Then every boardtest local test (`git_test.go`,
  `sync_test.go`, `concurrency_test.go`) drops its `t.Setenv(...)` calls, passes the flag
  explicitly via config/options, and adds `t.Parallel()`.
- Rationale: The local tests are individually fast but run serially purely because
  `t.Setenv` forbids `t.Parallel()`; their sum (~26s) is the package's real wall-clock.
  Each test already uses `t.TempDir` + isolated `lyxtest` fixtures, so the only shared
  mutable state is the process-global env — once that's gone, they are parallel-safe.
  Keeping env as a fallback preserves the operational override and means **no production
  caller changes** (no production code currently *sets* these vars; `cmd/lyx board sync`
  calls package `Sync` with no options and falls through to the env check, unchanged).
- Rejected:
  - *Drop the env seams entirely*: broader production surface; the user chose
    config-fields-with-env-fallback.
  - *Convert all package funcs to methods on `Board`*: larger churn than threading a
    small options seam; `Pull` needs no skip flag at all (it only does `pull --ff-only`).

### C. Trim the weft-bare copy from the paired fixture for non-pushing tests

- Decision: Add a lean variant to `internal/lyxtest` (an option on `CopyPaired`, or a
  sibling builder like `CopyPairedLocal`) that copies hub + **host** bare + weft-prime
  but **omits the weft-bare**. All current `Add` tests pass `SkipPush:true` (verified: 12
  call sites in add_test.go/weft_test.go/remove_test.go, zero with the weft push
  enabled), so they all switch to the lean variant. **There is no existing weft-pushing
  test to "keep" on the full fixture** — the weft push path (`pushWeftBranch`) is
  currently uncovered. To keep the full `CopyPaired` exercised and to close that
  pre-existing gap, the plan adds **one** new weft-pushing test (`AddOptions{}` — neither
  skip set) on the full fixture, asserting the weft branch lands on the weft-bare. The
  full (all-four-repos) builder stays the default so nothing silently changes.
- Rationale: `worktree`'s ~32s is not git logic (`git worktree add` is ~100ms; a paired
  `Add` is ~10 git spawns ≈ 0.3s). A single paired-Add test is ~4.4s alone and balloons
  to ~10s under parallel load — it is **filesystem-I/O-bound**: `CopyPaired` copies four
  full git repos (hub + host-bare + weft-prime + weft-bare) per test, every small file
  scanned by Windows Defender, plus `t.TempDir` teardown. **Correction (review r1):**
  `Add` pushes the *host* branch unconditionally (add.go:172, step 13), so the host bare
  is a live target and cannot be dropped; only the weft push is gated by `SkipPush`
  (add.go:182-183), so only the **weft-bare** is dead weight for SkipPush tests.
  Dropping it cuts one of four copied repos (~25% of the per-test copy), a smaller but
  real reduction. (`git worktree add -b` does not touch any remote; `git remote`,
  add.go:107, only reads config.)
- Rejected:
  - *Drop both bare repos*: breaks the unconditional host push — SkipPush tests assert
    `err==nil` and would fail (verified: `add_test.go`/`weft_test.go`/`remove_test.go`
    happy paths). Only the weft-bare is safe to drop.
  - *Leave worktree at ~32s* (scope option 2): would leave worktree as the new floor
    once boardtest is parallelized, capping the speedup. User chose to attack it,
    accepting that the corrected payoff is modest.
  - *Disable Defender / environmental fix*: not a code change, not portable, out of our
    control.
  - *Repack/shrink the template `.git`*: smaller payoff, more complexity.

### D. Equivalence guardrail = justified subset, not strict superset

- Decision: The post-change top-level test-name set is a **subset** of the pre-change
  set (the two `TestIntegration*` names are removed), documented as a justified drop in
  the timing-doc history block, with each removed name mapped to its covering local
  test: `TestIntegrationCommitPush` → `git_test.go:TestCommitPush`,
  `TestIntegrationPull` → `git_test.go:TestPull`. No *other* test name is dropped; B and
  C add `t.Parallel()` and swap fixtures/seams without renaming or removing tests, so
  the rest of the set is preserved (verify with a `-list` + `=== RUN` diff).
- Rationale: This mirrors the relaxation already adopted by `prune-board-tests`
  (2026-06-22): subset-with-coverage-justification rather than strict superset. Deleting
  the network tests is the only intentional name drop.
- Rejected: *Strict superset* — impossible once the network tests are deleted.

### E. RunCLI tests stay serial

- Decision: Do not touch `internal/worktree/cli_test.go:TestRunCLI` or
  `internal/weft/weft_integration_test.go:TestRunCLI_EnvMapToOption`. They keep
  `t.Chdir` / `t.Setenv` and remain serial.
- Rationale: ~2s combined, well under the floor. De-serializing means refactoring
  `RunCLI` to take `cwd` as a parameter instead of `os.Chdir` — production-code churn in
  the CLI for marginal time.
- Rejected: *Refactor away `os.Chdir`* — payoff far too small for the surface.

### F. Acceptance = cheap wins, then measure (no hard target)

- Decision: Do A+B+C, then record a fresh median-of-several-warm-runs timing block. No
  fixed wall-clock number to hit; workstream C stops when the gain flattens.
- Rationale: Tier 2 is noisy; a single run is not truth, and a hard target would push C
  into diminishing-returns territory (fighting Windows I/O) for little benefit.

## Technical context

- **Two tiers** are documented in `docs/benchmarks/running-tests.md` and
  `docs/benchmarks/test-suite-timing.md`. Tier 2 = Tier 1 + the `//go:build integration`
  tests. `go test` runs packages in parallel, so Tier 2 wall-clock = its slowest
  package. Reproduce numbers with `go run ./cmd/testtiming -full` (adds
  `-tags integration`).

- **boardtest network surface (workstream A):**
  - `internal/board/boardtest/integration_test.go` — defines `const testRepoURL =
    "https://github.com/Knatte18/loomyard-test.git"` and the two `TestIntegration*`
    tests. Delete the file.
  - `internal/board/boardtest/bench_git_test.go` — `BenchmarkSyncGit` /
    `BenchmarkSyncGitNoPush`, both clone `testRepoURL`. Delete the file (it is the only
    other `testRepoURL` user; confirmed by grep). After deletion `testRepoURL` no longer
    exists and the package compiles under plain `-tags integration`.
  - Repo-wide check confirmed **no other test** references a real `https://github…`
    remote — every other integration test uses a local bare repo in `t.TempDir`.

- **board skip-seam consumption sites (workstream B):**
  - `board.go:83` — `os.Getenv("BOARD_SKIP_GIT")` gates the detached `lyx board sync`
    spawn inside `writeOp`.
  - `sync.go:32` — `BOARD_SKIP_GIT` short-circuits package `Sync(boardPath)`.
  - `sync.go:103` — `BOARD_SKIP_PUSH` short-circuits `pushUnpushed`.
  - `git.go:69` — `BOARD_SKIP_PUSH` short-circuits the push in `CommitPush`.
  - `Board` (board.go:24) stores only `boardPath` + `out`; `New(cfg)` (board.go:30)
    currently drops the rest of `cfg`. `Config` (config.go:18) has `Path`, `Home`,
    `Sidebar`, `ProposalPrefix` — add `SkipGit`/`SkipPush` here.
  - `(b *Board) Sync()` (board.go:172) just delegates to package `Sync(b.boardPath)`,
    which reads env directly (sync.go:32, sync.go:103) — there is **no Config seam there
    today**. The plan must give package `Sync` (and `CommitPush`) a new options signature
    so `(b *Board) Sync()` can pass its resolved `SkipGit`/`SkipPush` in. The production
    path `RunCLI` (cli.go:263, the `sync` case → `b.Sync()`) resolves env→options **once**
    at that entry point, so the internal `Sync`/`pushUnpushed` sites read the resolved
    options, not env.
  - Test callers: `sync_test.go` uses `board.New(cfg).Sync()` and `w.Sync()` →
    config-based. `git_test.go` calls `board.CommitPush(path, …)` and `board.Pull(path)`
    directly → needs the options seam (`Pull` needs none). `concurrency_test.go` uses
    `board.New(cfg)` writes with `BOARD_SKIP_GIT=1` → `cfg.SkipGit = true`.
  - **`concurrency_test.go` has no `//go:build integration` tag** — it is a *Tier 1*
    test (its `BOARD_SKIP_GIT=1` runs the no-git path). Converting it (env→`cfg.SkipGit`)
    is still in scope for consistency and parallel-safety, but it does **not** affect
    Tier 2 wall-clock; do not count it toward the speedup.

- **Fixtures (workstream C):** `internal/lyxtest/lyxtest.go`. Templates are built once
  per test binary via `sync.Once` (`buildHostHub`, `buildWeftPrime`, `buildWeftOnly`);
  each `Copy*` does a `copyDirRecursive` of the template(s) into `tb.TempDir()` (pure
  filesystem, **zero per-test git spawns** — a deliberate invariant) then rewrites the
  origin URL via `rewriteOriginURLInConfig` (text edit, no spawn). `CopyPaired` copies
  hub + host-bare + weft-prime + weft-bare and returns a `PairedFixture` (lyxtest.go:324)
  with `Hub`, `Bare`, `WeftPrime`, `WeftBare`, `Layout`. The lean variant omits **only
  the weft-bare** (the host bare must stay — `Add` pushes the host branch unconditionally,
  add.go:172) and returns the same `PairedFixture` with **`WeftBare == ""`** and the
  **weft-prime's origin URL left unrewritten** (it still points at the shared template
  weft-bare, but is never reached because the weft push is suppressed). This is safe:
  verified that no `SkipPush:true` test reads `f.WeftBare` (grep: zero `.WeftBare`
  references in `*_test.go`), and the lean variant must not be used by the new
  weft-pushing test (which uses full `CopyPaired`). Consumers in
  `internal/worktree/weft_test.go`, `add_test.go`, `remove_test.go`, etc. call
  `Add(..., AddOptions{SkipPush:true})` — the weft push (add.go:182-183 →
  `pushWeftBranch`) is the only push `SkipPush` suppresses.

- **Timing harness:** `cmd/testtiming` (`-full` adds `-tags integration`). No
  `network` flag is needed (Decision A). No CI exists (`.github/workflows` absent) — the
  integration tier is manual/local only.

## Constraints

- **Behaviour-preserving:** no production code path changes observable behaviour. The
  env-fallback in B specifically guarantees `cmd/lyx board sync` is unaffected.
- **Windows reality:** measurements are noisy (file I/O + Defender + ~30ms process tax
  per git spawn). Always measure warm, `-count=1`, median of several runs.
- **lyxtest invariant:** fixtures must not introduce per-test git spawns (template-once
  + filesystem-copy). The lean variant must stay a pure file copy.
- **fslink:** worktree junction handling goes through `internal/fslink` (directory
  junctions on Windows); the fixture trim must not change link semantics — it only drops
  the unused bare-repo copies.

## Testing

This task *is* test-suite work; the "tests" here are the integration tests being made
faster, plus a verification that nothing regressed.

- **Workstream A (delete network tests):** after deleting the two files, confirm
  `go test -tags integration ./internal/board/boardtest -count=1` builds and passes
  (proves `testRepoURL` has no remaining referent), and confirm the package no longer
  spawns any network git (it should be all-local). The dropped names
  (`TestIntegrationCommitPush`, `TestIntegrationPull`) must each remain covered by
  `git_test.go:TestCommitPush` / `git_test.go:TestPull` respectively — re-read those to
  confirm before deleting.
- **Workstream B (parallelize boardtest):** TDD-ish — flip each local test to explicit
  flags + `t.Parallel()` and re-run `go test -tags integration ./internal/board/boardtest
  -count=1` to confirm green and faster. Add a focused assertion that the new config
  flags actually skip (e.g. a `Sync` with `SkipPush` set commits but does not push;
  a `Sync` with `SkipGit` set is a no-op) so the seam is covered, not just env. Verify
  the env fallback still works (a test or manual check that `BOARD_SKIP_PUSH=1` with no
  flag still skips). Watch for ordering assumptions now that tests run concurrently
  (filenames using `time.Now().Unix()` are fine — each test has its own `t.TempDir`).
  - **Ambient-env leakage (review r1):** `t.Setenv` currently does double duty — it both
    sets *and neutralizes* ambient env (e.g. `sync_test.go` does `t.Setenv("BOARD_SKIP_GIT","")`
    precisely to clear any inherited value). Removing it means a flag-converted test that
    needs git to *run* must set the flag explicitly to `false` (not merely omit it), and
    the consumption sites must let that explicit `false` win over an ambient
    `BOARD_SKIP_GIT=1` (the explicit-precedence rule in Decision B). Without both, an
    ambient `BOARD_SKIP_*=1` would silently no-op the very Sync the test means to
    exercise. mill-plan must verify a test running under an ambient `BOARD_SKIP_GIT=1`
    still exercises the real git path.
- **Workstream C (fixture trim):** switch the `SkipPush:true` worktree/weft tests to the
  lean fixture and confirm they still pass (the junction/exclude/branch assertions don't
  depend on the weft-bare). No existing test pushes the weft branch, so **add one new
  weft-pushing test** (`Add(..., AddOptions{})`) on the full `CopyPaired` that asserts
  the weft branch lands on the weft-bare — this keeps the full fixture exercised and
  covers `pushWeftBranch` (currently uncovered). The lean variant itself is the unit
  under test — assert it produces a working hub + host-bare + weft-prime (no weft-bare)
  and that `Add(SkipPush:true)` succeeds against it (including the unconditional host
  push to the host bare).
- **Equivalence check:** diff `go test -tags integration ./... -list '.*'` (and a
  `=== RUN` capture) before vs after; the only removed names must be the two network
  tests. Everything else identical.
- **Timing:** `go run ./cmd/testtiming -full` several times warm; record the median
  per-package and wall-clock in a new `test-suite-timing.md` history block, with the
  subset-drop justification (Decision D).
- Follow `golang-testing` conventions (table-driven where it fits, `t.Parallel()` on the
  now-isolated tests).

## Q&A log

- **Q:** The brief says boardtest is ~82s and the network push is the biggest cost — is
  that current? **A:** No. Measured now: boardtest ~42s, of which ~26s is *local* serial
  git tests; removing the network test only moves the floor ~42→~38s. The real speedup
  lever is parallelizing the local tests (B), not gating the network test.
- **Q:** Is the brief's "secondary lever" (parallelize worktree/weft) still needed?
  **A:** No — `TestRemoveHostJunctionRemoved` / `TestAddRollback` /
  `TestWeftSpawnPairedWorktrees` already call `t.Parallel()` (done by the two prior
  optimization tasks). The remaining worktree lever is fixture-I/O trim (C), not adding
  `t.Parallel()`.
- **Q:** Why is worktree ~32s if `git worktree add` is fast? **A:** It isn't the git op
  — a paired Add is ~10 spawns (~0.3s). The cost is copying four full git repos per test
  (Defender-scanned) + tempdir teardown; I/O-bound, so parallel runs contend and each
  test inflates ~4s→~10s. Hence the fixture trim (C).
- **Q:** Network test — gate behind `integration && network`, or delete? **A:** Delete
  (option 2). Logic is already covered by the local `TestCommitPush`/`TestPull`; deleting
  removes all network dependency and the need for a `network` tag. `bench_git_test.go`
  goes too (shares the remote).
- **Q:** Skip-seam approach for parallelizing boardtest? **A:** Add `SkipGit`/`SkipPush`
  to `board.Config` (+ functional options for the package funcs), keep `os.Getenv` as
  fallback so production is untouched.
- **Q:** Touch the serial `RunCLI` tests (`t.Chdir`)? **A:** No (option 1) — ~2s, below
  the floor; not worth CLI-signature churn.
- **Q:** Acceptance criterion — hard target or cheap wins? **A:** Cheap wins, then record
  median timing (option 1). No fixed number; C stops at diminishing returns.
- **Q (review r1 GAP):** Is the host bare really dead weight under `SkipPush:true`?
  **A:** No — verified false. `Add` pushes the *host* branch unconditionally (add.go:172);
  `SkipPush` gates only the *weft* push (add.go:182-183). So C drops only the **weft-bare**
  (~25% of the copy), not both bares; the host bare stays. Premise and payoff corrected.
- **Q (review r1 NOTE):** Does keeping env as fallback leak into parallel tests?
  **A:** Yes if "flag OR env"; fixed to explicit-flag-precedence — an explicit `false`
  overrides ambient `BOARD_SKIP_*=1`. Flag-converted tests set the flag explicitly;
  mill-plan verifies the real-git path runs even under an ambient `BOARD_SKIP_GIT=1`.
- **Q (review r1 NOTE):** `concurrency_test.go` isn't integration-tagged — still convert?
  **A:** Yes, for consistency/parallel-safety, but it's a Tier 1 test so it does not move
  the Tier 2 floor; not counted toward the speedup.
- **Q (review r2 GAP):** "Keep a weft-pushing test on the full fixture" — but does one
  exist? **A:** No (verified: all 12 `Add` calls use `SkipPush:true`; `pushWeftBranch` is
  uncovered). Resolution: the plan **adds one** weft-pushing test (`AddOptions{}`) on full
  `CopyPaired`, closing the pre-existing gap and keeping the full builder exercised.
- **Q (review r2 GAP):** What does the lean fixture return for `WeftBare`/weft-prime
  origin? **A:** Same `PairedFixture` with `WeftBare == ""` and weft-prime origin left
  unrewritten (points at the shared template bare, never reached under `SkipPush`).
  Verified no `SkipPush` test reads `.WeftBare`; the lean variant is not used by the new
  weft-pushing test.
- **Q (review r2 NOTE):** Decision B glosses the package-`Sync` seam — clarify? **A:**
  Package `Sync`/`CommitPush` get a new options signature; `(b *Board) Sync()` passes its
  resolved flags through; `RunCLI`'s sync case (cli.go:263) resolves env→options once.
