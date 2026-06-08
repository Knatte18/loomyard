# Architecture: wiki-go-port

## File structure

Module: `github.com/Knatte18/mhgo`

```
github.com/Knatte18/mhgo/
├── cmd/mhgo/
│   └── main.go          CLI entrypoint: routes the <module> argument to the right module
└── internal/wiki/
    ├── task.go          Task type + NewTask / ApplyPatch
    ├── store.go         Store: in-memory CRUD over tasks.json
    ├── layer.go         ComputeLayers, RenderOrder, ExtendedTitle
    ├── render.go        Render: tasks → Home.md, _Sidebar.md, proposal-*.md
    ├── git.go           PathGuard, AtomicWrite, Pull, CommitPush
    ├── lock.go          AcquireWriteLock / AcquireReadLock (gofrs/flock)
    ├── cli.go           RunCLI: parses wiki subcommands, calls wiki.go, writes JSON
    ├── wiki.go          Wiki facade: writeOp sequences everything
    └── wikitest/        Cross-cutting tests: benchmarks, concurrency, integration
```

The per-file unit tests (`task_test.go`, `store_test.go`, …) sit next to the
source they test in `internal/wiki`. The "on-the-side" suites — benchmarks,
concurrency stress, and git-backed integration — live in the black-box
`internal/wiki/wikitest` package; see [benchmarks.md](benchmarks.md).

## Dependency graph

```
main.go
  └── wiki.go            (the only entry point from outside)
        ├── lock.go      (write + read locks)
        ├── git.go       (pull / commit / push)
        ├── store.go     (load / mutate / save)
        │     ├── task.go      (Task type, NewTask, ApplyPatch)
        │     └── layer.go     (ComputeLayers – used by ListTasksBrief)
        └── render.go    (produces the wiki files)
              └── layer.go     (RenderOrder)
```

`internal/wiki` is a single Go package — every file is in `package wiki` and can
use the others' types and functions directly. `cmd/mhgo` is `package main` and is
the only thing that imports `internal/wiki`.

---

## The files in depth

### task.go
Defines the `Task` struct (what gets stored in `tasks.json`) and two helpers:

- **`NewTask(fields, nextID)`** — builds a new Task from a `map[string]any`. Uses
  a JSON round-trip: fields are marshalled to JSON and unmarshalled into Task, so
  wrong types (e.g. `depends_on` that is not a `[]string`) become validation errors.
- **`ApplyPatch(existing, fields)`** — updates an existing Task. Same JSON
  round-trip, but starts by serialising `existing` to a map, overlays the new
  fields, and unmarshals to Task. Fields not present in `fields` are left unchanged.

`Status *string` is a pointer because `nil` means "not set" and is omitted from
JSON (`omitempty`). An empty string would be included in JSON.

---

### store.go
Holds the task list in memory and exposes CRUD operations.

- **`Store`** — struct with `tasks []Task` and the path to `tasks.json`.
- **`Load()`** — reads `tasks.json` from disk under a shared swap lock. A missing
  file or invalid JSON yields an empty list (no error); a genuine read error is
  surfaced rather than masked as an empty wiki.
- **`Save(wikiPath, relPath)`** — marshals tasks to formatted JSON and calls
  `AtomicWrite` (temp + rename) while holding the exclusive swap lock.
- **`validateWrite(snapshot, incoming)`** — runs before every mutation. Checks:
  1. Dangling dependency: every slug in `DependsOn` must exist in the snapshot
  2. A dependency on an isolated/deferred task is forbidden
  3. Cycle detection via DFS (white/gray/black colouring)
  4. Reverse-isolate: no existing task may depend on a task being marked isolated
  5. Reverse-defer: no non-deferred task may depend on a task being marked deferred
- **`UpsertTask`** / **`RemoveTask`** / **`SetPhase`** / **`SetDeps`** — single operations
- **`UpsertTasksBatch`** — validates all entries against a projected snapshot before any mutation
- **`MergeTasks`** — remove + upsert + set_phase atomically: validates against the snapshot minus removed slugs, then applies everything

---

### layer.go
Topological sorting of tasks into "buckets".

- **`ComputeLayers(tasks)`** — returns `map[slug]bucket`:
  - `"__done__"` — status == "done"
  - `"__deferred__"` — Deferred == true
  - `"Z"` — Isolated == true
  - `"A"`–`"Y"` — based on depth in the dependency graph. Depth 0 = A (no
    dependencies), depth 1 = B, and so on. Depth ≥ 25 is an error.

  Implementation: two DFS passes. Pass 1 detects cycles (white/gray/black). Pass 2
  computes depth with memoization.

- **`RenderOrder(tasks)`** — returns tasks sorted by bucket order (A→Y, Z, deferred, done), then by ID.
- **`ExtendedTitle(task, layer)`** — returns the title with a `[layer]` suffix for active tasks, without a suffix for done/deferred.

---

### render.go
Produces the contents of the wiki's markdown files.

- **`Render(tasks)`** — returns `map[relPath]content`:
  - `"Home.md"` — sectioned per bucket with `# Layer X` / `# Someday` / `# Done`
    headings. Each task: `## **#NNN:** Title [Layer]`, a slug line, an optional
    `Depends on:`, and an optional brief.
  - `"_Sidebar.md"` — one line per task, grouped per bucket with a blank line between groups.
  - `"proposal-<slug>.md"` — one file per task with a non-empty `Body`.

---

### git.go
Everything that touches the filesystem and git.

- **`PathGuard(relPath)`** — rejects empty paths, absolute paths (including
  Windows `C:\...`), and paths with `..` components.
- **`AtomicWrite(wikiPath, relPath, content)`** — writes to a temp file in the
  same directory, then `os.Rename` to the destination. Rename is atomic within
  the same filesystem — readers never see a half-written file. The swap fence
  around the rename lives in `store.Save` / `store.Load`.
- **`Pull(wikiPath)`** — `git pull --ff-only`. Returns `true` if the repo was updated.
- **`CommitPush(wikiPath, paths, message)`** — `git add → diff --cached → commit
  → push`. On non-fast-forward: tries `git pull --rebase` once, then pushes
  again. A rebase conflict → `rebase --abort` + error.

---

### lock.go
A wrapper around `github.com/gofrs/flock`. `FileLock` backs both an exclusive and
a shared lock, and coordinates across processes (one short-lived process per command).

- **`AcquireWriteLock(lockPath)`** — exclusive lock. Blocks until free; no other
  exclusive *or* shared lock is granted while it is held.
- **`AcquireReadLock(lockPath)`** — shared lock. Multiple readers may hold it at
  once; it blocks only while a writer holds the exclusive lock.
- **`Release()`** — releases the lock (also released automatically if the process dies).

Two independent locks are used:

- **`tasks.json.lock`** — the coarse write lock, held across the *whole* `writeOp`
  (including git pull/push). Serialises writers against each other.
- **`tasks.json.swaplock`** — the fine lock, held only around the file swap itself
  in `store.Save` / `store.Load`. Readers take it shared, the rename takes it
  exclusive. So on Windows the rename never loses a sharing-violation race against
  an open reader, and a reader never sees a half-written file. Held for
  microseconds — not across git.

---

### wiki.go
The facade used by `main.go`. No business logic here — only orchestration.

- **`writeOp(mutate, slugForMsg)`** — run by every write operation:
  1. Acquire the write lock
  2. `Pull` (unless `WIKI_SKIP_GIT=1`)
  3. `store.Load()`
  4. Call `mutate(store)` — the change itself
  5. `Render(tasks)`
  6. `AtomicWrite` all output files
  7. Delete orphan `proposal-*.md` (tasks that lost their body)
  8. `store.Save()`
  9. `CommitPush` (unless `WIKI_SKIP_GIT=1`)
  10. Release the lock (deferred)

- Read operations (`GetTask`, `ListTasksBrief`, `ListTasksFull`) bypass `writeOp`
  — they read straight from disk, taking only the shared swap lock around the read
  itself, so they are never blocked by an in-flight git operation.

---

### cmd/mhgo/main.go
A thin module router. Takes the first argument (`<module>`) and delegates the
rest to that module's own CLI handler — `mhgo wiki ...` calls `wiki.RunCLI`. Each
module owns its own flags, subcommands, and output. As more modules are added,
only the `switch` here grows.

### internal/wiki/cli.go
The wiki module's CLI handler. `RunCLI(out, args)` parses
`[--wiki-path <path>] <subcommand> [json-payload]`, resolves the wiki path
(flag → `MHGO_WIKI_PATH` → `../gowiki`), deserialises the JSON argument, calls one
method on `wiki.Wiki`, and writes the result to `out` as JSON. Returns the exit
code (0/1) to `main`.

All responses are JSON: `{"ok": true, "task": {...}}` on success,
`{"ok": false, "error": "..."}` on failure (exit code 1).

---

## Data flow: upsert

Command: `mhgo wiki upsert '{"slug": "my-task", "title": "Do something"}'`

```
main.go → wiki.RunCLI (cli.go)
  │  parse args → subcommand="upsert", jsonPayload='{"slug":...}'
  │  json.Unmarshal → fields map[string]any
  │  w.UpsertTask(fields)
  │
wiki.go / UpsertTask
  │  extracts slug for the commit message
  │  calls writeOp(mutate, slug)
  │
wiki.go / writeOp
  1. AcquireWriteLock("tasks.json.lock")   ← blocks if another process holds it
  2. Pull(wikiPath)                         ← git pull --ff-only
  3. store.Load()                           ← read tasks.json from disk
  4. store.UpsertTask(fields)               ← mutation (see below)
  5. Render(store.Tasks())                  ← produce Home.md, _Sidebar.md, proposal-*.md
  6. AtomicWrite(each output filename)      ← temp+rename per file
  7. delete orphan proposal-*.md            ← files no longer in the render output
  8. store.Save()                           ← write tasks.json atomically
  9. CommitPush(paths, "wiki: my-task")     ← git add → commit → push (with rebase retry)
 10. lock.Release()                         ← deferred, released regardless

store.go / UpsertTask(fields)
  │  slugIndex() → map[slug]*Task
  │  slug not found → NewTask(fields, nextID())
  │      json.Marshal(fields) → JSON
  │      json.Unmarshal(JSON, &task) → validate types
  │      set ID and slug explicitly
  │  validateWrite(s.tasks, incoming)
  │      (1) dangling dep check
  │      (2) dep on isolated/deferred forbidden
  │      (3) DFS cycle detection
  │      (4) reverse-isolate check
  │      (5) reverse-defer check
  │  append(s.tasks, incoming)
  └→ returns Task

cli.go
  outputSuccessWithTask(out, task)
  → stdout: {"ok":true,"task":{"id":0,"slug":"my-task","title":"Do something",...}}
```

---

## Environment variables

| Variable          | Effect                                              |
|-------------------|-----------------------------------------------------|
| `WIKI_SKIP_GIT=1` | Skips pull, commit, and push. Render + write only.  |
| `WIKI_SKIP_PUSH=1`| Pull and commit run; push is skipped.               |
| `MHGO_WIKI_PATH`  | Sets the wiki directory (alternative to `--wiki-path`) |
