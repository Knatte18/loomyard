# PATTERN-cwd-resolution

`internal/lyxcwd` owns cwd resolution alone — never weft, a junction path, or any per-module subdirectory.

- `root` is the worktree or repo root and `cwd` is the current working directory; the two are never conflated.
- `Resolve` requires cwd to be a git worktree root and to equal `Join(worktreeRoot, AnchorRel)`.
- It exposes only `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel`, `WorktreePath()` and `AnchorPath()`.
- Every cwd or worktree-root query goes through `lyxcwd.Getwd()` or `Resolve()`; raw `os.Getwd` and `git rev-parse --show-toplevel` are banned elsewhere.
- A module's own durable subdirectory is its own constant joined onto `AnchorPath()`, never a `lyxcwd` call.
- `lyxcwd` imports the standard library and `internal/dotgit` only.
