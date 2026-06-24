# Discussion: Fix failing TestRunCLI in internal/worktree

```yaml
task: Fix failing TestRunCLI in internal/worktree
slug: fix-worktree-runcli-test
status: discussing
parent: main
```

## Problem

`go test -tags integration ./internal/worktree/` fails — specifically `TestRunCLI`
(`internal/worktree/cli_test.go`), in the `List` and `RemoveWithForceFlag` subtests.
The failure is **pre-existing on `main`** and surfaced 2026-06-24 during the
`weft-orphan-branches` plan review, whose `verify:` had to scope around it with
`-run 'TestWeft|TestAdd|TestSeeder'` to get a green gate.

Root cause is now confirmed: the test fixture `setupCLIRepo` writes the worktree
config to the **stale** path `_lyx/worktree.yaml`, but config loading was migrated to
`_lyx/config/<module>.yaml` in PR #20 ("Extract yamlengine engine and migrate config
loading via lyx update/init"). `worktree.RunCLI` → `LoadConfig` now resolves
`_lyx/config/worktree.yaml` (via `internal/paths`), so it reports
`config file ...\_lyx\config\worktree.yaml not found; run "lyx update"` and returns exit
1. The fixture was never updated in the #20 migration. The sibling unit test
`internal/worktree/config_test.go` already does it correctly via `paths.ConfigFile`.

The operator broadened the scope during discussion: the underlying defect is **hardcoded
path segments in test fixtures** instead of resolving through the `internal/paths`
library. `TestRunCLI` is the only one that *fails today* (the migrated read path no
longer matches its stale write path), but the same anti-pattern — `filepath.Join(base,
"_lyx", "config")` and literal `"board.yaml"` / `"worktree.yaml"` / `"weft.yaml"` — is
repeated across the 15 test files enumerated in the In list below (closed by the
reproducible discovery query + triage rule under Scope). Those happen to still pass
because their literal matches the current
layout, but they are latent breakage: the next path migration re-breaks every one of
them. The hard rule going forward: **tests resolve all `_lyx`/config paths via
`internal/paths`, never hardcode the segments.**

**Why now:** the integration suite for `internal/worktree` is red on `main`, forcing
every future plan touching this package to scope around it. Fixing it removes that tax
and, by extension, hardens every other test against the same migration hazard.

## Scope

**In:**

- Fix `internal/worktree/cli_test.go` `setupCLIRepo` to write config via
  `paths.ConfigDir(f.Hub)` (mkdir) + `paths.ConfigFile(f.Hub, "worktree")` (write) — no
  literal `"_lyx"` or `"worktree.yaml"` segments.
- Apply the same "resolve via `internal/paths`, never hardcode" rule to every other
  **test** that constructs an `_lyx`/config path from string literals, converting:
  - `filepath.Join(X, "_lyx", "config")` → `paths.ConfigDir(X)`
  - `filepath.Join(configDir, "board.yaml")` → `paths.ConfigFile(X, "board")`
  - `filepath.Join(configDir, "worktree.yaml")` → `paths.ConfigFile(X, "worktree")`
  - bare `filepath.Join(X, "_lyx")` → `filepath.Join(X, paths.LyxDirName)`
  Files in scope (config-path violations):
  - `internal/worktree/cli_test.go` (the failing target)
  - `internal/worktree/config_test.go`
  - `cmd/lyx/main_test.go`
  - `internal/config/config_test.go`
  - `internal/config/edit_test.go`
  - `internal/configcli/configcli_test.go`
  - `internal/board/cli_test.go`
  - `internal/board/config_test.go`
  - `internal/board/boardtest/bench_test.go`
  - `internal/initcli/initcli_test.go`
  - `internal/update/update_test.go`
  - `internal/ide/menu_test.go`
  - `internal/configsync/configsync_test.go` (lines 13/19, 67/73, 113, 133 — every
    `configDir := filepath.Join(tmpDir, "_lyx", "config")` construction + `board.yaml`/`weft.yaml`)
  - `internal/weft/config_test.go` (lines 49-56 — `_lyx`/`config` mkdir in
    `TestLoadConfig_HappyPath`; it already writes via `paths.ConfigFile(tmpDir, "weft")`,
    only the mkdir is stale. The `Dirs()` test-data `"_lyx"`/`"_codeguide"` strings at
    lines 21-25 are **string-split inputs, not config paths** — leave them hardcoded.)
  - `internal/configcli/configcli_integration_test.go` (the one borderline relative-path
    assert at line ~78 → `paths.ConfigFile(".", "worktree")`)

  **Sweep discovery & triage (reproducible):** Enumerate candidates with
  `rg -l '"_lyx"' --glob '**/*_test.go'` (≈24 files). Triage each hit:
  - **In** — the literal builds an `_lyx`/config **file or dir** path that a `paths`
    helper covers (`filepath.Join(base, "_lyx", "config")`, `"board.yaml"`,
    `"worktree.yaml"`, `"weft.yaml"`, or a bare `filepath.Join(base, "_lyx")` used as a
    real directory).
  - **Out** — `internal/paths/*_test.go` (the literals *are* the spec under test);
    `_lyx` used as a junction/link **target geometry** or in **string-content / grep-style
    assertions** (`portals_test.go`, `weft_test.go`); `_lyx` used as **test-data input**
    to a parser (`weft/config_test.go` `Dirs()` cases); and `git config` subcommands.
- Confirm `go test -tags integration ./internal/worktree/` is green, and that the full
  build + all touched packages' tests stay green (`go test -tags integration ./...` for
  the affected packages).

**Out:**

- **No production-code changes.** `worktree.RunCLI`, `LoadConfig`, and `internal/paths`
  are correct; this is a test-fixture fix only. If review finds the *production* path
  resolution wrong, that is a separate task.
- `internal/paths/*_test.go` — these test the `paths` library itself; their literal
  `"_lyx"` / `"config"` strings are the specification under test and must stay
  hardcoded.
- `_lyx` used as **junction/link-target geometry** or in **string-content assertions**
  (`internal/worktree/portals_test.go`, `internal/worktree/weft_test.go`) — these are not
  config-path resolution. Not in scope (may optionally adopt `paths.LyxDirName` later,
  but no behavioural reason to touch them now).
- `git config ...` invocations (the word "config" as a git subcommand) — unrelated.
- No new `paths` helpers are added — the existing `ConfigDir`, `ConfigFile`, and
  `LyxDirName` cover every case.

## Decisions

### fix-the-test-fixture-not-the-code

- Decision: Fix the test fixtures; leave production code untouched.
- Rationale: `RunCLI`/`LoadConfig` resolve `_lyx/config/worktree.yaml` correctly via
  `internal/paths`, matching the #20 migration and the on-disk layout produced by
  `lyx init`/`lyx update`. The sibling `config_test.go` already passes against this path.
  Only the `cli_test.go` fixture writes to the obsolete location. Changing production to
  re-accept the old path would regress the migration.
- Rejected: Make `LoadConfig` fall back to `_lyx/<module>.yaml` — reintroduces the
  pre-#20 layout and undoes the migration.

### resolve-paths-via-internal-paths-never-hardcode

- Decision: All test path construction for `_lyx`/config goes through `internal/paths`
  (`paths.ConfigDir`, `paths.ConfigFile`, `paths.LyxDirName`). No literal `"_lyx"`,
  `"config"`, or `"<module>.yaml"` segments in test fixtures.
- Rationale: The bug exists precisely because a fixture hardcoded the layout and the
  layout moved. Routing through the library makes the tests track any future migration
  automatically — a single source of truth. This is an explicit operator constraint.
- Rejected: Fix only `cli_test.go` and leave the other 14 In-list files hardcoded — they are
  latent breakage that the next migration re-breaks; the operator chose the repo-wide
  consistency fix.

### scope-the-sweep-to-config-path-violations-only

- Decision: Limit the sweep to test files that construct `_lyx`/config **file** paths
  from literals. Exclude `internal/paths` self-tests and `_lyx` link-geometry/content
  asserts.
- Rationale: `internal/paths` tests must hardcode (they verify the library);
  junction-target and grep-style `_lyx` usages are not config resolution and converting
  them adds churn without removing migration risk.
- Rejected: Mechanically replace every `"_lyx"` literal repo-wide — would corrupt the
  `paths` library's own spec tests and touch geometry tests for no benefit.

## Technical context

- **`internal/paths/paths.go`** exports exactly the helpers needed:
  - `paths.ConfigDir(baseDir string) string` → `<baseDir>/_lyx/config`
  - `paths.ConfigFile(baseDir, module string) string` → `<baseDir>/_lyx/config/<module>.yaml`
  - `paths.LyxDirName` (const) = `"_lyx"` — use for a bare `_lyx` dir:
    `filepath.Join(baseDir, paths.LyxDirName)`.
  - `configDirName` ("config") is **unexported**; a relative config path is still
    expressible as `paths.ConfigFile(".", "worktree")` → `_lyx/config/worktree.yaml`
    (used by the `configcli` integration assert).
- **`internal/worktree/cli.go:78`** — `RunCLI` calls `LoadConfig(cwd, "worktree")`, which
  resolves via `internal/config` against `paths.ConfigFile`. The error message
  `config file ... not found; run "lyx update"` comes from this path.
- **Reference implementation already correct:** `internal/worktree/config_test.go` uses
  `paths.ConfigFile(tmpDir, "worktree")` — but still hardcodes the `_lyx`/`config` mkdir
  via `filepath.Join(tmpDir, "_lyx")` + `"config"`; the sweep tightens that mkdir to
  `paths.ConfigDir`/`paths.LyxDirName` too.
- **`setupCLIRepo` target shape:**
  ```go
  configDir := paths.ConfigDir(f.Hub)
  if err := os.MkdirAll(configDir, 0755); err != nil {
      t.Fatalf("create _lyx/config: %v", err)
  }
  if err := os.WriteFile(paths.ConfigFile(f.Hub, "worktree"),
      []byte("branch_prefix: wt-\n"), 0644); err != nil {
      t.Fatalf("write worktree config: %v", err)
  }
  ```
- **Behavioural note on `TestRunCLI/UnknownSubcommand`:** it currently passes *by
  accident* — config load fails first and returns exit 1 / `ok:false`, which coincidentally
  matches the expected unknown-subcommand envelope. After the fix, config loads
  successfully and the `"bogus"` subcommand reaches the `default` case
  (`internal/worktree/cli.go:147`), which also returns exit 1 / `ok:false`. So the subtest
  keeps passing — now for the right reason. No assertion change needed.
- **`lyxtest.CopyHostHub`** already provides an `origin` remote (per the existing
  `RemoveWithForceFlag` comment), so no `addRemote` is required.
- Each affected test package must keep compiling: confirm `paths` is already imported (most
  config-adjacent tests import it; add the import where missing) and remove now-unused
  intermediate `lyxDir`/`configDir` locals where the refactor obsoletes them.

## Constraints

- `CONSTRAINTS.md` **exists** at the hub root and already mandates a **Path Invariant**:
  all worktree/hub geometry must be resolved through `internal/paths`, not raw primitives
  (`paths.Getwd`/`paths.Resolve` + `Layout` methods; `os.Getwd`/`git rev-parse` banned
  outside `internal/paths` and `cmd/lyx/main.go`, enforced by
  `internal/paths/enforcement_test.go`). This task's rule is the **config-path corollary**
  of that invariant — the existing invariant covered cwd/worktree-root resolution but not
  `_lyx`/config **file** segments, which is exactly the gap that broke `TestRunCLI`.
  CONSTRAINTS.md has been extended with a "`_lyx` and config-file paths" subsection naming
  `paths.LyxDirName` / `paths.ConfigDir` / `paths.ConfigFile` and noting it applies to test
  code (not caught by the enforcement test).
- **Operator hard rule:** test path resolution for `_lyx`/config must go through
  `internal/paths`; no hardcoded path segments. (Drives
  `resolve-paths-via-internal-paths-never-hardcode`; now also codified in CONSTRAINTS.md.)
- Integration tests are gated behind the `integration` build tag (`//go:build
  integration`); the failing test only runs under `-tags integration`. Verification
  commands must pass `-tags integration`.
- Windows host: config-not-found error renders backslash paths; assertions must not
  depend on path separators (using `paths.*` helpers makes this automatic).

## Testing

- **`internal/worktree`** (primary): the existing `TestRunCLI` (`List`,
  `UnknownSubcommand`, `RemoveWithForceFlag`) is the regression gate. Driver:
  `go test -tags integration -run TestRunCLI -v ./internal/worktree/` must go from FAIL →
  PASS. Then the whole package: `go test -tags integration ./internal/worktree/`.
- **Each swept package** must stay green after the literal→helper substitution. Because
  these tests already pass, the refactor is behaviour-preserving; the test *is* its own
  verification — run each touched package under `-tags integration` (and without the tag
  where its tests are non-integration, e.g. `internal/config`, `internal/board`,
  `cmd/lyx`). No new test cases are required: the change is a fixture-path refactor, not a
  new behaviour. Adding assertions would be over-scoping.
- **No TDD candidates** — there is no new production behaviour to drive out; this is a
  red→green fixture fix plus a mechanical consistency refactor. The failing
  `TestRunCLI` already encodes the desired behaviour.
- **Whole-build sanity:** `go build ./...` plus `go test -tags integration ./...`
  restricted to (or at least covering) the affected packages, to catch import/compile
  breakage from removed locals.

## Q&A log

- **Q:** Fix the failing test, or change production `LoadConfig` to accept the old
  `_lyx/worktree.yaml` path? **A:** [auto-pick] Fix the test fixture; leave production
  untouched. **Why:** `RunCLI`/`LoadConfig`/`internal/paths` correctly implement the #20
  migration and the on-disk layout; only the `cli_test.go` fixture is stale. Reverting
  production would undo the migration.
- **Q:** How should the fixture build the config path — hardcode `_lyx/config/worktree.yaml`,
  or resolve via `internal/paths`? **A:** [auto-pick] Resolve via `paths.ConfigDir` /
  `paths.ConfigFile` / `paths.LyxDirName`; zero hardcoded segments. **Why:** the bug *is*
  a hardcoded segment that drifted from the layout; the library is the single source of
  truth and tracks future migrations automatically. (Operator-confirmed hard rule.)
- **Q:** Scope — fix only the failing `cli_test.go`, or sweep every test that hardcodes
  `_lyx`/config paths? **A:** [auto-pick] Sweep all 15 config-path-hardcoding test files
  (the In list; enumerated via `rg -l '"_lyx"' --glob '**/*_test.go'` + the triage rule in Scope);
  exclude `internal/paths` self-tests and `_lyx` link-geometry/content asserts. **Why:**
  the others are latent breakage the next migration re-breaks; operator explicitly asked
  to fix them too. The excluded files either *are* the path spec or don't resolve config.
- **Q:** Convert the borderline relative-path assert in
  `configcli_integration_test.go:78`? **A:** [auto-pick] Yes —
  `paths.ConfigFile(".", "worktree")`. **Why:** keeps the "never hardcode segments" rule
  uniform; the helper produces the same relative `_lyx/config/worktree.yaml`.
- **Q:** (review r1 GAP) The sweep omitted `internal/configsync/configsync_test.go` and
  `internal/weft/config_test.go` — fold them in or justify excluding? **A:** [auto-pick]
  Fold both into the In list; they are structural twins of the in-scope files. **Why:**
  both build genuine `_lyx/config` paths from literals (configsync: `board.yaml`/`weft.yaml`;
  weft: the `LoadConfig` mkdir), so the "no latent migration breakage" rationale requires
  them. weft's `Dirs()` test-data `_lyx` strings stay (parser inputs, not paths). Also
  recorded the reproducible discovery query + triage rule so the set is closeable.
