# Discussion: Port the wiki module to Go

```yaml
task: Port the wiki module to Go
slug: wiki-go-port
status: discussing
parent: main
```

## Problem

Millhouse's wiki module is a task tracker that renders a GitHub wiki as its view layer. The Python implementation (~2300 lines across seven files) carries substantial daemon/socket machinery that exists purely because Python starts slowly and to keep the backend language-swappable. Neither reason applies to Go.

MHGO is an experimental repo for a Go reimplementation of Millhouse, and this is its first module. Porting the wiki module to Go is both a practical goal (a faster, simpler task tracker) and a Go learning vehicle for the developer.

## Scope

**In:**
- `internal/wiki/` library package: `task.go`, `store.go`, `layer.go`, `render.go`, `git.go`, `lock.go`
- `cmd/mhgo/main.go` — thin CLI over the library
- Unit tests for all pure logic (store, layer, render)
- Integration tests for git operations using `WIKI_SKIP_GIT=1`
- `go.mod` and dependency management (`github.com/gofrs/flock`)

**Out:**
- Millhouse mill-skills — mhgo is a standalone repo, no integration with millhouse
- Daemon/socket machinery (dropped entirely)
- TinyDB compatibility/migration (fresh `tasks.json` in Go format)
- `parse_home_md` (Home.md is a derived file, never parsed back)
- `migrate_group_to_deps` (one-time Python migration, not needed)
- `health` and `shutdown` ops (daemon-only)
- WSL2 build optimisation (follow-up if needed)

## Decisions

### No daemon

- **Decision:** No long-lived process, no TCP socket, no token auth. Each `mhgo wiki` invocation is a short-lived process.
- **Rationale:** Go starts fast (~1–5 ms), so amortising startup cost via a daemon is unnecessary. Removing the daemon deletes ~1000 lines of machinery from the port.
- **Rejected:** Keeping a daemon for concurrency serialisation — replaced by a file lock.

### Concurrency: file lock + git rebase-retry

- **Decision:** An exclusive file lock (`github.com/gofrs/flock`) serialises same-machine writers. Cross-machine races are handled by `git pull --rebase` retry in `commit_push`.
- **Rationale:** `flock` is cross-platform, auto-releases on process death (no stale locks), and requires only one small dependency. With lock-free reads the write lock is only held during the `pull → mutate → render → commit → push` sequence.
- **Rejected:** Pure-stdlib `O_EXCL`/mkdir lockfile — requires manual stale-lock detection.

### Lock-free reads

- **Decision:** `tasks.json` is written atomically (temp + rename). Read operations (`get`, `list`) take no lock.
- **Rationale:** An atomic rename guarantees readers see either the old or new file, never a torn write. This eliminates read contention entirely.

### Synchronous push under write lock (v1)

- **Decision:** The write lock wraps `pull → mutate → render → commit → push`. Push is synchronous.
- **Rationale:** GitHub wiki view stays fresh on every write. The cost (two concurrent writers serialise for ~1–3 s) is acceptable for v1.
- **Rejected:** Async push — requires a surviving background process (re-introduces daemon complexity) and makes the GitHub view staler.

### tasks.json format

- **Decision:** Plain formatted JSON array of task objects (indented, human-readable). Fresh start — no TinyDB migration. Go binary initialises a new `tasks.json` if none exists.
- **Rationale:** TinyDB's `{"_default": {"1": {...}}}` envelope is Python-library internals. In Go, `encoding/json` with `json.MarshalIndent` gives a clean, readable file. mhgo is a new repo with no prior data requiring migration.
- **Rejected:** TinyDB format compatibility, one-time migration command.

### go.mod module path

- **Decision:** `github.com/Knatte18/mhgo`
- **Rationale:** Matches the GitHub remote; conventional for Go modules.

### CLI surface

- **Decision:** `mhgo wiki <subcommand>` with full parity to the Python daemon ops (minus daemon-only ops). Mutation subcommands take a JSON string argument. Read subcommands output JSON to stdout. Errors output `{"ok": false, "error": "..."}` to stdout with exit code 1.
- **Subcommands:** `upsert`, `set-phase`, `remove`, `get`, `list`, `list-full`, `merge`, `set-deps`, `rerender`
- **Rationale:** JSON input matches the daemon's payload shape exactly, trivial to script. JSON output is consistent and machine-readable. Uniform error format makes error handling in callers straightforward.
- **Rejected:** Per-field flags (verbose, hard to keep in sync with schema); plain-text output (hard to parse).

### Testing approach

- **Decision:** Unit tests for all pure logic (task validation, cycle detection, layer computation, render output). Integration tests with `WIKI_SKIP_GIT=1` env var for store operations. Git rebase-retry logic tested with a real temp git repo.
- **Rationale:** Pure logic is fast to test in isolation. `WIKI_SKIP_GIT=1` mirrors the Python test pattern and keeps most tests dependency-free. The rebase-retry path must be covered since it is the cross-machine concurrency guarantee.

## Technical context

### Source being ported

The installed Python source (not the dev repo) lives at the mill plugin cache and is the reference for all logic. Key files:

- `wiki/_store.py` — `Store` class: `upsert_task`, `get_task`, `remove_task`, `set_phase`, `list_tasks_brief`, `list_tasks_full`, `set_deps`, `upsert_tasks_batch`, `merge_tasks`. Cycle detection via DFS. Validation: dangling deps, isolated/deferred constraints, reverse-dep checks.
- `wiki/_render.py` — `compute_layers` (topological sort → letters A-Y, Z for isolated, `__done__`, `__deferred__`), `render` (tasks → `Home.md`, `_Sidebar.md`, `proposal-*.md`), `render_order`, `extended_title`.
- `wiki/_sync.py` — `path_guard`, `atomic_write` (temp + rename), `pull` (--ff-only), `commit_push` (add → diff → commit → push with one rebase-retry on non-fast-forward).
- `wiki/_server.py` — orchestration: `_render_and_commit_all` wraps pull + store.reload + render + atomic_write + commit_push. Also manages orphan `proposal-*.md` deletion.

### Task schema (Go)

```go
type Task struct {
    ID        int      `json:"id"`
    Slug      string   `json:"slug"`
    Title     string   `json:"title"`
    DependsOn []string `json:"depends_on"`
    Isolated  bool     `json:"isolated"`
    Deferred  bool     `json:"deferred"`
    Brief     string   `json:"brief"`
    Body      string   `json:"body"`
    Status    *string  `json:"status,omitempty"`
}
```

### Package layout

```
github.com/Knatte18/mhgo/
  go.mod
  internal/wiki/
    task.go      — Task type, validation helpers
    store.go     — CRUD over tasks.json (load/save/mutate)
    layer.go     — compute_layers, render_order, extended_title
    render.go    — render() → map[string]string (filename → content)
    git.go       — path_guard, atomic_write, pull, commit_push
    lock.go      — AcquireWriteLock / release via gofrs/flock
  cmd/mhgo/
    main.go      — CLI entrypoint, subcommand dispatch
```

### Render logic key details

- Layer letters A–Y (max depth 24). Depth ≥ 25 is an error.
- Buckets in order: letter buckets (A, B, … sorted) + Z + `__deferred__` + `__done__`.
- Home.md heading format: `## **#NNN:** Title [Layer]` (no layer suffix for done/deferred).
- Slug line: `[slug](proposal-slug.md)` if task has body, else `[slug]`. Append `[status]` for active/done/pr-pending/ready-to-merge/abandoned.
- Sidebar: one bullet per task, grouped by bucket, empty line between groups.
- Orphan `proposal-*.md` files must be deleted when a task loses its body.

### Commit_push sequence

1. `git add -- <paths>`
2. `git diff --cached --quiet` — if exit 0 (nothing staged), return (idempotent)
3. `git commit -m "wiki: <slug>"`
4. `git push` — on non-fast-forward: `git pull --rebase`, retry push once. On rebase conflict: `git rebase --abort`, return error.

### Write lock sequence (every mutating operation)

1. Acquire exclusive file lock (`tasks.json.lock` adjacent to `tasks.json`)
2. `pull --ff-only`
3. Load/mutate tasks
4. `render` → `atomic_write` each output file
5. Delete orphan proposal files
6. `commit_push`
7. Release lock (deferred)

### env vars

- `WIKI_SKIP_GIT=1` — skips pull, commit, push entirely (render + write only). For unit/integration tests.
- `WIKI_SKIP_PUSH=1` — pull + commit run, push is skipped. For commit-log tests.

## Constraints

- Go 1.26.4, portable install at `C:\Code\tools\go1.26.4.windows-amd64`. Generics and `slices`/`maps`/`cmp` packages available.
- Cortex XDR inflates Go builds (~1.7 s incremental, ~18 s cold). Do not run `go clean -cache`.
- Only external dependency permitted: `github.com/gofrs/flock` for the write lock.
- Windows target (primary). Cross-platform correctness required for `flock`.

## Testing

### task.go
- Validate all field types: `depends_on` must be `[]string`, `isolated`/`deferred` must be `bool`.
- `group` key rejected.

### store.go
- `upsert_task`: new task gets sequential ID, defaults applied correctly.
- `upsert_task`: update merges fields (existing fields preserved if not in update).
- Cycle detection: A→B→A detected and rejected.
- Dangling dependency: depends on non-existent slug rejected.
- Isolated task cannot have dependents.
- Deferred task cannot have non-deferred dependents.
- `merge_tasks`: validate-before-mutate; remove + upsert + set_phase atomic.
- `remove_task`: no-op if slug not found.
- `set_phase`: nil clears status field.
- TDD candidates: cycle detection DFS, `_validate_write` edge cases.

### layer.go
- Single task with no deps → Layer A.
- Chain A→B→C → layers A=C, B=B, C=A (C blocked by B blocked by A... wait, depends_on means "I depend on X" so if A depends_on B, A is in a higher layer than B).
- `done` tasks excluded from layer computation (treated as satisfied deps).
- Isolated → Z, deferred → `__deferred__`, done → `__done__`.
- Depth ≥ 25 returns error.

### render.go
- Home.md heading format correct for all bucket types.
- Proposal file generated when task has non-empty body.
- No proposal file when body is empty.
- Orphan detection: tasks with body removed → proposal file removed from output.
- Sidebar grouping and blank-line separators.
- `render_order` sort: letter buckets first (alphabetical), then Z, deferred, done, secondary sort by ID.

### git.go
- `path_guard`: empty string, absolute path, `..` component all rejected.
- `atomic_write`: file written correctly, temp file cleaned up on error.
- `pull`: returns true if updated, false if already up to date.
- `commit_push`: idempotent when nothing staged.
- Rebase-retry path: tested with real temp git repo simulating non-fast-forward.
- Rebase conflict path: `rebase --abort` called, error returned.

### lock.go
- Lock acquired and released correctly.
- Lock auto-releases on process death (gofrs/flock guarantee, test via subprocess).

### cmd/mhgo
- Each subcommand produces correct JSON output.
- Error cases produce `{"ok": false, "error": "..."}` with exit code 1.
- `WIKI_SKIP_GIT=1` flows through correctly.

## Q&A log

- **Q:** Lock implementation — `gofrs/flock` or stdlib O_EXCL? **A:** `github.com/gofrs/flock` — cross-platform, auto-releases on death.
- **Q:** go.mod module path? **A:** `github.com/Knatte18/mhgo`.
- **Q:** tasks.json migration from TinyDB format? **A:** No migration needed — mhgo starts fresh, no prior data.
- **Q:** CLI input format for mutations? **A:** JSON string argument.
- **Q:** Testing without git? **A:** `WIKI_SKIP_GIT=1` env var pattern.
- **Q:** Full CLI parity or reduced v1? **A:** Full parity.
- **Q:** Mill-skill integration in scope? **A:** No — mhgo is standalone, no millhouse dependency.
- **Q:** CLI output format? **A:** JSON to stdout for reads; `{"ok":...}` for writes/errors.
- **Q:** Test coverage scope? **A:** All relevant logic — unit tests for pure logic, integration for git ops.
- **Q:** CLI error format? **A:** `{"ok": false, "error": "..."}` to stdout, exit code 1.
