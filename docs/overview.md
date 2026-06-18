# Overview: Loomyard

Loomyard is a Go toolkit of one-shot CLI modules. Each invocation starts a process,
runs one command, writes JSON to stdout, and exits — there is no daemon and no
shared memory. State lives on disk per module and is coordinated with file locks,
so concurrent `lyx` processes on a machine cooperate through the filesystem. The
first module, **board** (a task tracker), is implemented; **worktree** is
implemented; **muxpoc**, a proof-of-concept orchestrator, is shipped; and the
planned clean `internal/mux` remains design (see [roadmap.md](roadmap.md)).

In the long term, Loomyard is intended to **replace mill/millhouse (Python)** entirely.
We get there by building these modules as self-contained toolkits first;
orchestration comes last. See [Principles](#principles).

Module path: `github.com/Knatte18/loomyard`

## Naming: `lyx` (binary) vs `loom` (skills)

`lyx` is the binary/CLI — **L**oom**Y**ard e**X**ecutable — one binary with a namespaced
subcommand tree (`lyx board`, `lyx weft`, `lyx config`, …). It is the analog of millhouse's
`millpy` backend. The skill / orchestration plugin (the analog of `mill`) is **`loom`**; skills
are `/loom-*`. **Never name skills `lyx-*`** — that recreates the millhouse `mill-spawn`
skill-vs-script ambiguity (which forced the `mill` → `millpy` rename). Internal Go packages
(`internal/board`, `internal/weft`, …) keep their own names and are not user-facing.

## Principles

1. **Toolkit-first.** Build small, composable primitives (board, worktree, mux)
   before any orchestrator that ties them together. mill's Agent Dispatch
   orchestrates for now.
2. **Self-contained modules, deep internal tests.** All of a module's domain logic
   and its test suite live in its own package. What modules share is a thin layer of
   infrastructure plumbing — see [shared-libs/README.md](shared-libs/README.md).
3. **One-shot, daemonless, file-coordinated.** A command does its work, writes JSON,
   exits. Processes cooperate through files + locks, not a server. (The future mux
   daemon is the one deliberate exception, for crash recovery psmux can't self-detect.)
4. **cwd-authoritative; cwd ≠ git-repo-path.** Config and state resolve from the
   current working directory, which need *not* equal the git-repo root. Designed in
   from the start — this was repeatedly forgotten in millpy and caused constant
   trouble.
5. **Full control, incremental milestones.** Land one milestone at a time;
   refactors are behaviour-preserving with the existing test suite as guardrail.
6. **Correctness by tool-design, not by recall.** A `lyx` command should make the *correct* path
   the path of least resistance and make drift *detectable* (`status` / a future `doctor`), rather
   than relying on an agent or operator remembering a rule. No on-disk operation is truly
   un-bypassable when a shell is available, so the achievable bar is "right path is easiest +
   mistakes are detectable," **not** "wrong path impossible." Hard blocks (hooks, permission rules)
   are brittle and out of scope. Example: `lyx weft` owns the overlay's git so raw `git -C` is
   never *needed* (it would be strictly more work), and `lyx weft status` flags drift — but it is a
   friction asymmetry, not a wall.

## Path Invariants

**All worktree and Hub geometry resolves through `internal/paths`.**

The `internal/paths` package is the sole owner of cwd and worktree-root geometry math. It
exposes two entry points:

- `Getwd()` — the only permitted call to `os.Getwd` outside `cmd/lyx/main.go`.
- `Resolve(cwd)` → `Layout` — one-stop geometry: cwd, repo root (from `git rev-parse
  --show-toplevel`), Hub, relative path, and Prime worktree.

The `Layout` type provides geometry methods: `LyxDir()`, `WorktreePath(slug)`,
`PortalsDir()`, `PortalTarget(slug)`, `LaunchersDir()`, `LauncherDir(slug)`, `PrimeName()`.

**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside `internal/paths`
and `cmd/lyx/main.go`. The ban is enforced at `go test` / CI time by
`internal/paths/enforcement_test.go`, which walks the entire source tree and fails the build
if either literal token is found in any non-test `.go` file outside the allowlist.

See [CONSTRAINTS.md](../CONSTRAINTS.md) for details.

## Weft overlay model

lyx organizes overlay artifacts (configuration, task state, codeguide docs, and the board) into a **weft repo** — a companion git repository that stays separate from the host repo, keeping the host pristine.

### Topology

```
<hub>/                              (top-level container, NOT a git repo)
  ├── <prime>/                      (host worktree, main branch; git repo root)
  ├── <prime>-weft/                 (weft Prime worktree; git repo root)
  ├── <slug>/                       (additional host worktree; git repo root)
  └── <slug>-weft/                  (weft worktree for <slug>; git repo root)
```

### Git ownership

The **host repo** is the project's source of truth, maintained by developers. All lyx-specific artifacts live in the **weft repo**, a separate git repository that lyx controls. This separation keeps host commits focused on project code and delegates lyx infrastructure to the weft.

### Artifacts location

| Artifact | Location | Repo | Purpose |
|----------|----------|------|---------|
| `_lyx/config/` | Weft worktree | Weft | Configuration files for all modules |
| `_codeguide/` | Weft worktree | Weft | Codeguide documentation (task 008) |
| `_board/` | Weft worktree | Board | Task board (separate board repo) |
| Host source | Host worktree | Host | Project source code |

### Junction model

Each host worktree has a sibling weft worktree. Host worktrees use **junctions** (Windows) or symlinks to route writes into the sibling weft worktree:
- `<host>/_lyx` → `<hub>/<slug>-weft/_lyx` (config junction)
- `<host>/_codeguide` → `<hub>/<slug>-weft/_codeguide` (codeguide junction, task 008)

Junctions are listed in `.git/info/exclude` per worktree and are never committed to `.gitignore`. From the CLI's perspective, reads and writes happen transparently — code that writes to `_lyx/config/board.yaml` writes through the junction into the weft repo without awareness of the indirection.

### Weft suffix convention

The weft worktree for any host worktree is deterministic:
- Host: `<hub>/<slug>/` → Weft: `<hub>/<slug>-weft/`
- Host: `<prime>/` → Weft: `<prime>-weft/` (prime is the name of the main worktree)

The `-weft` suffix is fixed and non-configurable. Weft paths are computed on demand from geometry and do not require a registry.

### Status

- **Go implementation** (paths geometry, paired spawn, `lyx weft` command): task 006
- **`_codeguide` junction activation** (`lyx config` TUI, `_lyx/config/` schema): task 008
- **This task** documents the canonical architecture; Go code lands in downstream tasks.

```
github.com/Knatte18/loomyard/
├── cmd/lyx/
│   └── main.go                   entrypoint: routes the <module> argument to a module
├── internal/board/               the board module (see modules/board.md)
├── internal/worktree/            the worktree module (see modules/worktree.md)
├── internal/ide/                 the ide module (see modules/ide.md)
├── internal/muxpoc/              the muxpoc POC module (see modules/muxpoc.md)
├── internal/paths/               geometry resolver (the sole owner of cwd/root math)
├── internal/config/              shared config resolution
├── internal/git/                 shared git operations
├── internal/lock/                shared file locking
└── internal/output/              shared JSON output
```

`cmd/lyx` is `package main`; everything else is in `internal/`. `main` is the
only thing that imports a module.

## Module dispatch

`cmd/lyx/main.go` is a thin router. `run(args, out)` reads the first argument
(`<module>`) and hands the rest to that module's CLI handler — `lyx board ...`
calls `board.RunCLI`. Each module owns its own flags, subcommands, and JSON
output. Adding a module is one more `case`; nothing else in `main` changes.

```go
switch module {
case "init":
    return board.RunInit(out, moduleArgs)
case "board":
    return board.RunCLI(out, moduleArgs)
case "ide":
    return ide.RunCLI(out, moduleArgs)
case "muxpoc":
    return muxpoc.RunCLI(out, moduleArgs)
case "worktree":
    return worktree.RunCLI(out, moduleArgs)
}
```

`main()` is just `os.Exit(run(os.Args[1:], os.Stdout))`, which keeps `run`
testable without spawning the binary or trapping `os.Exit`.

All commands print JSON: `{"ok":true, ...}` on success,
`{"ok":false,"error":"..."}` on failure (exit code 1).

## Modules

User-facing modules each get one `lyx <module>` namespace:

- **board** — the task-tracker board (`internal/board`). ✅ Implemented. See
  [modules/board.md](modules/board.md).
- **worktree** — git-worktree lifecycle (create / track / tear down). ✅ Implemented. See
  [modules/worktree.md](modules/worktree.md).
- **ide** — one-shot VS Code launcher with interactive menu. ✅ Implemented. See
  [modules/ide.md](modules/ide.md).
- **muxpoc** — shipped proof-of-concept psmux orchestrator proving the risky parts of the
  planned mux module. See [modules/muxpoc.md](modules/muxpoc.md).
- **mux** — psmux session layout (column per worktree; daemon later). Design:
  [modules/mux.md](modules/mux.md).

**init** is not a module but a cross-cutting setup command (`lyx init`) that
scaffolds the shared `_lyx/` config dir for every module. See
[modules/board.md#init](modules/board.md#init).

The user-facing modules sit on a thin layer of shared infrastructure
(`internal/config`, `internal/git`, `internal/lock`, `internal/state` **(planned)**) — defined in
[shared-libs/README.md](shared-libs/README.md).

## Tests

Per-file unit tests sit next to the source they test (`store.go` ↔
`store_test.go`). The cross-cutting suites — benchmarks, concurrency stress, and
git-backed integration — live in the black-box `internal/board/boardtest` package.

## Other docs

- [modules/board.md](modules/board.md) — the board module in depth.
- [modules/worktree.md](modules/worktree.md) — worktree lifecycle (implemented).
- [modules/ide.md](modules/ide.md) — VS Code launcher (implemented).
- [modules/muxpoc.md](modules/muxpoc.md) — muxpoc POC proof-of-concept orchestrator.
- [modules/mux.md](modules/mux.md) — psmux session layout (design).
- [benchmarks/](benchmarks/board-performance.md) — board performance, tracked across revisions.
- [shared-libs/](shared-libs/README.md) — the shared `internal/{config,git,lock,state}` plumbing.
- [roadmap.md](roadmap.md) — numbered milestones and long-term direction.
- [vendor/psmux_scripting.md](vendor/psmux_scripting.md) — upstream psmux command reference (not our design).
