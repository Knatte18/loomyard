# Overview: mhgo

`mhgo` is a one-shot CLI task tracker. Each invocation starts a process, runs one
command, writes JSON to stdout, and exits — there is no daemon and no shared
memory. State lives on disk per module and is coordinated with file locks, so
concurrent `mhgo` processes on a machine cooperate through the filesystem.

Module path: `github.com/Knatte18/mhgo`

## Structure

```
github.com/Knatte18/mhgo/
├── cmd/mhgo/
│   └── main.go          entrypoint: routes the <module> argument to a module
└── internal/board/      the board module (see board.md)
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

- **board** — the task-tracker board (`internal/board`). See [board.md](board.md).
- **init** — scaffolds the config layer (top-level, `mhgo init`). See [board.md#init](board.md#init).

## Tests

Per-file unit tests sit next to the source they test (`store.go` ↔
`store_test.go`). The cross-cutting suites — benchmarks, concurrency stress, and
git-backed integration — live in the black-box `internal/board/boardtest` package.

## Other docs

- [board.md](board.md) — the board module in depth.
- [benchmarks.md](benchmarks.md) — performance, tracked across revisions.
- [roadmap.md](roadmap.md) — deferred work and long-term direction.
