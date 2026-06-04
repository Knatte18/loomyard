# Batch: Bootstrap

```yaml
task: Port the wiki module to Go
batch: Bootstrap
number: 1
cards: 3
verify: PYTHONPATH= go test ./internal/wiki/
depends-on: []
```

## Batch Scope

Creates the Go module, declares the only external dependency (`github.com/gofrs/flock`), and implements the `Task` type that every subsequent batch builds on. After this batch, `go test ./internal/wiki/` compiles and passes with task-level tests. The external interface consumed by batch 2 is the `Task` struct and the `applyPatch` / `newTask` helpers exported from `task.go`.

## Cards

### Card 1: go.mod and go.sum

- **Context:** none
- **Edits:** none
- **Creates:**
  - `go.mod`
  - `go.sum`
- **Deletes:** none
- **Requirements:** Create `go.mod` with module path `github.com/Knatte18/mhgo` and `go 1.26`. Add `require github.com/gofrs/flock` at latest stable version by running `go get github.com/gofrs/flock@latest` then `go mod tidy` to generate `go.sum` and download the dependency. The file must be valid for `go build ./...` after the batch completes.
- **Commit:** `chore: init go module with gofrs/flock dependency`

### Card 2: internal/wiki/task.go

- **Context:** none
- **Edits:** none
- **Creates:**
  - `internal/wiki/task.go`
- **Deletes:** none
- **Requirements:** Package `wiki`. Define the `Task` struct with fields exactly matching the JSON schema: `ID int`, `Slug string`, `Title string`, `DependsOn []string`, `Isolated bool`, `Deferred bool`, `Brief string`, `Body string`, `Status *string`. JSON tags: `id`, `slug`, `title`, `depends_on`, `isolated`, `deferred`, `brief`, `body`, `status` (Status uses `omitempty`). DependsOn must have `json:"depends_on"` and default to `[]string{}` (never nil in a stored task — see `newTask`). Define `newTask(fields map[string]interface{}, nextID int) (Task, error)` which starts from defaults (`DependsOn: []string{}`, `Isolated: false`, `Deferred: false`, `Brief: ""`, `Body: ""`, `Status: nil`), overlays the provided `fields` via JSON round-trip (marshal fields → unmarshal into Task), and sets `ID = nextID` and `Slug` from `fields["slug"]`. Returns an error if `slug` key is missing or empty. Define `applyPatch(existing Task, fields map[string]interface{}) (Task, error)` which marshals `existing` to a `map[string]interface{}`, overlays `fields`, then unmarshals back into a `Task`. Returns error if slug is missing in the result. The JSON round-trip approach is the canonical merge strategy — do not hand-code per-field merging. Reject the `group` key: if `fields["group"]` is present (non-nil), return `fmt.Errorf("group key is not allowed; use depends_on, isolated, deferred instead")`. Both functions check for the `group` key before the round-trip.
- **Commit:** `feat(wiki): add Task struct and patch helpers`

### Card 3: internal/wiki/task_test.go

- **Context:**
  - `internal/wiki/task.go`
- **Edits:** none
- **Creates:**
  - `internal/wiki/task_test.go`
- **Deletes:** none
- **Requirements:** Package `wiki_test`. Test `newTask`: (a) creates task with correct defaults when only slug provided; (b) ID is set to the provided nextID; (c) missing slug returns error; (d) `group` key present returns error. Test `applyPatch`: (a) overlays title onto existing task, other fields unchanged; (b) `DependsOn` is updated when provided; (c) `group` key returns error; (d) existing Status is preserved when not in patch; (e) Status can be cleared by patching with `"status": nil`. All tests use `go test` assertions via the standard `testing` package (no third-party test framework).
- **Commit:** `test(wiki): task struct and patch helper tests`

## Batch Tests

`go test ./internal/wiki/` compiles and runs all `*_test.go` files in `internal/wiki/`. After this batch only `task_test.go` exists. Tests cover `newTask` and `applyPatch` with 5 cases each as described in Card 3. No network or filesystem access in this batch.
