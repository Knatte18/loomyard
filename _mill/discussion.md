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
  - `internal/ide`: `cli_test.go` + `menu_test.go` (`package ide`, spawn the binary /
    drive the TUI via `exec.Command`) — gated **in place** (internal package).
- **Reuse `internal/lyxtest`** shared fixtures for board's git fixtures (template-built-once
  + per-test filesystem copy, no per-test git spawn), extending `lyxtest` only if no
  existing fixture fits (see Decisions: board-fixtures).
- **Parallelise** (`t.Parallel()`) the migrated/gated tests **where the env seam does not
  block it** (best-effort; see Decisions: board-git-seam for why board unit tests stay serial).
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
- **No unit-ification of the `ide` menu/CLI tests** (no mocking the spawn) — gate them.
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
- Rationale: Both files are already `package board_test` (external), so the move is clean
  (no exported-symbol friction). Consolidates all board git-integration tests in one gated
  package rather than splitting them across `internal/board` + `boardtest`. Matches
  `boardtest/doc.go`'s stated purpose.
- Note: `boardtest` already sets `BOARD_SKIP_GIT=1` in its bench/concurrency tests; the
  moved sync/git tests need git **enabled**, so watch for env contention (the moved tests
  deliberately run with `BOARD_SKIP_GIT` unset/empty — they spawn real git directly, not
  via the detached sync). Keep per-test env handling correct; this is another reason those
  tests may stay serial.
- Rejected: Gate in place inside `internal/board` — less churn but leaves board's git
  integration tests in two locations.

### board-fixtures

- Decision: **Reuse existing `lyxtest` fixtures** (`CopyHostHub` → `{Hub, Bare}`,
  `CopyWeft` → `{WeftPath, Bare}` with upstream tracking) for board's bare+clone(+upstream)
  git needs where the shape fits. Add a dedicated exported board fixture to `lyxtest` **only
  if** no existing fixture fits. Default to reuse; decide concretely during implementation.
- Rationale: The proposal's step 2 mandates reuse, and board's `newSyncRepo` helper
  (bare + clone + upstream) closely matches `CopyWeft`'s shape; `TestPull`/`TestCommitPush`'s
  bare+clone+configured-user matches `CopyHostHub`. lyxtest's "template-once + per-test
  filesystem copy" design (no per-test git spawn, pure-text origin-URL rewrite) is exactly
  what board's git tests need to stop spawning `git init`/`clone` per test.
- Rejected: Add a `CopyBoardRepo` fixture up front regardless — premature if existing
  fixtures fit. Keep the fixture local to `boardtest` — re-implements what lyxtest already
  provides and violates the reuse mandate.

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
- **`internal/board/boardtest`** (`doc.go`, `integration_test.go`, `bench_git_test.go`,
  `bench_test.go`, `concurrency_test.go`) — `package boardtest`, `integration`/`smoke` gated;
  the documented home for board's git-backed integration + bench + concurrency tests. The
  moved files land here.
- **Board git seam**: `internal/board/board.go:83` (`if os.Getenv("BOARD_SKIP_GIT") != "1"`)
  and `internal/board/sync.go:32` — both stay as-is. Test files toggle via `t.Setenv`.
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
  - `internal/ide/cli_test.go` (4 funcs) + `menu_test.go` (5 funcs) — `package ide`, spawn
    via `exec.Command(args[0], …)`. → gate in place; keep `color_test.go`, `spawn_test.go`,
    `vscode_test.go` offline.
- **Board unit suite to prune (78 funcs, `internal/board/*_test.go`):** `render_test.go` 20,
  `store_test.go` 19, `config_test.go` 8, `cli_test.go` 8, `board_test.go` 7,
  `sync_test.go` 5 (moving out), `init_test.go` 4, `layer_test.go` 3, `task_test.go` 2,
  `git_test.go` 2 (moving out). Pruning targets the `render`/`store` overlap.
- **Module path:** `github.com/Knatte18/loomyard`.
- **Benchmarks doc:** `docs/benchmarks/test-suite-timing.md` — append a new dated block;
  do not edit prior blocks (the file's own convention).

## Constraints

- **Equivalence guardrail (non-negotiable, from the parent):** the post-change test-name
  set must be a **superset** of the pre-change set per package, verified by diffing `-list`
  + `=== RUN` baselines. Intentional table-driven folds are allowed only when assertions
  are preserved; no named (sub)test or assertion may be silently dropped. Record the
  superset note in the timing doc, exactly as the 2026-06-21 block does.
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
