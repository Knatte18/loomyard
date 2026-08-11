# Batch: batcher-config-module

```yaml
task: 'batcher: split out of webster into a standalone configreg module with its own batcher.yaml'
batch: 'batcher-config-module'
number: 1
cards: 4
verify: go build ./... && go test ./internal/batcher/... ./internal/configreg/... && go test -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain|TestSandboxCoverage_AllModulesCoveredOrExcluded' ./cmd/lyx/...
depends-on: []
```

## Batch Scope

This batch gives `internal/batcher` its own config module and registers it with `internal/configreg`, without touching a single existing call site.
It is one batch because the four cards are one shape: a `template.yaml`/`template.go` pair, the `Active` accessor that reads it, the test that pins `Active`, and the registry entry that makes `lyx config reconcile` materialize the file.
Nothing here is observable to `webster` yet — `websterengine.Config.Batcher` and both `batcher.Select` call sites remain exactly as they are, so the tree builds and every existing test passes untouched at the end of this batch.

The external interface batch 2 consumes is `batcher.Active(baseDir string) (Batcher, error)` and `batcher.ConfigTemplate() string`.

Batch-local decision beyond `## Shared Decisions`: the new `internal/batcher/config_test.go` carries the same Tier-1 header the package's three existing test files carry, and defines its own package-local `seedConfig` helper rather than importing one — `internal/websterengine/config_test.go`'s copy is unexported and lives in a different package.

## Cards

### Card 1: batcher.yaml template and its embed accessor

- **Context:**
  - `internal/websterengine/template.go`
  - `internal/websterengine/template.yaml`
  - `internal/batcher/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/batcher/template.yaml`
  - `internal/batcher/template.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `internal/batcher/template.yaml` holds exactly one key, `active`, with the empty-string default and a trailing `#` comment in the same style as `internal/websterengine/template.yaml`'s own lines.
  The value must be the two-character literal `""` so that `strings.Replace(batcher.ConfigTemplate(), "active: \"\"", …, 1)` in batch 2's relocated gate test has an unambiguous target.
  The comment must name the identity batchifier as what an empty value resolves to and must NOT name `internal/batcher.Select` (batch 3 removes every remaining `Select`-as-config-entry-point claim);
  name `internal/batcher.Active` instead.
  File content:

```yaml
active: ""  # name of the active batchifier (internal/batcher.Active) that groups the plan's flat card list into execution batches; empty (the default) resolves to the identity batchifier (one card, one batch)
```

  `internal/batcher/template.go` copies `internal/websterengine/template.go`'s embed-and-accessor shape exactly: a file-header comment, `package batcher`, `import _ "embed"`, a `//go:embed template.yaml` directive over `var configTemplate string`, and an exported `func ConfigTemplate() string` returning it.
  `configTemplate` is unexported and `ConfigTemplate` is the only exported symbol the file adds.
  Both new files live in package `batcher`.
- **Commit:** `feat(batcher): add batcher.yaml template and ConfigTemplate accessor`

### Card 2: Active resolves the configured batchifier from batcher.yaml

- **Context:**
  - `internal/batcher/registry.go`
  - `internal/batcher/batcher.go`
  - `internal/batcher/template.go`
  - `internal/configengine/config.go`
  - `internal/websterengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/batcher/config.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `internal/batcher/config.go` in package `batcher` with a file-header comment in the repo's own style, an unexported config struct, and the exported `Active`.
  The struct has exactly one field, carrying the yaml tag `active`, on an unexported struct type — so the struct name, not the field name, is what stays unexported, and the exported function `Active` never collides with it:

```go
type config struct {
	Active string `yaml:"active"`
}
```

  `Active(baseDir string) (Batcher, error)` must:
  call `configengine.Load(baseDir, moduleName, []byte(ConfigTemplate()))` where `moduleName` is a package-level unexported constant equal to `"batcher"` — the module name is batcher's own constant, never a parameter;
  on error, apply the two-path rule from `## Shared Decisions` → two-distinct-error-paths — if `strings.Contains(err.Error(), "not initialized")` return `fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"")`, otherwise return the error unchanged;
  `yaml.Unmarshal` the resolved bytes into the config struct, wrapping a failure as `fmt.Errorf("unmarshal batcher config: %w", err)` to match `websterengine.LoadConfig`'s `unmarshal webster config: %w` shape;
  then `return Select(cfg.Active)` so the empty-string default and the `batcher: unknown batcher %q` error both come from the existing `Select` unchanged, never re-implemented here.
  The new imports are `fmt`, `strings`, `github.com/Knatte18/loomyard/internal/configengine`, and `gopkg.in/yaml.v3`.
  Do not add `internal/lyxcwd` — `Active` takes an already-resolved `baseDir` from its caller and never resolves cwd itself (Cwd Resolution Invariant).
  Do not add a `DefaultName` fallback for the absent-file case.
- **Commit:** `feat(batcher): add Active to resolve the configured batchifier from batcher.yaml`

### Card 3: Tier-1 tests for Active

- **Context:**
  - `internal/websterengine/config_test.go`
  - `internal/batcher/batcher_test.go`
  - `internal/batcher/registry.go`
  - `internal/batcher/config.go`
  - `internal/batcher/template.go`
  - `internal/configengine/config.go`
  - `CONSTRAINTS.md`
  - `cmd/lyx/tierpurity_test.go`
- **Edits:** none
- **Creates:**
  - `internal/batcher/config_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/batcher/config_test.go` in the external test package `batcher_test`, matching `internal/websterengine/config_test.go`'s own `package websterengine_test` choice.
  Its first non-empty line is a file-header comment (NOT a `//go:build` line) and that header must carry the same "Tier-1 (pure logic, no git, no TestMain)" phrasing the package's three existing test files use, so the Test Tier Purity Invariant reads correctly at a glance.
  Add no `TestMain` to this package.
  Define a package-local `seedConfig(t *testing.T, baseDir, module, content string)` helper copied from `internal/websterengine/config_test.go`'s helper — `os.MkdirAll(configengine.ConfigDir(baseDir), 0o755)` then `os.WriteFile(configengine.ConfigFile(baseDir, module), []byte(content), 0o644)`, each with a `t.Fatalf` on error and `t.Helper()` at the top.
  Never call `lyxtest.SeedConfig`, `gitexec.RunGit`, `exec.Command`, or `exec.CommandContext` anywhere in this file, in code or in a string literal — `cmd/lyx/tierpurity_test.go` matches raw substrings.
  Tests to write:
  `TestConfigTemplate_ParsesAsYAML` — `yaml.Unmarshal([]byte(batcher.ConfigTemplate()), &map[string]any{})` succeeds, mirroring `websterengine`'s test of the same name.
  `TestActive_TemplateDefaultResolvesIdentity` — seed `batcher.ConfigTemplate()` verbatim under a `t.TempDir()` base, call `batcher.Active(baseDir)`, assert nil error and `Name() == batcher.DefaultName`.
  This is the assertion moving out of `internal/websterengine/config_test.go`'s `cfg.Batcher != "identity"` check, re-expressed at `Active` level.
  `TestActive_ExplicitNameResolves` — seed `active: "identity"` and assert `Name() == "identity"`.
  `TestActive_UnknownNameErrors` — seed `active: "does-not-exist"`, assert a non-nil error whose message contains both `unknown batcher` and `does-not-exist`, proving the message comes from `Select` unchanged.
  `TestActive_AbsentConfigIsHardError` — `os.MkdirAll(filepath.Join(baseDir, "_lyx"), 0o755)` and write no config file, then assert the error message contains `batcher.yaml`, contains `lyx config reconcile`, and does NOT contain `fabric reconcile`, pinning error path 2 of the two-distinct-error-paths decision against a later silent-default regression.
  `TestActive_UninitializedTreeNamesFabricReconcile` — call `Active` on a bare `t.TempDir()` with no `_lyx` directory and assert the message is exactly `not initialized here; run "lyx fabric reconcile"`, pinning error path 1 and proving the two paths are distinct.
  Do not edit `internal/batcher/batcher_test.go`, `registry_test.go`, or `identity_test.go` — they are the standing proof that only the configuration source moved and the batching itself did not.
- **Commit:** `test(batcher): pin Active against batcher.yaml and both error paths`

### Card 4: register batcher in configreg

- **Context:**
  - `internal/batcher/template.go`
  - `internal/configcli/configcli_test.go`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/configreg/configreg.go`, add `github.com/Knatte18/loomyard/internal/batcher` to the import block in its correct sorted position (first, before `boardengine`), and add `{Name: "batcher", Template: batcher.ConfigTemplate},` as the FIRST entry of the slice `Modules()` returns, before the `board` entry.
  Omit `SeedOnly` entirely per `## Shared Decisions` → seed-only-omitted.
  `Modules()`' doc comment already states the order is alphabetical and that a misordered entry is user-visible;
  `batcher` sorts before `board`, so no comment change is needed.
  In `internal/configreg/configreg_test.go`, `TestNames`'s `want` slice becomes `[]string{"batcher", "board", "burler", "fabric", "loom", "models", "perch", "reed", "shuttle", "webster"}`.
  Do NOT edit `TestModules_SeedOnly` — its `want` is computed as `m.Name == "models" || m.Name == "burler"`, already correct for `batcher`.
  Verify no import cycle is introduced: `internal/batcher` imports only `internal/planparser` and (as of card 2) `internal/configengine`, and `configengine` imports only `envsource`, `lyxdirs`, and `yamlengine` — nothing in that set imports `batcher` or `configreg`.
  Do not add a `**Covers:** batcher` line to any `tools/sandbox/*SUITE.md` file per `## Shared Decisions` → no-cobra-command-no-sandbox-tag.
- **Commit:** `feat(configreg): register batcher as a config module`

## Batch Tests

`verify:` runs `go build ./...` to prove the new package files compile and the `configreg` import introduces no cycle, then `go test ./internal/batcher/... ./internal/configreg/...` for the two packages this batch touches.

`internal/batcher/config_test.go` is the batch's own new evidence (card 3).
`internal/configreg/configreg_test.go`'s `TestNames` and `TestModules_SeedOnly` cover card 4, and `internal/batcher`'s three existing test files must pass with no edit.

The third `verify:` clause is a `-run`-scoped invocation of three named guard tests in `./cmd/lyx/...` rather than the whole package, because this batch's two structural risks both live there and neither is visible from `./internal/...`: adding an untagged test file to `internal/batcher` is exactly what `TestTierPurity_UntaggedTestsSpawnNothing` and `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` police, and registering a new module is what `TestSandboxCoverage_AllModulesCoveredOrExcluded` would trip on if the sandbox tag were mistakenly added.
The `-run` filter keeps the cost to three tests instead of the full `cmd/lyx` suite;
the repo-wide `go test ./... && go test -tags integration ./...` done-gate covers the rest.
