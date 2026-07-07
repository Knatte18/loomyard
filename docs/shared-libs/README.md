# Shared internal libraries

Loomyard's user-facing modules (`board`, `warp`, `ide`, `muxpoc`) are self-contained: all
of a module's *domain* logic and its deep test suite live in that module's package
and nowhere else. What they share is a thin layer of **infrastructure plumbing** —
mechanical helpers with no opinion about tasks, worktrees, or panes. (The planned `mux` module is design; see [overview.md](../overview.md).)

**The line we hold:** a shared lib does one mechanical thing — run a git command,
take a lock, resolve a config, read a state file. It carries *no* domain logic. The
command *sequences* (which git calls, which lock files, which config keys) stay in
the modules. Each shared lib also carries its own deep tests, so it is vetted
plumbing, not an untested dependency.

See [roadmap.md](../roadmap.md) milestones 2–3 for the extraction order.

## Libraries

- [hubgeometry.md](hubgeometry.md) — `internal/hubgeometry`: canonical geometry resolver, sole owner of cwd/root math
- [yamlengine.md](yamlengine.md) — `internal/yamlengine`: pure YAML engine for env expansion and config reconciliation
- [envsource.md](envsource.md) — `internal/envsource`: single source of truth for environment variable sourcing (`.env` + OS overlay)
- [configengine.md](configengine.md) — `internal/configengine`: strict YAML config loading backed by yamlengine and envsource
- [stencil.md](stencil.md) — `internal/stencil`: fill marker fields in a markdown template → prompt (fails on an unfilled marker); 🚧 design — not built

## Implementation-only libraries

The following libraries ship in code and tests; their mechanics are documented there per the [doc-lifecycle convention](../overview.md#documentation-lifecycle):

- `internal/fsx` — atomic file writes + relative-path guard
- `internal/gitexec` — windowless `RunGit` primitive
- `internal/gitignore` — shared `.gitignore` block manager for multiple modules
- `internal/lock` — cross-process file locking
- `internal/proc` — cross-OS child-process window-hide (`HideWindow`) and detached-spawn (`Detach`) primitives
- `internal/state` — generic locked typed JSON I/O
