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

- A new `lyxtest` **golden pre-paired template** that captures a healthy host+weft
  worktree pair (on the mirrored branch) built **once** via `sync.Once`, then a
  **copy-on-write** helper that hands each test a cheap private copy with the git
  pointers rewritten to its new path.
- Migration of **setup-only `.Add()` tests** to the copy-on-write fixture: tests that
  call `.Add()` solely to establish an existing pair and then exercise a *different*
  subject (status / drift / prune / reconcile / remove / cleanup / checkout, and the
  eligible `weftwiring` cases). They re-wire portal junctions / launchers per-copy only
  if the test needs them (cheap filesystem syscalls, no git spawn).
- **Subtest consolidation where it is provably safe** (read-only scenario groups that can
  share one copy run sequentially): collapse several near-duplicate test functions into
  one parent that builds the fixture once.
- Micro-cleanups: ensure every push-irrelevant test uses `CopyPairedLocal` (SkipPush)
  rather than `CopyPaired`; remove redundant per-test git setup the template now
  provides; widen `SkipGit`/`SkipPush` use where a test does not assert on push state.
- Optional, low-priority production-code trims in the `warp` git path **only if** a
  clearly-safe win surfaces (see Decisions → production-code).
- Re-run the verification protocol (operator-run) and record before/after numbers in
  the plan's result file.

**Out:**

- **Sharing one *live* worktree pair across multiple concurrently-running tests.**
  Rejected on technical grounds (see Decisions → rejected-broad-sharing): git serializes
  on its own `index`/refs/`*.lock` files even for read commands, so parallel tests
  against one repo race; and most warp tests mutate their fixture in test-specific ways.
  Copy-on-write is what makes "one expensive build, many cheap isolated tests" safe.
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

- Decision: Add a `sync.Once`-built **golden** template containing a **single
  already-created paired worktree** (fixed slug, e.g. `"task"`) on the mirrored branch.
  The builder assembles a **fresh single container** in one `MkdirTemp` root with the
  hub and weft-prime **co-located as siblings** (so `WeftRepoRoot = <hub-base>-weft` and
  `WeftWorktreePath = <slug>-weft` resolve correctly), commits the seed content, then
  runs the two `git worktree add` calls (host worktree + weft worktree) directly —
  mirroring `warp.Add`'s git-worktree-creation steps **only**, with no portal, no
  launcher, no hook, and no push. Expose a copy-on-write helper (e.g.
  `CopyPairedWithWorktree`) that copies the whole container into `tb.TempDir()` and
  rewrites all absolute git pointers so the copy is self-consistent at its new path.
- Rationale: `git worktree add` is the costly, EDR-scanned step. Doing it once and
  copying the result removes ~8–10 git spawns from every setup-only test. Building the
  hub and weft-prime in **one** container (not the two separate `MkdirTemp` roots the
  current `buildHostHub`/`buildWeftPrime` use) is required because `git worktree add`
  bakes absolute sibling paths into the pair; a fresh combined container keeps the
  geometry consistent for the copy-rewrite.
- Rejected:
  - *Copy a fixture produced by `warp.Add` (portals + launchers included).*
    `copyDirRecursive` explicitly refuses symlinks/junctions, and Windows junctions
    store absolute reparse targets that would dangle after copy. Reproducing the git
    worktree pair directly in the template builder avoids both problems.
  - *Reuse the existing separate hub/weft-prime templates as-is.* They live in distinct
    temp roots, so a worktree pair added across them would bind paths that the copy
    cannot keep consistent.

### copy-on-write isolation — why not share one live pair

- Decision: Every test that needs a pair takes its **own** cheap copy of the golden
  template (filesystem copy + pointer rewrite). Full per-test isolation is retained; all
  migrated tests keep `t.Parallel()`.
- Rationale: This is the safe realization of "build once, run many." At ~14-way
  parallelism only ~14 copies are ever live at once, so the suite already behaves like "a
  few worktrees exercised by many tests" — just with temporal isolation. The cost traded
  away (a filesystem copy) is far cheaper than the cost removed (8–10 git spawns), and
  isolation keeps tests order-independent and parallel-safe.
- Rejected: see next decision.

### rejected-broad-sharing — one repo, many concurrent tests

- Decision: Do **not** share a single live worktree pair across multiple
  concurrently-running tests (zero-copy shared fixture).
- Rationale: Two independent reasons make it unsafe, not merely unconventional:
  1. **Git serializes on its own locks.** Even "read-only" git commands (`status`,
     `worktree list`, `rev-parse`) write `.git/index`, refs, and `*.lock` files. Two
     parallel tests issuing git against the same repo race on `index.lock`/ref locks →
     flaky failures. Warp operations all spawn git, so almost no warp test is git-free.
  2. **Most tests mutate their fixture in test-specific ways** — pollute `_lyx`
     (`TestStatus_LyxPollutionDetected`), create `_codeguide` pollution
     (`TestStatus_CodeguidePollutionReportOnly`), remove a junction
     (`TestStatus_JunctionHealth`), diverge a branch (drift tests), delete worktrees /
     branches (remove / cleanup / prune / reconcile). Sharing one pair would corrupt
     siblings.
- Rejected alternative kept here for the record: a zero-copy package-level golden used
  directly by "read-only" tests. It only stays safe for tests that issue **no git** on
  the fixture **and** run sequentially — a small minority — so it is not worth a second
  fixture tier. The safe sharing we *do* take is the narrower "subtest consolidation"
  below.

### subtest-consolidation — the bounded extra win

- Decision: Where several existing test functions build the same fixture and differ only
  in a cheap assertion/mutation, consolidate them into one parent test that obtains the
  fixture once. Read-only scenarios run as **sequential** subtests sharing that one copy
  (sequential avoids the git-lock race); any scenario that mutates state runs against its
  **own** copy-on-write pair (either a fresh copy per mutating subtest, or kept as a
  separate top-level test). Parents stay `t.Parallel()` so subjects still overlap.
- Rationale: This captures the "many tests, few builds" intent safely: it cuts the number
  of fixture builds for read-mostly clusters (e.g. the status scenarios) without exposing
  shared mutable state to concurrent git. The win is bounded (read-only clusters are a
  minority) but it also reduces EDR file-churn, which matters on this machine.
- Rejected: *Blanket consolidation of mutating tests under one shared copy.* Sequential
  mutating subtests corrupt state for later subtests; restoring between them reintroduces
  git work. Mutating scenarios therefore keep independent copies.

### which tests migrate vs keep real Add

- Decision: Migrate a test to the copy-on-write fixture **iff** it calls `.Add()` only to
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

### fixed-slug template + permitted assertion changes

- Decision: The golden template pre-adds **one** pair under a fixed slug and default
  (empty) `BranchPrefix`, so `branch == slug`. The returned fixture **exposes the slug,
  the branch, and the host + weft worktree paths as struct fields** (alongside the
  existing `Hub`/`Bare`/`WeftPrime`/`WeftBare`/`Layout`). Migrated tests read those
  fields; the **only** permitted assertion change during migration is substituting the
  old test-chosen slug/branch string for the fixture's exposed value. Behaviour and
  coverage are otherwise identical. Tests needing a *specific* slug, a second pair, or a
  custom prefix keep using `CopyPairedLocal` + `.Add()`. Extending the existing
  `PairedFixture` struct with the new slug/branch/host-worktree/weft-worktree fields is
  backward-compatible (current callers ignore the added fields), so that is the preferred
  shape over a brand-new struct; the plan picks one concrete struct + helper name (the
  `CopyPairedWithWorktree` name here is illustrative) so migrated tests bind to a fixed
  contract.
- Rationale: Keeps the template and pointer-rewrite logic simple and deterministic, while
  being explicit that "behaviourally identical" still allows the mechanical slug-string
  swap. Each per-test copy is isolated in its own `tb.TempDir()`, so the shared fixed
  slug is safe under `t.Parallel()`.
- Rejected: *Parameterized multi-pair / arbitrary-slug template.* More rewrite surface
  and build cost for little gain; the common case is a single existing pair.

### git-pointer rewrite on copy

- Decision: Extend the copy step (analogous to the existing `rewriteOriginURLInConfig`)
  to rewrite the absolute paths that bind a git worktree to its main repo, for **both**
  the host and weft pairs:
  1. The worktree's `.git` **file**: `gitdir: <newMainRepo>/.git/worktrees/<name>`.
  2. The per-worktree back-pointer `<newMainRepo>/.git/worktrees/<name>/gitdir`:
     `<newWorktreePath>/.git`.
  3. `<newMainRepo>/.git/worktrees/<name>/commondir`: expected to be the relative
     `../..` (which survives a copy). **Fail-fast assert** if it is absolute — the
     template invariant guarantees relative; an absolute value means a git-version
     surprise and must error loudly rather than be silently rewritten, keeping the copy
     deterministic across git versions.
  4. Existing origin-URL rewrites for hub and weft-prime (already implemented).
  Use forward-slash formatting (`filepath.ToSlash`) to match the existing rewrite
  helper's Windows-safe approach.
- Rationale: A git worktree is bound to its main repo by two absolute paths (the
  worktree `.git` file → `.git/worktrees/<name>`, and that dir's `gitdir` → worktree's
  `.git`). Both must be relocated or git refuses to operate on the copy. This mechanic
  was confirmed by inspecting a live worktree's pointer files.
- Rejected: *Relative gitdir links.* Git records absolute paths for `worktree add`;
  rewriting on copy is the established pattern in this codebase (origin URLs already do
  exactly this).

### portals / launchers / hook omitted from the template

- Decision: The golden template captures the git worktree pair **only**. Portal
  junctions, launchers, and the **post-checkout hook** are deliberately omitted and
  recreated on-demand per copy where a test needs them (`createPortal` / `writeLaunchers`
  / `InstallPostCheckoutHook` are filesystem-only, no git spawn).
- Rationale: Junctions can't survive `copyDirRecursive` (it refuses them) and store
  absolute targets; keeping them out avoids that entirely. The hook is a non-fatal
  belt-and-suspenders side effect of `Add` (add.go:153); omitting it is low-impact
  because the hook tests (`hook_test.go`) call `InstallPostCheckoutHook` themselves and
  do not rely on the fixture pre-installing it.
- Rejected: *Bake portals/hook into the template.* Reintroduces the junction-copy
  problem and stale absolute targets for negligible benefit.

### production-code changes — allowed but gated

- Decision: Production (`internal/warp/*.go`) changes are **permitted** but only for a
  clearly-safe, self-contained win (e.g. eliminating a provably-redundant `rev-parse`,
  or wider use of an existing `SkipGit` fast-path). Do **not** restructure the
  transactional Add/rollback flow. If no clean win surfaces, leave production code
  untouched — the fixture lever delivers the bulk of the speedup.
- Rationale: The user explicitly allowed production changes, but the goal is "as fast as
  *safely* achievable." The fixture work is where the safe, large win is.
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
    commit, **own MkdirTemp root**), `buildWeftPrime` (sibling `<hub>-weft` with
    `_lyx/config/placeholder` + bare, **separate MkdirTemp root**), `buildWeftOnly`
    (upstream-tracking weft). The new golden builder must instead co-locate hub +
    weft-prime in one container (see pre-paired-template decision).
  - Per-test copies: `CopyHostHub`, `CopyPaired`, `CopyPairedLocal` (omits weft-bare for
    SkipPush, ~25% cheaper), `CopyWeft`. Each uses `copyDirRecursive` (which **refuses
    symlinks/junctions**) and `rewriteOriginURLInConfig` (single `url =` line under
    `[remote "origin"]`, forward-slash formatted — the model to follow for the new
    gitdir rewrites).
  - **Leaf invariant (CONSTRAINTS.md):** `lyxtest` may import only stdlib +
    `internal/paths`. The new template/copy code must stay within that — no `configreg`
    or feature-package imports.
- **Production `warp.Add`** (`internal/warp/add.go`): the git steps the template builder
  must reproduce directly (host `git worktree add -b <branch> <target>`; weft create via
  the equivalent of `createWeftWorktree` forking from the parent weft branch). The
  template builder reproduces **only** those git-worktree-creation steps — not
  `createPortal`, `writeLaunchers`, `InstallPostCheckoutHook`, or the pushes.
- **Portals/launchers/hook** (`internal/warp/portals.go`, `launchers.go`, `hook.go`,
  `internal/fslink`): junction creation is a filesystem syscall via
  `fslink.CreateDirLink` (Windows directory junctions, no privileges) — cheap, no git
  spawn. Tests re-wire on demand after copy.
- **Git worktree pointer mechanics (confirmed):** worktree `.git` file holds
  `gitdir: <mainRepo>/.git/worktrees/<name>`; `<mainRepo>/.git/worktrees/<name>/gitdir`
  holds `<worktree>/.git`; `commondir` is relative. These absolute pointers are what the
  copy step must rewrite.
- **Git concurrency:** git commands write `.git/index`, refs, and `*.lock` even for
  read-style operations; this is why two parallel tests cannot safely share one live
  repo (motivates copy-on-write and the sequential-only rule for shared read-only
  subtests).
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

- **`lyxtest` golden template + copy (TDD candidate):** Add a focused unit test (in a
  package that may legally exercise `lyxtest`, e.g. `internal/lyxtest/lyxtest_test.go` or
  a warp test) asserting that a copy of the pre-paired fixture is a *working* git
  worktree pair: `git status` clean in both host and weft worktrees, `git worktree list`
  from each main repo lists the copied worktree at its **new** path, the branch is the
  expected mirrored branch, and no pointer references the template's original temp path.
  This is the guard that the gitdir rewrites (and the `commondir` fail-fast) are correct.
  The guard test is **`//go:build integration`-tagged** (its natural home,
  `internal/lyxtest/lyxtest_test.go`, is integration-tagged), so it adds no git spawns to
  the untagged `go test ./...` regression run.
- **Migrated warp tests:** behaviourally identical — same assertions, same coverage —
  except for the permitted slug/branch-string substitution to the fixture's exposed
  fields, with setup swapped from `.Add()` to the copy-on-write fixture (+ on-demand
  portal/launcher/hook creation where the test needs it). After migration, each
  previously-passing test must still pass.
- **Consolidated subtests:** where read-only scenarios are folded under one parent, the
  subtests run sequentially and assert the same things they did as separate functions;
  any mutating scenario in the cluster keeps its own copy. No assertion is dropped.
- **Retained real-Add tests:** `TestAdd*`, `TestWeftSpawnPushesWeftBranch`, and the
  non-empty-`BranchPrefix` cases keep exercising the live git path.
- **Parallel safety:** every migrated top-level test keeps `t.Parallel()`; each gets its
  own `tb.TempDir()` copy. Shared read-only subtests run sequentially within their
  parent. Confirm no shared mutable state is introduced by the fixed-slug template.
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
  setup-only-`Add` tests targeted by the copy-on-write fixture.
- Cheap tests (config/list/derive/template) are already <0.05s and are not targets.

## Q&A log

- **Q:** Is the suite slow because of missing parallelism? **A:** No — 68 `t.Parallel()`
  calls already saturate the 14 cores; 85s is the parallel-compressed time. The cost is
  per-test git spawns + EDR scanning.
- **Q:** Which levers should the task pursue? **A:** Pre-paired fixture template (primary)
  + test micro-cleanups + safe subtest consolidation. **Not** an EDR exclusion.
- **Q:** Why one worktree per test instead of a few worktrees shared by many tests?
  **A:** Because git serializes on its own `index`/refs/`*.lock` files even for read
  commands (parallel tests would race), and most warp tests mutate their fixture in
  test-specific ways. Copy-on-write gives "build once, many cheap isolated tests" while
  staying parallel- and git-safe; broad live-sharing is rejected.
- **Q:** Is a Cortex-XDR-excluded temp path available? **A:** No — the employer monitors
  everything and does not permit excluded paths. Solutions must not depend on it.
- **Q:** May production (non-test) code change? **A:** Allowed, but only for clearly-safe
  self-contained wins; do not restructure Add/rollback. Fixture work carries the speedup.
- **Q:** Success bar? **A:** As fast as *safely* achievable — pick what works best; keep
  fixtures correct; no hard number.
- **Q:** How is speedup verified given the EDR constraint? **A:** Operator runs the timed
  suite (single, non-bursty invocation); the implementer analyzes and never triggers
  heavy/oversubscribed runs (it killed VS Code twice during exploration).
- **Q:** Can a `warp.Add`-produced fixture (with portals/launchers/hook) just be copied?
  **A:** No — `copyDirRecursive` refuses junctions and Windows junctions store absolute
  targets. The template captures only the git worktree pair; portals/launchers/hook are
  recreated per-copy (cheap `fslink`/file ops).
