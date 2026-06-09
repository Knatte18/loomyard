# `internal/config`

Resolves a module's configuration from the current working directory. This is the
one place that knows the `_mhgo/` layout and the config grammar.

> **Status:** target design. board currently has its own loader with a three-layer
> model (including a gitignored `.mhgo/<module>.yaml` override). Milestone 2 lifts
> it here and redesigns it to the model below — the `.mhgo/` config layer is
> **removed**.

## Layout

```
<cwd>/                  ← where `mhgo init` was run
├── _mhgo/              git-TRACKED config — the only config source
│   ├── board.yaml
│   ├── worktree.yaml
│   └── mux.yaml
├── .env                git-IGNORED — local env values (KEY=value)
└── .mhgo/              git-IGNORED — machine-local RUNTIME state (see state.md)
    └── local-state.json
```

`_mhgo/` presence is what makes a directory "initialised". If it is absent,
`config` errors with `not initialized here; run "mhgo init"`. Resolution is
**cwd-authoritative** — the cwd does **not** need to equal the git-repo root (a
first-class constraint; it caused constant trouble in millpy precisely because it
was designed in and then forgotten).

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
env expanded, relative `path` resolved against cwd (absolute paths used as-is).
Callers never see raw YAML or unexpanded tokens.
