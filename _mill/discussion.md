# Discussion: Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks

```yaml
task: 'Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks'
slug: test-suite-regression
status: discussing
parent: main
```

## Problem

A fresh timing run (2026-07-11, repeated and confirmed 2026-07-12 with medians of
3 runs) shows the test suite regressed hard since the benchmarks were last
recorded (2026-06-23): Tier 1 — the default offline loop documented as ~3.5 s —
now runs **~44 s** (~13×), Tier 2 runs **~181 s** (was ~65 s), and **two Tier 2
packages fail** (`internal/initengine`, `internal/ideengine`). The ~a-dozen
modules landed since (mux/shuttle/burler/perch/warp/stencil/selfreport/
modelspec/…) brought untagged git-spawning tests into the offline tier, and one
of the red packages exposes a real product bug in `lyx ide menu`.

The benchmark docs have already been re-baselined with the fresh numbers and the
where-the-time-goes analysis (commit `932c6f9`, 2026-07-12, on this task branch).
**This discussion covers the remaining implementation work**: fix the two reds,
restore the fast offline Tier 1, machine-enforce the tier premise so it cannot
rot silently again, and re-baseline the docs once more after the fixes land.

## Scope

**In:**

- Fix `internal/initengine` `TestInit_FirstRun` (stale module-count assertion).
- Fix the `lyx ide menu` product bug (`ideengine/menu.go` never sets
  `boardengine.Config.Path`) — this also fixes the three red `ideengine` menu
  tests.
- Re-tier the untagged git-spawning / fixture-copying tests in
  `internal/boardcli`, `internal/perchcli`, `internal/muxcli`,
  `internal/configcli`, and `cmd/lyx` behind `//go:build integration`, keeping
  genuinely spawn-free tests untagged.
- Add a machine-enforced **Test Tier Purity Invariant** (grep-guard test +
  CONSTRAINTS.md entry, same commit).
- Post-fix re-baseline of `docs/benchmarks/test-suite-timing.md` and
  `running-tests.md` (new dated current block; the 2026-07-12 regression block
  moves to the trend log; the "premise violated" warnings in running-tests.md
  are removed and the honest new numbers recorded).

**Out:**

- The sandbox suite (S0–S9) and smoke tests (`//go:build smoke`) — not run by
  the timing harness, not part of this task.
- Tier 2 / `warpengine` optimization. warpengine (~127 s, floor of Tier 2) is
  real git-worktree/junction I/O by design; its cost is re-baselined in the
  docs, not optimized here. If wanted, that is its own future task.
- Any change to per-batch `verify:` cadence (guardrail below).
- Optimizing test *implementations* beyond re-tiering (e.g. spawn-free `.git`
  skeleton fixtures, shared-binary builds) — rejected as invasive; see
  Decisions.
- `docs/benchmarks/board-performance.md` — checked 2026-07-12; nothing stale.
- `docs/roadmap.md` — this is regression fixing, not a planned milestone.

## Decisions

### initengine-assertion-derives-from-registry

- Decision: Replace the hardcoded `len(result.Modules) != 3` in
  `internal/initengine/init_test.go:62` with a registry-derived expectation:
  `want := len(configreg.Modules())` (import `internal/configreg` in the test).
  `Init` → `configsync.ReconcileAll` iterates `configreg.Modules()`, so the
  registry is the single source of truth and the assertion can never go stale
  again. The existing loop asserting `board`/`warp`/`weft` config files exist
  stays as-is (those three remain registered).
- Rationale: the assertion broke precisely because it was hardcoded (3 → 7
  registered modules); deriving kills the failure class, matching the repo's
  "generate mechanical facts from source" principle (CONSTRAINTS.md, CLI
  invariant).
- Rejected: updating `3` → `7` (rots again on the next registered module).

### ide-menu-sets-board-path

- Decision: In `internal/ideengine/menu.go` (`Menu`, after the
  `boardengine.LoadConfig` call at line 40), set
  `cfg.Path = hubgeometry.BoardDir(l.Hub)` before `boardengine.New(cfg)`.
- Rationale: `boardengine.Config.Path` is `yaml:"-"` — LoadConfig never sets it;
  the caller must (per the board-dir-geometry migration; see
  `internal/boardengine/config.go:20-24` and the Hub Geometry Invariant:
  the board dir is geometry, `hubgeometry.BoardDir(hub)`). `boardcli/cli.go:103`
  was migrated; `menu.go` was missed, so `HealthCheck()` stats an empty path —
  the exact red-test error (`Stat : path not found`). A grep of all
  `boardengine.LoadConfig` / `boardengine.New` production callers confirms
  `menu.go` is the only missed caller. The ideengine menu tests construct
  `Layout{Hub: container}` and place the board at `<container>/_board` =
  `hubgeometry.BoardDir(Hub)`, so the fix makes the three red tests pass
  without touching them. This was verified end-to-end on 2026-07-12: with this
  one-line fix (plus the initengine assertion fix), both red packages pass
  under `-tags integration`.
- Rejected: dismissing the failure as environment-specific (disproved — it is
  deterministic and reproducible); adding a `--board-path` flag to `ide menu`
  (YAGNI — menu is an interactive picker, not a scripting surface).

### restore-tier1-by-consistent-tagging

- Decision: Move the git-spawning / fixture-copying untagged tests behind
  `//go:build integration`, following the pattern `idecli`, `initcli`,
  `weftcli`, and `warpcli` already use. Genuinely spawn-free tests stay
  untagged so every package keeps a Tier 1 presence where one exists. The
  concrete split (verified compiling and green on 2026-07-12 before being
  reverted pending this review):
  - `internal/boardcli/cli_test.go` → whole file integration-tagged **except**
    `TestCLINoArg` and `TestCLIUnknownSubcommand` (neither reaches layout
    resolution), which move to a new untagged `cli_unit_test.go` together with
    the in-process `runCLI` helper (an untagged file's symbols are visible to
    the tagged file in the same package; `seedCwd` stays in the tagged file).
  - `internal/perchcli/cli_test.go` → the five fixture tests
    (`TestRunCLI_Pause_InvalidRunID`, `_FinishedBlockRefused`,
    `_NestedInitAnchorsRunDirsAtCwd`, `_NoSuchRun`,
    `_WritesFlagAndIsIdempotent`) plus the `seedPerchFixture` helper move to a
    new integration-tagged `cli_integration_test.go`; the cobra-seam tests
    (`TestRunCLI_NoArgs`, `_UnknownSubcommand`, `_GroupGuard_OutsideGitRepo`,
    `TestCommand_EveryCommandHasShort`, `TestRunCLI_Pause_MissingRunID`) stay.
  - `internal/perchcli/run_test.go` → the three weft-sync tests
    (`TestRunCLI_Run_WeftSyncRunsOnEngineError`, `_WeftCommitExcludesLockFiles`,
    `_BusyBlockSkipsWeftSync`) plus the `gitLsFiles`/`gitLogOneline` helpers
    move to a new integration-tagged `run_integration_test.go`; the flag-shape
    and `decodeProfile` tests stay.
  - `internal/muxcli/cli_test.go` → the three fixture tests
    (`TestRunCLI_ResolvesLayoutAndConfig`, `TestRunCLI_AddNotUp_FriendlyError`,
    `TestRunCLI_RemoveNotUp_FriendlyError`) move to a new integration-tagged
    `cli_integration_test.go`; `TestRunCLI_NoArgs`, `_UnknownSubcommand`,
    `_NotAGitRepo`, `TestAttachArgv` stay.
  - `internal/configcli` → `TestReconcile_DryRun`, `TestReconcile_Apply`
    (reconcile_test.go) and `TestDispatchSet_PreservedKeyDetectedByReconcile`
    (configcli_test.go) spawn `git init`; move them into the existing
    integration-tagged `configcli_integration_test.go` (or a sibling
    integration file per source file — implementer's choice, same effect).
  - `cmd/lyx/main_test.go` → `TestRunDispatchesToBoard`,
    `TestRunBoardErrorPropagatesExitCode`, `TestRunDispatchesToConfigReconcile`
    spawn `git init`; move to a new integration-tagged file. The remaining
    dispatch/help-tree/registration/drift tests are in-process cobra and stay.
  - `cmd/lyx/crosscompile_test.go` (`TestCrossCompileLinux`) → add
    `//go:build integration` to the whole file. It cross-compiles the entire
    module (`GOOS=linux go build ./...`); the durable Linux gate still runs on
    every Tier 2 run. Its file doc comment must be updated in the same commit
    to say the gate lives in the integration tier now.
- Rationale: consistent with the established sibling precedent; verified to
  drop boardcli from ~38 s to ~2.4 s and perchcli from ~23-28 s to ~3.2 s in
  Tier 1 during the 2026-07-12 dry run. Expected post-change Tier 1 wall-clock:
  roughly 10–15 s (contention floor across ~50 packages) — the docs record
  whatever is actually measured, no promised number. Tier 2 absorbs the moved
  tests (they already ran there; its wall-clock change is marginal).
- Rejected: optimize-in-place (spawn-free `.git` skeleton fixtures, building
  the lyx binary once) — partial win at best since `hubgeometry.Resolve` spawns
  `git rev-parse` per `RunCLI` on the production path, and far more invasive;
  docs-only re-baseline (leaves the offline loop 13× slower than its purpose);
  keeping `TestCrossCompileLinux` untagged (an 8 s whole-module cross-compile
  has no place in the constantly-run loop) or giving it a third tag (new
  machinery, YAGNI).

### test-tier-purity-invariant

- Decision: Add a repo-wide grep-guard test `cmd/lyx/tierpurity_test.go`
  (untagged, so it runs on every `go test`) that walks the module's `*_test.go`
  files and **fails** if a file lacking a `//go:build integration` (or `smoke`)
  tag references any banned token: `gitexec.RunGit`, `exec.Command`,
  `exec.CommandContext`, or `lyxtest.Copy`. Matching is **raw substring over
  the file's source text** (prefix semantics: `lyxtest.Copy` matches
  `lyxtest.CopyPaired`, `lyxtest.CopyPairedLocal`, `lyxtest.CopyHostHub`, and
  any future `Copy*` fixture; `exec.Command` matches `exec.CommandContext`) —
  never whole-token or AST matching, so the guard cannot silently miss a new
  fixture-copy variant. Because matching is raw substring, a banned token
  appearing in a comment or string literal of an untagged test file also
  trips the guard — that is accepted (rename the mention or tag the file),
  and it is exactly why the guard file must allowlist itself. Allowlist (with
  one-line reasons, mirroring the sandbox-coverage test's `excludedModules`
  style): `internal/proc` (process control is the package's subject — its
  tests must spawn) and `cmd/lyx/tierpurity_test.go` itself (contains the
  banned token strings as its own test data).
  Record the invariant in CONSTRAINTS.md in the same commit, in the established
  format (statement, mechanics, **Enforced by** line).
- Rationale: the regression happened silently across a dozen module landings
  because the tier premise was review-discipline only; the repo's culture is
  exactly this kind of cheap machine guard (`hubgeometry/enforcement_test.go`,
  `lyxtest/leaf_enforcement_test.go`, `cmd/lyx/sandbox_coverage_test.go`).
  `cmd/lyx` is the established home for repo-wide guards. Token grep over test
  files is deliberately shallow: it catches direct spawn/fixture usage — the
  entire observed failure class — without import-graph analysis. Tests that
  merely call production code which spawns internally (e.g. `RunCLI` →
  `hubgeometry.Resolve` on an error path) are not caught and not banned: a
  single failing `rev-parse` is cheap; the guard targets fixtures and loops.
- Rejected: review-discipline only (already proven to rot); a full
  import-graph/AST analysis (heavier machinery for no additional observed
  failure class — YAGNI).

### tier2-rebaseline-only

- Decision: Tier 2's regression (~65 s → ~181 s, warpengine ~127 s floor) is
  recorded honestly in the docs; no optimization work in this task.
- Rationale: the cost is real integration I/O by design (the doc's own framing:
  "slow by design"); warpengine has ~2× the test surface of the `worktree`
  module it replaced. Optimization is a separable, independently-reviewable
  task.
- Rejected: bundling a warpengine fixture/parallelism optimization pass into
  this task (scope creep on a regression-fix task).

### guardrail-verify-stays-package-scoped

- Decision: Nothing in this task changes per-batch `verify:` scope. A fast
  offline tier is a test-hygiene goal, not a verify-cadence change. Plan
  batches verify with package-scoped `go test ./<pkg>/...` (plus
  `-tags integration` where the touched tests are gated) as usual.
- Rationale: explicit guardrail from the task body / builder discussion.
- Rejected: n/a (constraint, not a choice).

## Technical context

- **Fresh numbers and full analysis** live in
  `docs/benchmarks/test-suite-timing.md` ("Current best times", 2026-07-12
  block, commit `932c6f9`): Tier 1 ~44 s / Tier 2 ~181 s medians, per-package
  cost tables for both tiers, slowest-test tables, and the attribution-noise
  caveat (per-package elapsed is contention-inflated; sum ~300–450 s vs ~44 s
  wall — trust wall-clock only).
- **Where Tier 1 time goes:** `boardcli` ~38–40 s (31 `seedCwd` `git init`s +
  `git rev-parse` per `RunCLI` via `hubgeometry.Resolve` —
  `internal/hubgeometry/hubgeometry.go:101`), `perchcli` ~23–28 s
  (`lyxtest.CopyPaired[Local]` fixture copies + git-assertion helpers),
  `cmd/lyx` ~22–24 s (`TestCrossCompileLinux` + 3 `git init` tests), `muxcli`
  ~16–18 s (fixture copies), `configcli` ~6–10 s (3 `git init`s).
- **The brief's clihelp attribution was wrong:** `internal/clihelp`
  `TestExecute_*` are pure in-memory cobra trees; their 4–5 s elapsed is
  scheduler noise under parallel-package contention, not real cost. Do NOT
  "optimize" clihelp.
- **boardengine config contract:** `Config.Path` is `yaml:"-"`; caller sets it
  post-`LoadConfig` (`internal/boardengine/config.go`). `boardcli/cli.go:103`
  is the reference implementation of the correct pattern.
- **Same-package build-tag mechanics:** an untagged `_test.go` file's
  identifiers are visible to an integration-tagged file in the same package
  (the tagged file sees a superset), so shared helpers must live in the
  *untagged* file when both need them (`runCLI` in boardcli), and
  fixture-touching helpers must live in the *tagged* file (`seedCwd`,
  `seedPerchFixture`, `gitLsFiles`, `gitLogOneline`) or the untagged build
  breaks on unused imports / unreferenced spawn code.
- **`lyxtest` is production-side helper code** (not `_test.go`), so the
  grep-guard on test files does not flag `lyxtest`'s own implementation; its
  package tests are already integration-tagged.
- **Timing harness:** `cmd/testtiming` (`-full` for Tier 2, `-top N` for the
  slowest-tests table). Warm the build first; `-count=1` is set by the
  harness. Exit code mirrors `go test`.
- **Prior art for the guard:** `cmd/lyx/sandbox_coverage_test.go`
  (allowlist-with-reason pattern, vacuous-glob protection),
  `internal/hubgeometry/enforcement_test.go` (token-scan mechanics),
  `internal/lyxtest/leaf_enforcement_test.go` (import allowlist).
- **Existing integration-tagged siblings** (the precedent to match):
  `internal/idecli/cli_test.go`, `internal/initcli/initcli_test.go`,
  `internal/weftcli/cli_test.go`, `internal/warpcli/warp_test.go`,
  `internal/configcli/configcli_integration_test.go`.
- Scratch full timing outputs (tier1-run[1-3].txt, tier2-run[1-3].txt) were
  captured in the session scratchpad; the doc tables are the durable record.

## Constraints

- **Hub Geometry Invariant** (CONSTRAINTS.md): the board dir is
  `hubgeometry.BoardDir(hub)` — the menu.go fix must use it, never a literal
  `_board`. Geometry tokens stay out of every other package.
- **lyxtest Leaf Invariant**: untouched — no new lyxtest imports of feature
  packages.
- **CLI / Cobra Invariant**: re-tiering must not delete any contract test —
  tests move between build tags, names preserved. Help texts unaffected
  (except crosscompile_test.go's file doc comment). The pinned help-tree /
  drift / registration tests in `cmd/lyx` stay untagged (in-process cobra).
- **New invariant added by this task**: Test Tier Purity Invariant — untagged
  test files must not spawn processes or copy git fixtures
  (`gitexec.RunGit` / `exec.Command*` / `lyxtest.Copy*` banned outside the
  allowlist); enforced by `cmd/lyx/tierpurity_test.go` on every `go test`;
  recorded in CONSTRAINTS.md in the same commit.
- **Documentation Lifecycle / Task completion**: docs update in the same
  commit as the behaviour change; `docs/roadmap.md` untouched (bugfix/hygiene,
  not a milestone).
- **Guardrail**: per-batch `verify:` stays narrowly package-scoped.

## Testing

- **TDD candidates:**
  - `cmd/lyx/tierpurity_test.go` — write the guard first; it must FAIL on the
    current tree (boardcli/perchcli/muxcli/configcli/cmd-lyx untagged
    spawners), then pass after the re-tiering. This proves the guard actually
    detects the failure class it exists for.
  - The two red packages are natural red→green TDD: `go test -tags integration
    ./internal/initengine ./internal/ideengine -count=1` fails before, passes
    after (verified achievable 2026-07-12).
- **Re-tiering equivalence:** the post-change test-name set must equal the
  pre-change set — nothing deleted, only re-tiered. Check with
  `go test -tags integration ./... -list '.*'` union vs baseline (the
  established equivalence-guardrail discipline from the trend log). Tests keep
  their names; only file/tag placement changes.
- **Both tiers green:** `go test ./... -count=1` (Tier 1, now spawn-free) and
  `go test -tags integration ./... -count=1` (Tier 2, including the two fixed
  packages) both exit 0.
- **Timing re-baseline:** after fixes, run `go run ./cmd/testtiming` and
  `go run ./cmd/testtiming -full` (median of 3 warm runs each, same method as
  the 2026-07-12 block), then update `test-suite-timing.md` (new current
  block; 2026-07-12 block into the trend log) and `running-tests.md` (drop the
  premise-violated warnings, record the new honest figures).
- **Scenario coverage to preserve:** boardcli JSON/exit-code contract, perch
  pause/run flag shapes, mux friendly errors — all continue to run in Tier 2;
  the untagged remainders keep each package visible in Tier 1.

## Q&A log

- **Q:** Tier 1 restoration strategy — tag consistently, optimize in place, hybrid, or docs-only? **A:** [auto-pick] Tag consistently per the idecli/initcli/weftcli/warpcli precedent, keeping spawn-free tests untagged. **Why:** consistent with established pattern, low-risk, verified effective in a dry run (boardcli ~38 s → ~2.4 s); optimize-in-place is invasive and partial, docs-only leaves the loop 13× slow.
- **Q:** Machine-enforce the tier premise? **A:** [auto-pick] Yes — grep-guard test + CONSTRAINTS.md invariant. **Why:** the premise rotted silently precisely because it was review-discipline only; cheap guards are repo culture.
- **Q:** initengine fix — derive from registry or update the constant? **A:** [auto-pick] Derive from `configreg.Modules()`. **Why:** kills the staleness class, matches "generate mechanical facts from source".
- **Q:** Tier 2 scope — re-baseline only or optimize warpengine too? **A:** [auto-pick] Re-baseline only. **Why:** integration cost is by design; optimization is a separable reviewed task.
- **Q:** `TestCrossCompileLinux` destination? **A:** [auto-pick] Behind `integration`. **Why:** whole-module cross-compile doesn't belong in the constantly-run loop; the durable gate still fires on every Tier 2 run; a third tag is new machinery (YAGNI).
- **Q:** Is the ideengine red environment-specific or a real bug? **A:** Diagnosed as a real product bug (missed `Config.Path` migration in `menu.go`) — deterministic, reproduced, single missed caller confirmed by grep; fix verified green on 2026-07-12.
