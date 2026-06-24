# Batch: docs

```yaml
task: "Extract yamlengine and migrate config via lyx update"
batch: docs
number: 7
cards: 1
verify: null
depends-on: [5, 6]
```

## Batch Scope

Bring the durable design docs in line with the new config architecture: the pure
`yamlengine` engine, the `envsource` env layer, the strict template-backed
`config.Load`, the centralized path helpers, and the new `lyx update` command +
reworked `lyx init`. Per the documentation-lifecycle convention
(`docs/overview.md#documentation-lifecycle`), this batch updates the durable
shared-lib and overview docs and adds two new shared-lib docs; it does not add
mechanical per-module docs for the new CLI packages.

## Cards

### Card 22: update architecture docs for yamlengine + update

- **Context:**
  - `internal/yamlengine/resolve.go`
  - `internal/yamlengine/reconcile.go`
  - `internal/envsource/envsource.go`
  - `internal/config/config.go`
  - `internal/paths/paths.go`
  - `internal/update/update.go`
  - `internal/configsync/configsync.go`
  - `internal/initcli/initcli.go`
  - `_mill/discussion.md`
- **Edits:**
  - `docs/shared-libs/config.md`
  - `docs/shared-libs/README.md`
  - `docs/shared-libs/paths.md`
  - `docs/overview.md`
- **Creates:**
  - `docs/shared-libs/yamlengine.md`
  - `docs/shared-libs/envsource.md`
- **Deletes:** none
- **Requirements:**
  - `docs/shared-libs/config.md`: rewrite to describe the strict, template-backed `config.Load(baseDir, module, template []byte) ([]byte, error)`: it reads `_lyx/config/<module>.yaml`, errors (pointing at `lyx update`) on a missing file or any missing template key-path, builds env via `envsource`, and resolves `${env:...}` via `yamlengine`. Remove the old "defaults + overlay" / `$env:NAME ? fallback` description and the `DefaultConfig()` mention. Note the `${env:NAME}` / `${env:NAME:-default}` grammar and that defaults now live in each module's live-YAML template.
  - Create `docs/shared-libs/yamlengine.md`: document `Resolve`, `Reconcile`, and `MissingKeys`; the `${env:...}` grammar (required vs `:-default`, interpolation, literal defaults, no escape, no recursion); the `yaml.Node`/nested-leaf model; and the purity guarantee (no I/O, caller supplies env).
  - Create `docs/shared-libs/envsource.md`: document `Build(baseDir)` — `.env` parsing + OS overlay (OS wins), eager, the single env-sourcing policy point.
  - `docs/shared-libs/paths.md`: document the new `LyxDirName` constant and `ConfigDir`/`ConfigFile`/`DotEnv` helpers.
  - `docs/shared-libs/README.md`: update the `config.md` line to reflect the strict engine-backed loader; add `yamlengine.md` and `envsource.md` entries.
  - `docs/overview.md`: update the CLI module list to add `update` (reconcile module configs against templates) and note that `init` now scaffolds all module configs (board, worktree, weft) plus `.gitignore`; update the `_lyx/config/` description if it references the old defaults model. Keep edits factual and aligned with the shipped code.
  - Add a short MIGRATION note (in `config.md` or `overview.md`): existing installs whose config files are in the old commented format — or pre-existing weft worktrees lacking `_lyx/config/weft.yaml` — will fail strict `Load` until `lyx update --apply` is run. Running `lyx update --apply` from the host worktree reconciles `weft.yaml` too, because the host `_lyx` is a directory junction into the weft worktree's `_lyx` (so the single host baseDir reaches the weft config file); no separate command in the weft sibling is required.
  - All Markdown follows the markdown skill's conventions.
- **Commit:** `docs: document yamlengine, envsource, strict config.Load, and lyx update`

## Batch Tests

`verify: null` — this is a pure documentation batch with no runnable surface. The
content is validated by review against the implemented code (the source files are
listed in `Context:` so the reviewer can confirm accuracy).
