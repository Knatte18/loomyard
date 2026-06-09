# Overview: mhgo

`mhgo` is a Go toolkit of one-shot CLI modules. Each invocation starts a process,
runs one command, writes JSON to stdout, and exits — there is no daemon and no
shared memory. State lives on disk per module and is coordinated with file locks,
so concurrent `mhgo` processes on a machine cooperate through the filesystem. The
first module, **board** (a task tracker), is implemented; **worktree** and **mux**
are designed and coming next (see [roadmap.md](roadmap.md)).

In the long term, mhgo is intended to **replace mill/millhouse (Python)** entirely.
We get there by building these modules as self-contained toolkits first;
orchestration comes last. See [Principles](#principles).

Module path: `github.com/Knatte18/mhgo`

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

## Structure

```
github.com/Knatte18/mhgo/
├── cmd/mhgo/
│   └── main.go          entrypoint: routes the <module> argument to a module
└── internal/board/      the board module (see modules/board.md)
    ├── task.go store.go layer.go render.go    domain + storage
    ├── git.go lock.go sync.go spawn_*.go      git, locking, background sync
    ├── cli.go board.go                         CLI router + facade
    ├── config.go init.go                       configuration + scaffolding
    └── boardtest/                              benchmarks, concurrency, integration
```

`cmd/mhgo` is `package main`; everything else is `package board` in `internal/board`.
`main` is the only thing that imports a module.

## Module dispatch

`cmd/mhgo/main.go` is a thin router. `run(args, out)` reads the first argument
(`<module>`) and hands the rest to that module's CLI handler — `mhgo board ...`
calls `board.RunCLI`. Each module owns its own flags, subcommands, and JSON
output. Adding a module is one more `case`; nothing else in `main` changes.

```go
switch module {
case "board":
    return board.RunCLI(out, moduleArgs)
case "init":
    return board.RunInit(out, moduleArgs)
// case "<next-module>": ...
}
```

`main()` is just `os.Exit(run(os.Args[1:], os.Stdout))`, which keeps `run`
testable without spawning the binary or trapping `os.Exit`.

All commands print JSON: `{"ok":true, ...}` on success,
`{"ok":false,"error":"..."}` on failure (exit code 1).

## Modules

User-facing modules each get one `mhgo <module>` namespace:

- **board** — the task-tracker board (`internal/board`). ✅ Implemented. See
  [modules/board.md](modules/board.md).
- **worktree** — git-worktree lifecycle (create / track / tear down). Sketch:
  [modules/worktree.md](modules/worktree.md).
- **mux** — psmux session layout (column per worktree; daemon later). Sketch:
  [modules/mux.md](modules/mux.md).

**init** is not a module but a cross-cutting setup command (`mhgo init`) that
scaffolds the shared `_mhgo/` config dir for every module. See
[modules/board.md#init](modules/board.md#init).

The user-facing modules sit on a thin layer of shared infrastructure
(`internal/config`, `internal/git`, `internal/lock`, `internal/state`) — defined in
[shared-libs/README.md](shared-libs/README.md).

## Tests

Per-file unit tests sit next to the source they test (`store.go` ↔
`store_test.go`). The cross-cutting suites — benchmarks, concurrency stress, and
git-backed integration — live in the black-box `internal/board/boardtest` package.

## Other docs

- [modules/board.md](modules/board.md) — the board module in depth.
- [modules/worktree.md](modules/worktree.md) — worktree lifecycle (sketch).
- [modules/mux.md](modules/mux.md) — psmux session layout (sketch).
- [benchmarks.md](benchmarks.md) — board performance, tracked across revisions.
- [shared-libs/](shared-libs/README.md) — the shared `internal/{config,git,lock,state}` plumbing.
- [roadmap.md](roadmap.md) — numbered milestones and long-term direction.
- [vendor/psmux_scripting.md](vendor/psmux_scripting.md) — upstream psmux command reference (not our design).
