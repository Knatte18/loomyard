# Batch: configfilerel-accessor

```yaml
task: 'fabric: clone doesn''t commit written module configs'
batch: 'configfilerel-accessor'
number: 1
cards: 1
verify: go test -count=1 ./internal/configengine/...
depends-on: []
```

## Batch Scope

This batch adds one exported path accessor, `configengine.ConfigFileRel(module string) string`, returning the anchor-relative `_lyx/config/<module>.yaml` path, together with its unit test and its entry in `internal/configengine`'s exported-surface doc.
It is one batch because it is a self-contained, dependency-free addition to a leaf-ish path package: nothing else in the plan compiles against it until batch 3.

The external interface batch 3 consumes is exactly `configengine.ConfigFileRel(module)` — the single expression `internal/fabriccli/clone.go` uses to build its commit pathspec, so `configDirName` stays unexported and the `_lyx` / `config` / `<module>.yaml` segments are joined in one place.

No batch-local decisions beyond the overview's `docs-land-with-their-own-behaviour-change` and `markdown-semantic-line-breaks`.

## Cards

### Card 1: add `configengine.ConfigFileRel` with unit coverage and doc entry

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/configengine/config.go`
  - `internal/configengine/config_test.go`
  - `docs/shared-libs/configengine.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an exported function `ConfigFileRel(module string) string` to `internal/configengine/config.go`, placed immediately after the existing `ConfigFile` function so the three path accessors `ConfigDir` / `ConfigFile` / `ConfigFileRel` sit together.
  Its body returns `filepath.Join(lyxdirs.LyxDirName, configDirName, module+".yaml")` — the three segments joined directly, never a fused `"_lyx/config"` literal, per the per-segment join rule in `CONSTRAINTS.md`'s Cwd Resolution Invariant and the Lyxdirs Single-Declarer Invariant.
  No new import is needed;
  `path/filepath` and `internal/lyxdirs` are already imported by this file.

  Give it a godoc comment stating that the returned path is anchor-relative — the same shape `fabricengine.OriginRecordRel` returns and the shape `fabricengine.CommitAnchoredPaths` expects for its `relPaths` argument — and that it is deliberately not derivable from `ConfigFile` by cancelling a base path.

  In `internal/configengine/config_test.go`, add `TestConfigFileRel` beside the existing `TestConfigDir` and `TestConfigFile`, in the same `package configengine_test` external test package.
  It asserts three things: the returned value for at least two module names (`"loom"` and `"board"`) equals `filepath.Join(lyxdirs.LyxDirName, "config", module+".yaml")`;
  `filepath.IsAbs` on the result is false;
  and, tying the two accessors together so they can never drift, that `ConfigFile(base, m)` equals `filepath.Join(base, ConfigFileRel(m))` for a sample `base`.
  Keep it table-free, matching the shape of the neighbouring `TestConfigDir`/`TestConfigFile`.
  The file is untagged and must stay untagged: it calls no `hubforge.NewHub`, no `gitexec.Run`, no `exec.Command` and no `gitkit.Copy*`, so `CONSTRAINTS.md`'s Test Tier Purity Invariant is satisfied without a build tag.

  In `docs/shared-libs/configengine.md`, add a `### \`ConfigFileRel(module string) string\`` subsection under `## Exported functions`, immediately after the existing `### \`ConfigFile(baseDir, module string) string\`` entry and before `### \`FindBaseDir(cwd string) (string, error)\``.
  Describe it in the same one-or-two-sentence register the neighbouring entries use: it returns `filepath.Join(LyxDirName, "config", module+".yaml")`, the anchor-relative form used to build weft commit pathspecs, as opposed to `ConfigFile`'s base-joined absolute form.
  Write the prose with one sentence per line, per this repo's markdown rule.
- **Commit:** `feat(configengine): add ConfigFileRel anchor-relative path accessor`

## Batch Tests

`verify: go test -count=1 ./internal/configengine/...` runs `internal/configengine`'s untagged unit suite, which is where the new `TestConfigFileRel` lives alongside `config_test.go`'s existing `TestConfigDir` and `TestConfigFile`.
The scope is one package because that is the entire surface this batch touches: `ConfigFileRel` has no callers until batch 3, and `docs/shared-libs/configengine.md` has no runnable surface.
`-count=1` defeats Go's test result cache so the run reflects this batch's own edits rather than a prior cached pass.
