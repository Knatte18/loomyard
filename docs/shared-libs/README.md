# Shared internal libraries

`mhgo`'s user-facing modules (`board`, `worktree`, `mux`) are self-contained: all
of a module's *domain* logic and its deep test suite live in that module's package
and nowhere else. What they share is a thin layer of **infrastructure plumbing** —
mechanical helpers with no opinion about tasks, worktrees, or panes.

**The line we hold:** a shared lib does one mechanical thing — run a git command,
take a lock, resolve a config, read a state file. It carries *no* domain logic. The
command *sequences* (which git calls, which lock files, which config keys) stay in
the modules. Each shared lib also carries its own deep tests, so it is vetted
plumbing, not an untested dependency.

See [roadmap.md](../roadmap.md) milestones 2–3 for the extraction order.

## Libraries

- [config.md](config.md) — `internal/config`: two-layer YAML config (defaults + `_mhgo/<module>.yaml`), env expansion, `.env` loading
- [git.md](git.md) — `internal/git`: windowless `RunGit` primitive
- [lock.md](lock.md) — `internal/lock`: cross-process file locking
- [state.md](state.md) — `internal/state` **(planned)**: machine-local runtime state registry
