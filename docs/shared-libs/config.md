# `internal/config`

Loads and resolves a module's configuration from the current working directory. This is the
one place that knows the `_lyx/` layout and enforces strict validation against a template.

## Layout

```
<cwd>/                  ← where `lyx init` was run (the current working directory)
├── _lyx/               git-TRACKED config Hub
│   ├── config/         git-TRACKED config files (only source of live values)
│   │   ├── board.yaml      (must match board module template)
│   │   ├── worktree.yaml   (must match worktree module template)
│   │   └── weft.yaml       (must match weft module template)
│   ├── discussion.md    lyx task discussion (artifact)
│   ├── plan.md         lyx task plan (artifact)
│   └── reviews/        lyx code reviews (artifact directory)
├── .env                git-IGNORED — local env values (KEY=value)
```

`_lyx/` presence is what makes a directory "initialised". If it is absent,
`config` errors with `not initialized: _lyx/ directory not found in <dir>` (the
raw error from `FindBaseDir`; the board rewraps it into `not initialized here; run "lyx init"`).
Resolution is **cwd-authoritative** — the cwd does **not** need to equal the git-repo root (a
first-class constraint; it caused constant trouble in millpy precisely because it
was designed in and then forgotten).

## Resolution model

**Strict, template-backed loading:**

The `Load(baseDir, module, template []byte)` function reads the on-disk config file, validates it against a template, and resolves environment variables.

**Flow:**

1. Call `FindBaseDir(baseDir)` — check that `_lyx/` exists at baseDir.
2. Read the config file at `paths.ConfigFile(baseDir, module)` (e.g., `_lyx/config/board.yaml`). If absent, return an error instructing the user to run `lyx update`.
3. Check for missing template keys via `yamlengine.MissingKeys(template, fileBytes)`. If any keys are missing, return an error naming the file, the missing key-paths, and instructing the user to run `lyx update`.
4. Build the environment via `envsource.Build(baseDir)` (reads `.env`, overlays OS env).
5. Resolve environment variables via `yamlengine.Resolve(fileBytes, env)` (expands `${env:...}` markers).
6. Return the resolved bytes. Typed wrappers unmarshal into their own config structs.

**Key properties:**

- **All defaults live in the template YAML file**, not in code. The template is embedded via `//go:embed` and passed to `Load()`.
- **Errors are strict**: missing template keys, absent files, or unset required env vars cause hard errors with clear messages naming the file and the problem.
- **Extra/stale keys are tolerated** by `Load()` and cleaned up by `lyx update` (reconciliation).
- **A key present with an empty value counts as present** and is not flagged missing.

## Environment variable grammar

Config values use POSIX-style brace-delimited env markers:

- **`${env:NAME}`** (required) — Substituted with the value of `NAME` from the environment. If `NAME` is absent, a hard error is returned. If `NAME` is present but empty, the empty string is used.
- **`${env:NAME:-default}`** (optional) — Substituted with the value of `NAME` if present and non-empty; otherwise, substituted with the literal default text between `:-` and the closing `}`. Spaces, special characters, and all text are preserved verbatim in the default (no trimming, no quote-stripping).

**Interpolation:** Markers may appear inside a larger string:

```yaml
path: ${env:LYX_BOARD_PATH:-../_board}/sub
url: https://${env:HOST:-localhost}:${env:PORT:-8080}
```

Multiple markers in one value are all expanded. A value with no marker is a literal.

**No recursion or escaping:** Resolved text is never re-expanded. There is no escape mechanism for a literal `${env:` or a literal `}` inside a default.

## `.env` loading

Environment variables are sourced by `envsource.Build(baseDir)`, which reads `paths.DotEnv(baseDir)` (typically `<cwd>/.env`) and overlays the OS environment.

- **Format**: `KEY=VALUE` lines, blank lines skipped, lines starting with `#` are comments, split on first `=` only.
- **Precedence: OS env wins.** Any variable set in the process environment overrides the corresponding `.env` value.
- **If `.env` is absent**, only OS environment variables are used (no error).

## What it returns

Resolved YAML bytes (as returned by `yamlengine.Resolve`). Typed wrappers (`board.LoadConfig`, `worktree.LoadConfig`, `weft.LoadConfig`) unmarshal this into their own config structs. Callers never see raw YAML or unexpanded tokens.

## Migration from old format

Existing config files in the old commented format (all lines commented out) are treated as empty by `Reconcile`. Running `lyx update --apply` from the host worktree reconciles all module configs against their templates, rewriting old-format files to live templates with all keys present. Because the host `_lyx` is a directory junction into the weft worktree's `_lyx`, a single host `lyx update` reaches all config files (board, worktree, and weft). No separate command in the weft sibling is needed.

## Exported functions

### `FindBaseDir(cwd string) (string, error)`

Checks whether the given directory is an initialized Loomyard base directory.

**Behavior:** Performs a strict check that `<cwd>/_lyx` exists; it never walks up to parent directories. This is the cwd-authoritative model — the provided `cwd` must itself be initialized.

**Returns:** On success, the `cwd` itself (unchanged). On failure, an empty string and an error.

**Error messages:**
- If `<cwd>/_lyx` does not exist: `not initialized: _lyx/ directory not found in <dir>`.
- If stat fails for another reason: `stat _lyx: <underlying error>`.

**Note on error rewrapping:** The `board.LoadConfig` and other typed wrappers match the substring `"not initialized"` in the error text to rewrap it into a module-level message like `not initialized here; run "lyx init"`. Do not conflate:
- Raw `FindBaseDir` error: `not initialized: _lyx/ directory not found in <dir>`
- Board-level rewrapped: `not initialized here; run "lyx init"`

### `Load(baseDir, module string, template []byte) ([]byte, error)`

Loads and resolves a module's configuration from disk.

**Behavior:** Implements the five-step flow described in the Resolution model section above: check `_lyx/` exists, read the config file, validate against the template, build environment, and resolve env vars.

**Error cases:**

- **Config file absent:** Returns error `config file <path> not found; run "lyx update"`.
- **Missing template keys:** Returns error `config file <path>: missing keys: <comma-separated key-paths>; run "lyx update"`.
- **Unset required env var:** Returns error `config file <path>: unset required env var "NAME"`.
- **Env build failure:** Returns error `config file <path>: build environment: <underlying error>`.
- **YAML syntax error:** Returns error `config file <path>: <parse/marshal error>`.

All error messages include the file path and context to guide the user.

**Returns:** On success, the resolved YAML bytes. On error, nil bytes and an error message.

Typed wrappers like `board.LoadConfig(cwd, "board")` call `Load` with the board template, then unmarshal the result into a typed struct.
