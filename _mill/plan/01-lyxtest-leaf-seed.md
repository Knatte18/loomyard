# Batch: lyxtest-leaf-seed

```yaml
task: "Move config templates home by removing the lyxtest->configreg edge"
batch: lyxtest-leaf-seed
number: 1
cards: 6
verify: go build ./... && go vet -tags integration ./... && go test -tags integration ./...
depends-on: []
```

## Batch Scope

This batch removes the `lyxtest → configreg` import edge — the single edge that closes the
test-build cycle — and re-routes the one responsibility that edge served (seeding real config into
test fixtures) to the test sites. After this batch, `internal/lyxtest` imports only the standard
library and `internal/paths`; `internal/configtmpl` and `internal/configreg` are untouched (the
template revert is batch 2), so the build stays green and the cycle is gone. The external interface
this batch publishes, consumed by batch 2 and all seeding sites, is `lyxtest.SeedConfig(tb, repoDir,
map[string]string)`. The batch is one unit because `lyxtest.go` (drop edge + neutral fixture + new
helper) and the seeding call sites must change together for the integration suite to stay green.

Batch-local decision: the **complete set** of fixture consumers that need a `SeedConfig` call is not
statically known — it is discovered empirically by running the integration suite with the neutral
fixture and seeding each test that then fails on a missing/incomplete config (`config file … not
found` / `missing keys`). Cards 3 and 4 cover the two verified cases plus the sweep.

## Cards

### Card 1: Make lyxtest a leaf — drop the configreg edge, neutralize buildWeftPrime

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Remove the `github.com/Knatte18/loomyard/internal/configreg` import from
  `lyxtest.go`. In `buildWeftPrime`, delete the seeding loop `for _, mod := range
  configreg.Modules() { ... os.WriteFile(paths.ConfigFile(weftPrime, mod.Name), ...) }` and replace
  it with the neutral pre-`6d24098` fixture: create the config dir via `os.MkdirAll(paths.ConfigDir(
  weftPrime), 0o755)` and write a single placeholder file at `filepath.Join(paths.ConfigDir(
  weftPrime), "placeholder")` with content `[]byte("weft config")` and mode `0o644`. Keep the
  surrounding `git add`/`git commit` of the fixture unchanged. Ensure remaining imports still
  compile (`fmt` is still used elsewhere in the file; only drop `configreg`). Do not touch
  `buildHostHub` or `buildWeftOnly`.
- **Commit:** `refactor(lyxtest): drop configreg import, seed neutral weft-prime fixture`

### Card 2: Add the configreg-free SeedConfig helper

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add an exported `func SeedConfig(tb testing.TB, repoDir string, configByModule
  map[string]string)` to `lyxtest`. It must call `tb.Helper()`, `os.MkdirAll(paths.ConfigDir(
  repoDir), 0o755)`, then for each `module, content` in `configByModule` write
  `paths.ConfigFile(repoDir, module)` with `[]byte(content)` at mode `0o644`, then stage and commit
  via the existing `MustRun(tb, repoDir, "git", "add", ".")` and `MustRun(tb, repoDir, "git",
  "commit", "-m", "seed config")` test-git helpers (so the seeded config is **committed** — a git
  worktree only checks out committed content). The parameter type must remain `map[string]string`
  (or a lyxtest-local struct); it must never reference any `configreg` type. Add a godoc comment
  explaining the leaf-preserving signature.
- **Commit:** `feat(lyxtest): add configreg-free SeedConfig fixture helper`

### Card 3: Seed real config in TestE2ESyncIntegration (configcli)

- **Context:**
  - `internal/configreg/configreg.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/configcli/configcli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `TestE2ESyncIntegration`, immediately after `f := lyxtest.CopyPaired(t)` and
  **before** `worktree.Add(...)`, build a `map[string]string` by looping over `configreg.Modules()`
  (`seeds[m.Name] = m.Template()`) and call `lyxtest.SeedConfig(t, f.WeftPrime, seeds)`. Add the
  `internal/configreg` import. This restores the real `weft.yaml` (and the other modules) the
  `weft.RunCLI("commit")` path requires for its pathspec `LoadConfig`. `package configcli` may
  import `configreg` with no cycle (configreg does not import configcli).
- **Commit:** `test(configcli): seed fixture config via SeedConfig in E2E sync test`

### Card 4: Seed feature-internal consumers (empirical sweep)

- **Context:**
  - `internal/weft/cli.go`
  - `internal/weft/config.go`
  - `internal/config/config.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/weft/weft_integration_test.go`
  - `internal/worktree/add_test.go`
  - `internal/worktree/remove_test.go`
  - `internal/worktree/weft_test.go`
  - `internal/worktree/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Run `go test -tags integration ./...` after cards 1–3. For **every** test that
  now fails with `config file … not found` or `missing keys` (the `config.Load` errors), add a
  `lyxtest.SeedConfig` call right after its `CopyPaired`/`CopyPairedLocal` call, seeding only the
  module(s) the failure names. Obtain each template from the module's own package — e.g.
  `weft.ConfigTemplate()` for `weft.yaml`, `worktree.ConfigTemplate()` for `worktree.yaml` — adding
  that feature-package import to the test file if needed; **never** import `configreg` from a
  feature-internal package. Verified case to fix: `TestRunCLI_EnvMapToOption` (`package weft`,
  `weft_integration_test.go`) — seed `map[string]string{"weft": weft.ConfigTemplate()}` into
  `fixture.WeftPrime` before `RunCLI([]string{"push"})`. The listed `worktree` files are candidates
  only: edit a candidate **only if** the sweep flags it; leave unflagged candidates unchanged. Do
  not modify the unrelated `_lyx/placeholder` literal in `weft_integration_test.go` here (that is
  batch 3).
- **Commit:** `test: seed feature-internal fixtures via own ConfigTemplate where config is loaded`

### Card 5: Unit-test SeedConfig and the neutral fixture

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/lyxtest/lyxtest_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a test for `SeedConfig`: seed a temp git repo with a known
  `map[string]string`, assert each `paths.ConfigFile(repoDir, module)` exists with the expected
  content and is tracked (`git ls-files` lists it / `git status` is clean). Add a test asserting the
  neutral paired fixture from `CopyPaired` contains `paths.ConfigDir(...)/placeholder` and does
  **not** contain a real `paths.ConfigFile(..., "weft")`. Follow the existing test style in this
  file (build tags, helpers).
- **Commit:** `test(lyxtest): cover SeedConfig and the neutral weft-prime fixture`

### Card 6: Record the lyxtest-leaf invariant

- **Context:**
  - `internal/paths/enforcement_test.go`
- **Edits:**
  - `internal/lyxtest/doc.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/lyxtest/doc.go`, add a paragraph stating the invariant:
  `internal/lyxtest` must stay a stdlib + `internal/paths` leaf and must never import
  `internal/configreg` or any feature package (`board`/`worktree`/`weft`), because feature packages
  have internal tests that import `lyxtest`; such an import closes a test-build cycle. Tests that
  need real config seed it via `SeedConfig`, converting `configreg.Modules()` (or a feature's own
  `ConfigTemplate()`) to a `map[string]string` at the test site. Add a corresponding short section
  to `CONSTRAINTS.md` (a new `## lyxtest Leaf Invariant` heading) capturing the same rule, in the
  style of the existing Path Invariant section. Note explicitly that this is a code-review /
  planning-discipline rule (there is no enforcement test, per the task decision).
- **Commit:** `docs: record the internal/lyxtest leaf invariant`

## Batch Tests

`verify: go build ./... && go vet -tags integration ./... && go test -tags integration ./...`.

`go build ./...` confirms production code still compiles. `go vet -tags integration ./...` is the
import-cycle gate: with `lyxtest` no longer importing `configreg`, the previously-cycling
feature-internal test builds (`package weft`, `package worktree`) must compile. `go test -tags
integration ./...` runs the full suite — the justified scope here because `lyxtest` is imported by
nearly every integration test package (`board/boardtest`, `configcli`, `gitclone`, `ide`, `paths`,
`weft`, `worktree`), so the affected surface is the whole suite and the empirical seeding sweep
(card 4) needs the full run to discover every consumer. Key tests: `TestE2ESyncIntegration`
(configcli), `TestRunCLI_EnvMapToOption` (weft), the new `SeedConfig`/neutral-fixture tests
(lyxtest), and any worktree CopyPaired test the sweep flags.
