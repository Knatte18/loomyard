# Batch: paths-config-helpers

```yaml
task: "Extract yamlengine and migrate config via lyx update"
batch: paths-config-helpers
number: 3
cards: 1
verify: go test ./internal/paths/...
depends-on: []
```

## Batch Scope

Centralize the config-layout literals (`_lyx` dir name, the `config/` subdir, the
`.env` filename) in `internal/paths` as a constant plus baseDir-style helpers, and
refactor `paths.go`'s own hardcoded `"_lyx"` literals to use the constant. Downstream
batches (envsource, config, configsync, edit, initcli) consume these helpers so the
layout can be changed in one place — satisfying the build-enforced path invariant.
This batch only adds helpers and refactors `paths.go` internals; consumer files in
other packages are refactored in their own batches.

## Cards

### Card 4: LyxDirName constant + ConfigDir/ConfigFile/DotEnv helpers

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/paths/enforcement_test.go`
- **Edits:**
  - `internal/paths/paths.go`
  - `internal/paths/paths_test.go`
- **Creates:**
  - `internal/paths/paths_unit_test.go`
- **Deletes:** none
- **Requirements:**
  - In `internal/paths/paths.go` add an exported `const LyxDirName = "_lyx"` and unexported `const configDirName = "config"` and `const dotEnvName = ".env"`.
  - Add three baseDir-style helper functions (free functions, NOT `Layout` methods, since callers like `config.Load` have a plain `baseDir` string): `func ConfigDir(baseDir string) string` returning `filepath.Join(baseDir, LyxDirName, configDirName)`; `func ConfigFile(baseDir, module string) string` returning `filepath.Join(ConfigDir(baseDir), module+".yaml")`; `func DotEnv(baseDir string) string` returning `filepath.Join(baseDir, dotEnvName)`.
  - Refactor every literal `"_lyx"` inside `paths.go` to use `LyxDirName` — including `LyxDir()`, the weft methods (`WeftLyxDir`, `WeftLyxDirFor`, and any others), `HostLyxLink`, `HostLyxLinkHere`, and the `HostJunction` `Name:` field. Behavior must be byte-identical; this is a literal→constant substitution only.
  - Godoc every new exported symbol per the golang-comments skill.
  - paths_test.go: add tests asserting `ConfigDir`, `ConfigFile`, `DotEnv` produce the expected joined paths for a sample baseDir; add/keep an assertion that `LyxDir()` and the weft/host-link methods return the same strings as before the refactor (guard against accidental drift).
  - The existing `internal/paths/enforcement_test.go` must still pass (the refactor introduces no `os.Getwd`/`git rev-parse` usage).
- **Commit:** `feat(paths): centralize _lyx/config/.env layout in LyxDirName + helpers`

## Batch Tests

`verify: go test ./internal/paths/...` covers the new helpers plus the existing
`paths_test.go` and `enforcement_test.go`. Scope is the single package; the
enforcement test guards the path invariant.
