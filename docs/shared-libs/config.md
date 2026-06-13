# `internal/config`

Resolves a module's configuration from the current working directory. This is the
one place that knows the `_mhgo/` layout and the config grammar.

> **Status:** target design. board currently has its own loader with a three-layer
> model (including a gitignored `.mhgo/<module>.yaml` override). Milestone 2 lifts
> it here and redesigns it to the model below — the `.mhgo/` config layer is
> **removed**.

## Layout

```
<repo-root>/            ← where `mhgo init` was run; the worktree root, NOT necessarily cwd
├── _mhgo/              git-TRACKED config — the only config source
│   ├── board.yaml
│   ├── worktree.yaml
│   └── mux.yaml
├── .env                git-IGNORED — local env values (KEY=value)
└── .mhgo/              git-IGNORED — machine-local RUNTIME state (see state.md)
    └── local-state.json
```

`_mhgo/` presence is what makes a directory "initialised". It is git-TRACKED and
lives at the **repo root** — so it is present at the root of every worktree, not just
the hub. If it is absent, `config` errors with
`not initialized: _mhgo/ directory not found in <dir>` (the raw error from
`FindBaseDir`; the board rewraps it into `not initialized here; run "mhgo init"`).

**What "cwd-authoritative" means here (and what it does NOT mean).** Resolution is
anchored at the repo/worktree root that holds `_mhgo/` — discovered *from* the cwd.
The cwd does **not** need to equal that root: you may invoke from a nested
subdirectory (`worktree/internal/foo/`). This is a first-class constraint — it caused
constant trouble in millpy precisely because it was designed in and then forgotten.
The operative word is *anchored at the root*, not *literally the cwd*.

> **Known gap (milestone 2).** The current `FindBaseDir` does a **strict** check of
> `<cwd>/_mhgo` and never walks up (see its section below). So today, resolution only
> succeeds when cwd *is* the root. Until `FindBaseDir` is taught to resolve the root
> (e.g. via [`internal/git.FindRoot`](git.md) / `git rev-parse --show-toplevel`, then
> check `<root>/_mhgo`), a caller that wants cwd ≠ git-root must resolve the root
> itself and pass it in. A module must never paper over this by deriving paths from
> `filepath.Dir(cwd)` — see overview principle 4.

## Resolution model

Two layers, merged per key:

1. **Built-in defaults** — in code, per module.
2. **`_mhgo/<module>.yaml`** (git-tracked) — overlaid on the defaults.

There is **no** `.mhgo/` config layer. Machine-local variation does not get its own
file; it is expressed *inside* the tracked YAML via env references, so the full
shape of a module's config is always visible in one tracked file and only *values*
vary per machine.

## Env references and the `? default` grammar

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

## `.env` loading

Before expansion, `config` loads `<cwd>/.env` (gitignored, `KEY=value` lines, like
the Python `.env` files this replaces) into the env namespace it resolves `$env:`
tokens against.

- **Precedence: real OS env wins.** `.env` only fills vars not already set in the
  process environment (`override=False`, matching python-dotenv). This lets a single
  invocation override `.env` with a real environment variable.

## What it returns

A typed, fully-resolved config struct for the requested module — defaults merged,
env expanded, relative `path` resolved against the base dir passed in (the repo root
under the cwd-authoritative intent; absolute paths used as-is). Callers never see raw
YAML or unexpanded tokens.

## Exported helpers

### `FindBaseDir(cwd) (string, error)`

Checks whether the given directory is an initialized mhgo base directory.

**Behavior:** Performs a strict check that `<cwd>/_mhgo` exists; it never walks up
to parent directories. The provided directory must itself be initialized.

> **Caveat — this strict check is the cwd ≠ git-root gap.** "cwd-authoritative" is
> the *intent* (resolve anchored at the repo root, tolerating a nested cwd); this
> strict no-walk-up implementation does **not** yet deliver that intent. Callers that
> must support a nested cwd should resolve the worktree root with
> [`internal/git.FindRoot`](git.md) and pass that root to `Load`/`FindBaseDir`, rather
> than the raw cwd. Slated to be folded into `FindBaseDir` itself in milestone 2.

**Returns:** On success, the directory itself (unchanged). On failure, an empty string
and an error.

**Error messages:**
- If `<cwd>/_mhgo` does not exist: `not initialized: _mhgo/ directory not found in <dir>`
  (the raw error returned by `FindBaseDir`).
- If stat fails for another reason: `stat _mhgo: <underlying error>`.

**Note on error rewrapping:** `internal/board/config.go` `LoadConfig` matches the
substring `"not initialized"` in the error text to rewrap it into the board-level
message `not initialized here; run "mhgo init"`. Do not conflate the two:
- Raw `FindBaseDir` error: `not initialized: _mhgo/ directory not found in <dir>`
- Board-level rewrapped: `not initialized here; run "mhgo init"`

**Delegation:** `Load` calls `FindBaseDir` for its existence check.
