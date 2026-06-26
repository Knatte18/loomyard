# Discussion: Speed up internal/warp integration tests

```yaml
task: Speed up internal/warp integration tests
slug: warp-test-speedup
status: discussing
parent: main
```

## Problem

The `internal/warp` integration suite (`go test -tags integration ./internal/warp/`)
takes **~85.5s of wall-clock** on a 14-core dev machine, and it is already
parallel-compressed — 68 `t.Parallel()` calls spread across the test files, so the
machine's cores are saturated and the 85s is the *parallel* time, not the serial sum.
The serial sum is far higher (many individual tests are 7–16s). The dominant cost is
**real `git` subprocess spawns plus endpoint-protection (Cortex XDR) file-scanning of
the files those spawns create in temp dirs**. Every `warp.Add` spawns ~8–10 git
processes (`status`, `rev-parse` ×2, `remote`, `worktree add` ×2, `push` ×2) and
creates a paired host+weft worktree on disk; the EDR scans each new file synchronously.

A prior task already harvested the obvious win: `internal/lyxtest` builds its git
templates **once** (`sync.Once`) and does per-test **filesystem copies** with origin-URL
rewrites (`CopyPaired` / `CopyPairedLocal` / `CopyHostHub` / `CopyWeft`), eliminating
per-test `git init` / `git clone`. What remains is that **many tests still run a full
`warp.Add` purely as setup** — to obtain an *existing* paired worktree — before testing
something else (status, drift, prune, reconcile, remove, cleanup, checkout). That setup
Add is the single largest remaining source of git spawns and EDR file-churn.

**Why now:** The suite is slow enough to be painful in the normal dev loop, and the
EDR scanning makes it disproportionately expensive on this Windows environment. The
template-once infrastructure already exists in `lyxtest` and can be extended to cover
the pre-paired case, so the marginal cost of this optimization is low and the payoff is
concentrated in the heaviest tests.

## Scope

**In:**

- A new `lyxtest` template + `Copy*` helper that captures a **pre-added paired
  worktree** (host worktree + weft worktree on the mirrored branch) built **once** via
  `sync.Once`, then copied per-test with the git-pointer rewrites needed to relocate it
  (`.git` files, per-worktree `gitdir` back-pointers, origin URLs).
- Migration of **setup-only `.Add()` tests** to the new fixture: tests that call
  `.Add()` solely to establish an existing pair and then exercise a *different* subject
  (status / drift / prune / reconcile / remove / cleanup / checkout, and the eligible
  `weftwiring` cases). They recreate portal junctions / launchers per-test only if the
  test actually needs them (cheap filesystem syscalls, no git spawn).
- Micro-cleanups: ensure every push-irrelevant test uses `CopyPairedLocal` (SkipPush)
  rather than `CopyPaired`; remove redundant per-test git setup that the template now
  provides; widen `SkipGit`/`SkipPush` use where a test does not assert on push state.
- Optional, low-priority production-code trims in the `warp` git path **only if** a
  clearly-safe win surfaces (see Decisions → production-code).
- Re-run the verification protocol (operator-run) and record before/after numbers in
  the plan's result file.

**Out:**

- **Cortex XDR / EDR temp-path exclusion.** Confirmed not available — the corporate
  machine is fully monitored and excluded paths are not permitted. No solution may
  depend on an EDR exclusion.
- Adding more parallelism / raising `-parallel`. The suite is already core-saturated;
  oversubscription gave no benefit and triggers the EDR (it killed VS Code during
  exploration). Do **not** introduce burst/oversubscribed test runs.
- Rewriting the `warp` operations' transactional/rollback semantics. Correctness of
  `Add` rollback is not in scope; only test-setup speed is.
- Changing the public CLI behaviour of `lyx warp`.
- The non-warp suites (`board`, `worktree`, `weft`, `paths`) — out of scope except
  insofar as shared `lyxtest` helpers are touched (existing callers must keep compiling
  and passing).

## Decisions

### pre-paired-template — the core lever

- Decision: Add a `sync.Once`-built template that contains a **single already-created
  paired worktree** (fixed slug, e.g. `"task"`) on the mirrored branch — host worktree
  under the hub container and the weft worktree under the weft repo — plus the existing
  hub / bare / weft-prime / weft-bare. Expose a `Copy*` function (e.g.
  `CopyPairedWithWorktree`) that copies the whole container into `tb.TempDir()` and
  rewrites all absolute git pointers so the copy is self-consistent at its new path.
  The template captures **only the expensive git work** (`git worktree add` ×2); it does
  **not** include portal junctions or launchers.
- Rationale: `git worktree add` is the costly, EDR-scanned step. Doing it once and
  copying the result removes ~8–10 git spawns from every setup-only test. Portals
  (`fslink` junctions) and launchers are cheap filesystem operations and can be created
  per-test on demand, so leaving them out of the template keeps it simple and avoids the
  `copyDirRecursive` symlink/junction refusal entirely.
- Rejected:
  - *Copy a fixture produced by `warp.Add` (portals + launchers included).*
    `copyDirRecursive` explicitly refuses symlinks/junctions, and Windows junctions
    store absolute reparse targets that would dangle after copy. Reproducing the git
    worktree pair directly in the template builder (via `git worktree add`, mirroring
    Add's git steps minus portal/launchers/push) avoids both problems.
  - *Batch/replace git in production `Add` to cut spawns.* High blast radius against
    correctness-critical rollback logic for a test-speed goal; deferred to the optional
    production-code decision below.

### git-pointer rewrite on copy

- Decision: Extend the copy step (analogous to the existing
  `rewriteOriginURLInConfig`) to rewrite the absolute paths that bind a git worktree to
  its main repo, for **both** the host and weft pairs:
  1. The worktree's `.git` **file**: `gitdir: <newMainRepo>/.git/worktrees/<name>`.
  2. The per-worktree back-pointer `<newMainRepo>/.git/worktrees/<name>/gitdir`:
     `<newWorktreePath>/.git`.
  3. Verify `<...>/.git/worktrees/<name>/commondir` (normally the relative `../..`,
     which survives a copy — assert/handle if absolute).
  4. Existing origin-URL rewrites for hub and weft-prime (already implemented).
  Use forward-slash formatting (`filepath.ToSlash`) to match the existing rewrite
  helper's Windows-safe approach.
- Rationale: A git worktree is bound to its main repo by two absolute paths (the
  worktree `.git` file → `.git/worktrees/<name>`, and that dir's `gitdir` →
  worktree's `.git`). Both must be relocated or git refuses to operate on the copy.
  This mechanic was confirmed by inspecting a live worktree's pointer files.
- Rejected: *Relative gitdir links.* Git records absolute paths for `worktree add`;
  rewriting on copy is the established pattern in this codebase (origin URLs already do
  exactly this) and is more robust than trying to coerce git into relative links.

### which tests migrate vs keep real Add

- Decision: Migrate a test to the pre-paired fixture **iff** it calls `.Add()` only to
  obtain an existing pair and then asserts on a *different* operation. **Keep real
  `.Add()`** for tests whose subject *is* Add itself: `TestAdd*` (happy path, rollback,
  adopt-existing-weft-branch, dormant), `TestWeftSpawnPushesWeftBranch` (needs the live
  push to weft-bare → must keep `CopyPaired`), and any test asserting on the non-empty
  `BranchPrefix` Add path (e.g. `TestCleanup_LiveBranchNeverDeleted_NonEmptyBranchPrefix`)
  unless the template is made branch-prefix-agnostic.
- Rationale: The fixture's value is removing *setup* Adds; tests that verify Add's own
  behaviour must continue to call it. The fixed-slug, default-`BranchPrefix` template
  cannot stand in for tests that assert on a different slug/prefix.
- Rejected: *Migrate everything.* Would lose coverage of Add's real git side effects
  (push, rollback, adopt).

### fixed-slug, single-pair template

- Decision: The template pre-adds **one** pair under a fixed slug and default
  `BranchPrefix`. Tests needing a different slug, a second pair, or a custom prefix keep
  using `CopyPairedLocal` + `.Add()`.
- Rationale: Keeps the template and the pointer-rewrite logic simple and deterministic.
  Each per-test copy is isolated in its own `tb.TempDir()`, so the shared fixed slug is
  safe under `t.Parallel()`.
- Rejected: *Parameterized multi-pair template.* More rewrite surface and template
  build cost for little gain; the common case is a single existing pair.

### production-code changes — allowed but gated

- Decision: Production (`internal/warp/*.go`) changes are **permitted** but only for a
  clearly-safe, self-contained win (e.g. eliminating a provably-redundant `rev-parse`,
  or a `SkipGit` fast-path that already exists being used more widely). Do **not**
  restructure the transactional Add/rollback flow. If no clean win surfaces, leave
  production code untouched — the fixture lever delivers the bulk of the speedup.
- Rationale: The user explicitly allowed production changes, but the goal is "as fast as
  *safely* achievable." The fixture work is where the safe, large win is; production
  micro-trims are bonus and must not risk rollback correctness.
- Rejected: *Aggressive production refactor of the git path.* Out of proportion to a
  test-speed task and risky.

### verification protocol — operator-run, never bursty

- Decision: The implementer must **not** run heavy or oversubscribed test bursts.
  Verification is operator-driven: a single, non-bursty invocation
  `go test -tags integration -count=1 -v ./internal/warp/ 2>&1` run by the operator,
  compared against the captured baseline. The implementer reasons from
  **git-spawn-count reduction** as the primary proxy and asks the operator to run the
  timed suite at checkpoints.
- Rationale: Cortex XDR flags rapid/parallel git-spawn bursts as suspicious and kills
  VS Code (observed twice during exploration). The operator decides when to run.
- Rejected: *Implementer-run timed suites.* Trips the EDR; unsafe in this environment.

## Technical context

- **Suite entry point:** `go test -tags integration ./internal/warp/`. All 15 warp test
  files carry `//go:build integration`. The package has 32 `.Add()` call sites across 8
  files (`add`, `cleanup`, `drift`, `hook`, `prune`, `reconcile`, `remove`,
  `weftwiring`).
- **Fixture layer — `internal/lyxtest/lyxtest.go`** (the place to extend):
  - Template builders cached via `sync.Once`: `buildHostHub` (hub + bare, README
    commit), `buildWeftPrime` (sibling `<hub>-weft` with `_lyx/config/placeholder` +
    bare), `buildWeftOnly` (upstream-tracking weft).
  - Per-test copies: `CopyHostHub`, `CopyPaired`, `CopyPairedLocal` (omits weft-bare for
    SkipPush, ~25% cheaper), `CopyWeft`. Each uses `copyDirRecursive` (which **refuses
    symlinks** — important: the pre-paired template must not contain junctions) and
    `rewriteOriginURLInConfig` (single `url =` line under `[remote "origin"]`, forward-
    slash formatted — the model to follow for the new gitdir rewrites).
  - **Leaf invariant (CONSTRAINTS.md):** `lyxtest` may import only stdlib +
    `internal/paths`. The new template/copy code must stay within that — no `configreg`
    or feature-package imports.
- **Production `warp.Add`** (`internal/warp/add.go`): the git steps the template builder
  must reproduce directly (host `git worktree add -b <branch> <target>`; weft create via
  `createWeftWorktree` forking from the parent weft branch, or adopt). The template
  builder should mirror just the git-worktree-creation steps — **not** `createPortal`,
  `writeLaunchers`, or the pushes.
- **Portals/launchers** (`internal/warp/portals.go`, `launchers.go`,
  `internal/fslink`): junction creation is a filesystem syscall via
  `fslink.CreateDirLink` (Windows directory junctions, no privileges) — cheap, no git
  spawn. Tests that need a portal/launcher after using the pre-paired fixture create it
  on demand.
- **Git worktree pointer mechanics (confirmed):** worktree `.git` file holds
  `gitdir: <mainRepo>/.git/worktrees/<name>`; `<mainRepo>/.git/worktrees/<name>/gitdir`
  holds `<worktree>/.git`; `commondir` is relative. These absolute pointers are what the
  copy step must rewrite.
- **Paths invariant (CONSTRAINTS.md):** all geometry via `internal/paths` helpers
  (`paths.ConfigDir`, `paths.ConfigFile`, `paths.LyxDirName`, `Layout.*`), in test code
  too. No raw `os.Getwd` / `git rev-parse --show-toplevel` outside `internal/paths` and
  `cmd/lyx/main.go` (enforced by `internal/paths/enforcement_test.go`).

## Constraints

From `CONSTRAINTS.md`:

- **Path invariant:** resolve all cwd/worktree/`_lyx`/config paths through
  `internal/paths` helpers — including in tests. Banned primitives (`os.Getwd`,
  `git rev-parse --show-toplevel`) are enforced at `go test` time.
- **lyxtest leaf invariant:** `internal/lyxtest` imports only stdlib + `internal/paths`;
  never `configreg` or a feature package. Enforced by
  `internal/lyxtest/leaf_enforcement_test.go` on every `go test ./...`. The new
  template/copy code lives here and must obey it.

Environmental (from discussion):

- **Cortex XDR** monitors the machine; heavy/bursty parallel git-spawn runs are flagged
  and can kill VS Code. No EDR temp-path exclusion is available. Implementer must avoid
  bursty test runs; verification is operator-driven.

## Testing

The "tests" here are largely the fixtures being optimized, so correctness of the new
fixture is paramount — a subtly-broken copied worktree would silently weaken every
migrated test.

- **`lyxtest` new template/copy (TDD candidate):** Add a focused unit test (in a package
  that may legally exercise `lyxtest`, e.g. `internal/lyxtest/lyxtest_test.go` or a warp
  test) asserting that a copy of the pre-paired fixture is a *working* git worktree pair:
  `git status` clean in both host and weft worktrees, `git worktree list` from each main
  repo lists the copied worktree at its **new** path, the branch is the expected
  mirrored branch, and no pointer references the template's original temp path. This is
  the guard that the gitdir rewrites are complete.
- **Migrated warp tests:** must remain behaviourally identical — same assertions, same
  coverage, only the setup swapped from `.Add()` to the pre-paired fixture (+ on-demand
  portal/launcher creation where the test needs it). After migration, each previously-
  passing test must still pass.
- **Retained real-Add tests:** `TestAdd*`, `TestWeftSpawnPushesWeftBranch`, and the
  non-empty-`BranchPrefix` cases keep exercising the live git path.
- **Parallel safety:** every migrated test keeps `t.Parallel()`; each gets its own
  `tb.TempDir()` copy. Confirm no shared mutable state is introduced by the fixed-slug
  template.
- **Regression guard for sibling suites:** `go test ./...` (untagged) and the other
  integration suites that import `lyxtest` must still build and pass — the new helper
  must not change existing `Copy*` signatures/behaviour.
- **Verification (operator-run):** baseline is the single-invocation
  `go test -tags integration -count=1 -v ./internal/warp/`. Record per-test and total
  before/after. Implementer reasons from git-spawn-count reduction between checkpoints
  and requests an operator run to confirm.

### Baseline (captured during discussion, single clean run)

- Total: `ok internal/warp 85.462s`.
- Heaviest tests (illustrative): `TestWeftSpawnNoExcludeEntry`/forkpoint/pair-in-sync and
  reconcile/prune/status/checkout cluster at **7–16s each**; these are precisely the
  setup-only-`Add` tests targeted by the pre-paired fixture.
- Cheap tests (config/list/derive/template) are already <0.05s and are not targets.

## Q&A log

- **Q:** Is the suite slow because of missing parallelism? **A:** No — 68 `t.Parallel()`
  calls already saturate the 14 cores; 85s is the parallel-compressed time. The cost is
  per-test git spawns + EDR scanning.
- **Q:** Which levers should the task pursue? **A:** Pre-paired fixture template (primary)
  + test micro-cleanups. **Not** an EDR exclusion.
- **Q:** Is a Cortex-XDR-excluded temp path available? **A:** No — the employer monitors
  everything and does not permit excluded paths. Solutions must not depend on it.
- **Q:** May production (non-test) code change? **A:** Allowed, but only for clearly-safe
  self-contained wins; do not restructure Add/rollback. Fixture work carries the speedup.
- **Q:** Success bar? **A:** As fast as *safely* achievable — take the high-confidence
  wins, keep fixtures correct; no hard number.
- **Q:** How is speedup verified given the EDR constraint? **A:** Operator runs the timed
  suite (single, non-bursty invocation); the implementer analyzes and never triggers
  heavy/oversubscribed runs (it killed VS Code twice during exploration).
- **Q:** Can a `warp.Add`-produced fixture (with portals/launchers) just be copied?
  **A:** No — `copyDirRecursive` refuses junctions and Windows junctions store absolute
  targets. The template captures only the git worktree pair; portals/launchers are
  recreated per-test (cheap `fslink` syscalls).
