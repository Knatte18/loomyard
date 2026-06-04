# Batch: Wiki and CLI

```yaml
task: Port the wiki module to Go
batch: Wiki and CLI
number: 6
cards: 3
verify: PYTHONPATH= go test ./...
depends-on: [2, 4, 5]
```

## Batch Scope

Implements the `Wiki` orchestration type (lock + pull + store + render + commit/push) and the `mhgo wiki` CLI. After this batch the binary is fully functional: `mhgo wiki <subcommand>` accepts JSON input and produces JSON output for all supported operations.

## Cards

### Card 15: internal/wiki/wiki.go + internal/wiki/wiki_test.go

- **Context:**
  - `internal/wiki/task.go`
  - `internal/wiki/store.go`
  - `internal/wiki/layer.go`
  - `internal/wiki/render.go`
  - `internal/wiki/git.go`
  - `internal/wiki/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/wiki/wiki.go`
  - `internal/wiki/wiki_test.go`
- **Deletes:** none
- **Requirements:** Package `wiki`. Define `Wiki` struct: `wikiPath string`. Define `New(wikiPath string) *Wiki`. Implement the full write sequence as a private method `(w *Wiki) writeOp(mutate func(*Store) (interface{}, error)) (interface{}, error)`: (1) acquire write lock on `filepath.Join(wikiPath, "tasks.json.lock")`; defer release; (2) if `WIKI_SKIP_GIT != "1"` and `WIKI_SKIP_PUSH != "1"`: call `pull(wikiPath)`, ignoring pull errors (mirrors Python behavior); (3) create `NewStore(filepath.Join(wikiPath, "tasks.json"))` and call `Load()`; (4) call `mutate(store)` to get result and error; return on error; (5) call `render(store.Tasks())`; on error return; (6) for each entry in the render map: call `atomicWrite(wikiPath, relPath, content)`; (7) delete orphan `proposal-*.md` files: list existing `proposal-*.md` in wikiPath, delete those not in the render map; (8) call `store.Save(wikiPath, "tasks.json")`; (9) if `WIKI_SKIP_GIT != "1"`: collect commit paths (render keys + `"tasks.json"` + orphan deletions), call `commitPush(wikiPath, paths, "wiki: "+slug_for_msg)`; return result. Implement exported methods that call `writeOp` with appropriate mutate closures: `UpsertTask(fields map[string]interface{}) (Task, error)`, `SetPhase(idOrSlug interface{}, phase *string) error`, `RemoveTask(idOrSlug interface{}) error`, `MergeTasks(removeSlugs []string, upsert map[string]interface{}, setPhase *[2]interface{}) (Task, error)`, `SetDeps(slug string, dependsOn []string) error`, `UpsertTasksBatch(tasks []map[string]interface{}) error`, `Rerender() error`. Implement read methods (no lock): `GetTask(idOrSlug interface{}) (Task, bool, error)` — loads store, returns task; `ListTasksBrief() ([]BriefTask, error)` — loads store, returns brief list; `ListTasksFull() ([]Task, error)` — loads store, returns all tasks. The `slug_for_msg` in the commit message is derived from the upserted/mutated slug where available; use `"rerender"` for Rerender. In `wiki_test.go`: set `WIKI_SKIP_GIT=1` via `t.Setenv`. Test `UpsertTask`: (a) creates task, tasks.json written, Home.md written; (b) update preserves other fields. Test `RemoveTask`: (c) error for missing slug. Test `Rerender`: (d) writes all output files without error on an empty store.
- **Commit:** `feat(wiki): Wiki orchestration type`

### Card 16: cmd/mhgo/main.go

- **Context:**
  - `internal/wiki/task.go`
  - `internal/wiki/store.go`
  - `internal/wiki/wiki.go`
- **Edits:** none
- **Creates:**
  - `cmd/mhgo/main.go`
- **Deletes:** none
- **Requirements:** Package `main`. The CLI entry point. Parse `os.Args`: `mhgo wiki <subcommand> [json-payload]`. If args are wrong, print usage to stderr and exit 1. The `--wiki-path` flag (default: `$MHGO_WIKI_PATH` env var, or `".wiki"` if unset) sets the wiki directory path. All output goes to stdout as JSON. On success, mutation subcommands output `{"ok": true}` (plus `"task": <task>` where the operation returns a task). On error, output `{"ok": false, "error": "<message>"}` and exit 1. Read subcommands output their data directly (no `"ok"` wrapper for list/get: `get` outputs `{"ok": true, "task": <task_or_null>}`, `list` outputs `{"ok": true, "tasks": [...]}`, `list-full` outputs `{"ok": true, "tasks": [...]}`). Subcommand dispatch: `upsert` — read JSON payload arg, unmarshal to `map[string]interface{}`, call `w.UpsertTask(fields)`, output `{"ok":true,"task":<task>}`; `upsert-batch` — payload is `{"tasks": [...]}`, unmarshal, call `w.UpsertTasksBatch(tasks)`, output `{"ok":true,"count":<n>}`; `set-phase` — payload `{"id_or_slug": ..., "phase": ...}` where phase may be null, call `w.SetPhase`; `remove` — payload `{"id_or_slug": ...}`, call `w.RemoveTask`; `get` — payload `{"id_or_slug": ...}`, call `w.GetTask`, output `{"ok":true,"task":<task_or_null>}`; `list` — no payload, call `w.ListTasksBrief`, output `{"ok":true,"tasks":[...]}`; `list-full` — no payload, call `w.ListTasksFull`; `merge` — payload `{"remove_slugs":[...],"upsert":{...},"set_phase":[id_or_slug,phase_or_null]}`, call `w.MergeTasks`; `set-deps` — payload `{"slug":"...","depends_on":[...]}`, call `w.SetDeps`; `rerender` — no payload, call `w.Rerender`. Use `encoding/json` for all marshaling. Exit codes: 0 for success, 1 for any error.
- **Commit:** `feat(cmd/mhgo): CLI subcommand dispatch`

### Card 17: cmd/mhgo/main_test.go

- **Context:**
  - `internal/wiki/task.go`
  - `internal/wiki/wiki.go`
  - `cmd/mhgo/main.go`
- **Edits:** none
- **Creates:**
  - `cmd/mhgo/main_test.go`
- **Deletes:** none
- **Requirements:** Package `main`. Integration tests using `os/exec` to invoke the compiled binary, or by calling the dispatch logic directly if extracted to a testable function. Set `WIKI_SKIP_GIT=1` in the test environment. Tests use a `t.TempDir()` as the wiki path (initialized as a bare git repo with `git init` if needed). Test: (a) `mhgo wiki upsert '{"slug":"foo","title":"Foo task"}'` → exit 0, stdout parses as `{"ok":true,"task":{...}}`; (b) `mhgo wiki list` → exit 0, `tasks` array contains the upserted task with `layer` and `has_proposal` fields; (c) `mhgo wiki get '{"id_or_slug":"foo"}'` → exit 0, task returned; (d) `mhgo wiki get '{"id_or_slug":"nonexistent"}'` → exit 0, `{"ok":true,"task":null}`; (e) `mhgo wiki remove '{"id_or_slug":"nonexistent"}'` → exit 1, `{"ok":false,"error":"task not found: nonexistent"}`; (f) `mhgo wiki set-phase '{"id_or_slug":"foo","phase":"active"}'` → exit 0; (g) `mhgo wiki rerender` → exit 0, Home.md exists in wiki path. If building the binary per test is slow, build once in `TestMain` using `go build`.
- **Commit:** `test(cmd/mhgo): CLI integration tests`

## Batch Tests

`go test ./...` runs all tests in all packages. CLI tests in `cmd/mhgo/` use `WIKI_SKIP_GIT=1` and `t.TempDir()` for full isolation. The test for the rebase-retry path (batch 5 card 12k) also runs here as part of `./internal/wiki/`.
