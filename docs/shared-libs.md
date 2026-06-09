# Shared internal libraries

`mhgo`'s user-facing modules (`board`, `worktree`, `mux`) are self-contained: all
of a module's *domain* logic and its deep test suite live in that module's package
and nowhere else. What they share is a thin layer of **infrastructure plumbing** —
mechanical helpers with no opinion about tasks, worktrees, or panes.

This document defines that layer. Four packages, each one extracted from board's
existing code (except `state`, which is new). The split keeps modules
understandable in isolation while avoiding the millpy failure mode of the same bug
reimplemented in four places.

**The line we hold:** a shared lib does one mechanical thing — run a git command,
take a lock, resolve a config, read a state file. It carries *no* domain logic. The
command *sequences* (which git calls, which lock files, which config keys) stay in
the modules. Each shared lib also carries its own deep tests, so it is vetted
plumbing, not an untested dependency.

See [roadmap.md](roadmap.md) milestones 2–4 for the extraction order. The
extractions are behaviour-preserving — board's suite is the guardrail.

---

## `internal/config`

Resolves a module's configuration from the current working directory. This is the
one place that knows the `_mhgo/` layout and the config grammar.

> **Status:** target design. board currently has its own loader with a three-layer
> model (including a gitignored `.mhgo/<module>.yaml` override). Milestone 2 lifts
> it here and redesigns it to the model below — the `.mhgo/` config layer is
> **removed**.

### Layout

```
<cwd>/                  ← where `mhgo init` was run
├── _mhgo/              git-TRACKED config — the only config source
│   ├── board.yaml
│   ├── worktree.yaml
│   └── mux.yaml
├── .env                git-IGNORED — local env values (KEY=value)
└── .mhgo/              git-IGNORED — machine-local RUNTIME state (see internal/state)
    └── local-state.json
```

`_mhgo/` presence is what makes a directory "initialised". If it is absent,
`config` errors with `not initialized here; run "mhgo init"`. Resolution is
**cwd-authoritative** — the cwd does **not** need to equal the git-repo root (a
first-class constraint; it caused constant trouble in millpy precisely because it
was designed in and then forgotten).

### Resolution model

Two layers, merged per key:

1. **Built-in defaults** — in code, per module.
2. **`_mhgo/<module>.yaml`** (git-tracked) — overlaid on the defaults.

There is **no** `.mhgo/` config layer. Machine-local variation does not get its own
file; it is expressed *inside* the tracked YAML via env references, so the full
shape of a module's config is always visible in one tracked file and only *values*
vary per machine.

### Env references and the `? default` grammar

After the layers are merged, every string value is scanned for `$env:NAME` tokens
(`NAME` matches `[A-Za-z_][A-Za-z0-9_]*`):

- **`$env:NAME`** (no `?`) — **required**. Unset ⇒ hard error
  (`referenced env var NAME is not set`). May appear mid-value for composition:
  `path: $env:HOME/board`. *(This is board's existing behaviour, preserved.)*
- **`$env:NAME ? fallback`** — **optional**. `NAME` set ⇒ its value; `NAME` unset ⇒
  `fallback`. The fallback runs to the end of the value, so a `?`-form token must be
  the last thing in the value (you cannot follow it with more text).

```yaml
# _mhgo/board.yaml
home:  $env:MHGO_HOME ? Home.md          # README.md on some machines, default Home.md
path:  $env:MHGO_BOARD ? ../_board       # default sibling dir
model: $env:MHGO_CODE_REVIEWER ? sonnetmax  # default to the fast model
```

**Comments work after the default.** Config is parsed with a real YAML library, so
a trailing `# comment` (whitespace + `#`) is stripped *before* env expansion runs.
The fallback is the YAML scalar after comment-strip and trim. (A literal `#`
*inside* a value still needs quoting — normal YAML rules.)

Not supported in v1: a fallback that itself contains `$env:` (chaining). The
fallback is a literal.

### `.env` loading

Before expansion, `config` loads `<cwd>/.env` (gitignored, `KEY=value` lines, like
the Python `.env` files this replaces) into the env namespace it resolves `$env:`
tokens against.

- **Precedence: real OS env wins.** `.env` only fills vars not already set in the
  process environment (`override=False`, matching python-dotenv). This lets a single
  invocation override `.env` with a real environment variable.

### What it returns

A typed, fully-resolved config struct for the requested module — defaults merged,
env expanded, relative `path` resolved against cwd (absolute paths used as-is).
Callers never see raw YAML or unexpanded tokens.

---

## `internal/git`

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

---

## `internal/lock`

Cross-process file locking, wrapping `github.com/gofrs/flock`. Coordinates the
short-lived `mhgo` processes on a machine through the filesystem.

- **`AcquireWriteLock(lockPath)`** — exclusive; blocks until free.
- **`AcquireReadLock(lockPath)`** — shared; many readers at once, blocked only by an
  exclusive holder.

That is the whole surface: two primitives over a lock file. Each module decides
*which* lock files it needs and what they guard. board uses three (write / swap /
push) — all the same primitive over different files. The lib has no concept of what
is being protected.

---

## `internal/state`

Typed read/write of the machine-local runtime registry at
`<cwd>/.mhgo/local-state.json`. **New** — nothing in board needs it; it exists for
worktree and mux.

Note the `.mhgo/` directory here is the **gitignored runtime-state dir**, a
different role from the (now removed) `.mhgo/` config layer. It holds machine-local
data only — never config, never anything portable across machines.

### What it stores

A single typed document, shared by the modules that write to it:

- **worktree** records the worktree registry: `slug → { path, branch, container }`.
- **mux** records the layout/session mapping: `worktree → { window, pane } →
  claude_session`.

Session IDs and pane IDs are machine-local (they reference JSONL files under
`%USERPROFILE%\.claude\projects\` and a running psmux server), which is exactly why
this file is gitignored.

### How it writes

Atomic writes (temp + rename) under the locking primitive from `internal/lock`, so
two `mhgo` processes never corrupt the registry. `state` owns the schema and the
read/write/merge operations; the modules own *what* the fields mean.

---

## A note on `AtomicWrite` / `PathGuard`

board's generic safe-file-write helpers (`AtomicWrite` = temp + rename;
`PathGuard` = reject empty/absolute/`..` paths) are filesystem safety, not git. They
will likely fall out as a tiny `internal/fsx`, or ride inside `internal/state`
(which needs atomic writes anyway). Exact home is decided when milestone 3/4 lands —
flagged here so it is not forgotten.
