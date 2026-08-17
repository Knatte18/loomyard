# Batch: producer-loaders-repointed

```yaml
task: "config degrades to embedded template"
batch: "producer-loaders-repointed"
number: 2
cards: 4
verify: go test ./internal/shuttleengine/... ./internal/reedengine/... ./internal/perchengine/... ./internal/websterengine/...
depends-on: [1]
```

## Batch Scope

This batch repoints the four producer config loaders — `shuttleengine.LoadConfig`, `reedengine.LoadConfig`, `perchengine.LoadConfigWithRegistry`, `websterengine.LoadConfig` — from `configengine.Load` onto `configengine.LoadOrTemplate`, deletes the now-dead `not initialized` rewrap block and the `strings` import each carries solely for it, rewrites the doc comments that promise an error on an absent `_lyx/`, and inverts each package's `TestLoadConfig_NotInitialized` plus the three `TestLoadConfig_ModuleArgIsThreadedThrough` negative halves.

It is one batch because all four callers share one shape — `configengine.Load(baseDir, module, []byte(ConfigTemplate()))`, an `if err != nil` block containing a `strings.Contains(err.Error(), "not initialized")` rewrap, then an unmarshal — so the four edits are the same edit four times with per-package trimmings.
Each card is one package's production file plus its test file, kept together because the test inversion is meaningless without the production change that makes it pass.

Batch-local decisions beyond `## Shared Decisions`:

- Only `perchengine.LoadConfigWithRegistry` is repointed, not `perchengine.LoadConfig`.
  `LoadConfig` wraps it after calling `modelspec.LoadRegistry(baseDir)`, which already degrades to `builtins()` on an absent file, so repointing the inner function covers both entry points.
- A reed fallback assertion must target GOOS-invariant keys.
  `internal/reedengine/template_posix.yaml` and `internal/reedengine/template_windows.yaml` differ on `tmux`/`shell`, so those two fields are off-limits as assertion targets.
- No file under `internal/burlerengine`, `internal/modelspec`, or `internal/scoutengine` is opened.
  Those three are own-loader modules that already degrade;
  routing them through `LoadOrTemplate` would either break burler's open-ended key set or violate the Modelspec Leaf Invariant.

## Cards

### Card 4: repoint `shuttleengine.LoadConfig` and invert its tests

- **Context:**
  - `internal/configengine/config.go`
  - `internal/shuttleengine/template.go`
  - `internal/shuttleengine/template.yaml`
- **Edits:**
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/shuttleengine/config.go`, change `LoadConfig`'s call from `configengine.Load` to `configengine.LoadOrTemplate`, keeping the same three arguments.
  Delete the whole `strings.Contains(err.Error(), "not initialized")` block and its `not initialized here; run "lyx fabric reconcile"` rewrap, leaving a plain `if err != nil { return Config{}, err }`.
  Remove `"strings"` from the import set;
  keep `"fmt"`, which the `unmarshal shuttle config` wrap still uses.
  Rewrite `LoadConfig`'s doc comment, whose last sentence today reads "If `<baseDir>/_lyx/` does not exist, it returns an error with recovery instructions" — it must instead state that an absent `_lyx/` directory or an absent config file resolves the embedded template, while a config file that exists but is invalid still errors.
  Rewrite the file-header comment's `internal/configengine.Load` reference to name `LoadOrTemplate`.

  In `internal/shuttleengine/config_test.go`, rename `TestLoadConfig_NotInitialized` to `TestLoadConfig_UninitializedFallsBackToTemplate` and invert it: a bare `t.TempDir()` with nothing in it, asserting a nil error and the template defaults `PollIntervalMS == 500`, `LivenessEveryNPolls == 10`, `RunTimeoutMin == 30`, `StartupTimeoutS == 90`, and `ClaudeDenyAgentTool == true`.
  Rework `TestLoadConfig_ModuleArgIsThreadedThrough`'s second half, which today asserts that `shuttleengine.LoadConfig(tmpDir, "shuttle")` errors, into a positive discrimination assertion: seed the `othershuttle` module with a config whose `poll_interval_ms` differs from the template's `500`, then assert loading under `othershuttle` returns that seeded value and loading under the never-seeded `shuttle` returns the template default `500`.
  Both halves are needed — the seeded half alone would not catch a hardcoded module name, and the default half alone would not prove the file is read.
  Remove `"strings"` from the test file's import set once the inversion removes its last use.
  Update the file-header comment, which cites "the not-initialized error path", to describe the template-fallback path instead.
  Leave `seedLyxConfig`, `TestLoadConfig_TemplateDefaultsResolve`, and `TestLoadConfig_EnvOverride` unchanged.
- **Commit:** `feat(shuttleengine): degrade to the embedded template when config is absent`

### Card 5: repoint `reedengine.LoadConfig` and invert its tests

- **Context:**
  - `internal/configengine/config.go`
  - `internal/reedengine/template.go`
  - `internal/reedengine/template_posix.yaml`
  - `internal/reedengine/template_windows.yaml`
- **Edits:**
  - `internal/reedengine/config.go`
  - `internal/reedengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/config.go`, change `LoadConfig`'s call from `configengine.Load` to `configengine.LoadOrTemplate`, keeping the same three arguments.
  Delete the whole `strings.Contains(err.Error(), "not initialized")` block and its rewrap, leaving a plain `if err != nil { return Config{}, err }`.
  Remove `"strings"` from the import set;
  keep `"fmt"` for the `unmarshal reed config` wrap.
  Give `LoadConfig` a doc comment stating that an absent `_lyx/` directory or an absent config file resolves the embedded template, while a config file that exists but is invalid still errors.
  Rewrite the file-header comment's `internal/configengine.Load` reference to name `LoadOrTemplate`.

  In `internal/reedengine/config_test.go`, rename `TestLoadConfig_NotInitialized` to `TestLoadConfig_UninitializedFallsBackToTemplate` and invert it: a bare `t.TempDir()` with nothing in it, asserting a nil error and the GOOS-invariant template defaults `Width == 220`, `CollapsedStripRows == 3`, and `Header.HeightRows == 1`.
  Keep the assertions away from `Tmux` and `Shell`, whose defaults differ between `template_posix.yaml` and `template_windows.yaml`.
  Rework `TestLoadConfig_ModuleArgIsThreadedThrough`'s second half, which today asserts that `reedengine.LoadConfig(tmpDir, "reed")` errors, into a positive discrimination assertion: seed the `otherreed` module with a config whose `width` differs from the template's `220`, then assert loading under `otherreed` returns that seeded value and loading under the never-seeded `reed` returns the template default `220`.
  Remove `"strings"` from the test file's import set once the inversion removes its last use;
  keep `"runtime"`, which an existing test still uses.
  Update the file-header comment, which cites "the not-initialized error path", to describe the template-fallback path instead.
- **Commit:** `feat(reedengine): degrade to the embedded template when config is absent`

### Card 6: repoint `perchengine.LoadConfigWithRegistry` and invert its tests

- **Context:**
  - `internal/configengine/config.go`
  - `internal/modelspec/load.go`
  - `internal/perchengine/template.go`
  - `internal/perchengine/template.yaml`
- **Edits:**
  - `internal/perchengine/config.go`
  - `internal/perchengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/perchengine/config.go`, change `LoadConfigWithRegistry`'s call from `configengine.Load` to `configengine.LoadOrTemplate`, keeping the same three arguments.
  Delete the whole `strings.Contains(err.Error(), "not initialized")` block and its rewrap, leaving a plain `if err != nil { return Config{}, err }`.
  Remove `"strings"` from the import set;
  keep `"bytes"`, `"fmt"`, and the `internal/logger` and `internal/modelspec` imports, all of which are still used.
  Leave `perchengine.LoadConfig` structurally unchanged: it calls `modelspec.LoadRegistry(baseDir)` — which already degrades to `builtins()` on an absent file — before delegating to `LoadConfigWithRegistry`, so repointing the inner function covers both entry points.
  Keep the `ResolveModelSpec` call that runs after the decode, so the template's `judge_model: haiku` default resolves through the supplied registry on the fallback path exactly as it does on the on-disk path.
  Give `LoadConfigWithRegistry` a doc comment stating that an absent `_lyx/` directory or an absent config file resolves the embedded template, while a config file that exists but is invalid still errors.
  Rewrite the file-header comment's `internal/configengine.Load` reference to name `LoadOrTemplate`.

  In `internal/perchengine/config_test.go`, rename `TestLoadConfig_NotInitialized` to `TestLoadConfig_UninitializedFallsBackToTemplate` and invert it: a bare `t.TempDir()` with nothing in it, called through `perchengine.LoadConfig` so the registry also falls back to `builtins()`, asserting a nil error, `JudgeModel` resolved from the template's `haiku` alias, and `RoundCaps` equal to `[5, 8, 10]`.
  This test covers the fallback and the model-spec resolution running on top of it.
  Rework `TestLoadConfig_ModuleArgIsThreadedThrough`'s second half, which today asserts that `perchengine.LoadConfig(tmpDir, "perch")` errors, into a positive discrimination assertion: seed the `otherperch` module with a config whose `judge_model` differs from the template's `haiku`, then assert loading under `otherperch` returns that seeded model and loading under the never-seeded `perch` returns the template default.
  Keep `"strings"` in the test file's import set;
  three other tests still use it.
  Update the file-header comment, which cites "the not-initialized error path", to describe the template-fallback path instead.
- **Commit:** `feat(perchengine): degrade to the embedded template when config is absent`

### Card 7: repoint `websterengine.LoadConfig` and invert its test

- **Context:**
  - `internal/configengine/config.go`
  - `internal/websterengine/template.go`
  - `internal/websterengine/template.yaml`
- **Edits:**
  - `internal/websterengine/config.go`
  - `internal/websterengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/websterengine/config.go`, change `LoadConfig`'s call from `configengine.Load` to `configengine.LoadOrTemplate`, keeping the same three arguments.
  Delete the whole `strings.Contains(err.Error(), "not initialized")` block and its rewrap, leaving a plain `if err != nil { return Config{}, err }`.
  Remove `"strings"` from the import set;
  keep `"fmt"` and the `internal/modelspec` import, both still used.
  Keep the `modelspec.Parse` grammar check over the `master` and `recovery` role specs, so the template defaults `sonnet` and `opus[effort=high]` are validated on the fallback path exactly as an on-disk config's values are.
  Rewrite `LoadConfig`'s doc comment, whose last sentence today promises an error containing `not initialized here; run "lyx fabric reconcile"` on an absent `_lyx/` — it must instead state that an absent `_lyx/` directory or an absent config file resolves the embedded template, while a config file that exists but is invalid still errors.
  Rewrite the file-header comment's `internal/configengine.Load` reference to name `LoadOrTemplate`.

  In `internal/websterengine/config_test.go`, rename `TestLoadConfig_NotInitialized` to `TestLoadConfig_UninitializedFallsBackToTemplate` and invert it: a bare `t.TempDir()` with nothing in it, asserting a nil error and the template defaults `Master == "sonnet"`, `SelfFixCap == 2`, and `MasterTimeoutMin == 480`.
  This package has no `TestLoadConfig_ModuleArgIsThreadedThrough`, so the inversion is the only test change here.
  Keep `"strings"` in the test file's import set;
  other tests still use it.
  Update the file-header comment if it cites the not-initialized error path, describing the template-fallback path instead.
- **Commit:** `feat(websterengine): degrade to the embedded template when config is absent`

## Batch Tests

`verify: go test ./internal/shuttleengine/... ./internal/reedengine/... ./internal/perchengine/... ./internal/websterengine/...` covers exactly the four packages this batch edits — the eight files in the cards' `Edits:` lists plus each package's other tests, which must keep passing unchanged.
The scope is deliberately per-batch rather than repo-wide;
`pipeline.done_gate` already runs `go test ./...` before the task is marked done, which is what catches anything outside these four packages.

Per package the batch asserts three things: the fallback returns template defaults from a bare `t.TempDir()`, the module argument still selects the config file path (via the reworked discrimination assertion in shuttle, reed and perch), and every pre-existing test — template-defaults-resolve, env-override, and the model-spec negative tests in perch and webster — keeps passing against the repointed loader.
Webster's inverted test additionally exercises `modelspec.Parse` over the template's two role specs, and perch's exercises `ResolveModelSpec` against `builtins()`, both for free.

Two packages named in the discussion are deliberately out of this batch's verify scope because no card touches them: `internal/webstercli`, whose `TestStatusCmd_NotInitialized` is about an absent `state.json` rather than config, and `internal/reedcli`, whose integration test asserts an error is *not* a config-resolution error — a claim the fallback makes strictly more true.
Both are covered by the repo-wide done gate.
