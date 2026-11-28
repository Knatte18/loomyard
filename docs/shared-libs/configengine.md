# `internal/configengine`

Loads, resolves and writes a module's YAML configuration under a base directory's `_lym/config/`.
It is the one place that knows the `_lym/` layout, and it enforces strict validation against a template.

## Layout

```
<anchor>/
├── _lym/
│   └── config/         git-TRACKED: the worktree layer's files, one <module>.yaml each
├── .lym/
│   └── config/         git-IGNORED: the local layer's <module>.local.yaml files
├── .env                git-IGNORED: local env values (KEY=value)

<hub>/_bolt/_lym/config/
└── <module>.yaml       hub-wide: one per hub (must match the module's template)
```

A module marked hub-wide in `configreg` (`Module.HubWide`) keeps one config file under the hub's board dir, not one in each worktree's `_lym/config/`.

`_lym/` presence at the base layer's directory is what makes it initialised for `Load`;
when it is absent, `Load` errors with an error wrapping `ErrNotInitialized` (see `FindBaseDir`'s error messages below).
`LoadOrTemplate` resolves the caller's embedded template instead of erroring on that same absence — see "Resolution model" below.
Resolution is **cwd-authoritative**: the cwd does **not** need to equal the git-repo root.

## Resolution model

`Dirs` tells the geometry: `Anchor` is the reading worktree's anchor, and `Board` is the hub's board dir, empty standalone.
Both are told, never derived.
A per-worktree module stacks the template, the repo default, the worktree's tracked file and the untracked local override, lowest first;
a hub-wide module stacks the template, the hub file and the local override.
The package doc lists the layers in full.

**Two policies, one shared body:** `Load(dirs, spec)` is strict — an absent base layer, `_lym/` directory or config file, is an error.
`LoadOrTemplate(dirs, spec)` degrades — either absence resolves the spec's template as the base instead.
Both read the base layer, stack the upper layers present, validate against the template,
and resolve environment variables whenever a base file is present.

**Flow (the strict path, shared by both entry points whenever a base file is present):**

1. Call `FindBaseDir` on the base layer's directory (`Board` when told, else `Anchor`) — check that `_lym/` exists there.
2. Read the base file (e.g., `_lym/config/loom.yaml`).
   If absent, `Load` returns an error naming the way forward for the file's kind, such as `lym config reconcile`;
   `LoadOrTemplate` instead takes the degrading path below.
3. Merge the present layers unexpanded in stack order, then fill the template keys the merged stack lacks in memory
   via `yamlengine.FillMissing`.
   A shape mismatch or unparseable YAML returns an error naming the file and the key-path.
   When it filled any key, one `logger.Info` line (`configengine: filled missing keys from template`) carries the module, the file and the filled key-paths;
   a hub's sparse base logs no fill.
   No file is written;
   `lym config reconcile` stays the only writer of config files.
   Then run `yamlengine.MissingKeys(template, filled)` as a backstop:
   `FillMissing` never descends into lists, so a template key missing inside a list element of mappings is still reported.
   That returns an error naming the file and the key-paths, without a reconcile hint,
   since reconcile carries lists whole and cannot add the key either.
4. Build the environment via `envsource.Build` on the expansion directory (`Board` for a hub-wide module, `Anchor` otherwise);
   it reads `.env` and overlays OS env.
5. Resolve environment variables via `yamlengine.Resolve(filled, env)` (expands `${env:...}` markers).
6. Return the resolved bytes (see "What it returns" below).

**The degrading path (`LoadOrTemplate` only):**

`LoadOrTemplate` diverges from the flow above at exactly two points, each gated on *proven* absence — `errors.Is(err, ErrNotInitialized)` at step 1, `os.IsNotExist(err)` at step 2 — never on any other failure at either point.
On either proven absence, it takes the template's bytes as the base and skips step 3's fill:
neither `yamlengine.FillMissing` nor `yamlengine.MissingKeys` is run, since a template compared against itself is vacuously satisfied.
It then continues through steps 4-6 unchanged, still stacking the upper layers present.
A non-absence failure at either point — a stat error, a permission or IO read error — propagates unchanged, exactly as under `Load`.
A config file that exists but is invalid still errors exactly as under `Load`;
only its *absence* degrades.

**Key properties:**

- **All defaults live in the template YAML file**, not in code.
  The template is embedded via `//go:embed` and passed to `Load`/`LoadOrTemplate` as the spec's template.
- **A key missing from the layer stack resolves to its template default and is logged.**
  Only key-paths in the template are filled, only with the template's own value, and only where the stack lacks the key and every ancestor it holds is a mapping.
  The fill is in memory; no file is written.
- **Errors are strict on `Load`**: an absent base file, shape mismatches, a template key missing inside a list element, or unset required env vars cause hard errors with clear messages naming the file and the problem.
  Under `LoadOrTemplate`, the shape-mismatch, list-element and unset-required-env-var halves still hold;
  only the absent-base half is replaced by a resolved template default.
- **Extra/stale keys are tolerated** by both entry points and cleaned up by `lym config reconcile` (reconciliation).
  A default- or worktree-layer file holding them is logged at Warn, once per file and key list in a process, naming the reconcile that removes them: `lym config reconcile --layer default --apply` from the hub's prime for a file under the board dir, `lym config reconcile --apply` for a worktree file.
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
path: ${env:LYM_EXAMPLE_PATH:-../_bolt}/sub
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
Each caller unmarshals this into its own config struct;
a strict caller reads through `Load`, and a degrading caller reads through `LoadOrTemplate`.
Callers never see raw YAML or unexpanded tokens.

## Reconciling config files

`lym config reconcile --apply` from the code worktree reconciles all module configs against their templates.
The code worktree's `_lym` is a directory junction into its records sibling's `_lym`,
so a single `lym config reconcile` there reaches the worktree's own and local config files;
no separate command in the records sibling is needed.
The files under the hub's board dir, the hub-wide modules' files and the per-worktree repo defaults,
are reconciled by `lym config reconcile --layer default`, applied from the hub's prime, and by the hub reconcile the first start verb after a binary change runs.

## The `_lym` token

[`internal/lymdirs` is the sole declarer](../../PATTERN.md) of the `"_lym"` token (`lymdirs.LymDirName`);
`internal/configengine` uses `lymdirs.LymDirName` like every other caller and does not declare or export the literal.
Every module joins its own private relative-path constant onto a `baseDir` directly (e.g. `filepath.Join(baseDir, lymdirs.LymDirName, "plan")`), never onto a fused `"_lym/..."` literal — see the per-segment join rule in [PATTERN-cwd-resolution](../../pattern/PATTERN-cwd-resolution.md).

## Exported functions

### `ConfigDir(baseDir string) string`

Returns `filepath.Join(baseDir, lymdirs.LymDirName, "config")` — the directory where module configuration YAML files are stored.

### `ConfigFile(baseDir, module string) string`

Returns `filepath.Join(ConfigDir(baseDir), module+".yaml")` — the path to a specific module's configuration file (e.g. `_lym/config/loom.yaml`).

### `StagingFile(baseDir, module string) string`

Returns `filepath.Join(baseDir, lymdirs.DotLymDirName, "config", module+".yaml")` — the ephemeral `.lym` counterpart of `ConfigFile` at the mirrored subpath, where an interactive edit works on a copy that is never tracked.

### `ConfigFileRel(module string) string`

Returns `filepath.Join(lymdirs.LymDirName, "config", module+".yaml")` — the anchor-relative form used to build records commit pathspecs, as opposed to `ConfigFile`'s base-joined absolute form.

### `FindBaseDir(cwd string) (string, error)`

Checks whether the given directory is an initialized Loomyard base directory.

**Behavior:** Performs a strict check that `<cwd>/_lym` exists;
it never walks up to parent directories.
This is the cwd-authoritative model — the provided `cwd` must itself be initialized.

**Returns:** On success, the `cwd` itself (unchanged).
On failure, an empty string and an error.

**Error messages:**
- If `<cwd>/_lym` does not exist: `not initialized: _lym/ directory not found`.
  This error wraps the exported `ErrNotInitialized` sentinel;
  `errors.Is(err, configengine.ErrNotInitialized)` is the supported way to detect absence, not text matching.
- If stat fails for another reason: `stat _lym: <underlying error>`.
  This branch does not wrap `ErrNotInitialized`, since a stat failure (permission, IO) is not absence.

**Note on error rewrapping:** Some strict callers of `Load` match the substring `"not initialized"` in the error text to rewrap it into a module-level message: `not initialized here; run "lym fabric reconcile"`.
`boardengine.LoadConfig` instead detects absence with `errors.Is(err, configengine.ErrNotInitialized)` and an absent-file check, and names the hub's `board.yaml`: `board config <path> not initialized; run "lym fabric reconcile"`.
Both the substring match and `errors.Is` against the `ErrNotInitialized` sentinel are supported.
Do not conflate:
- Raw `FindBaseDir` error: `not initialized: _lym/ directory not found`
- Strict-caller rewrapped: `not initialized here; run "lym fabric reconcile"`

### `Load(dirs Dirs, spec Spec) ([]byte, error)`

Loads and resolves a module's configuration from its layer stack under `dirs`.

**Behavior:** Implements the six-step flow described in the Resolution model section above:
check the base layer's `_lym/` exists, read the base file, merge the layers and fill missing template keys, build the environment, and resolve env vars.
A present file that fails to parse or resolve, or lacks a key inside a list element, yields an error satisfying `ErrInvalid`.
A caller marks its own unmarshal and validation failures with `MarkInvalid`.

**Error cases:**

- **Base config file absent:** Returns an error naming the file and the way forward for its kind, such as `config file <path> not found; run "lym config reconcile"`.
- **Template key missing inside a list element:** Returns error `config file <path>: missing keys: <comma-separated key-paths>`, with no reconcile hint.
- **Shape mismatch against the template:** Returns error `config file <path>: <error naming the key-path>`.
- **Unset required env var:** Returns error `config file <path>: unset required env var "NAME"`.
- **Env build failure:** Returns error `config file <path>: build environment: <underlying error>`.
- **YAML syntax error:** Returns error `config file <path>: <parse/marshal error>`.

All error messages include the file path and context to guide the user.

**Returns:** On success, the resolved YAML bytes.
On error, nil bytes and an error message.

### `LoadOrTemplate(dirs Dirs, spec Spec) ([]byte, error)`

Takes the same arguments and returns the same results as `Load`.
A provably-absent base-layer `_lym/` directory or base file resolves the spec's template through `envsource.Build` then `yamlengine.Resolve` instead of erroring — see the "degrading path" in the Resolution model section above.
The fill and `yamlengine.MissingKeys` are skipped on that path, since the bytes being resolved are the template itself.
A config file that exists but is invalid still errors exactly as under `Load`;
any non-absence failure — a stat error, a permission or IO read error — propagates unchanged.

**Error cases:** Identical to `Load`'s, except that config-file absence and `_lym/` absence produce no error — those two cases resolve the template instead.
A fallback-path error is keyed on the module, not on a config-file path that does not exist: `<module> config template: <underlying error>`.

**Returns:** On success, the resolved YAML bytes (either the on-disk config, or the resolved template on proven absence).
On error, nil bytes and an error message.

### `Set(dirs Dirs, spec Spec, layer Layer, pairs []yamlengine.KV) ([]string, error)`

Writes an explicit list of key=value pairs into one layer of a module's config.
It is the non-interactive path of the `lym config <module> --set key=value` CLI — no editor is invoked and there is no validation loop.

**Behavior:**

1. Checks that the layer applies and that `_lym/` exists at the layer's owner directory, then scaffolds the file from the template when it is an absent standalone or seed-only base layer.
2. Writes a standalone or seed-only base through `yamlengine.SetValues`, which validates every requested pair's key against the template's leaf-key set and, when all keys are known, applies the pairs and marshals the merged result.
   A hub's base layer and every layer above the base are sparse: only the keys set are written, through `yamlengine.SetSparse`, and nothing is scaffolded.
3. Rejects the whole call — freshly-scaffolded file removed, existing file left untouched — when any *requested* pair's key is absent from the template's leaf-key set;
   see `internal/yamlengine`'s `SetValues` documentation for the full validation and preservation mechanism.
4. Checks a write to the local layer with `ValidateLocal` and one to the machine layer with `ValidateMachine`;
   a refused write writes nothing.
5. On any error return, a freshly-scaffolded file is removed before returning, mirroring `Edit`'s abort-removes-scaffold contract:
   a failed `--set` never leaves a fresh default-valued file behind on disk.

**Error cases:**

- **Not initialized:** Propagates `FindBaseDir`'s error.
- **Layer does not apply:** Returns an error saying the layer of the module's config does not apply here.
- **Scaffold failure:** Propagates the underlying filesystem error.
- **Unknown config key(s):** Returns error `unknown config key(s): <requested keys> (known: <template's known keys>)`.
- **Read/write failure:** Propagates the underlying filesystem error.

**Returns:** On success, the sorted list of pre-existing top-level config keys not present in the spec's template that were preserved verbatim rather than dropped (see `internal/yamlengine`'s `SetValues` documentation for the full preservation mechanism),
and a nil error.
This list is nil/empty when no such orphaned key was present.
On any error return, the returned `[]string` is always nil.

### `Edit(dirs Dirs, spec Spec, layer Layer, edit EditorFunc) error`

Opens one layer's file of a module in an editor, validates the YAML syntax, and loops on a validation failure.
An absent base layer is scaffolded first:
the header for a hub's sparse base, the template for a standalone or seed-only one;
an absent layer above the base is scaffolded empty.
On abort it removes any scaffolded file and returns `ErrAborted`.
Validation is syntactic only, except that a saved local override must pass `ValidateLocal` and a saved machine override `ValidateMachine`.
A refused save is put back to its previous bytes, or removed when the call created it.

### `EditPath(path, template string, edit EditorFunc) error`

The interactive edit of a config file at a told path, with no `_lym/` check.
It scaffolds `path` from `template` when absent, creating its parent directory, then runs the editor and loops on a YAML parse error.
An unchanged unparseable save returns `ErrAborted`, and every abort removes a file `EditPath` itself scaffolded.
Validation is syntactic only.
