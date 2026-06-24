# Plan: Extract yamlengine and migrate config via lyx update

```yaml
task: "Extract yamlengine and migrate config via lyx update"
slug: yamlengine
approved: false
started: "20260624-080534"
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: yamlengine-engine
    file: 01-yamlengine-engine.md
    depends-on: []
    verify: go test ./internal/yamlengine/...
  - number: 2
    name: envsource
    file: 02-envsource.md
    depends-on: [3]
    verify: go test ./internal/envsource/...
  - number: 3
    name: paths-config-helpers
    file: 03-paths-config-helpers.md
    depends-on: []
    verify: go test ./internal/paths/...
  - number: 4
    name: templates-live-yaml
    file: 04-templates-live-yaml.md
    depends-on: [1]
    verify: go test ./internal/board/ ./internal/worktree/ ./internal/weft/
  - number: 5
    name: config-engine-integration
    file: 05-config-engine-integration.md
    depends-on: [1, 2, 3, 4]
    verify: go test ./internal/config/ ./internal/board/... ./internal/worktree/ ./internal/weft/ ./internal/ide/
  - number: 6
    name: lyx-update-init
    file: 06-lyx-update-init.md
    depends-on: [1, 3, 4]
    verify: go test ./internal/configreg/ ./internal/configsync/ ./internal/update/ ./internal/initcli/ ./internal/configcli/... ./cmd/lyx/
  - number: 7
    name: docs
    file: 07-docs.md
    depends-on: [5, 6]
    verify: null
```

## Shared Decisions

### Decision: env-marker grammar

- **Decision:** Config values use POSIX-style brace-delimited env markers: `${env:NAME}` (required — error if the var is absent) and `${env:NAME:-default}` (optional — use the literal default text between `:-` and the closing `}` when the var is absent or empty). The default is taken verbatim: spaces preserved, NO quote-stripping, NO trimming; `${env:VAR:-}` yields an empty-string default. Markers may appear inside a larger string (interpolation), e.g. `path: ${env:LYX_BOARD_PATH:-../_board}/sub`. A value with no `${...}` marker is a literal (the YAML-decoded scalar, verbatim). No escape exists for a literal `${env:` or a literal `}` inside a default (out of scope). No recursive re-expansion of resolved text.
- **Rationale:** Brace-delimiting is the generic, recognized convention (shell, docker-compose, envsubst) and the only clean way to allow interpolation once defaults exist; the `env:` namespace keeps the placeholder signal.
- **Applies to:** all batches (engine implements it; templates use it; tests assert it).

### Decision: nested config via yaml.Node

- **Decision:** The engine works on `gopkg.in/yaml.v3` `yaml.Node` trees, not `map[string]string`. `Resolve` walks every scalar leaf (at any depth) and expands `${env:...}`. Typed wrappers unmarshal the resolved YAML into their own structs. `map[string]string` is dropped from the config layer.
- **Rationale:** loom will grow nested per-reviewer sub-configs; Node-based handling makes nesting work for both Resolve and Reconcile and removes manual field-mapping.
- **Applies to:** all batches.

### Decision: engine is pure; I/O lives in callers

- **Decision:** `internal/yamlengine` does NO I/O (no file reads, no OS env). The caller supplies the env map. `internal/envsource.Build` is the single place that sources env (`.env` + OS overlay). `internal/config.Load` does the file reads and strict-key policy.
- **Rationale:** keeps the engine reusable and unit-testable; isolates "how env enters the system."
- **Applies to:** batches 1, 2, 5, 6.

### Decision: paths resolved through internal/paths

- **Decision:** The `_lyx` directory name, the `config/` subdir, and `.env` filename are centralized in `internal/paths` (constant + helpers). No new code hardcodes these literals; all geometry goes through `paths` per CONSTRAINTS.md.
- **Rationale:** one place to change the layout later; satisfies the build-enforced path invariant.
- **Applies to:** batches 3, 5, 6.

### Decision: Go conventions, JSON CLI output, atomic writes

- **Decision:** Standard-library `testing` with table-driven tests; godoc on every exported symbol per the golang-comments skill. All CLI commands emit JSON on stdout via `internal/output` (exit 1 on error). Config-file writes that mutate disk use `internal/fsx.AtomicWriteBytes` (temp + rename).
- **Rationale:** matches the existing codebase conventions.
- **Applies to:** all batches.

### Decision: registry ownership and no import cycle

- **Decision:** The module registry (name → embedded template) moves out of `internal/configcli` into a neutral `internal/configreg` package that imports `board`/`worktree`/`weft`. `internal/configsync` (shared reconcile-over-registry) is consumed by both `internal/update` (lyx update) and `internal/initcli` (lyx init). `lyx init` moves out of the `board` package into `internal/initcli` so `board` never imports the registry (avoids a board↔registry cycle).
- **Rationale:** init is no longer board-specific; a neutral registry breaks the cycle and lets init/update/config share one source of truth.
- **Applies to:** batch 6.

## All Files Touched

- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `docs/overview.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/config.md`
- `docs/shared-libs/envsource.md`
- `docs/shared-libs/paths.md`
- `docs/shared-libs/yamlengine.md`
- `internal/board/board_test.go`
- `internal/board/boardtest/bench_test.go`
- `internal/board/boardtest/concurrency_test.go`
- `internal/board/boardtest/sync_test.go`
- `internal/board/cli.go`
- `internal/board/config.go`
- `internal/board/config_test.go`
- `internal/board/init_test.go`
- `internal/board/render_test.go`
- `internal/board/template.go`
- `internal/board/template.yaml`
- `internal/board/template_test.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/config/edit.go`
- `internal/config/edit_test.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_test.go`
- `internal/configcli/menu.go`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/configsync/configsync.go`
- `internal/configsync/configsync_test.go`
- `internal/envsource/envsource.go`
- `internal/envsource/envsource_test.go`
- `internal/ide/menu.go`
- `internal/ide/menu_test.go`
- `internal/initcli/initcli.go`
- `internal/initcli/initcli_test.go`
- `internal/paths/paths.go`
- `internal/paths/paths_test.go`
- `internal/update/update.go`
- `internal/update/update_test.go`
- `internal/weft/config.go`
- `internal/weft/config_test.go`
- `internal/weft/template.go`
- `internal/weft/template.yaml`
- `internal/weft/template_test.go`
- `internal/worktree/config.go`
- `internal/worktree/config_test.go`
- `internal/worktree/template.go`
- `internal/worktree/template.yaml`
- `internal/worktree/template_test.go`
- `internal/yamlengine/reconcile.go`
- `internal/yamlengine/reconcile_test.go`
- `internal/yamlengine/resolve.go`
- `internal/yamlengine/resolve_test.go`
