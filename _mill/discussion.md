# Discussion: Optimise and slim the rest of the test suite

```yaml
task: Optimise and slim the rest of the test suite
slug: optimize-remaining-test-suites
status: discussing
parent: main
```

## Problem

The parent task `optimize-test-suite` (merged in `e58734f`) split the suite into two
tiers — an offline default loop (`go test ./...`, zero git spawns) and a gated
`-tags integration` loop — for **three** packages only: `internal/worktree`,
`internal/weft`, `internal/paths`. The headline result was dramatic: the full offline
loop dropped from ~82 s to ~27.6 s, and `internal/worktree` from 53.6 s to 1.06 s
(see `docs/benchmarks/test-suite-timing.md`).

But the whole-suite floor did not fall to seconds — it just shifted to the packages the
parent never touched. Today `go test ./...` still **spawns real `git`** from
`internal/board` (`git_test.go`, `sync_test.go`), `internal/git` (`git_test.go`), and
spawns the binary / drives a TUI from `internal/ide` (`cli_test.go`, `menu_test.go`).
Those packages set the new ~24 s (`board`) + ~12 s (`ide`) default-loop floor and keep
the default loop non-hermetic. **Why now:** the parent just landed and established the
exact playbook (build-tag gating, `lyxtest` shared fixtures, `t.Parallel()`, conservative
pruning, equivalence guardrail); this task applies that same playbook to the remaining
packages so the **entire** suite finally hits the seconds-class, offline target.

The parent was hard to implement (the mill-go builder struggled on the prod-code
refactors and the team switched to a Sonnet implementer to land it). The de-risking
decision here is to **run the Sonnet implementer from the start** and to **keep the
risky production-code refactor out of scope** (see Decisions: board-git-seam).

## Scope

**In:**

- **Gate** the remaining untagged subprocess-spawning tests behind `//go:build integration`:
  - `internal/board`: `git_test.go` + `sync_test.go` (both `package board_test`, both
    entirely git-spawning) — **moved into `internal/board/boardtest`** (see Decisions).
  - `internal/git`: `git_test.go` (`package git_test`, 3 funcs testing `git.RunGit`
    directly) — gated **in place** (it has no other test file).
  - `internal/ide`: `cli_test.go` + `menu_test.go` (`package ide`) — gated **in place**
    (internal package). These call the SUT (`RunCLI`/`Menu`) **in-process** and spawn
    **git** only for fixtures (`git init`/`config`/`commit`/`worktree add` via the local
    `mustRun`/`mustRunMenu` helpers) — *not* the binary, *not* a TUI. Because they spawn
    git per-test, they get the same lyxtest treatment as board (see below).
- **Reuse `internal/lyxtest`** shared fixtures for **board's and ide's** git fixtures
  (template-built-once + per-test filesystem copy, no per-test git spawn), extending
  `lyxtest` only if no existing fixture fits — **migrate where it's worth it** (fixture
  shape fits, real win), don't force-fit (see Decisions: board-fixtures, ide-scope).
- **Parallelise** (`t.Parallel()`) the migrated/gated tests **where no process-global seam
  blocks it** (best-effort; see Decisions: board-git-seam / ide-scope for which tests stay
  serial — board unit tests and ide `menu_test` use `t.Setenv`; ide `cli_test` uses
  `os.Chdir`; the board sync/git tests use `t.Setenv`). In practice most migrated tests stay
  serial; the headline offline win comes from gating, not parallelism.
- **Prune `internal/board`'s oversized unit suite conservatively** (78 non-`boardtest`
  funcs; `render_test.go`=20 + `store_test.go`=19 overlap) — fold pure overlap into
  table-driven cases, drop nothing behaviourally distinct, enforce the equivalence guardrail.
- **Update `docs/benchmarks/test-suite-timing.md`** with a new dated block: whole-suite
  offline + integration wall-clock, the equivalence-guardrail superset note, and the
  parallel-safety note (mirroring the parent's 2026-06-21 block format).

**Out:**

- **No `BOARD_SKIP_GIT` env → option refactor.** Production code (`internal/board/board.go`,
  `internal/board/sync.go`) keeps reading `os.Getenv("BOARD_SKIP_GIT")`. This is the
  prod-code refactor that hurt the parent; it is explicitly excluded.
- **`internal/muxpoc`** — its only subprocess test (`muxpoc_smoke_test.go`) is **already**
  gated behind `//go:build smoke`, so it already does not run in the default loop. The
  other muxpoc files (`cli_test.go`, `cmd_test.go`, `state_test.go`) spawn nothing. No
  change. The proposal's claim that muxpoc smoke runs untagged is **stale** — verified
  during exploration.
- **No tag unification of muxpoc's `smoke` tag** into `integration` — leave `smoke` as-is.
- **No rewrite of ide's SUT invocation.** ide tests already call `RunCLI`/`Menu` in-process;
  we only gate them and migrate their **git fixtures** to lyxtest (where worth it). We do
  not mock or restructure how the SUT itself is exercised.
- **No new automated "offline guard" test.** The parent did not add one; the offline
  guarantee rests on the build tag + lyxtest's no-per-test-spawn design. Out of scope.
- **No aggressive pruning / no numeric target.** Conservative fold only.
- No changes to the already-migrated `worktree`/`weft`/`paths` packages.

## Decisions

### scope-and-risk-posture

- Decision: Apply the **full parent playbook** (gate + lyxtest fixtures + parallelise +
  prune + docs) to the remaining packages, with two explicit de-riskings: (1) keep the
  `BOARD_SKIP_GIT` env seam (no prod refactor), (2) conservative pruning only. The
  mill-go builder runs the **Sonnet implementer from the start**.
- Rationale: The user wants the same outcome quality as the parent. Gating alone already
  delivers the headline win (offline, seconds-class default loop); the other steps are
  refinements that bring the integration tier and unit-suite size in line with the parent.
- Rejected: "Gating only, rest best-effort" — user wants the full playbook. "Full playbook
  including the env→option refactor" — that prod refactor is the highest-risk part and is
  deliberately excluded.

### board-git-seam

- Decision: **Keep `BOARD_SKIP_GIT` as a process-level env var.** Do not thread a
  `SkipGit` option through `board.Config`. Only gate the git-spawning board tests.
- Rationale: Once `git_test.go` + `sync_test.go` are gated, the remaining ~70 board unit
  tests already run file-I/O-only under `BOARD_SKIP_GIT=1` and are fast — parallelising
  them is marginal. Avoiding the `board.Config` refactor removes the exact prod-code risk
  that hurt the parent.
- Consequence: Board unit tests that use `t.Setenv("BOARD_SKIP_GIT", …)` cannot call
  `t.Parallel()` (Go forbids `t.Parallel` after `t.Setenv`). They stay serial — accepted.
  The gated board git/sync integration tests parallelise only if they avoid per-test
  `t.Setenv`; parallelism there is **best-effort, not required** (board's integration tier
  is small and runs on demand).
- Rejected: Env→option refactor mirroring weft's `SyncOptions` — cleaner and enables full
  parallelism, but touches production code and is the risky path.

### board-test-home

- Decision: **Move `git_test.go` + `sync_test.go` into `internal/board/boardtest`** (the
  existing, already-`integration`-gated package documented as board's home for "git-backed
  integration tests"). After the move, `internal/board` itself needs no build tag and is
  fully offline.
- **The move is a package-clause rewrite, not just a relocation.** Both files are currently
  `package board_test`; in `boardtest` they become **`package boardtest`** + a
  `//go:build integration` line (tag line first, one blank line, then `package boardtest`,
  per Go's build-constraint placement). Verified safe: both files are already black-box
  (they import `internal/board` and use only its exported API — `board.CommitPush`,
  `board.Pull`; no unexported `board.*` references), so changing the package name compiles.
- **No name collisions (verified against the current `boardtest`):**
  - Top-level test funcs: `boardtest` already has `TestIntegrationCommitPush`,
    `TestIntegrationPull`, `TestConcurrentReadsDuringUpserts`,
    `TestConcurrentUpsertsDoNotLoseWrites` (+ benchmarks). The moved funcs are `TestPull`,
    `TestCommitPush`, and the five `TestSync*` — **none clash**.
  - Package-level helpers: the moved files add `newSyncRepo` and `dirty` (from
    `sync_test.go`); `git_test.go` has no package-level helpers (its `run`/setup are
    closures). `boardtest`'s existing helpers are `cloneBenchWiki`, `benchmarkSync`,
    `seedWiki`, `setupIntegrationRepo` — **none clash**.
- **Not redundant with the existing `TestIntegration*` tests.** `boardtest`'s
  `TestIntegrationPull`/`TestIntegrationCommitPush` push to a **real network remote**
  (`testRepoURL`) and verify via a fresh clone. The moved `TestPull`/`TestCommitPush`/
  `TestSync*` are **local/offline** (`git init` + bare clone in `t.TempDir()`,
  `BOARD_SKIP_PUSH=1`) — they exercise the local commit/coalesce/pull plumbing without
  network. Distinct coverage; keep both.
- Note (env): the moved tests toggle env via `t.Setenv` (see env-seam in Technical
  context), so they cannot call `t.Parallel()` and stay **serial** — independent of any
  contention with `boardtest`'s `BOARD_SKIP_GIT=1` bench/concurrency tests (`t.Setenv` is
  per-test scoped and restored, so there is no actual cross-test contention).
- Rejected: Gate in place inside `internal/board` — less churn but leaves board's git
  integration tests in two locations.

### board-fixtures

- Decision: **Reuse existing `lyxtest` fixtures** (`CopyHostHub` → `{Hub, Bare}`,
  `CopyWeft` → `{WeftPath, Bare}` with upstream tracking) for board's bare+clone(+upstream)
  git needs where the shape fits. Add a dedicated exported board fixture to `lyxtest` **only
  if** no existing fixture fits. Default to reuse; decide concretely during implementation.
- Rationale: The proposal's step 2 mandates reuse, and board's `newSyncRepo` helper
  (bare + clone + upstream) closely matches `CopyWeft`'s shape. **`CopyWeft` is the better
  fit for `TestPull`**: `CopyHostHub`'s bare is **empty/never-pushed** (`buildHostHub`
  commits in the hub but does not push to the bare, `lyxtest.go:47-133`), whereas `TestPull`
  needs an upstream that **already has a commit to pull** — so `CopyHostHub` does not fit
  `TestPull` without an extra push, while `CopyWeft` (which does `push -u origin main`,
  establishing history + upstream) does. lyxtest's "template-once + per-test filesystem
  copy" design (no per-test git spawn, pure-text origin-URL rewrite) is exactly what
  board's git tests need to stop spawning `git init`/`clone` per test.
- **Concrete fit risk to settle (the decider for reuse-vs-new-fixture):** default-branch
  naming. `newSyncRepo` does `git push -u origin HEAD` and deliberately counts via `@{u}`
  because "the bare repo's HEAD symref [may point] at a different default branch"
  (`sync_test.go:47-60`). `CopyWeft`'s template does `git init -b main` + `push -u origin
  main` (`lyxtest.go:239,301`). So a `CopyWeft`-based board fixture lands the working
  commits on `main`, whereas `newSyncRepo`'s assertions are branch-agnostic (`HEAD`/`@{u}`).
  If the migrated board tests assert against `main` (or stay `HEAD`-relative), `CopyWeft`
  fits directly; if any assertion hard-codes a different branch, prefer a small dedicated
  board fixture. **Concretely, `git_test.go::TestPull` hard-codes `push -u origin master`
  (`:62`)** — a `master`-vs-`main` mismatch with `CopyWeft`'s `main`, so that test in
  particular needs the push target reconciled (rename to `main`, or use a fixture whose
  default branch matches its assertions). The plan writer must resolve this
  `master/HEAD`-vs-`main` detail when choosing reuse vs. a new fixture — it is the single
  blocking unknown for this decision.
- Rejected: Add a `CopyBoardRepo` fixture up front regardless — premature if existing
  fixtures fit. Keep the fixture local to `boardtest` — re-implements what lyxtest already
  provides and violates the reuse mandate.

### ide-scope

- Decision: ide gets the **same playbook as board** — gate `cli_test.go` + `menu_test.go`
  behind `integration` **and** migrate their git fixtures (`newTestGitRepo`,
  `newTestGitRepoWithWorktrees`) onto lyxtest **where it's worth it**. **Both stay serial**
  (not parallelisable — see blockers below); the lyxtest value here is removing per-test
  **fixture-build** git spawns from Tier 2, not parallelism.
- **Serial blockers (verified):** `cli_test.go`'s four funcs each call `os.Chdir(gitRepo)`
  + `defer os.Chdir(oldCwd)` (process-global cwd, illegal under `t.Parallel`); `RunCLI`
  takes no dir argument, so parallelising would require a prod-code change (out of scope —
  keep serial). `menu_test.go` uses `t.Setenv("BOARD_SKIP_GIT","1")` (illegal under
  `t.Parallel`). So neither ide file is parallelised.
- **"Where it's worth it" judgement for menu:** `menu_test.go` also does `git worktree
  add`/`remove`/`branch -D` **in the test bodies** (`:99-102`), so migrating only the base
  repo to lyxtest leaves those in-body spawns in Tier 2 — the migration win for menu is
  partial. `CopyPaired` yields independent sibling repos, **not** `git worktree`-linked
  children, so it covers only the base. The plan writer may reasonably migrate just
  `cli_test.go`'s base (`newTestGitRepo`) and leave `menu_test.go` gated-but-unmigrated if
  the base-only saving isn't worth it. Decide at plan time.
- Rationale: User's directive — "lyxtest was introduced to be the common fixture point;
  use it where it's worth it." ide spawns git per-test for fixtures (not the binary), so it
  is lyxtest-migratable like board; gate-only would leave per-test fixture spawns in Tier 2.
  "Where it's worth it" = migrate when the fixture shape fits and the saving is real; don't
  force-fit a fixture that doesn't match (esp. menu's worktree-linked layout).
- Note: This **supersedes** the operator's initial "gate ide only" pick, which was made on
  the mistaken premise that ide spawned the binary / drove a TUI (corrected in
  discussion-review round 2).
- Rejected: Gate-only (no lyxtest migration) — achieves the offline win but ignores the
  reuse mandate and leaves ide's Tier 2 slow. Unit-ify / mock the SUT — unnecessary; the
  SUT already runs in-process, only the git fixtures spawn.

### tag-string

- Decision: Use **`//go:build integration`** for every new gate (board/git/ide). Leave
  `internal/muxpoc/muxpoc_smoke_test.go`'s existing `//go:build smoke` tag untouched.
- Rationale: `integration` is the repo's established convention (17 files already use it).
  muxpoc's smoke test is already excluded from the default loop, so re-tagging it is churn
  with no offline-loop benefit.
- Rejected: Unify `smoke` → `integration` — needless churn, out of scope.

### pruning

- Decision: **Conservative fold, no numeric target.** Collapse only pure-overlap
  `render_test.go` / `store_test.go` cases into table-driven tests; drop nothing
  behaviourally distinct. Enforce the parent's equivalence guardrail.
- Rationale: Pruning is the highest coverage-risk, lowest-speed-value step (gating already
  delivers the speed). A rule ("fold pure overlap, keep all distinct coverage") is safer
  than chasing a count.
- Rejected: Aggressive (~45 target) — more review churn and coverage-loss risk. Skip
  pruning entirely — user wants the full playbook applied.

## Technical context

- **Two-tier model** (from the parent, `docs/benchmarks/test-suite-timing.md`): Tier 1 =
  default `go test ./...`, must spawn zero git/subprocesses; Tier 2 = `go test -tags
  integration ./...`, runs the git-spawning suite. This task moves the remaining spawners
  into Tier 2.
- **`internal/lyxtest`** (`internal/lyxtest/lyxtest.go`) — shared fixtures the parent built.
  Exported API: `MustRun`, `CopyHostHub() HostFixture{Hub,Bare}`, `CopyPaired() PairedFixture`,
  `CopyWeft() WeftFixture{WeftPath,Bare}`. Design invariant: template built once, each test
  gets a fresh **filesystem copy** (`copyDirRecursive`); origin URL is rewritten as **pure
  text** in `.git/config` (`rewriteOriginURLInConfig`) — never `git remote set-url`, to
  preserve the zero-per-test-spawn guarantee. Reuse this pattern for board.
- **`internal/board/boardtest`** (`package boardtest`) — **mixes untagged and gated files**
  (verified): `integration_test.go` + `bench_git_test.go` carry `//go:build integration`;
  `bench_test.go`, `concurrency_test.go`, `doc.go` are **untagged** and run in the default
  loop (they're no-git: `BOARD_SKIP_GIT=1`). There is **no `smoke` tag** here (that tag is
  muxpoc's). The moved `git_test.go`/`sync_test.go` get `//go:build integration`, so they
  compile **only** under `-tags integration` and **do not affect Tier 1** — they land
  alongside the untagged no-git files without changing the offline loop.
- **Board git seam — two distinct env vars (verified):** `BOARD_SKIP_GIT` (gates the
  detached `lyx board sync` spawn; `board.go:83`, `sync.go:32`) and `BOARD_SKIP_PUSH`
  (commits locally but skips the push). Both stay as-is in production. In the tests being
  moved:
  - `git_test.go` toggles **`BOARD_SKIP_PUSH`** via `t.Setenv` (never `BOARD_SKIP_GIT`),
    but only **partially**: two `TestCommitPush` subtests set `BOARD_SKIP_PUSH=1` (`:110`,
    `:154`), while `TestPull` does a **real** `git push -u origin master` (`:62`) and the
    rebase-retry subtest sets `BOARD_SKIP_PUSH=""` (`:229`) and pushes. So these are
    **local** (bare repo in `t.TempDir()`, **no network**) but **not** uniformly
    push-skipping — several need a **working upstream**. This reinforces the `CopyWeft`
    fixture fit (it establishes upstream tracking), with the branch-name caveat below.
    All still spawn real `git`, so they must be gated out of the offline loop.
  - `sync_test.go` uses **`BOARD_SKIP_GIT=""`** (`:25`, ensures sync is *not* disabled) and
    **`BOARD_SKIP_PUSH="1"`** (`:111`) via `t.Setenv`.
  - Because every one of these uses `t.Setenv`, the moved tests **stay serial** (`t.Parallel`
    is illegal after `t.Setenv`). `t.Setenv` is per-test scoped and restored, so there is
    no actual env contention with `boardtest`'s `BOARD_SKIP_GIT=1` bench/concurrency tests.
- **Files to gate/move (verified counts):**
  - `internal/board/git_test.go` — `package board_test`, 2 funcs (`TestPull`,
    `TestCommitPush`), entirely git-spawning. → move to `boardtest`, gated.
  - `internal/board/sync_test.go` — `package board_test`, 5 funcs (`TestSyncCommitsAndPushes`,
    `TestSyncCoalescesBurstIntoOneCommit`, `TestSyncSkipPushCommitsLocallyOnly`,
    `TestSyncCleanTreeIsNoOp`, `TestSyncIgnoresLockfiles`), helper `newSyncRepo`
    (bare+clone+upstream). → move to `boardtest`, gated.
  - `internal/git/git_test.go` — `package git_test`, 3 funcs testing `git.RunGit` directly;
    fundamentally needs real git. → gate in place. **Consequence:** `internal/git` then has
    zero default-loop tests (package still compiles). Accepted.
  - `internal/ide/cli_test.go` (4 funcs) + `menu_test.go` (5 funcs) — `package ide`. SUT
    (`RunCLI`/`Menu`) runs **in-process**; `exec.Command(args[0], …)` is the local
    `mustRun`/`mustRunMenu` helper that spawns **git** for fixtures: `newTestGitRepo`
    (plain `git init -b main` + config + commit) and `newTestGitRepoWithWorktrees` (main
    worktree + child worktrees via `git worktree add`). → gate in place; migrate these
    fixtures onto lyxtest where the shape fits (`newTestGitRepo` ≈ `CopyHostHub`;
    `newTestGitRepoWithWorktrees` ≈ `CopyPaired`/`CopyHostHub` — confirm at plan time).
    Both files stay **serial**: `menu_test.go` uses `t.Setenv("BOARD_SKIP_GIT","1")` (5
    sites), and `cli_test.go`'s four funcs each `os.Chdir(gitRepo)` with `defer
    os.Chdir(oldCwd)` (`:56-58,79-81,101-103,123-125`) — process-global cwd, illegal under
    `t.Parallel` (and `RunCLI` has no dir param to thread instead). `menu_test.go` also runs
    `git worktree add/remove/branch -D` **in the test bodies** (`:99-102`), so a lyxtest
    base-repo migration leaves those in-body spawns in Tier 2. Keep `color_test.go`,
    `spawn_test.go`, `vscode_test.go` offline (pure units).
- **Board unit suite to prune (78 funcs, `internal/board/*_test.go`):** `render_test.go` 20,
  `store_test.go` 19, `config_test.go` 8, `cli_test.go` 8, `board_test.go` 7,
  `sync_test.go` 5 (moving out), `init_test.go` 4, `layer_test.go` 3, `task_test.go` 2,
  `git_test.go` 2 (moving out). Pruning targets the `render`/`store` overlap.
- **Module path:** `github.com/Knatte18/loomyard`.
- **Benchmarks doc:** `docs/benchmarks/test-suite-timing.md` — append a new dated block;
  do not edit prior blocks (the file's own convention).

## Constraints

- **Equivalence guardrail (non-negotiable, from the parent):** the post-change test-name
  set must be a **superset** of the pre-change set, verified by diffing `-list` + `=== RUN`
  baselines. Intentional table-driven folds are allowed only when assertions are preserved;
  no named (sub)test or assertion may be silently dropped. Record the superset note in the
  timing doc, exactly as the 2026-06-21 block does.
  - **Baseline is computed per-final-package, and the board↔boardtest move crosses a package
    boundary.** Moving `git_test.go`/`sync_test.go` out of `internal/board` and into
    `internal/board/boardtest` shrinks board's `-list` set and grows boardtest's. So the
    superset check must be done against the **union across both final packages** (default-loop
    `board` + integration-tagged `boardtest`), not per-package in isolation — otherwise the
    move reads as a spurious "loss" in board and "gain" in boardtest. Capture the pre-move
    baseline for both packages (board untagged, boardtest under `-tags integration`) before
    touching either, and prove the post-move union is a superset.
- **Tier 1 must remain offline:** after this task, `go test ./...` must spawn **zero** git
  subprocesses repo-wide (the whole point). Verify by running the default loop and
  confirming the moved/gated tests do not execute.
- **`-race` is not a precondition:** the dev environment lacks CGO/a C compiler, so the
  race detector is opportunistic-CI-only (per the parent's note). Parallel safety is
  guaranteed by construction (isolated per-test `lyxtest` copies), not by `-race`.
- **Windows-noisy timings:** wall-clock numbers are order-of-magnitude on Windows
  (Defender + process tax). Record a new dated block; don't chase exact seconds.
- **Mill wiki:** never touch the wiki directly (CLAUDE.md) — irrelevant to code but noted.

## Testing

This task **is** test-suite work; "testing" here means preserving coverage while moving it
between tiers, plus measuring the result.

- **Gating / moving (board, git, ide):** No new assertions. The work is mechanical —
  add `//go:build integration` (and the matching blank-line + `package` placement Go
  requires), move the two board files into `boardtest`, fix imports/helpers. Validate:
  1. `go test ./...` (Tier 1) passes and **spawns no git** (moved/gated tests absent).
  2. `go test -tags integration ./...` (Tier 2) passes and **includes** every moved/gated
     test (run `-list` to confirm presence).
- **lyxtest reuse for board fixtures:** Migrate `newSyncRepo` / the `TestPull` /
  `TestCommitPush` setup onto `lyxtest.CopyWeft` / `CopyHostHub` (or a new fixture if
  needed). The behavioural assertions (commit counts, push/no-push, coalescing,
  lockfile-ignore, clean-tree no-op, pull) must be **byte-for-byte preserved** — only the
  repo-construction changes. Guard with the `-list` + `=== RUN` diff.
- **lyxtest reuse for ide fixtures:** Migrate `newTestGitRepo` (plain repo) onto a lyxtest
  copy where the shape fits; behavioural assertions in `cli_test.go` / `menu_test.go`
  preserved byte-for-byte. **Both files stay serial** (`cli_test.go`: `os.Chdir`;
  `menu_test.go`: `t.Setenv`) — the migration removes per-test fixture-build spawns, not
  parallelism. `menu_test.go`'s in-body `git worktree add/remove` spawns remain in Tier 2;
  `CopyPaired` covers only the base, so migrating menu may not be worth it — if a fixture
  doesn't fit, leave that test spawning git in Tier 2 rather than force-fitting.
- **Board pruning (the TDD-sensitive part):** Before touching `render_test.go` /
  `store_test.go`, capture `go test -list '.*' ./internal/board` + a `=== RUN` baseline.
  Fold only pure-overlap cases into table-driven subtests; after, diff to prove the
  post-set is a superset (every prior subtest name still present, modulo documented folds
  where assertions are preserved). No coverage regression.
- **Parallel safety:** Any test given `t.Parallel()` must take an isolated `lyxtest` copy
  with no shared mutable state — same construction-time guarantee as the parent. Tests
  using `t.Setenv` stay serial.
- **Measurement / done definition:** Whole-repo `go test ./... -count=1` (Tier 1) and
  `go test -tags integration ./... -count=1` (Tier 2) both green; Tier 1 offline; new dated
  block added to `docs/benchmarks/test-suite-timing.md` with before/after wall-clock for
  `board`/`ide`/`git`, the equivalence-superset note, and the parallel-safety note.

## Q&A log

- **Q:** Full parent playbook, or gating-only with the rest best-effort? **A:** Full
  playbook ("same as parent"); de-risk by running the Sonnet implementer from the start.
- **Q:** Refactor `BOARD_SKIP_GIT` env → a `board.Config` option (mirroring weft), or keep
  the env? **A:** Keep the env, just gate the git tests — avoids the prod-code refactor that
  hurt the parent; board unit tests stay serial as a result.
- **Q:** How aggressively to prune board's 78 funcs? **A:** Conservative fold, no numeric
  target; fold only pure render/store overlap, drop nothing distinct, enforce equivalence.
- **Q:** Gate ide, what tag, and muxpoc? **A:** Gate ide cli/menu behind `integration`;
  use `integration` for all new gates; leave muxpoc's existing `smoke` tag untouched
  (muxpoc smoke is already gated — the proposal's "untagged" claim is stale).
- **Q:** Where do board's gated git tests live? **A:** Move `git_test.go` + `sync_test.go`
  into `internal/board/boardtest` (already the gated home); both are `package board_test`
  so the move is clean.
- **Q:** Source board fixtures from lyxtest or local? **A:** Reuse existing lyxtest fixtures
  (`CopyHostHub`/`CopyWeft`) where they fit; extend lyxtest only if none fits.

### Discussion-review round 1 (GAPS_FOUND) — resolutions

- **GAP:** The board→boardtest move is a package-clause rewrite, not just a relocation.
  **Resolved:** documented — moved files become `package boardtest` + `//go:build integration`;
  verified both are black-box (exported API only), so the rename compiles.
- **GAP:** Name-collision risk against existing `boardtest` tests. **Resolved:** verified
  against code — **no** collision. boardtest's tests are `TestIntegrationPull`/
  `TestIntegrationCommitPush` (real-network), distinct from the moved local/offline
  `TestPull`/`TestCommitPush`/`TestSync*`; helpers `newSyncRepo`/`dirty` don't clash with
  `cloneBenchWiki`/`seedWiki`/`setupIntegrationRepo`. Equivalence baseline now specified as
  the union across both final packages (cross-boundary move).
- **NOTE:** git tests use `BOARD_SKIP_PUSH`, not `BOARD_SKIP_GIT`. **Resolved:** corrected
  the env-seam section — `git_test.go`→`BOARD_SKIP_PUSH`; `sync_test.go`→`BOARD_SKIP_GIT=""`
  + `BOARD_SKIP_PUSH`; all via `t.Setenv` ⇒ serial, no real contention.
- **NOTE:** Fixture reuse left to implementation-time; `HEAD`-vs-`main` default-branch
  mismatch. **Resolved:** flagged the `push -u origin HEAD` (newSyncRepo) vs `init -b main`
  + `push -u origin main` (CopyWeft) detail as the single blocking decider for reuse-vs-new.

### Discussion-review round 2 (GAPS_FOUND) — resolutions

- **Q:** ide tests were mischaracterised as binary/TUI spawners; they actually run the SUT
  in-process and spawn git for fixtures. Gate-only or full lyxtest treatment? **A:** Use
  lyxtest as the common fixture point — migrate ide's git fixtures where it's worth it.
  Recorded as the `ide-scope` decision; supersedes the earlier "gate ide only" pick (made
  on the false binary-spawn premise). (Parallelism for ide was retracted in round 3 — both
  ide files stay serial; see below.)
- **NOTE:** boardtest mischaracterised as wholly `integration`/`smoke` gated. **Resolved:**
  corrected — boardtest mixes untagged no-git files (`bench_test`, `concurrency_test`,
  `doc`) that run in Tier 1 with `integration`-gated files; no `smoke` tag here. Moved
  files compile only under `-tags integration`, leaving Tier 1 unaffected.
- **NOTE:** `BOARD_SKIP_PUSH` usage in `git_test.go` is partial. **Resolved:** corrected —
  only two `TestCommitPush` subtests skip push; `TestPull` does a real `push -u origin
  master` and the rebase-retry subtest pushes. All still local (no network) but several
  need a working upstream, reinforcing `CopyWeft` fit and the `master`-vs-`main` caveat.

### Discussion-review round 3 (GAPS_FOUND) — resolutions

- **GAP:** `cli_test.go` was claimed parallelisable, but all four funcs use `os.Chdir`
  (process-global cwd), illegal under `t.Parallel`. **Resolved:** retracted the ide
  parallelism claim — both ide files stay serial (`cli_test`: `os.Chdir`, no dir param on
  `RunCLI`; `menu_test`: `t.Setenv`). lyxtest's value for ide is removing fixture-build
  spawns, not parallelism.
- **NOTE:** `menu_test.go` does `git worktree add/remove` in the test bodies, not just
  fixture build; `CopyPaired` yields sibling repos, not worktree-linked children.
  **Resolved:** noted that migrating menu's base leaves in-body worktree spawns in Tier 2;
  `CopyPaired` covers only the base, so migrating menu may not be worth it (plan-time call).
- **NOTE:** `CopyHostHub`'s bare is empty/never-pushed, so `TestPull` (needs history to
  pull) can't reuse it directly. **Resolved:** corrected `board-fixtures` — `CopyWeft` (with
  `push -u origin main`) is the fit for `TestPull`, not `CopyHostHub`.
