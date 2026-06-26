# Discussion: Speed up internal/warp integration tests

```yaml
task: Speed up internal/warp integration tests
slug: warp-test-speedup
status: discussing
parent: main
```

## Problem

The `internal/warp` integration suite (`go test -tags integration ./internal/warp/`)
takes **~85.5s of wall-clock** on a 14-core dev machine. It is already parallel-
compressed — 68 `t.Parallel()` calls — yet measurement shows the suite is **EDR/disk-
bound, not CPU-bound**: the serial sum of work is only ~2–3× the wall time, i.e. adding
cores barely helps. Endpoint protection (Cortex XDR) scans every file the tests create
in temp dirs, and that scanning plus git process spawns serialize the suite.

The cost concentrates in **per-test fixture builds**. Each test copies a git-repo
template (via `lyxtest.CopyPairedLocal` / `CopyPaired` / `CopyHostHub`) and runs git
operations against it. There are **76 such fixture builds** across the suite. A prior
task already removed the obvious cost (per-test `git init`/`clone` → build-once-copy via
`sync.Once`), so what remains is the sheer **number and size** of those copies plus the
git spawns each test performs.

**Why now:** The suite is slow enough to hurt the dev loop, and the EDR scanning makes
it disproportionately expensive on this Windows environment.

### Measured economics (captured during discussion)

- Single-threaded fixture-build test (copy + a few git spawns, no `Add`): **2.86s**;
  with `Add`: **3.76s** (so `Add` ≈ 0.9s). Of the 2.86s, the filesystem copy is ~1.5s
  and **git process spawns are ~1.3s** — git spawns are roughly half the per-test cost.
- Each template is **45 files, 28 of them inert `.git/hooks/*.sample`** (62%). A
  `CopyPairedLocal` copies ~3 such repos (~135 files, ~84 of them hook samples).
- Stripping hook samples from the templates: **2.86s → 2.44s (~15%)**, test still passes.
- Effective parallelism ≈ 2–3× (≈180–250s serial work → 85s wall). **More parallelism
  is not the lever; reducing total file churn and git spawns is.**

## Scope

**In:**

1. **Strip inert git hook samples** (`.git/hooks/*.sample`) from the `lyxtest` template
   builders so every fixture build copies ~60% fewer files. One small change in
   `internal/lyxtest/lyxtest.go`, benefiting all ~76 builds.
2. **Consolidate fixture builds** per the 13-group map below: fold near-duplicate / read-
   only tests into siblings as subtests or pre-condition assertions, reducing builds
   **76 → 59 with no loss of coverage**.
3. **Aggressively delete low-value tests** per the delete list below: 1 exact-duplicate +
   8 defensive/subset paths covered elsewhere, reducing builds **59 → 50** (−34% from 76).
4. Re-run the verification protocol (operator-run) and record before/after numbers in the
   plan's result file.

**Out:**

- **`copy-on-write` / pre-paired-worktree template.** Rejected: it swaps `Add` (~0.9s of
  git spawns) for a *larger* filesystem copy — net ≈ zero, possibly negative. It does not
  address the bottleneck (file churn + spawns).
- **Hardlinking `.git/objects` in the copy.** Verified hardlinks need no admin on this
  machine, but the templates have a tiny history (one commit), so object files are a
  handful — hardlinking saves almost nothing here. Dropped.
- **Dropping the bare remote from fixtures.** Same reason — marginal for these tiny repos;
  the residual cost is git spawns, not bytes. Dropped.
- **Cortex XDR / EDR temp-path exclusion.** Confirmed unavailable — the corporate machine
  is fully monitored; excluded paths are not permitted. No solution may depend on it.
- **More parallelism / raising `-parallel`.** The suite is EDR/disk-bound (~2–3× ceiling);
  oversubscription gave no benefit and tripped the EDR (it killed VS Code during
  exploration). Do **not** introduce burst/oversubscribed test runs.
- Rewriting `warp` operation semantics or the transactional Add/rollback flow.
- Changing the public CLI behaviour of `lyx warp`.
- The non-warp suites except insofar as the shared `lyxtest` helper is touched (existing
  callers must keep compiling and passing).

## Decisions

### hook-sample-strip — biggest per-build, trivial, safe

- Decision: In `internal/lyxtest/lyxtest.go`, after each repo is initialized, delete the
  `*.sample` files git copies into `.git/hooks/` (and `hooks/` for the bare). Apply in
  `initRepo` (covers hub, weft-prime, weft-only) and `initBareRemote` (covers the bares).
- Rationale: 28 of 45 template files are inert sample hooks that never execute (only
  non-`.sample` hook files run). Removing them cuts ~60% of files per copied repo. Measured
  ~15% wall reduction on a fixture-bound test, and it compounds across every build. Zero
  behaviour change — verified a representative test still passes with samples stripped.
- Rejected: *Leave them* — they are pure EDR/copy overhead. *`git init --template=<empty>`*
  — works too, but an explicit post-init strip is simpler and obviously correct.

### consolidate-builds — fewer fixtures, no coverage loss (76 → 59)

- Decision: Apply the 13 consolidation groups below. Each folds near-duplicate or read-
  only tests into a sibling — either as extra assertions on an existing fixture, or as
  sequential subtests sharing one build. Read-only scenarios fold as pre-condition checks
  before a sibling's mutation; sequential mutating scenarios (where the first call leaves
  state intact for the second) share one fixture run in order. No assertion is dropped.
- Rationale: Wall time scales with total fixture work; halving redundant builds removes
  copy + spawn cost with full coverage retained. Sequential subtests are fine because
  parallelism is already EDR-capped at ~2–3×, so serializing within a shared fixture costs
  little wall-clock while cutting churn.
- Rejected: *Per-test isolation everywhere* — keeps redundant builds for no coverage gain.

The groups (parent ← folded tests), each saving the stated number of builds:

| # | Action | Saved |
|---|--------|-------|
| A | Fold `TestAddDormant`, `TestWeftSpawnCreatesWeftDirectory`, `TestWeftSpawnNoExcludeEntry`, `TestWeftSpawnPairedWorktrees` assertions into `TestAdd/HappyPath` | 4 |
| B | Merge `TestWireJunctionsPreservesBehavior` into `TestWireJunctionsIdempotent` (add its line-exact `_lyx` check) | 1 |
| C | Fold `TestPairInSync_InSync` into `TestPairInSync_BrokenJunction` as a pre-remove check | 1 |
| D | Fold `TestStatus_PairedViewFields` into `TestStatus_InSyncVsDrifted` before the drift mutation | 1 |
| E | Delete `TestWeftPrechecks/HardRequireWeftRepo` (exact duplicate of `TestAdd/NoWeftRepo`; keep `RejectExistingWeftWorktree`) | 1 |
| F | Merge `TestPrune_DryRunReportsStaleWeft` + `TestPrune_ApplyRemovesStaleWeft` into one sequential test | 1 |
| G | Merge `TestCleanup_DryRunReportsOrphanBranch` + `TestCleanup_ForceAloneReportsOnly` (both report-only) onto one fixture | 1 |
| H | Combine `TestCleanup_LiveBranchNeverDeleted` + `_NonEmptyBranchPrefix` (both live pairs on one fixture, one Cleanup) | 1 |
| I | Merge `TestRemove/HostDirty{Without,With}Force` and `WeftDirty{Without,With}Force` into two sequential tests | 2 |
| J | Delete `TestInstallPostCheckoutHook_WritesScript` (covered by `_Idempotent` first install) | 1 |
| K | Delete `TestInstallPostCheckoutHook_ChainsExistingHook` (covered by `_ChainIdempotent` first install; add its `post-checkout.user` check) | 1 |
| L | Fold `TestList/SingleWorktree` into `TestList/TwoWorktrees` as a pre-add check | 1 |
| M | Share one `setupCLIRepo` between read-only `TestRunDispatchesToWarp/List` + `/UnknownSubcommand` | 1 |

Total: **17 builds saved (76 → 59).**

### aggressive-delete — drop low-value paths (59 → 50)

- Decision: Delete the following tests outright, accepting the named (minor) coverage loss.
  1 exact-duplicate + 8 defensive/subset paths whose production behaviour is structurally
  guaranteed or covered by a higher-value sibling.
- Rationale: The owner judged the suite over-tested. Each deletion removes one whole
  fixture build. The deleted paths are defensive/duplicate; the load-bearing coverage stays.
- Rejected: deleting the KEEP list below — each is the sole coverage of a real path.

| Test | Why low-value / coverage lost | Risk |
|------|------------------------------|------|
| `TestWriteLaunchers/DotRelPath` | Exact duplicate of `EmptyRelPath` (`"."` resolves identically) | SAFE |
| `TestAdd/UnbornBranch` | Same "detached HEAD" guard as `TestAdd/DetachedHEAD` | LOW |
| `TestWeftForkPointMirrorsHost` | Subset of `_SubtaskIsolation` (which also asserts ≠ main tip) | LOW |
| `TestRemoveHostJunctionRemoved` | Flat-topology case; nested case in `TestRemoveSubpathJunction` exercises the load-bearing path | LOW |
| `TestCreatePortalMultipleSubpaths` | Subpath-collision avoidance is structurally guaranteed by RelPath-mirrored paths | LOW |
| `TestPairInSync_JunctionPointsElsewhere` | Wrong-target case; broken/missing-junction + `Status_JunctionHealth` cover junction failure | LOW |
| `TestStatus_CodeguidePollutionReportOnly` | Detection covered by the `_lyx` pollution test; only checks a transitional report-only flag | LOW |
| `TestWeftMissingParentBranch` | Third rollback test; rollback covered by `TestAddRollback` + white-box `rollbackAdd` | LOW |
| `TestAdd/TargetDirExists` | Defensive precheck; `git worktree add` guards it structurally | LOW |

Total: **9 builds saved (59 → 50).** Combined reduction from original: **76 → 50, −34%.**

### keep-list — do not delete despite looking redundant

- `TestWeftSpawnPushesWeftBranch` — the **only** `CopyPaired` fixture and the **only**
  real-`git push` + weft-bare ref-landing coverage.
- `TestWeftRollbackOnPostHostCreateFailure` — only white-box `rollbackAdd` entry point.
- `TestCleanup_LiveBranchNeverDeleted_NonEmptyBranchPrefix` — cited prefix-mismatch
  regression guard (merged in group H, not deleted).
- `TestWeftPrechecks/RejectExistingWeftWorktree` — unique error path.
- All four `Reconcile_*` and four `Checkout_*` — each plants a distinct failure mode on a
  clean fixture; none read-only.

### production-code changes — allowed, minimal

- Decision: The only production-adjacent change is the hook-sample strip in
  `internal/lyxtest/lyxtest.go` (a test-support package). No `internal/warp/*.go`
  production change is required; do not restructure Add/rollback.
- Rationale: The whole speedup lives in the test/fixture layer. Leave production logic
  untouched.

### expected-gain — honest, operator-verified

- Decision: Target is **~85s → ~50–60s (~25–35% faster)**, not a 2–3× win. The floor is
  that ~50 tests still each copy a real git repo and spawn git under EDR; the transformative
  lever (EDR exclusion) is unavailable.
- Rationale: Modeled from measured per-build cost (~2.4–3.8s), the build-count reduction
  (76 → 50), the ~15% hook-strip, and the EDR-contention relief from less concurrent churn.
  Stated as a range because the contention-relief component is real but not precisely
  measurable without a full run.

### verification protocol — operator-run, never bursty

- Decision: The implementer must **not** run heavy or oversubscribed test bursts. Single
  isolated `-run`/`-p 1` checks are acceptable; the full timed comparison
  (`go test -tags integration -count=1 -v ./internal/warp/ 2>&1`) is run by the **operator**
  and compared against the 85.5s baseline. The implementer reasons from build-count and
  spawn-count reduction as the proxy.
- Rationale: Cortex XDR flags rapid/parallel git-spawn bursts and kills VS Code (observed
  twice during exploration). The operator decides when to run the full suite.

## Technical context

- **Suite entry point:** `go test -tags integration ./internal/warp/`. 15 test files carry
  `//go:build integration`. Baseline `ok internal/warp 85.462s`.
- **Fixture layer — `internal/lyxtest/lyxtest.go`** (where the hook-strip lands):
  - `sync.Once` builders: `buildHostHub`, `buildWeftPrime`, `buildWeftOnly`; low-level
    `initRepo` (git init + config) and `initBareRemote` (git init --bare + remote add) are
    the strip points. `commitAll`, `mustGit` are helpers.
  - Per-test copies: `CopyHostHub` (hub+bare), `CopyPairedLocal` (hub+bare+weft-prime, no
    weft-bare), `CopyPaired` (all four), `CopyWeft`. Each uses `copyDirRecursive` (refuses
    symlinks/junctions) and `rewriteOriginURLInConfig`.
  - **Leaf invariant (CONSTRAINTS.md):** `lyxtest` imports only stdlib + `internal/paths`.
    The hook-strip uses only `os`/`filepath` — invariant preserved.
- **Build inventory (current 76):** CopyPairedLocal 59, CopyPaired 1, CopyHostHub 16. The
  per-file detail and per-test mutate/read classification are in the supporting maps
  produced during discussion (consolidation + aggressive-deletion analyses); the actionable
  group and delete lists are embedded in Decisions above so this file is self-contained.
- **Consolidation mechanics:** read-only tests fold as pre-condition assertions before a
  sibling's mutation (groups C, D, L); sequential mutating tests share a fixture where the
  first call leaves state intact for the second (F, G, I); structural Add-property checks
  collapse into `TestAdd/HappyPath` (A). Sequential subtests must NOT use `t.Parallel`
  between the shared steps; top-level tests keep `t.Parallel`.
- **Paths invariant (CONSTRAINTS.md):** all geometry via `internal/paths` helpers, in test
  code too. No raw `os.Getwd` / `git rev-parse --show-toplevel` outside `internal/paths` and
  `cmd/lyx/main.go` (enforced by `internal/paths/enforcement_test.go`).
- **Template leak (noted, optional):** `buildHostHub` etc. use `os.MkdirTemp` without
  cleanup, leaving `%TEMP%/lyxtest-*` dirs to accumulate (hundreds observed). Not a per-run
  speed cost (built once per binary) but adds EDR backlog; a cleanup hook is an optional
  hygiene follow-up, out of this task's critical path.

## Constraints

From `CONSTRAINTS.md`:

- **Path invariant:** resolve all cwd/worktree/`_lyx`/config paths through `internal/paths`
  helpers — including in tests. Enforced at `go test` time.
- **lyxtest leaf invariant:** `internal/lyxtest` imports only stdlib + `internal/paths`;
  never `configreg` or a feature package. Enforced by
  `internal/lyxtest/leaf_enforcement_test.go`. The hook-strip stays within stdlib.

Environmental (from discussion):

- **Cortex XDR** monitors the machine; heavy/bursty parallel git-spawn runs are flagged and
  can kill VS Code. No EDR temp-path exclusion is available. Implementer avoids bursty runs;
  verification is operator-driven.

## Testing

- **Hook-strip:** after the change, the full suite must still pass (the strip removes only
  inert `*.sample` files; real hooks like the post-checkout hook are unaffected). The hook
  tests (`hook_test.go`) install their own real hooks and do not depend on samples.
- **Consolidated tests:** each merged/folded test asserts exactly what its source tests did
  — same assertions, same coverage — only the fixture build is shared. Read-only fold-ins
  add their assertion before the sibling's mutation; sequential merges assert the
  intermediate (pre-second-call) state too. No assertion dropped. Top-level tests keep
  `t.Parallel`; shared sequential steps do not parallelize among themselves.
- **Deleted tests:** confirm each deleted test's production path remains exercised by the
  sibling named in the delete table (or is structurally guaranteed). The KEEP list must
  remain intact.
- **Regression guard for sibling suites:** `go test ./...` (untagged) and the other
  integration suites that import `lyxtest` must still build and pass — the hook-strip must
  not change any `Copy*` signature or observable fixture behaviour.
- **Verification (operator-run):** baseline `go test -tags integration -count=1 -v
  ./internal/warp/` = 85.462s. Record per-test and total after each milestone (hook-strip;
  consolidation; deletes). Target ~50–60s.

### Baseline (captured during discussion, single clean run)

- Total: `ok internal/warp 85.462s`. Heaviest tests 7–16s each under contention (the
  fixture-bound Add/status/reconcile/prune/weftwiring cluster). Cheap unit-style tests
  (config/list/derive/template) are <0.05s and untouched.

## Q&A log

- **Q:** Is the suite slow because of missing parallelism? **A:** No — 68 `t.Parallel()`
  already; it is EDR/disk-bound (~2–3× effective parallelism). Reducing file churn + git
  spawns is the lever.
- **Q:** Does copy-on-write (pre-paired template) help? **A:** No — measured `Add` ≈ 0.9s,
  swapped for a bigger copy; net ≈ zero. Dropped.
- **Q:** What actually dominates a build? **A:** ~1.5s copy + ~1.3s git spawns; templates
  are 62% inert hook samples. Hence hook-strip (~15%) + fewer builds.
- **Q:** Can we just delete tests? **A:** Yes — owner chose the aggressive tier. 1 exact
  duplicate + 8 defensive/subset deletes (59 → 50), with a KEEP list for sole-coverage paths.
- **Q:** Do hardlinks help (and do they need admin)? **A:** Verified hardlinks need no admin
  here, but templates are tiny (few objects) so hardlinking saves ~nothing. Dropped, along
  with bare-remote-drop (same reason).
- **Q:** Is an EDR-excluded temp path available? **A:** No — employer monitors everything.
- **Q:** Realistic gain? **A:** ~85s → ~50–60s (~25–35%), operator-verified. Not a 2–3×; the
  floor is ~50 real git-repo copies + spawns under EDR.
- **Q:** How is speedup verified given the EDR constraint? **A:** Operator runs the full
  timed suite at milestones; the implementer never triggers heavy/oversubscribed runs.
