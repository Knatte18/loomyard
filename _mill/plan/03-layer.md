# Batch: Layer

```yaml
task: Port the wiki module to Go
batch: Layer
number: 3
cards: 2
verify: go test ./internal/wiki/
depends-on: [1]
```

## Batch Scope

Implements topological layer computation and the rendering helpers that annotate tasks with their layer bucket. After this batch, `computeLayers`, `renderOrder`, and `extendedTitle` are available for use by batch 4 (render.go) and batch 2 (store.go's `ListTasksBrief`).

## Cards

### Card 7: internal/wiki/layer.go

- **Context:**
  - `internal/wiki/task.go`
- **Edits:** none
- **Creates:**
  - `internal/wiki/layer.go`
- **Deletes:** none
- **Requirements:** Package `wiki`. Implement `computeLayers(tasks []Task) (map[string]string, error)`. The function assigns each task a bucket string: `"__done__"` if `Status == "done"`; `"__deferred__"` if `Deferred`; `"Z"` if `Isolated`; otherwise a letter `"A"`–`"Y"` derived from topological depth. Topological depth of a task = 1 + max depth of its effective dependencies (those whose status is not `"done"`). A task with no effective dependencies has depth 0 = Layer A. Depth 24 = Layer Y. Return an error if any non-special task has depth ≥ 25 (`"layer depth exceeds A..Y cap"`). Use a two-phase approach matching the Python source: (1) DFS to detect cycles (color white/gray/black — skip done tasks' deps); return error on gray node encounter; (2) memoized recursive depth calculation. Dependencies on done tasks are excluded from the depth calculation (they are satisfied). Implement `renderOrder(tasks []Task) ([]Task, error)`: calls `computeLayers`, returns tasks sorted by bucket order then by ID. Bucket order: letter buckets A–Y (alphabetical), then Z, then `__deferred__`, then `__done__`. Each task in the returned slice has a synthetic `Status` pointer set to the layer string only if it is a letter or Z — do NOT overwrite the task's actual Status. Instead, annotate by returning a new slice of tasks with an added unexported `layer` string tracked separately. Actually: define `TaskWithLayer` struct embedding `Task` plus `Layer string`. `renderOrder` returns `[]TaskWithLayer`. Implement `extendedTitle(t Task, layer string) string`: returns `title + " [" + layer + "]"` for letter/Z buckets; plain title for `__done__` and `__deferred__`.
- **Commit:** `feat(wiki): layer computation and render order`

### Card 8: internal/wiki/layer_test.go

- **Context:**
  - `internal/wiki/task.go`
  - `internal/wiki/layer.go`
- **Edits:** none
- **Creates:**
  - `internal/wiki/layer_test.go`
- **Deletes:** none
- **Requirements:** Package `wiki_test`. Test `computeLayers`: (a) single task no deps → `"A"`; (b) A depends on B → A=`"B"`, B=`"A"` (B is a prerequisite so lower layer); (c) done task → `"__done__"`, excluded from depth of dependents; (d) deferred task → `"__deferred__"`; (e) isolated task → `"Z"`; (f) chain of 3 (A→B→C, C is root) → C=A, B=B, A=C; (g) depth ≥ 25 returns error. Test `renderOrder`: (h) buckets appear in correct order (letter then Z then deferred then done); (i) tasks within same bucket sorted by ID. Test `extendedTitle`: (j) letter bucket → title + " [A]"; (k) done → plain title; (l) deferred → plain title.
- **Commit:** `test(wiki): layer computation tests`

## Batch Tests

`go test ./internal/wiki/` compiles task.go + layer.go and runs task_test.go + layer_test.go. No filesystem or git access.
