# `internal/git`

The one safe way to invoke `git` on this platform. Deliberately **not** a git API —
it is a single primitive plus output handling:

- **`RunGit(args, cwd) → (stdout, stderr, exitcode, err)`** — runs `git` with the
  given args in `cwd`, **windowless on Windows** (`CREATE_NO_WINDOW` via
  `SysProcAttr`), capturing stdout/stderr and the exit code, wrapping non-zero exits
  with the captured stderr.

Why centralise just one function: the windowless flag is the *only* part that
cannot be done by "just calling git directly", and forgetting it at any new call
site reintroduces the console-flash bug in detached/background processes (e.g.
board's background `sync`). It is a one-line mistake, so it lives in exactly one
place.

The actual command *sequences* stay in the modules that own them:

- board composes its own `pull` / `commit` / `push` flow on top of `RunGit`.
- worktree composes its own `worktree add|list|remove` calls on top of `RunGit`.

`internal/git` knows nothing about worktrees, boards, or remotes — it just executes.
