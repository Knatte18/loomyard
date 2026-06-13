# Module: board

The board module (`internal/board`) is the task tracker: it stores tasks in a
`tasks.json` file, renders human-readable board pages from them, and backs the
whole thing up to a GitHub repo. It is driven by `mhgo board <subcommand>` (see
[overview.md](../overview.md) for the dispatcher).

A write only touches the filesystem and returns; the git backup runs in a
detached background process (see [Background sync](#background-sync)).

## Internal dependency graph

```
cli.go            (RunCLI: parse + dispatch + JSON output)
  └── board.go    (Board facade: writeOp; spawns the sync)
        ├── lock.go      (write + read/swap + push locks)
        ├── store.go     (load / mutate / save)
        │     ├── task.go      (Task, NewTask, ApplyPatch)
        │     └── layer.go     (ComputeLayers – used by ListTasksBrief)
        ├── render.go    (tasks → board files)
        │     └── layer.go     (RenderOrder)
        ├── git.go       (AtomicWrite, RunGit, Pull, CommitPush)
        └── sync.go      (Sync: background commit + push)
              └── spawn_*.go   (launch the sync detached + windowless)
```

`internal/board` is a single Go package, so every file uses the others' types and
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
  rather than masked as an empty board.
- **`Save(boardPath, relPath)`** — marshals tasks to formatted JSON and calls
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
Owns **all** of the board's readable `.md` output — building it, writing it, and
cleaning it up. board.go never touches a markdown file.

- **`Render(tasks, out)`** — the pure core: tasks in, `map[relPath]content` out (no
  I/O). Takes an `Outputs` struct with configurable home/sidebar filenames and
  proposal prefix. Built by three helpers — `renderHome`, `renderSidebar`,
  `renderProposals`:
  - Home file (configurable name, default `Home.md`) — sectioned per bucket with
    `# Layer X` / `# Someday` / `# Done` headings. Each task: `## **#NNN:** Title [Layer]`,
    a slug line, an optional `Depends on:`, and an optional brief.
  - Sidebar file (configurable name, default `_Sidebar.md`) — one line per task,
    grouped per bucket.
  - Proposal files (using the configured prefix, default `proposal-`) — one file per
    task with a non-empty `Body` (content is the body verbatim).
- **`RenderToDisk(boardPath, tasks, out)`** — renders and persists: `AtomicWrite`s
  every file, then removes any proposal files (using the configured prefix) the
  render no longer produces (a task lost its body or was removed). This is the
  single call the write path makes for rendering.

### git.go
The filesystem + git plumbing.

- **`PathGuard(relPath)`** — rejects empty paths, absolute paths (incl. Windows
  `C:\...`), and `..` components.
- **`AtomicWrite(boardPath, relPath, content)`** — writes a temp file then
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

### board.go
The facade. No business logic — only orchestration.

- **`writeOp(mutate, _)`** — every write, **file-only**:
  1. `MkdirAll(boardPath)` — create the board directory if missing (must happen
     before acquiring the write lock, since the lock file lives in the board dir)
  2. Acquire the write lock
  3. `store.Load()`
  4. `mutate(store)` — the change itself
  5. `store.Save()` — `tasks.json`, the source of truth, persisted first
  6. `RenderToDisk(...)` — render.go writes the derived `.md` files and removes orphans
  7. Launch a detached `mhgo board sync` (unless `BOARD_SKIP_GIT=1`) and return
  8. Release the lock (deferred)
- Read ops (`GetTask`, `ListTasksBrief`, `ListTasksFull`) short-circuit when the
  board directory is absent — they return empty results (`list` → `[]`, `get` →
  not found) without taking the swap lock. Reads never `MkdirAll` (no filesystem
  side effects on read).

### cli.go
`RunCLI(out, args)` resolves the board configuration from the current working
directory (os.Getwd() + LoadConfig), then parses `<subcommand> [json-payload]`,
deserialises the JSON argument, calls one method on `board.Board`, and writes the
result to `out` as JSON. Returns the exit code (0/1).

Configuration resolution (cwd-authoritative): the cwd must contain `_mhgo/`
directory. If absent, `LoadConfig` errors with "not initialized here; run
\"mhgo init\"". Otherwise configuration is loaded from layered YAML files and
merged with defaults (see [Configuration](#configuration)).

When `--board-path` is present (internal flag for the detached sync child), it
bypasses config resolution entirely and uses the provided absolute path. The
child never checks for `_mhgo/` or re-resolves configuration.

Subcommands: `upsert`, `upsert-batch`, `set-phase`, `remove`, `get`, `list`,
`list-full`, `merge`, `set-deps`, `rerender`, `sync`.

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

Writes are file-only and fast; backing the board up to GitHub is handed to a
detached `mhgo board sync` process, so a write never waits on git. (A write is
~10 ms of work; a push ~4 s — see [benchmarks.md](benchmarks.md).)

**Why async.** Every `mhgo` process on a machine shares the same `tasks.json`, so
they see each other's changes immediately through the file — git is only for
replicating to GitHub. That makes remote backup a background concern the write
path can drop.

**sync.go.** `Sync(boardPath)` is what the detached process runs (and can be run by
hand to force a backup):
- Takes `push.lock` so only one pusher runs at a time.
- Loops: under the write lock, `git add -A` + commit pending changes; then,
  lock-free, push with a `pull --rebase` retry on non-fast-forward. Repeats until
  the tree is clean and nothing is unpushed.
- Ensures the board's committed `.gitignore` excludes the lock files (`*.lock`,
  `*.swaplock`) so they are never pushed.
- `BOARD_SKIP_GIT=1` disables it; `BOARD_SKIP_PUSH=1` commits but skips the push.

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

## Configuration

> **Target redesign (not yet implemented).** The model below is what the board
> code does *today*: a three-layer merge including a gitignored `.mhgo/board.yaml`
> override. A planned milestone extracts this into the shared `internal/config`
> package and **drops the `.mhgo/` config layer** in favour of `$env:NAME ? default`
> references plus a `.env` file. See [shared-libs/config.md](../shared-libs/config.md)
> and [roadmap.md](../roadmap.md). This section is updated when that milestone lands.

The board module's configuration is defined in a layered YAML system, read fresh
on every invocation from the current working directory. The system supports
environment variable expansion and path resolution.

### Layered model

Configuration is assembled from three sources, merged per key:
1. **Built-in defaults** — fallback for any unspecified key
2. **`<cwd>/_mhgo/board.yaml`** (optional, git-tracked) — team/repo-wide settings
3. **`<cwd>/.mhgo/board.yaml`** (optional, gitignored) — machine-local overrides

A key absent from a higher layer falls through to the layer below, then to the
built-in default. If `_mhgo/` does not exist, `LoadConfig` errors with the
message "not initialized here; run \"mhgo init\"".

### Keys and defaults

```yaml
path: ../_board       # board directory (tasks.json + rendered output)
                      # relative to cwd; may contain $env:... ; resolved via filepath.Join
home: Home.md         # home file name; set to README.md to render on a repo landing page
sidebar: _Sidebar.md  # sidebar file name
proposal_prefix: proposal-  # prefix for proposal-<slug>.md files
```

### Environment variable expansion

After all layers are merged, `$env:NAME` tokens (where `NAME` matches
`[A-Za-z_][A-Za-z0-9_]*`) anywhere within a string value are replaced with the
value of `os.Getenv("NAME")`. A referenced-but-unset environment variable is a
**hard error** — the command fails with a message like "referenced env var
`NAME` is not set". This is intentional: silent fallback to an empty string
would lead to silent failures downstream.

The token may appear mid-value (e.g. `path: $env:MHGO_BOARD_PATH/sub`).

### Path resolution

After environment expansion, a relative `path` (one not starting with `/` or a
Windows absolute path like `C:\...`) is resolved against `cwd` via
`filepath.Join(cwd, path)`. An absolute path is used as-is.

## init

`mhgo init` is a top-level command (at the `main.go` module dispatcher, beside
`mhgo board ...`) that scaffolds the configuration layer. It is fully idempotent
and re-run safe.

### What it does

1. Create `<cwd>/_mhgo/` if missing
2. Write a fully-commented `_mhgo/board.yaml` (if absent) — every key shown with
   its default value, commented out, with explanatory comments. Active values are
   not written, so code defaults always apply until the user uncomments and edits
   a key.
3. Maintain a `.gitignore` managed block (see below), containing `.mhgo/`
4. Print a single-line JSON action summary

### Example

```sh
$ mhgo init
{"ok":true,"mhgo_dir":"created","board_yaml":"created","gitignore":"updated"}
```

Subsequent runs are idempotent:

```sh
$ mhgo init
{"ok":true,"mhgo_dir":"exists","board_yaml":"exists","gitignore":"unchanged"}
```

### The managed block

`init` maintains an `# === mhgo-managed ===` ... `# === end mhgo-managed ===`
marker block in `<cwd>/.gitignore` (creating the file if absent). The block
contains `.mhgo/`. Idempotent: only rewrites when the block's interior content
differs (trailing newline and surrounding whitespace don't trigger rewrites).

The `mhgo-managed` marker deliberately does not collide with millpy's separate
`mill-managed` block.

## Data flow: upsert

Command: `mhgo board upsert '{"slug": "my-task", "title": "Do something"}'`

```
main.go → board.RunCLI (cli.go)
  │  LoadConfig(cwd, "board") — resolve from _mhgo/ layers
  │  parse args → subcommand="upsert", jsonPayload='{"slug":...}'
  │  json.Unmarshal → fields map[string]any
  │  b.UpsertTask(fields)
  │
board.go / writeOp                          (file-only — no git, returns in ms)
  1. MkdirAll(boardPath)                    ← create board dir if missing
  2. AcquireWriteLock("tasks.json.lock")   ← blocks if another process holds it
  3. store.Load()                           ← read tasks.json from disk
  4. store.UpsertTask(fields)               ← mutation (see below)
  5. store.Save()                           ← write tasks.json atomically (source of truth)
  6. RenderToDisk(boardPath, tasks, out)    ← render.go: write .md files + drop orphans
  7. spawnSync(boardPath)                    ← detached `mhgo board sync`, NOT waited on
  8. lock.Release()                         ← deferred

  (later, in the background)
sync.go / Sync(boardPath)                    ← own process; push.lock; commit + push
  loop: git add -A → commit "board sync" → push (pull --rebase on reject)
        until clean & nothing unpushed (coalesces a burst into ~1 push)

store.go / UpsertTask(fields)
  │  slug not found → NewTask(fields, nextID)  (JSON round-trip validates types)
  │  validateWrite(s.tasks, incoming)          (dangling/isolated/deferred/cycle)
  │  append(s.tasks, incoming) → returns Task

cli.go
  → stdout: {"ok":true,"task":{"id":0,"slug":"my-task",...}}
```

## Environment variables

| Variable          | Effect                                              |
|-------------------|-----------------------------------------------------|
| `BOARD_SKIP_GIT=1`| Writes do not spawn the background sync, and `Sync` is a no-op. File I/O only (used by tests). |
| `BOARD_SKIP_PUSH=1`| `Sync` commits locally but skips the network push.  |

## Tests

`task_test.go`, `store_test.go`, `layer_test.go`, `render_test.go`, `git_test.go`,
`lock_test.go`, `sync_test.go`, `cli_test.go` are the per-file unit tests (the
`sync_test.go` suite runs against a local bare repo — no network). The
cross-cutting benchmark, concurrency, and integration suites live in
`internal/board/boardtest`; see [benchmarks.md](benchmarks.md).
