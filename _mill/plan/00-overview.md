# Plan: Extract shared primitives (paths, output)

```yaml
task: Extract shared primitives (paths, output)
slug: mhgo-extract-primitives
approved: true
started: 20260611-090558
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shared-primitives
    file: 01-shared-primitives.md
    depends-on: []
    verify: go test ./internal/config/... ./internal/git/... ./internal/output/...
  - number: 2
    name: board-adoption-and-docs
    file: 02-board-adoption-and-docs.md
    depends-on: [1]
    verify: go test ./internal/board/...
```

## Shared Decisions

### Decision: behaviour-preserving extraction

- **Decision:** Every change in this plan preserves observable behaviour. board's
  existing test suite (`internal/board`, `internal/board/boardtest`) plus the
  existing `internal/config` tests are the guardrail; no JSON key, error string, or
  config-resolution semantics may change.
- **Rationale:** This task is a refactor milestone ahead of the `worktree` module
  (`docs/roadmap.md`). The roadmap mandates behaviour-preserving extraction so
  nothing observable changes until the consumer module arrives.
- **Applies to:** all batches

### Decision: cwd-authoritative, no upward walk

- **Decision:** `config.FindBaseDir(cwd)` performs a strict check that
  `<cwd>/_mhgo` exists; it never walks up to parent directories.
- **Rationale:** `docs/shared-libs/config.md` documents the cwd-authoritative rule.
  An upward walk would change board's "not initialized" semantics and break
  `TestLoad_UninitializedDir`.
- **Applies to:** shared-primitives

### Decision: preserve exact error text

- **Decision:** `config.FindBaseDir` returns the same generic message `Load` emits
  today — `"not initialized: _mhgo/ directory not found in %s"` — and wraps
  non-`IsNotExist` stat errors as `"stat _mhgo: %w"`. `Load` keeps its signature
  `Load(baseDir, module string, defaults map[string]string)` and delegates the
  existence check to `FindBaseDir`.
- **Rationale:** `internal/board/config.go` `LoadConfig` matches the substring
  `"not initialized"` to rewrap into `not initialized here; run "mhgo init"`.
  Preserving the text keeps that rewrap and board's behaviour unchanged.
- **Applies to:** shared-primitives

### Decision: output envelope shape and exit codes

- **Decision:** New package `internal/output` exposes
  `Ok(w io.Writer, fields map[string]any) int` (injects `fields["ok"] = true`,
  marshals the single map, writes one line, returns `0`) and
  `Err(w io.Writer, msg string) int` (marshals
  `map[string]any{"ok": false, "error": msg}`, writes one line, returns `1`).
  Marshal errors are ignored (`data, _ := json.Marshal(...)`), matching the
  existing `writeJSON`.
- **Rationale:** Single-map marshaling keeps Go's alphabetical key ordering
  identical to board's current output; returning the exit code matches board's
  `return outputSuccess(out)` / `return outputError(out, msg)` call pattern.
- **Applies to:** all batches

### Decision: Go test runner, scoped per batch

- **Decision:** `verify:` uses the native Go runner with no `PYTHONPATH=` prefix
  (this is a Go module, not a Python/mill project). Each batch scopes `go test` to
  the packages it touches.
- **Rationale:** `verify-not-isolated` is conditional on project language; Go
  projects use `go test` directly. Scoping keeps each verify fast.
- **Applies to:** all batches

## All Files Touched

- `docs/shared-libs/config.md`
- `docs/shared-libs/git.md`
- `internal/board/cli.go`
- `internal/board/init.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/git/git.go`
- `internal/git/git_test.go`
- `internal/output/output.go`
- `internal/output/output_test.go`
