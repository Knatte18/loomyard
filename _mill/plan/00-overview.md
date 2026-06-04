# Plan: Port the wiki module to Go

```yaml
task: Port the wiki module to Go
slug: wiki-go-port
approved: true
started: 20260604-121026
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: Bootstrap
    file: 01-bootstrap.md
    depends-on: []
    verify: PYTHONPATH= go test ./internal/wiki/

  - number: 2
    name: Store
    file: 02-store.md
    depends-on: [1, 3, 5]
    verify: PYTHONPATH= go test ./internal/wiki/

  - number: 3
    name: Layer
    file: 03-layer.md
    depends-on: [1]
    verify: PYTHONPATH= go test ./internal/wiki/

  - number: 4
    name: Render
    file: 04-render.md
    depends-on: [3]
    verify: PYTHONPATH= go test ./internal/wiki/

  - number: 5
    name: Git and Lock
    file: 05-git-lock.md
    depends-on: [1]
    verify: PYTHONPATH= go test ./internal/wiki/

  - number: 6
    name: Wiki and CLI
    file: 06-wiki-cli.md
    depends-on: [2, 4, 5]
    verify: PYTHONPATH= go test ./...
```

## Shared Decisions

### Decision: Go module and package name

- **Decision:** Module path `github.com/Knatte18/mhgo`. All library code lives in package `wiki` under `internal/wiki/`. CLI binary is package `main` under `cmd/mhgo/`.
- **Rationale:** Matches GitHub remote; `internal/` prevents external import of the library package.
- **Applies to:** all batches.

### Decision: Error handling

- **Decision:** All exported functions return `error` as the last return value. Errors are wrapped with `fmt.Errorf("context: %w", err)` at each layer boundary. No panics except for programmer errors (nil pointer dereference, index out of range).
- **Rationale:** Standard Go idiom; wrapping preserves the chain for `errors.Is` / `errors.As`.
- **Applies to:** all batches.

### Decision: JSON round-trip for map↔struct

- **Decision:** UpsertTask and related mutation ops accept `map[string]interface{}` as the input (the decoded JSON from the caller). The store marshals the map back to JSON, unmarshals into `Task` to validate types, then applies defaults and merges. On unmarshal failure (e.g. wrong field types), returns a validation error.
- **Rationale:** Mirrors Python's dict-merge semantics exactly — missing keys keep existing values. Avoids a separate `TaskPatch` pointer-field type while staying type-safe.
- **Applies to:** batches 2, 6.

### Decision: env vars for test isolation

- **Decision:** `WIKI_SKIP_GIT=1` — skip pull, commit, push entirely (render + write files only). `WIKI_SKIP_PUSH=1` — pull + commit, skip push. Checked via `os.Getenv` in `wiki.go`.
- **Rationale:** Mirrors Python pattern; keeps most tests fast and dependency-free.
- **Applies to:** batches 5, 6.

### Decision: tasks.json format

- **Decision:** `[]Task` JSON array, written with `json.MarshalIndent(tasks, "", "  ")`. On load, if the file is missing or not a valid `[]Task` array, initialize with an empty slice (no error). The Go binary is the canonical writer; TinyDB-format files from the Python daemon are silently ignored on first load.
- **Rationale:** Clean format; `encoding/json` handles the full lifecycle.
- **Applies to:** batch 2.

### Decision: write lock file path

- **Decision:** Lock file is `<wiki_path>/tasks.json.lock`. Acquired via `github.com/gofrs/flock` before every mutation; released on return (deferred).
- **Rationale:** Adjacent to tasks.json; the file's existence is the lock signal.
- **Applies to:** batches 5, 6.

### Decision: atomic_write implementation

- **Decision:** `atomicWrite(wikiPath, relPath, content string) error` resolves `dest = filepath.Join(wikiPath, relPath)`, writes to a temp file in the same directory, then `os.Rename`s to `dest`. Uses `os.CreateTemp` with the same parent dir to ensure rename is atomic (same filesystem).
- **Rationale:** Prevents torn reads; same approach as Python `_sync.py`.
- **Applies to:** batches 2, 5.

## All Files Touched

- `cmd/mhgo/main.go`
- `cmd/mhgo/main_test.go`
- `go.mod`
- `go.sum`
- `internal/wiki/git.go`
- `internal/wiki/git_test.go`
- `internal/wiki/layer.go`
- `internal/wiki/layer_test.go`
- `internal/wiki/lock.go`
- `internal/wiki/lock_test.go`
- `internal/wiki/render.go`
- `internal/wiki/render_test.go`
- `internal/wiki/store.go`
- `internal/wiki/store_test.go`
- `internal/wiki/task.go`
- `internal/wiki/task_test.go`
- `internal/wiki/wiki.go`
- `internal/wiki/wiki_test.go`
