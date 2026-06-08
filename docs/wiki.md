# Module: wiki

The wiki module (`internal/wiki`) is the task tracker: it stores tasks in a
`tasks.json` file, renders human-readable wiki pages from them, and backs the
whole thing up to a GitHub repo. It is driven by `mhgo wiki <subcommand>` (see
[overview.md](overview.md) for the dispatcher).

A write only touches the filesystem and returns; the git backup runs in a
detached background process (see [Background sync](#background-sync)).

## Internal dependency graph

```
cli.go            (RunCLI: parse + dispatch + JSON output)
  └── wiki.go     (Wiki facade: writeOp; spawns the sync)
        ├── lock.go      (write + read/swap + push locks)
        ├── store.go     (load / mutate / save)
        │     ├── task.go      (Task, NewTask, ApplyPatch)
        │     └── layer.go     (ComputeLayers – used by ListTasksBrief)
        ├── render.go    (tasks → wiki files)
        │     └── layer.go     (RenderOrder)
        ├── git.go       (AtomicWrite, RunGit, Pull, CommitPush)
        └── sync.go      (Sync: background commit + push)
              └── spawn_*.go   (launch the sync detached + windowless)
```

`internal/wiki` is a single Go package, so every file uses the others' types and
functions directly.

## The files in depth

### task.go
Defines the `Task` struct (what is stored in `tasks.json`) and two helpers:

- **`NewTask(fields, nextID)`** — builds a new Task from a `map[string]any` via a
  JSON round-trip: fields are marshalled to JSON and unmarshalled into Task, so
  wrong types (e.g. `depends_on` that is not a `[]string`) become validation
  errors.
- **`ApplyPatch(existing, fields)`** — updates an existing Task with the same JSON
  round-trip, starting from `existing` serialised to a map, overlaying the new
  fields. Fields not in `fields` are left unchanged.

`Status *string` is a pointer because `nil` means "not set" and is omitted from
JSON (`omitempty`); an empty string would be included.

### store.go
Holds the task list in memory and exposes CRUD operations.

- **`Load()`** — reads `tasks.json` under a shared swap lock. A missing file or
  invalid JSON yields an empty list (no error); a genuine read error is surfaced
  rather than masked as an empty wiki.
- **`Save(wikiPath, relPath)`** — marshals tasks to formatted JSON and calls
  `AtomicWrite` (temp + rename) while holding the exclusive swap lock.
- **`validateWrite(snapshot, incoming)`** — runs before every mutation:
  1. Dangling dependency: every slug in `DependsOn` must exist in the snapshot
  2. A dependency on an isolated/deferred task is forbidden
  3. Cycle detection via DFS (white/gray/black colouring)
  4. Reverse-isolate: no existing task may depend on a task being marked isolated
  5. Reverse-defer: no non-deferred task may depend on a task being marked deferred
- **`UpsertTask` / `RemoveTask` / `SetPhase` / `SetDeps`** — single operations.
- **`UpsertTasksBatch`** — validates all entries against a projected snapshot
  before any mutation.
- **`MergeTasks`** — remove + upsert + set_phase atomically: validates against the
  snapshot minus removed slugs, then applies everything.

### layer.go
Topological sorting of tasks into "buckets".

- **`ComputeLayers(tasks)`** — returns `map[slug]bucket`:
  - `"__done__"` — status == "done"
  - `"__deferred__"` — Deferred == true
  - `"Z"` — Isolated == true
  - `"A"`–`"Y"` — depth in the dependency graph (depth 0 = A, no dependencies;
    depth 1 = B; …). Depth ≥ 25 is an error.

  Two DFS passes: pass 1 detects cycles, pass 2 computes depth with memoization.
- **`RenderOrder(tasks)`** — tasks sorted by bucket order (A→Y, Z, deferred, done),
  then by ID.
- **`ExtendedTitle(task, layer)`** — title with a `[layer]` suffix for active
  tasks, none for done/deferred.

### render.go
Owns **all** of the wiki's readable `.md` output — building it, writing it, and
cleaning it up. wiki.go never touches a markdown file.

- **`Render(tasks)`** — the pure core: tasks in, `map[relPath]content` out (no
  I/O). Built by three helpers — `renderHome`, `renderSidebar`,
  `renderProposals`:
  - `"Home.md"` — sectioned per bucket with `# Layer X` / `# Someday` / `# Done`
    headings. Each task: `## **#NNN:** Title [Layer]`, a slug line, an optional
    `Depends on:`, and an optional brief.
  - `"_Sidebar.md"` — one line per task, grouped per bucket.
  - `"proposal-<slug>.md"` — one file per task with a non-empty `Body` (content is
    the body verbatim).
- **`RenderToDisk(wikiPath, tasks)`** — renders and persists: `AtomicWrite`s every
  file, then removes any `proposal-*.md` the render no longer produces (a task
  lost its body or was removed). This is the single call the write path makes for
  rendering.

### git.go
The filesystem + git plumbing.

- **`PathGuard(relPath)`** — rejects empty paths, absolute paths (incl. Windows
  `C:\...`), and `..` components.
- **`AtomicWrite(wikiPath, relPath, content)`** — writes a temp file then
  `os.Rename` to the destination (atomic within a filesystem — readers never see
  a half-written file). The swap fence around the rename lives in
  `store.Save`/`store.Load`.
- **`RunGit(args, cwd)`** — runs a git command (windowless on Windows, so no
  console flashes), returning stdout/stderr/exit code.
- **`Pull` / `CommitPush`** — git helpers retained for direct use and the
  integration tests; the live write path no longer calls them (see below).

### lock.go
A wrapper around `github.com/gofrs/flock`. `FileLock` backs both an exclusive and
a shared lock and coordinates **across processes** — the way mhgo is used, one
short-lived process per command.

- **`AcquireWriteLock(lockPath)`** — exclusive; blocks until free.
- **`AcquireReadLock(lockPath)`** — shared; many readers at once, blocked only by
  an exclusive holder.

See [Concurrency model](#concurrency-model) for how the three locks fit together.

### wiki.go
The facade. No business logic — only orchestration.

- **`writeOp(mutate, _)`** — every write, **file-only**:
  1. Acquire the write lock
  2. `store.Load()`
  3. `mutate(store)` — the change itself
  4. `store.Save()` — `tasks.json`, the source of truth, persisted first
  5. `RenderToDisk(...)` — render.go writes the derived `.md` files and removes orphans
  6. Launch a detached `mhgo wiki sync` (unless `WIKI_SKIP_GIT=1`) and return
  7. Release the lock (deferred)
- Read ops (`GetTask`, `ListTasksBrief`, `ListTasksFull`) bypass `writeOp` — they
  read straight from disk under only the shared swap lock, so they are never
  blocked by a write or a sync.

### cli.go
`RunCLI(out, args)` parses `[--wiki-path <path>] <subcommand> [json-payload]`,
resolves the wiki path (flag → `MHGO_WIKI_PATH` → `../gowiki`), deserialises the
JSON argument, calls one method on `wiki.Wiki`, and writes the result to `out` as
JSON. Returns the exit code (0/1). Subcommands: `upsert`, `upsert-batch`,
`set-phase`, `remove`, `get`, `list`, `list-full`, `merge`, `set-deps`,
`rerender`, `sync`.

## Concurrency model

Three independent file locks, each held only as long as needed:

- **`tasks.json.lock`** (write lock) — held during a writer's mutation and during
  the sync's commit. Writes are file-only, so it no longer spans git — held for
  milliseconds, not a network round-trip. Serialises file-state changes.
- **`tasks.json.swaplock`** (swap lock) — held only around the file swap in
  `store.Save` / `store.Load`. Readers take it shared, the rename takes it
  exclusive, so on Windows the rename never loses a sharing-violation race against
  an open reader, and a reader never sees a half-written file.
- **`tasks.json.push.lock`** — held by `Sync` for its whole loop, guaranteeing a
  single active pusher.

Reads take only the shared swap lock; writers take the write lock (briefly) plus
the swap lock for the rename; the sync takes the push lock plus the write lock for
its commit. No path holds a lock across the network.

## Background sync

Writes are file-only and fast; backing the wiki up to GitHub is handed to a
detached `mhgo wiki sync` process, so a write never waits on git. (A write is
~10 ms of work; a push ~4 s — see [benchmarks.md](benchmarks.md).)

**Why async.** Every `mhgo` process on a machine shares the same `tasks.json`, so
they see each other's changes immediately through the file — git is only for
replicating to GitHub. That makes remote backup a background concern the write
path can drop.

**sync.go.** `Sync(wikiPath)` is what the detached process runs (and can be run by
hand to force a backup):
- Takes `push.lock` so only one pusher runs at a time.
- Loops: under the write lock, `git add -A` + commit pending changes; then,
  lock-free, push with a `pull --rebase` retry on non-fast-forward. Repeats until
  the tree is clean and nothing is unpushed.
- Ensures the wiki's committed `.gitignore` excludes the lock files (`*.lock`,
  `*.swaplock`) so they are never pushed.
- `WIKI_SKIP_GIT=1` disables it; `WIKI_SKIP_PUSH=1` commits but skips the push.

**Coalescing & the wakeup guarantee.** `git push` sends all commits ahead of the
remote, so one push covers many commits — a burst of writes collapses into ~1
push. Every write spawns its own pusher; the `push.lock` lets one do the work
while the rest block, then exit once there is nothing left. Even if the running
pushers miss a change, that write's own pusher will acquire the lock, see the
dirty state, and push it.

**Failure & recovery.**
- *Offline:* the push fails but the commit stays local; `git push` is cumulative,
  so the next successful push (from any later write) sends the backlog. This is
  self-healing as long as writes keep happening while online.
- *Cross-machine:* the pusher's `pull --rebase` folds in another machine's
  commits; a rebase conflict aborts and retries on the next loop.
- *Launch:* `spawn_windows.go` / `spawn_other.go` start the sync detached and
  windowless (Windows `CREATE_NO_WINDOW`, so no console flashes). A failed spawn
  just defers backup to the next write.

The only change that can stay un-pushed is the last write before the machine goes
offline forever; it is safe on local disk, and there is deliberately no periodic
safety-net sync.

## Data flow: upsert

Command: `mhgo wiki upsert '{"slug": "my-task", "title": "Do something"}'`

```
main.go → wiki.RunCLI (cli.go)
  │  parse args → subcommand="upsert", jsonPayload='{"slug":...}'
  │  json.Unmarshal → fields map[string]any
  │  w.UpsertTask(fields)
  │
wiki.go / writeOp                           (file-only — no git, returns in ms)
  1. AcquireWriteLock("tasks.json.lock")   ← blocks if another process holds it
  2. store.Load()                           ← read tasks.json from disk
  3. store.UpsertTask(fields)               ← mutation (see below)
  4. store.Save()                           ← write tasks.json atomically (source of truth)
  5. RenderToDisk(wikiPath, tasks)          ← render.go: write .md files + drop orphans
  6. spawnSync(wikiPath)                     ← detached `mhgo wiki sync`, NOT waited on
  7. lock.Release()                         ← deferred

  (later, in the background)
sync.go / Sync(wikiPath)                     ← own process; push.lock; commit + push
  loop: git add -A → commit "wiki sync" → push (pull --rebase on reject)
        until clean & nothing unpushed (coalesces a burst into ~1 push)

store.go / UpsertTask(fields)
  │  slug not found → NewTask(fields, nextID())  (JSON round-trip validates types)
  │  validateWrite(s.tasks, incoming)            (dangling/isolated/deferred/cycle)
  │  append(s.tasks, incoming) → returns Task

cli.go
  → stdout: {"ok":true,"task":{"id":0,"slug":"my-task",...}}
```

## Environment variables

| Variable          | Effect                                              |
|-------------------|-----------------------------------------------------|
| `WIKI_SKIP_GIT=1` | Writes do not spawn the background sync, and `Sync` is a no-op. File I/O only (used by tests). |
| `WIKI_SKIP_PUSH=1`| `Sync` commits locally but skips the network push.  |
| `MHGO_WIKI_PATH`  | Sets the wiki directory (alternative to `--wiki-path`). |

## Tests

`task_test.go`, `store_test.go`, `layer_test.go`, `render_test.go`, `git_test.go`,
`lock_test.go`, `sync_test.go`, `cli_test.go` are the per-file unit tests (the
`sync_test.go` suite runs against a local bare repo — no network). The
cross-cutting benchmark, concurrency, and integration suites live in
`internal/wiki/wikitest`; see [benchmarks.md](benchmarks.md).
