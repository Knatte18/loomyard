# Plan: Extend codeintel lookup to non-Go languages via LSP

```yaml
task: Extend codeintel lookup to non-Go languages via LSP
slug: codeintel-multilang
approved: false
started: 20260717-172437
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: codeintelengine-core
    file: 01-codeintelengine-core.md
    depends-on: []
    verify: go test ./internal/codeintelengine/...
  - number: 2
    name: lsp-client-and-refs
    file: 02-lsp-client-and-refs.md
    depends-on: [1]
    verify: go test ./internal/codeintelengine/...
  - number: 3
    name: cli-wiring-and-docs
    file: 03-cli-wiring-and-docs.md
    depends-on: [2]
    verify: go test ./internal/codeintelcli/... ./cmd/lyx/...
  - number: 4
    name: measurement-and-writeup
    file: 04-measurement-and-writeup.md
    depends-on: [3]
    verify: go test -tags integration ./internal/codeintelengine/...
```

## Shared Decisions

### Decision: engine/CLI layering (engine returns `(T, error)`, CLI emits the envelope)

- **Decision:** `internal/codeintelengine` returns typed Go errors and typed result values
  and imports **no** `io.Writer` / exit-code / `internal/output` machinery. `internal/codeintelcli`
  is the sole layer that maps engine errors/results to the `internal/output` JSON envelope
  (`output.Ok` / `output.Err`). The engine's import allowlist is stdlib + `internal/hubgeometry`
  + `gopkg.in/yaml.v3` only.
- **Rationale:** CLI/Cobra Invariant (engine returns `(T, error)` with no cobra/`io.Writer`/exit
  codes; cli imports engine, engine never imports cli). Keeps the engine a cycle-free leaf
  importable by builder/webster later, exactly as `internal/modelspec`'s leaf excludes `output`.
- **Applies to:** all batches

### Decision: mirror `internal/modelspec` for the registry

- **Decision:** The language-server registry copies `internal/modelspec`'s shape: a pinned
  Go `builtins()` fallback, an optional operator-editable `servers.yaml` overlay loaded via
  `hubgeometry.ConfigFile(baseDir, "servers")`, an embedded `template.yaml` seed exposed by a
  `ConfigTemplate()` accessor, `yaml.Decoder.KnownFields(true)` strict decoding, whole-entry
  overlay replacement, and loud errors that name the offending entry + file path. An absent
  overlay is **not** an error (built-ins suffice).
- **Rationale:** Proven in-repo pattern; operator can add/repoint a server without a recompile.
- **Applies to:** codeintelengine-core

### Decision: Go native test runner, no `PYTHONPATH=` prefix

- **Decision:** This is a Go module. Every `verify:` uses `go test` directly (no `PYTHONPATH= `
  prefix, which is Python/mill-only). Tests that spawn a language-server subprocess or clone a
  target repo are `//go:build integration`-tagged; the plain `go test` verify runs only the
  untagged (offline, spawn-free) tests per the Test Tier Purity Invariant.
- **Rationale:** verify-not-isolated validator applies the `PYTHONPATH=` rule to Python projects
  only; Go uses the native runner.
- **Applies to:** all batches

### Decision: references-only + `workspace/symbol` resolver; deadline with hard-kill

- **Decision:** The LSP client implements exactly `initialize`, `initialized`,
  `textDocument/references`, `workspace/symbol` (name→position resolver only), `shutdown`,
  `exit`. No `callHierarchy`, no `implementation`. Every reference call is bounded by a
  `context.Context` deadline (`--timeout`, default 30s); on expiry the engine returns
  `ErrServerTimeout` and **hard-kills** the subprocess (`cmd.Process.Kill()`), never the
  graceful `shutdown`/`exit` handshake (which could re-block on the unresponsive server).
- **Rationale:** Exact measurement parity with #008's `refs` arm; the timeout closes the
  "server launches but hangs on initialize" failure mode.
- **Applies to:** lsp-client-and-refs, cli-wiring-and-docs

### Decision: typed error vocabulary

- **Decision:** The engine exposes sentinel errors: `ErrNoLanguage` (no marker matched),
  `ErrServerNotFound` (binary absent on `$PATH`; carries the install hint), `ErrSymbolNotFound`
  (name resolved to zero candidates), `ErrAmbiguousSymbol` (multiple candidates; carries the
  candidate `file:line:col` list), `ErrResolverUnsupported` (server lacks
  `workspaceSymbolProvider`), `ErrServerTimeout` (deadline expired; names the stalled phase).
  The CLI maps each to `output.Err`.
- **Rationale:** Distinguishes each failure mode so callers get an actionable signal instead of
  a generic error.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/sandbox_coverage_test.go`
- `docs/modules/codeintel.md`
- `docs/overview.md`
- `docs/research/codeintel-multilang.md`
- `internal/codeintelcli/cli.go`
- `internal/codeintelcli/cli_test.go`
- `internal/codeintelengine/detect.go`
- `internal/codeintelengine/detect_test.go`
- `internal/codeintelengine/errors.go`
- `internal/codeintelengine/leaf_enforcement_test.go`
- `internal/codeintelengine/load.go`
- `internal/codeintelengine/load_test.go`
- `internal/codeintelengine/lspclient.go`
- `internal/codeintelengine/lspclient_test.go`
- `internal/codeintelengine/position.go`
- `internal/codeintelengine/position_test.go`
- `internal/codeintelengine/refs.go`
- `internal/codeintelengine/refs_integration_test.go`
- `internal/codeintelengine/registry.go`
- `internal/codeintelengine/registry_test.go`
- `internal/codeintelengine/template.go`
- `internal/codeintelengine/template.yaml`
