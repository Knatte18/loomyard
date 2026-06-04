# Batch: Store

```yaml
task: Port the wiki module to Go
batch: Store
number: 2
cards: 3
verify: PYTHONPATH= go test ./internal/wiki/
depends-on: [1, 3, 5]
```

## Batch Scope

Implements the in-memory task store over `tasks.json`: load/save, full CRUD, and all validation logic (dangling-dependency checks, cycle detection, isolated/deferred constraints). After this batch the store is fully functional and tested. The external interface consumed by batch 6 is the `Store` type with all its exported methods.

## Cards

### Card 4: internal/wiki/store.go — Store type, load, save

- **Context:**
  - `internal/wiki/task.go`
  - `internal/wiki/git.go`
- **Edits:** none
- **Creates:**
  - `internal/wiki/store.go`
- **Deletes:** none
- **Requirements:** Package `wiki`. Define `Store` struct with unexported fields `tasks []Task` and `filePath string`. Define `NewStore(filePath string) *Store` returning a zeroed store. Define `Load() error`: read `filePath`; if the file does not exist, set `s.tasks = []Task{}` and return nil; otherwise unmarshal JSON as `[]Task` — on any parse error (wrong format, e.g. TinyDB envelope), set `s.tasks = []Task{}` and return nil (silent fallback per shared decision). On successful load, set `DependsOn` to `[]string{}` for any task where it is nil (normalize stored nulls). Define `Save(wikiPath, relPath string) error`: marshal `s.tasks` with `json.MarshalIndent(s.tasks, "", "  ")` then call `atomicWrite(wikiPath, relPath, content)` (from `git.go`, same package). Define `Tasks() []Task` returning a copy of `s.tasks`. Define `slugIndex() map[string]*Task` (unexported) returning a map from slug to pointer into `s.tasks` for O(1) lookups — used internally; callers must not store the pointers across mutations. Define `nextID() int` (unexported) returning `max(existing IDs) + 1`, or `0` if the store is empty.
- **Commit:** `feat(wiki): Store type with load/save`

### Card 5: internal/wiki/store.go — validation and CRUD methods

- **Context:**
  - `internal/wiki/task.go`
  - `internal/wiki/layer.go`
- **Edits:**
  - `internal/wiki/store.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add to `store.go`. Define unexported `validateWrite(snapshot []Task, incoming Task) error` implementing all validation rules from the Python `_validate_write`: (1) dangling-dependency check — every slug in `incoming.DependsOn` must exist in `snapshot`; (2) target-not-schedulable check — a dep cannot be isolated or deferred; (3) cycle detection via DFS — build adjacency list from `snapshot` (with `incoming` replacing its own entry), then DFS from `incoming.Slug`; a gray node encountered during traversal is a cycle; return `fmt.Errorf("cycle detected: %q -> %q", incoming.Slug, gray_slug)`; (4) reverse-isolate check — if `incoming.Isolated`, no task in `snapshot` may have `incoming.Slug` in its DependsOn; (5) reverse-defer check — if `incoming.Deferred`, no non-deferred task in `snapshot` may depend on `incoming.Slug`. Implement exported CRUD methods: `UpsertTask(fields map[string]interface{}) (Task, error)` — if slug exists call `applyPatch(existing, fields)`, else call `newTask(fields, s.nextID())`; call `validateWrite` with full snapshot (existing tasks replacing the updated one); on success append/replace in `s.tasks`; return the upserted task. `GetTask(idOrSlug interface{}) (Task, bool)` — match by int ID or string slug; return task and true, or zero-value and false. `RemoveTask(idOrSlug interface{}) error` — if not found return `fmt.Errorf("task not found: %v", idOrSlug)`; otherwise remove from `s.tasks`. `SetPhase(idOrSlug interface{}, phase *string) error` — find task, set `Status = phase` (nil clears it); no-op if not found (return nil). Note: unlike `SetDeps`, `SetPhase` returns nil for a missing task — this asymmetry is intentional and matches the Python behavior (SetPhase is idempotent/advisory; SetDeps is structural and must error on invalid input). `SetDeps(slug string, dependsOn []string) error` — if not found return error; otherwise update DependsOn and call `validateWrite`. `ListTasksBrief() []BriefTask` — compute layers via `computeLayers` (from `layer.go`); return `[]BriefTask` where each row has all Task fields except Body, plus `Layer string` and `HasProposal bool` (true if Body non-empty). `ListTasksFull() []Task` — return copy of all tasks. `UpsertTasksBatch(tasks []map[string]interface{}) error` — project the full post-operation snapshot, validate all incoming tasks against it, then execute all upserts; return first validation error encountered. `MergeTasks(removeSlugs []string, upsert map[string]interface{}, setPhase *[2]interface{}) (Task, error)` — project snapshot (snapshot minus removeSlugs); validate the upserted task against the projected snapshot; on success: remove all removeSlugs, upsert, and if setPhase non-nil call `SetPhase(setPhase[0], phase)`; return the upserted task. Define `BriefTask` struct in `store.go` (not `task.go`) with fields: `ID int` (`json:"id"`), `Slug string` (`json:"slug"`), `Title string` (`json:"title"`), `DependsOn []string` (`json:"depends_on"`), `Isolated bool` (`json:"isolated"`), `Deferred bool` (`json:"deferred"`), `Brief string` (`json:"brief"`), `Status *string` (`json:"status,omitempty"`), `Layer string` (`json:"layer"`), `HasProposal bool` (`json:"has_proposal"`).
- **Commit:** `feat(wiki): Store CRUD, validation, and batch operations`

### Card 6: internal/wiki/store_test.go

- **Context:**
  - `internal/wiki/task.go`
  - `internal/wiki/store.go`
  - `internal/wiki/layer.go`
- **Edits:** none
- **Creates:**
  - `internal/wiki/store_test.go`
- **Deletes:** none
- **Requirements:** Package `wiki_test`. Each test creates a `NewStore("")` (in-memory only; no file I/O). Test `UpsertTask`: (a) new task gets sequential ID starting at 0; (b) defaults applied (DependsOn=[], Isolated=false, Deferred=false); (c) update preserves unmentioned fields; (d) `group` key returns validation error (propagated from `applyPatch`). Test `validateWrite` paths: (e) dangling dependency rejected; (f) dependency on isolated task rejected; (g) dependency on deferred task rejected; (h) cycle A→B, B→A detected and rejected with "cycle detected" in error message; (i) chain A depends on B, B depends on C — no cycle, all upserts succeed. Test `RemoveTask`: (j) returns error for missing slug. Test `SetPhase`: (k) nil phase clears status; (l) no-op (nil return) for missing slug. Test `MergeTasks`: (m) remove + upsert + set_phase all execute atomically — after the call, removed slugs are gone, upserted task exists, phase is set; (n) validation error on upsert rolls back — nothing is mutated. Test `ListTasksBrief`: (o) returns Layer and HasProposal computed correctly. Test `SetDeps`: (p) valid update succeeds; (q) setting deps that create a cycle returns error and leaves store unchanged. Test `UpsertTasksBatch`: (r) valid batch of two tasks both upserted; (s) batch with one invalid task (dangling dep) returns error and neither task is mutated. Use `t.Fatal` on unexpected errors; use `t.Errorf` for assertion failures. No third-party assertion libraries.
- **Commit:** `test(wiki): Store CRUD and validation tests`

## Batch Tests

`go test ./internal/wiki/` compiles the full `internal/wiki` package (task.go + store.go + layer.go) and runs `task_test.go` and `store_test.go`. All tests are in-memory — no filesystem or git access. `store.go` calls `computeLayers` from `layer.go`, so this batch depends on both batch 1 (task.go) and batch 3 (layer.go) — reflected in `depends-on: [1, 3, 5]` in the DAG.
