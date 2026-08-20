# Shared internal libraries

Loomyard's user-facing modules (`board`, `warp`, `ide`, `reed`) are self-contained: all of a module's *domain* logic and its deep test suite live in that module's package and nowhere else.
What they share is a thin layer of **infrastructure plumbing** — mechanical helpers with no opinion about tasks, worktrees, or panes.
See [overview.md](../overview.md).

**The line we hold:** a shared lib does one mechanical thing — run a git command, take a lock, resolve a config, read a state file.
It carries *no* domain logic.
The command *sequences* (which git calls, which lock files, which config keys) stay in the modules.
Each shared lib also carries its own deep tests, so it is vetted plumbing, not an untested dependency.

See [roadmap.md](../../manifest/roadmap.md) milestones 2–3 for the extraction order.

## Libraries

- [lyxcwd.md](lyxcwd.md) — `internal/lyxcwd`: entry gate that resolves cwd into a legal worktree's coordinates;
  sole owner of cwd resolution, nothing else
- [yamlengine.md](yamlengine.md) — `internal/yamlengine`: pure YAML engine for env expansion and config reconciliation
- [envsource.md](envsource.md) — `internal/envsource`: single source of truth for environment variable sourcing (`.env` + OS overlay)
- [configengine.md](configengine.md) — `internal/configengine`: strict YAML config loading backed by yamlengine and envsource
- [stencil.md](stencil.md) — `internal/stencil`: fill marker fields in a markdown template → prompt (fails on an unfilled marker)

## Implementation-only libraries

The following libraries ship in code and tests;
their mechanics are documented there per the [doc-lifecycle convention](../overview.md#documentation-lifecycle):

- `internal/fsx` — atomic file writes + relative-path guard
- `internal/gitexec` — windowless git-spawn pair: `Run` is the checked default, `RunGit` is the raw form for the sites where a non-zero exit is an answer, not a failure
- `internal/gitignore` — shared `.gitignore` block manager for multiple modules
- `internal/lock` — cross-process file locking
- `internal/logger` — thin log/slog wrapper (Debug/Info/Warn), silent by default;
  `-v`/`-vv` wires to it in `cmd/lyx/main.go`, and `LYX_LOG_LEVEL`/`LYX_LOG_FILE` env vars activate it for entry points (e.g. `go test`) that bypass CLI flag parsing;
  every line also carries a process trace ID (`TraceID()`, exported via `LYX_TRACE_ID` so a spawned child continues its parent's trace) and, for callers holding one, an explicit-parent diagnostic span (`StartSpan`/`Child`/`End`) stamping `trace=`/`span=`;
  independent of stderr verbosity, every Info+ record also lands in a durable, `AnchorPath()`-anchored sink (`.lyx/logs`, opened, appended to and closed per record rather than held open, retained by age and count, capped at 8 MiB per file), which under `go test` requires the `LYX_TRACE=1` opt-in (alongside `LYX_LOG_LEVEL`/`LYX_LOG_FILE`) to activate
- `internal/proc` — cross-OS child-process window-hide (`HideWindow`) and detached-spawn (`Detach`) primitives
- `internal/state` — generic locked typed JSON I/O
- `internal/modelspec` — model-spec parser + models.yaml registry loader;
  the pinned contract is `contracts/specs/llm-model-spec.md`, the as-built API lives in the package doc
- `internal/buildinfo` — the ldflags-stamped build channel (`Channel`, `IsDev`), a zero-import leaf so any CLI can read it with no cycle risk;
  the mapping to a stencil mode lives in `stencilstore.ModeFor`
- `internal/standalonestate` — pure derivation from an absolute target path to a `hash8` and its per-OS state directory, creating nothing on disk
