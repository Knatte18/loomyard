# `internal/configengine`

Loads and resolves a module's configuration from the current working directory.
This is the one place that knows the `_lyx/` layout and enforces strict validation against a template.

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

`_lyx/` presence is what makes a directory "initialised" for `Load`;
if it is absent, `Load` errors (see `FindBaseDir`'s error messages below).
`LoadOrTemplate` resolves the caller's embedded template instead of erroring on that same absence — see "Resolution model" below.
Resolution is **cwd-authoritative** — the cwd does **not** need to equal the git-repo root (a first-class constraint;
it caused constant trouble in millpy precisely because it was designed in and then forgotten).

## Resolution model

**Two policies, one shared body:** `Load(baseDir, module, template []byte)` is strict — an absent `_lyx/` directory or an absent config file is an error.
`LoadOrTemplate(baseDir, module, template []byte)` degrades — either absence resolves `template` instead.
Both read the on-disk config file, validate it against a template, and resolve environment variables whenever a config file is present.

**Flow (the strict path, shared by both entry points whenever a config file is present):**

1. Call `FindBaseDir(baseDir)` — check that `_lyx/` exists at baseDir.
2. Read the config file at `configengine.ConfigFile(baseDir, module)` (e.g., `_lyx/config/board.yaml`).
   If absent, `Load` returns an error instructing the user to run `lyx config reconcile`;
   `LoadOrTemplate` instead takes the degrading path below.
3. Check for missing template keys via `yamlengine.MissingKeys(template, fileBytes)`.
   If any keys are missing, return an error naming the file, the missing key-paths, and instructing the user to run `lyx config reconcile`.
4. Build the environment via `envsource.Build(baseDir)` (reads `.env`, overlays OS env).
5. Resolve environment variables via `yamlengine.Resolve(fileBytes, env)` (expands `${env:...}` markers).
6. Return the resolved bytes (see "What it returns" below).

**The degrading path (`LoadOrTemplate` only):**

`LoadOrTemplate` diverges from the flow above at exactly two points, each gated on *proven* absence — `errors.Is(err, ErrNotInitialized)` at step 1, `os.IsNotExist(err)` at step 2 — never on any other failure at either point.
On either proven absence, it skips step 3 (`yamlengine.MissingKeys` is not run;
a template compared against itself is vacuously satisfied) and enters at step 4 with `template`'s bytes standing in for the file bytes, then continues through steps 4-6 unchanged.
A non-absence failure at either point — a stat error, a permission or IO read error — propagates unchanged, exactly as under `Load`.
A config file that exists but is invalid still errors exactly as under `Load`;
only its *absence* degrades.

**Key properties:**

- **All defaults live in the template YAML file**, not in code.
  The template is embedded via `//go:embed` and passed to `Load()`/`LoadOrTemplate()`.
- **Errors are strict on `Load`**: missing template keys, absent files, or unset required env vars cause hard errors with clear messages naming the file and the problem.
  Under `LoadOrTemplate`, the missing-template-keys and unset-required-env-var halves still hold;
  only the absent-file half is replaced by a resolved template default.
- **Extra/stale keys are tolerated** by both entry points and cleaned up by `lyx config reconcile` (reconciliation).
- **A key present with an empty value counts as present** and is not flagged missing.

## Environment variable grammar

Config values use POSIX-style brace-delimited env markers:

- **`${env:NAME}`** (required) — Substituted with the value of `NAME` from the environment.
  If `NAME` is absent, a hard error is returned.
  If `NAME` is present but empty, the empty string is used.
- **`${env:NAME:-default}`** (optional) — Substituted with the value of `NAME` if present and non-empty;
  otherwise, substituted with the literal default text between `:-` and the closing `}`.
  Spaces, special characters, and all text are preserved verbatim in the default (no trimming, no quote-stripping).

**Interpolation:** Markers may appear inside a larger string:

```yaml
path: ${env:LYX_EXAMPLE_PATH:-../_board}/sub
url: https://${env:HOST:-localhost}:${env:PORT:-8080}
```

Multiple markers in one value are all expanded.
A value with no marker is a literal.

**No recursion or escaping:** Resolved text is never re-expanded.
There is no escape mechanism for a literal `${env:` or a literal `}` inside a default.

## `.env` loading

Environment variables are sourced by `envsource.Build(baseDir)`, which reads `envsource.DotEnv(baseDir)` (typically `<cwd>/.env`) and overlays the OS environment.

- **Format**: `KEY=VALUE` lines, blank lines skipped, lines starting with `#` are comments, split on first `=` only.
- **Precedence: OS env wins.**
  Any variable set in the process environment overrides the corresponding `.env` value.
- **If `.env` is absent**, only OS environment variables are used (no error).

## What it returns

Resolved YAML bytes (as returned by `yamlengine.Resolve`).
Each caller unmarshals this into its own config struct — the strict callers are `fabricengine.LoadConfig`, `boardengine.LoadConfig`, `loomengine.LoadConfig`, and `batcher.Active`;
the degrading callers are `websterengine.LoadConfig`, `shuttleengine.LoadConfig`, and `reedengine.LoadConfig`.
Callers never see raw YAML or unexpanded tokens.

## Migration from old format

Existing config files in the old commented format (all lines commented out) are treated as empty by `Reconcile`.
Running `lyx config reconcile --apply` from the warp worktree reconciles all module configs against their templates, rewriting old-format files to live templates with all keys present.
Because the warp `_lyx` is a directory junction into the weft worktree's `_lyx`, a single warp `lyx config reconcile` reaches all config files (board, worktree, and weft).
No separate command in the weft sibling is needed.

## Exported functions

### `LyxDirName` (not exported here)

[`internal/lyxdirs` is the sole declarer](../../CONSTRAINTS.md#lyxdirs-single-declarer-invariant) of the `"_lyx"` token (`lyxdirs.LyxDirName`);
`internal/configengine` itself uses `lyxdirs.LyxDirName` like every other caller and does not declare or export the literal.
Every module joins its own private relative-path constant onto a `baseDir` directly (e.g. `filepath.Join(baseDir, lyxdirs.LyxDirName, "plan")`), never onto a fused `"_lyx/..."` literal — see the per-segment join rule in `CONSTRAINTS.md`'s Cwd Resolution Invariant.

### `ConfigDir(baseDir string) string`

Returns `filepath.Join(baseDir, LyxDirName, "config")` — the directory where module configuration YAML files are stored.

### `ConfigFile(baseDir, module string) string`

Returns `filepath.Join(ConfigDir(baseDir), module+".yaml")` — the path to a specific module's configuration file (e.g. `_lyx/config/board.yaml`).

### `ConfigFileRel(module string) string`

Returns `filepath.Join(LyxDirName, "config", module+".yaml")` — the anchor-relative form used to build weft commit pathspecs, as opposed to `ConfigFile`'s base-joined absolute form.

### `FindBaseDir(cwd string) (string, error)`

Checks whether the given directory is an initialized Loomyard base directory.

**Behavior:** Performs a strict check that `<cwd>/_lyx` exists;
it never walks up to parent directories.
This is the cwd-authoritative model — the provided `cwd` must itself be initialized.

**Returns:** On success, the `cwd` itself (unchanged).
On failure, an empty string and an error.

**Error messages:**
- If `<cwd>/_lyx` does not exist: `not initialized: _lyx/ directory not found`.
  This error wraps the exported `ErrNotInitialized` sentinel;
  `errors.Is(err, configengine.ErrNotInitialized)` is the supported way to detect absence, not text matching.
- If stat fails for another reason: `stat _lyx: <underlying error>`.
  This branch deliberately does not wrap `ErrNotInitialized` — a stat failure (permission, IO) is not absence.

**Note on error rewrapping:** The four strict callers of `Load` — `fabricengine.LoadConfig`, `boardengine.LoadConfig`, `loomengine.LoadConfig`, and `batcher.Active` — match the substring `"not initialized"` in the error text to rewrap it into a module-level message: `not initialized here; run "lyx fabric reconcile"`.
The `ErrNotInitialized` sentinel makes migrating these four onto `errors.Is` possible;
that migration is available, not done, and the substring match remains supported for callers that still use it.
Do not conflate:
- Raw `FindBaseDir` error: `not initialized: _lyx/ directory not found`
- Strict-caller rewrapped: `not initialized here; run "lyx fabric reconcile"`

### `Load(baseDir, module string, template []byte) ([]byte, error)`

Loads and resolves a module's configuration from disk.

**Behavior:** Implements the six-step flow described in the Resolution model section above: check `_lyx/` exists, read the config file, validate against the template, build environment, and resolve env vars.

**Error cases:**

- **Config file absent:** Returns error `config file <path> not found; run "lyx config reconcile"`.
- **Missing template keys:** Returns error `config file <path>: missing keys: <comma-separated key-paths>; run "lyx config reconcile"`.
- **Unset required env var:** Returns error `config file <path>: unset required env var "NAME"`.
- **Env build failure:** Returns error `config file <path>: build environment: <underlying error>`.
- **YAML syntax error:** Returns error `config file <path>: <parse/marshal error>`.

All error messages include the file path and context to guide the user.

**Returns:** On success, the resolved YAML bytes.
On error, nil bytes and an error message.

### `LoadOrTemplate(baseDir, module string, template []byte) ([]byte, error)`

Same signature as `Load`.
A provably-absent `_lyx/` directory or a provably-absent config file resolves `template` through `envsource.Build` then `yamlengine.Resolve` instead of erroring — see the "degrading path" in the Resolution model section above.
`yamlengine.MissingKeys` is skipped on that path, since the bytes being resolved are the template itself.
A config file that exists but is invalid still errors exactly as under `Load`;
any non-absence failure — a stat error, a permission or IO read error — propagates unchanged.

**Error cases:** Identical to `Load`'s, except that config-file absence and `_lyx/` absence no longer produce an error — those two cases resolve the template instead.
A fallback-path error is keyed on the module, not on a config-file path that does not exist: `<module> config template: <underlying error>`.

**Returns:** On success, the resolved YAML bytes (either the on-disk config, or the resolved template on proven absence).
On error, nil bytes and an error message.

### `Set(baseDir, module, template string, pairs []yamlengine.KV) ([]string, error)`

Writes an explicit list of key=value pairs into a module's config file.
This is the non-interactive counterpart to `Edit` used by the `lyx config <module> --set key=value` CLI path — no editor is invoked and there is no validation loop.

**Behavior:**

1. Calls `FindBaseDir(baseDir)` to check that `_lyx/` exists, then scaffolds the config file from `template` when it is absent (the same `scaffoldIfMissing` helper `Edit` uses, so both entry points create and roll back a fresh default-valued file identically).
2. Delegates the actual mutation to `yamlengine.SetValues(template, existingBytes, pairs)`, which validates every requested pair's key against the template's leaf-key set and, when all keys are known, applies the pairs and marshals the merged result.
3. Rejects the whole call — freshly-scaffolded file removed, existing file left untouched — when any *requested* pair's key is absent from the template's leaf-key set (existing, unchanged behavior;
   see `internal/yamlengine`'s `SetValues` documentation for the full validation and preservation mechanism).
4. On any error return, a freshly-scaffolded file is removed before returning, exactly mirroring `Edit`'s abort-removes-scaffold contract: a failed `--set` never leaves a fresh default-valued file behind on disk.

**Error cases:**

- **Not initialized:** Propagates `FindBaseDir`'s error.
- **Scaffold failure:** Propagates the underlying filesystem error.
- **Unknown config key(s):** Returns error `unknown config key(s): <requested keys> (known: <template's known keys>)`.
- **Read/write failure:** Propagates the underlying filesystem error.

**Returns:** On success, the sorted list of pre-existing top-level config keys not present in `template` that were preserved verbatim rather than dropped (see `internal/yamlengine`'s `SetValues` documentation for the full preservation mechanism),
and a nil error.
This list is nil/empty when no such orphaned key was present.
On any error return, the returned `[]string` is always nil.
