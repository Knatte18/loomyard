# Discussion: Move config templates home by removing the lyxtest→configreg edge

```yaml
task: Move config templates home by removing the lyxtest->configreg edge
slug: config-test-cleanup
status: discussing
parent: docs-move-psmux-tui
```

## Problem

The default-config `.yaml` templates (`board`, `worktree`, `weft`) currently live in a
synthetic leaf package `internal/configtmpl` instead of with the feature module each one
belongs to. That package only exists to work around a **test-build import cycle**, and the
cycle itself was introduced by a single avoidable import edge. We want the templates back
home — embedded via `//go:embed` in `internal/board/`, `internal/worktree/`,
`internal/weft/` (the pre-`c374501` state) — and `internal/configtmpl` deleted, *without*
reintroducing the cycle.

**Why now / what happened (verified against git history):**

1. Commit `6d24098` ("fix(lyxtest): initialize weft fixture config files via configreg
   templates") added `internal/lyxtest` → `internal/configreg` as a new import, so the
   weft-prime test fixture would contain real config instead of a neutral placeholder. Its
   stated purpose was to fix exactly one failing test: `TestE2ESyncIntegration`.
2. That single new edge closed a cycle. The other two edges were always harmless on their
   own: `configreg → feature` (to read `ConfigTemplate()`) and `feature` internal test →
   `lyxtest`. With `lyxtest → configreg` added, any feature package whose **internal**
   (`package <pkg>`) tests import `lyxtest` now had a test-build cycle:
   `weft/worktree internal test → lyxtest → configreg → weft/worktree`. `worktree` and
   `weft` failed to build under `-tags integration`; `board` escaped only because its tests
   use the external `package board_test`.
3. Commit `c374501` then invented `internal/configtmpl` to cut the *first* edge
   (`configreg → feature`) — a workaround for a workaround. It renamed each feature's
   `internal/<feature>/template.yaml` → `internal/configtmpl/<feature>.yaml`.

The correct fix is to remove the **new** edge (`lyxtest → configreg`) instead of cutting an
old harmless one. Then templates can live in their feature packages and `configtmpl`
is unnecessary.

## Scope

**In:**

- Revert the `configtmpl` extraction: restore each feature's own embedded template at
  `internal/<feature>/template.yaml` (clean revert to the pre-`c374501` filename) with the
  `//go:embed template.yaml` + `ConfigTemplate()` accessor in each feature's `template.go`.
- `internal/configreg/configreg.go`: `Modules()` references `board.ConfigTemplate` /
  `worktree.ConfigTemplate` / `weft.ConfigTemplate` again (re-importing the feature
  packages, as before `c374501`).
- **Delete `internal/configtmpl/` entirely** (`.go` + the three `.yaml` files).
- Remove the `lyxtest → configreg` edge: `internal/lyxtest/lyxtest.go` drops the
  `configreg` import; `buildWeftPrime` no longer seeds via `configreg.Modules()` and instead
  writes the neutral pre-6d24098 placeholder. `lyxtest` becomes a pure leaf again
  (stdlib + `internal/paths` only).
- Add a config-seeding helper to `lyxtest` with a **configreg-free signature** (see
  Decision: lyxtest-seed-helper) and call it from **every `CopyPaired` consumer that triggers
  a config `Load`** (at least `TestE2ESyncIntegration` and `TestRunCLI_EnvMapToOption`; the
  implementer must enumerate the full set — see Decision: where-real-config-is-needed) to
  seed the real config those tests depend on.
- Record the "lyxtest must stay a leaf" invariant in `internal/lyxtest/doc.go` and in
  `CONSTRAINTS.md`.
- Run `go vet -tags integration ./...` as part of this task's per-batch verify so the cycle
  (and any reintroduction during the work) fails immediately.
- **Separate batch (last):** tidy `internal/lyxtest` — extract shared git helpers, resolve
  the `buildWeftOnly` fixture asymmetry, dead-helper audit.

**Out:**

- Do **not** switch templates from `//go:embed` to runtime file reads.
- Do **not** keep `configtmpl` or introduce per-feature leaf sub-packages.
- **No CI.** This repo has no `.github/workflows` and we are not adding any. No GitHub
  Actions gate is created.
- **No enforcement test** for the import edge. A circular import here is a thing to simply
  never write; it does not warrant a dedicated source-scanning test. (The cycle is also
  build-fatal under `-tags integration`, so the per-batch verify catches a regression
  anyway.)
- No broad test-suite rewrite — the suite is healthy (no TODO/FIXME, no disabled tests).
  Keep it surgical outside the explicit tidy batch.
- Do not change `paths.ConfigDir` / `paths.ConfigFile` or the `_lyx/config/<module>.yaml`
  layout.

## Decisions

### templates-home-filename

- Decision: Restore the templates to `internal/<feature>/template.yaml` (singular
  `template.yaml`, embedded with `//go:embed template.yaml`), exactly reversing `c374501`'s
  rename. Each feature's `template.go` embeds its own `template.yaml` and exposes
  `ConfigTemplate() string`.
- Rationale: This is the literal pre-`c374501` state — the cleanest possible revert, lowest
  risk, and `git show c374501` confirms the original names were `template.yaml`.
- Rejected: Naming them `board.yaml` / `worktree.yaml` / `weft.yaml` in the feature dirs
  (as the proposal text loosely suggested) — introduces a needless rename vs. the original
  and buys nothing.

### configreg-references-features

- Decision: `configreg.Modules()` returns `{"board", board.ConfigTemplate}`,
  `{"worktree", worktree.ConfigTemplate}`, `{"weft", weft.ConfigTemplate}`, importing the
  three feature packages directly. The `Module` struct and `Template`/`Names` helpers are
  unchanged.
- Rationale: Restores the pre-`c374501` design. Safe because the only cycle-closing edge
  (`lyxtest → configreg`) is being removed in the same task; `configreg → feature` was
  always harmless on its own. Verified importers of `configreg` are `configcli`,
  `configcli/menu`, `configsync`, and `lyxtest` — none are feature packages, and the
  feature packages do not import `configcli`/`configsync`, so no new cycle forms.
- Note (round-1 review): `internal/configreg/configreg_test.go` is `package configreg` and
  already imports `weft` (line 8, uses `weft.ConfigTemplate()` at line 36). After the revert,
  production `configreg` also imports the feature packages, so the internal test shares that
  import — this is harmless: feature packages do not import `lyxtest` (or `configreg`) from
  non-test code, and `configreg` is never reached by a feature package's test build. No cycle.
- Rejected: Keeping `configtmpl` as the template source — defeats the entire purpose
  ("templates home").

### lyxtest-leaf

- Decision: `internal/lyxtest` imports only the standard library and `internal/paths`.
  `buildWeftPrime` writes the neutral pre-6d24098 fixture: create `_lyx/config/` and write a
  `placeholder` file (`paths.ConfigDir(weftPrime)` + a `placeholder` entry with content
  `"weft config"`), then commit. No `configreg` import, no real config in the shared
  template.
- Rationale: A feature package's internal tests import `lyxtest`; if `lyxtest` reaches
  `configreg` (which reaches the feature packages) the test build cycles. Keeping `lyxtest`
  a leaf is the structural fix. The placeholder restores the exact neutral fixture that
  existed and worked before `6d24098`, keeping the `_lyx/config/` directory present for any
  consumer that expects it to exist.
- Rejected: Embedding template copies inside `lyxtest` (duplicates the single source of
  truth); seeding real config in the shared template via any feature/registry import
  (reintroduces the edge).
- Note (round-1 review): the neutral placeholder lives at `paths.ConfigDir(weftPrime)` =
  `_lyx/config/placeholder`. A separate existing consumer,
  `internal/weft/weft_integration_test.go:104`, writes a raw-literal
  `filepath.Join(fixture.WeftPrime, "_lyx", "placeholder")` = **`_lyx/placeholder`** (directly
  under `_lyx`, *not* under `config/`) purely to have a file to modify. The two paths differ,
  so there is no collision with the neutral fixture's `_lyx/config/placeholder`. That literal
  also predates and is independent of this change; it qualifies for the path-helper cleanup
  (it should use `paths.LyxDirName`) — fold it into the tidy/constraint batch
  (Decision: lyxtest-tidy-separate-batch), not the cycle-fix batch.

### lyxtest-seed-helper

- Decision: Add `lyxtest.SeedConfig(tb testing.TB, repoDir string, configByModule map[string]string)`
  (or an equivalent local `ConfigSeed{ Name, Content string }` slice — a **lyxtest-local /
  stdlib-only type**, never `[]configreg.Module`). It writes each `paths.ConfigFile(repoDir,
  name)` with the given content, `git add`s, and commits, reusing lyxtest's git helpers.
  `TestE2ESyncIntegration` (in `package configcli`, which may legally import `configreg`)
  builds the plain data on the test side — loop over `configreg.Modules()`, put
  `m.Name → m.Template()` into a `map[string]string` — and calls
  `lyxtest.SeedConfig(t, f.WeftPrime, seeds)` after `CopyPaired`, before the config edit
  exercises the fixture. Feature-internal consumers (`package weft`/`worktree`) instead seed
  only their own feature's template, e.g. `lyxtest.SeedConfig(t, f.WeftPrime,
  map[string]string{"weft": weft.ConfigTemplate()})` — same package, no `configreg` import.
- Rationale (corrected after round-1 review — the original rationale was wrong): the real
  reason the fixture needs real config is **not** that the edited `worktree.yaml` must
  pre-exist. `config.Edit` *scaffolds* a missing `worktree.yaml` from the template
  (`internal/config/edit.go:90-104`) and the fake editor overwrites it, so the edit path
  alone does not need seeded config. The actual dependency is that `weft.RunCLI`
  (`commit`/`push`/`status`) calls `LoadConfig(weftBaseDir)` **before** dispatching any
  subcommand (`internal/weft/cli.go:98`), which calls `config.Load(weftBaseDir, "weft", …)`;
  `config.Load` returns a hard error if `_lyx/config/weft.yaml` is absent
  (`internal/config/config.go:57-60`) **or** missing template keys (`config.go:66-79`).
  The commit pathspec is derived from that `weft.yaml` (`cli.go:104`,
  `internal/weft/config.go` `Pathspec`/`Dirs`). So a fixture without a real, complete
  `weft.yaml` makes `weft commit`/`push` fail and the test fails at `code != 0` — exactly the
  failure `6d24098` (which changed only `lyxtest.go`, placeholder → real config) fixed. The
  helper produces a real, committed `weft.yaml` (and the other modules), which is what the
  consuming tests require. The **critical constraint** (user-mandated): the helper's parameter
  type must be configreg-free. If it took `[]configreg.Module`, `lyxtest` would import
  `configreg` purely for the type and the cycle returns. Passing pure `string→string` data
  keeps the `configreg.Modules()` conversion on the test side, so `lyxtest` still imports
  only stdlib + `internal/paths`.
- Rejected: (a) Raw inline `os.WriteFile` + `exec.Command` git in the test — duplicates the
  boilerplate the tidy batch removes. (b) Seeding into the weft worktree post-`Add` instead
  of committing into weft-prime — relies on edit-flow timing and is less faithful to a real
  committed-config fixture. (c) Any helper signature mentioning a `configreg` type.

### where-real-config-is-needed

- Decision: **Multiple** `CopyPaired` consumers depend on the fixture carrying real config —
  not just one. Every consumer that triggers a `config.Load` (directly or via
  `weft.RunCLI` / `worktree`/`board` config loaders) against the fixture must seed the
  module(s) it loads, using `lyxtest.SeedConfig` (Decision: lyxtest-seed-helper). The rule:
  - In a package that may import `configreg` (e.g. `package configcli`): seed the full set
    by looping over `configreg.Modules()`.
  - In a feature-internal package (`package weft` / `worktree` / `board`, which **cannot**
    import `configreg`): seed only that feature's own template via its in-package
    `ConfigTemplate()` (no import needed). If a feature-internal test needs a *different*
    feature's config, that is the one case requiring care — none found so far; if one
    appears, move the seeding to a package that may import `configreg` rather than
    reintroducing `lyxtest → configreg`.
  - Batch 1 must **enumerate the complete set empirically**: make `buildWeftPrime` neutral,
    run `go test -tags integration ./...`, and for each test that now fails on a missing/
    incomplete config (`config file … not found` / `missing keys`), add the appropriate
    `SeedConfig` call per the rule above.
- Known cases (verified):
  - `TestE2ESyncIntegration` (`internal/configcli/configcli_integration_test.go`,
    `package configcli`) — `dispatch` → `weft.RunCLI("commit")` → `LoadConfig` needs real
    `weft.yaml`. Seeds the full set from `configreg.Modules()`.
  - `TestRunCLI_EnvMapToOption` (`internal/weft/weft_integration_test.go`, `package weft`) —
    `weft.RunCLI("push")` → `LoadConfig` at `cli.go:98` needs real `weft.yaml`. Seeds only
    `{"weft": weft.ConfigTemplate()}` (own package, no `configreg`).
- Not affected (verified): `TestPushIntegration` (`package weft`) uses `CopyWeft` and passes
  the pathspec `["_lyx"]` directly to `Commit` (no config load). Tests that seed their **own**
  literal config inline (e.g. `worktree/cli_test.go:33` writes `branch_prefix: wt-\n` to its
  own `ConfigFile`) are independent of the fixture's seeded config. Whether any other
  `CopyPaired` consumer in `package worktree` (`add_test.go`, `remove_test.go`, `weft_test.go`,
  `cli_test.go`) triggers a config load is to be confirmed by the batch-1 empirical sweep.
- Rationale: round-1 review correctly showed the original "only `TestE2ESyncIntegration`"
  claim was false. The dependency is on config-loading code paths (`config.Load` errors on a
  missing/incomplete config), so the affected set is "every fixture consumer that loads
  config," determined by running the suite — not assumed.
- For the plan writer: the exact list of consumers to touch is **not statically estimable**
  and must not be guessed up front. It is determined at implementation time by the batch-1
  sweep (e.g. `worktree/weft_test.go` has full-`CopyPaired` cases at lines ~252/~336 that push
  to weft-bare and may or may not load config — confirmed only by running the suite). Plan
  the `SeedConfig` edits as "apply to each consumer the sweep flags," not as a fixed file set.
- Rejected: Seeding real config in the shared `buildWeftPrime` template (re-creates the
  `lyxtest → configreg` edge); reintroducing `lyxtest → configreg` to let a feature-internal
  test seed a non-own feature's config.

### no-ci-no-enforcement-test

- Decision: No GitHub Actions / CI is created, and **no** dedicated source-scanning
  enforcement test for the import edge is added. The cycle is guarded by (1) this task's
  per-batch verify running `go vet -tags integration ./...` (cycle is build-fatal there),
  and (2) the documented invariant in `CONSTRAINTS.md` + `internal/lyxtest/doc.go`.
- Rationale: User decision. The repo has no CI and won't grow one for this. The user's
  position: a circular import like this must simply never be written, and that does not need
  to be asserted by an explicit test. The integration build already fails loudly if the edge
  returns.
- Rejected: Standing up `.github/workflows/ci.yml`; adding a `lyxtest` leaf-enforcement test
  analogous to `internal/paths/enforcement_test.go`.

### lyxtest-tidy-separate-batch

- Decision: The `lyxtest` cleanup (extract shared git helpers `initRepo` /
  `initBareRemote` / `commitAll` used by `buildHostHub` / `buildWeftPrime` / `buildWeftOnly`;
  resolve the `buildWeftOnly` fixture asymmetry; dead-helper audit) is done in **its own
  final batch**, after the cycle fix lands and the full suite is green.
- Guard (round-2 review): `buildWeftOnly` writes a literal `_lyx/config.yaml` = `"test"`
  (lyxtest.go:269) — a path *outside* the `paths`-helper layout. Before aligning/renaming it,
  the tidy batch must first confirm no current consumer reads `_lyx/config.yaml` literally
  (grep the `CopyWeft` consumers, e.g. `TestPushIntegration` writes exactly that path at
  weft_integration_test.go:49). If a consumer depends on it, update that consumer in the same
  batch; do not change the fixture path in isolation.
- Rationale: It's logically independent from the cycle fix; isolating it as a separate batch
  keeps a behavior-affecting change (neutral fixture + seeding) apart from a pure refactor,
  so a regression is easy to localize. User chose "include as a separate batch."
- Rejected: Interleaving the refactor with the `buildWeftPrime` edits (mixes refactor with
  behavior change); deferring the tidy to a follow-up task (user wants it in scope).

## Technical context

Key files and current state (all paths under repo root
`internal/`):

- `configtmpl/configtmpl.go` + `configtmpl/{board,worktree,weft}.yaml` — the leaf to delete.
  Embeds the three yaml files; exposes `Board()`/`Worktree()`/`Weft()`.
- `configreg/configreg.go` — `Modules()` currently returns `configtmpl.Board` etc. `Module`
  is `{ Name string; Template func() string }`. Also `Template(name)` and `Names()`.
- `configreg/configreg_test.go:36` — already compares against `weft.ConfigTemplate()`; should
  still resolve after the revert (weft re-embeds its own template). Verify it still passes.
- `board/template.go`, `worktree/template.go`, `weft/template.go` — currently each delegates
  `ConfigTemplate()` to `configtmpl.*`. Revert each to `//go:embed template.yaml` + return the
  embedded string. `*/config.go` call `ConfigTemplate()` via `config.Load(...)` and are
  unaffected. `*/template_test.go` validate YAML shape and should keep passing.
- `lyxtest/lyxtest.go` (~650 lines) — builds three cached git fixtures once per test binary
  via `sync.Once` (`buildHostHub`, `buildWeftPrime`, `buildWeftOnly`), then per-test
  filesystem-copies into `tb.TempDir()` and rewrites the origin URL via
  `rewriteOriginURLInConfig` (pure text edit, no git spawn — a deliberate invariant: zero
  per-test git spawns). Public copy helpers: `CopyHostHub`, `CopyPaired`, `CopyPairedLocal`,
  `CopyWeft`. `buildWeftPrime` (lines ~135–221) is where the `configreg.Modules()` seeding
  loop (lines ~176–182) lives and must become the neutral placeholder.
- `lyxtest/doc.go` — package doc; add the leaf invariant here.
- `configcli/configcli_integration_test.go` — `//go:build integration`, `package configcli`,
  imports `lyxtest`, `paths`, `weft`, `worktree`. `TestE2ESyncIntegration` calls
  `lyxtest.CopyPaired(t)` then `worktree.Add(... SkipPush:true)`; the seeded config must be
  committed in `f.WeftPrime` **before** `worktree.Add` creates the weft worktree (a git
  worktree only checks out committed content), so `SeedConfig` must commit.
- `paths.ConfigDir(base)` → `<base>/_lyx/config`; `paths.ConfigFile(base, module)` →
  `<base>/_lyx/config/<module>.yaml`. Always use these (CONSTRAINTS.md), including in tests.

Import-graph facts (verified):

- Importers of `lyxtest`: `board/boardtest/{git,sync}_test.go`,
  `configcli/configcli_integration_test.go`, `gitclone/clone_integration_test.go`,
  `ide/cli_test.go`, `paths/{paths,worktreelist}_test.go`, `weft/{cli,status,sync,weft_integration}_test.go`,
  `worktree/{add,cli,launchers,list,portals,remove,weft}_test.go`, plus `configtmpl` (being
  deleted). Several are feature-internal (`package weft` / `package worktree`) — that is
  exactly why `lyxtest` must not reach `configreg`/features.
- Importers of `configreg`: `configcli/configcli.go`, `configcli/menu.go`,
  `configsync/configsync.go`, `lyxtest/lyxtest.go` (edge to remove). None are feature
  packages.
- Importers of `configtmpl`: `board/template.go`, `configreg/configreg.go`,
  `weft/template.go`, `worktree/template.go` — all rewritten/removed by the revert.

`buildWeftOnly` asymmetry to resolve in the tidy batch: it writes a singular
`_lyx/config.yaml` with literal content `"test"` (line ~269), a **different path** from the
`_lyx/config/<module>.yaml` layout `buildWeftPrime` uses. Decide whether this is intentional;
align to the `paths` helpers or add a comment explaining why it differs.

## Constraints

From `CONSTRAINTS.md` (repo root) — both apply directly here:

- **`_lyx` and config-file paths must go through `internal/paths` helpers, in test code too.**
  Use `paths.LyxDirName`, `paths.ConfigDir(base)`, `paths.ConfigFile(base, module)` — never
  string literals like `filepath.Join(base, "_lyx", "config")` or `"board.yaml"`. The new
  `SeedConfig` helper and the neutral-placeholder write in `buildWeftPrime` must use these
  helpers. (The placeholder file itself is not a `<module>.yaml` config path, but the
  `_lyx/config/` directory it sits in must be derived via `paths.ConfigDir`.) Exceptions are
  `internal/paths/*_test.go` and `_lyx` used as link-target geometry / string assertions.
- **Path Invariant:** cwd / worktree-root queries go through `internal/paths.Getwd()` /
  `Resolve()`; raw `os.Getwd` / `git rev-parse --show-toplevel` are banned outside
  `internal/paths` and `cmd/lyx/main.go`, enforced by `internal/paths/enforcement_test.go`.
  Nothing in this task should touch those primitives.

New invariant to record (this task): **`internal/lyxtest` must stay a leaf — it must not
import `internal/configreg` or any feature package (`board`/`worktree`/`weft`)**, because
feature packages have internal tests that import `lyxtest`; such an import closes a
test-build cycle (the `6d24098` trap). Tests that need real config seed it themselves from a
package that may import `configreg` (e.g. `configcli`), passing plain data into
`lyxtest.SeedConfig`. Record this in `internal/lyxtest/doc.go` and add a corresponding entry
to `CONSTRAINTS.md`.

## Testing

The change is behavior-preserving for production config loading; the risk is entirely in the
test build/fixtures. Verification per area:

- **Feature template revert** (`board`/`worktree`/`weft`): existing `template_test.go` in
  each package (`TestConfigTemplate_ValidYAML`, `_HasRequiredKeys`/`_HasPathspecKey`/
  `_HasBranchPrefixKey`, `_ResolvesTo*`) must still pass — they assert YAML shape from
  `ConfigTemplate()`. `configreg/configreg_test.go:36` (`want := weft.ConfigTemplate()`) must
  still pass.
- **`lyxtest` neutral fixture + seed helper**: this is the TDD-worthy new code. Add/extend a
  `lyxtest` test (in `internal/lyxtest/lyxtest_test.go`) for `SeedConfig` — that after
  seeding, `paths.ConfigFile(repoDir, module)` exists with the expected content and is
  committed (e.g. `git ls-files` lists it). Confirm `CopyPaired` of the neutral fixture
  contains the `_lyx/config/placeholder` and no real `<module>.yaml`.
- **Real-config consumers (more than one — see Decision: where-real-config-is-needed)**:
  `TestE2ESyncIntegration` (`configcli`) and `TestRunCLI_EnvMapToOption` (`weft`) must stay
  green after switching to seed via `lyxtest.SeedConfig`. The configcli case is the canary
  that the seeding-at-test-site approach works end to end (host worktree add → config edit →
  weft commit → host stays pristine); the weft case verifies the feature-internal
  seed-own-`ConfigTemplate()` path.
- **Completeness sweep (required before the cycle fix is considered done):** make
  `buildWeftPrime` neutral, then run the full `go test -tags integration ./...` and treat
  every failure of the form `config file … not found` / `missing keys` as a fixture consumer
  that needs seeding. Fix each with `SeedConfig` per the rule: configreg-importing packages
  seed the full set from `configreg.Modules()`; feature-internal packages seed their own
  feature's `ConfigTemplate()`. **Never** reintroduce `lyxtest → configreg`. Do not assume
  the set is closed at the two known cases — the suite run is the source of truth.
- **Build/cycle gate (per batch):** `go build ./...` clean; `go vet -tags integration ./...`
  reports no import cycle and no findings; full `go test -tags integration ./...` green
  (26 packages at last count), including `TestE2ESyncIntegration`.
- **Tidy batch**: after extracting `initRepo`/`initBareRemote`/`commitAll`, the full
  integration suite must remain green — the refactor is pure structure, no fixture-content
  change beyond the `buildWeftOnly` asymmetry resolution.

Done-conditions to assert at the end:

- `internal/configtmpl` no longer exists; `grep -rn "configtmpl" --include=*.go .` is empty.
- `grep -n "configreg" internal/lyxtest/lyxtest.go` is empty (lyxtest is a leaf again).
- Each template `.yaml` resides under its module's directory (`internal/<feature>/template.yaml`).
- `CONSTRAINTS.md` and `internal/lyxtest/doc.go` both record the leaf invariant.

## Q&A log

- **Q:** How does the one test that needs real config seed it once the fixture is neutral?
  **A:** A `lyxtest.SeedConfig` helper that does the write + git add + commit, called from
  the test. **Critical:** its parameter must be a configreg-free type (`map[string]string`
  module→yaml, or a lyxtest-local `ConfigSeed`), NOT `[]configreg.Module` — otherwise lyxtest
  imports configreg for the type and the cycle returns. The test (in `configcli`) builds the
  map from `configreg.Modules()` and passes plain data in.
- **Q:** Persistent gate against a future cycle, given no CI? **A:** No CI (repo has none,
  not adding one) and **no enforcement test** — a circular import must simply never be
  written and doesn't warrant an explicit test. Rely on this task's per-batch
  `go vet -tags integration ./...` (cycle is build-fatal) + the documented invariant.
- **Q:** Where to record the leaf invariant? **A:** `internal/lyxtest/doc.go` comment **and**
  `CONSTRAINTS.md` (which exists at repo root).
- **Q:** Scope of the lyxtest tidy (helper extraction, fixture asymmetry, dead-helper audit)?
  **A:** Include it in this task as its own separate, final batch (after the cycle fix is
  green).
- **Q:** Template filename when moved home? **A:** Restore `internal/<feature>/template.yaml`
  (exact revert of `c374501`), not `<feature>.yaml`.
- **Q:** Neutral fixture content for `buildWeftPrime`? **A:** The pre-6d24098 placeholder —
  `_lyx/config/placeholder` containing `"weft config"`, via `paths.ConfigDir`.
- **Q (round-1 review GAP):** Does a neutral placeholder suffice (drop `SeedConfig`), since
  `config.Edit` scaffolds the missing `worktree.yaml`? **A:** No — pushed back with evidence.
  The edited `worktree.yaml` *is* scaffolded+overwritten, but `weft.RunCLI` loads `weft.yaml`
  for its commit pathspec **before** any subcommand (`cli.go:98`), and `config.Load` errors on
  a missing/incomplete `weft.yaml` (`config.go:57-79`); `6d24098` (lyxtest-only change) fixed
  exactly this. So real config is required. **But** the review correctly exposed that the
  "only one test needs real config" claim was false — `TestRunCLI_EnvMapToOption` (`package
  weft`) needs it too. Resolution: keep `SeedConfig`, correct the rationale, and seed in every
  `CopyPaired` consumer that loads config (enumerated empirically in batch 1), with
  feature-internal tests seeding their own `ConfigTemplate()`.
- **Q (round-1 review NOTEs):** Placeholder path collision and configreg-test cycle-safety?
  **A:** Applied both. `_lyx/placeholder` (weft_integration_test.go:104) ≠ the neutral
  `_lyx/config/placeholder`, no collision; that literal is flagged for the path-helper tidy.
  `configreg`'s own internal test importing `weft` is harmless post-revert (features never
  import `lyxtest`/`configreg` in non-test code).
