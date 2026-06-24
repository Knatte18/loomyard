# Discussion: Extract yamlengine and migrate config via `lyx update`

```yaml
task: Extract yamlengine and migrate config via lyx update
slug: yamlengine
status: discussing
parent: main
```

## Problem

Every lyx config module (board, worktree, weft) carries its defaults in **two**
places: a hardcoded Go `DefaultConfig()` *and* the template comments
(`internal/board/template.go` etc.). The two can drift. The templates are also
built as awkward Go string-builders (`sb.WriteString(...)`), and because the
template lines are *commented out*, the existing resolution engine never sees
them — so env/default substitution is decoupled from the real defaults.

There already is a resolution engine, but it is buried inside `internal/config`
(alongside the `Edit` machinery and typed wrappers), it reads OS env directly,
and it only handles flat `map[string]string`. We want: one source of truth for
defaults (the template file itself), a clean general-purpose engine extracted to
its own module, support for **nested** config (loom will grow many reviewers,
each with its own sub-config), and an explicit **migration command** so config
files can be reconciled against their template without version numbers.

**Why now:** loomyard is the revised successor to millhouse and will need nested
per-reviewer config soon; the duplicated-defaults + commented-template design
blocks that and is error-prone today.

## Scope

**In:**

- **New module `internal/yamlengine`** — a general, pure (no I/O) YAML engine:
  - `Resolve` — parse YAML into a `yaml.Node`, walk **every scalar leaf**, expand
    `${env:...}` markers using a caller-supplied env map, return the resolved
    YAML. Supports nesting. Knows nothing about `.env`, OS env, or files.
  - `Reconcile` — render-with-overrides: comments + key order + default
    expressions come from the template; surviving keys keep the user's raw value;
    missing keys are added from the template; stale keys are removed. Recursive
    (handles nested maps). Reports added/removed key-paths.
- **New module `internal/envsource`** — `Build(baseDir)` reads `<baseDir>/.env`
  then overlays `os.Environ()` (OS wins), eager, returns the env map. This is the
  single place that decides *how* env vars enter the system; swap/extend it
  without touching the engine.
- **Templates become live YAML** (not commented-out) with `${env:...}`
  expressions; the default lives in the value. Embed them as real `.yaml` files
  via `//go:embed` instead of Go string-builders.
- **Remove `DefaultConfig()` / `DefaultOutputs()`** from board/worktree/weft — all
  defaults live in the template `.yaml`. The detached `--board-path` sync child
  (`internal/board/cli.go`) uses `Config{Path: *boardPathFlag}` directly (it only
  needs `Path`; `Sync` consumes nothing else).
- **Strict `Load`** in `internal/config`: signature changes to take the module's
  template bytes; reads the on-disk file, builds env via `envsource`, resolves via
  `yamlengine`, and **errors** if any template key-path is missing from the file
  (or the file is absent). The error must clearly state **what** is wrong (file
  path + missing key-paths) and tell the user to run `lyx update`.
- **New command `lyx update`** — the single place that structurally mutates config
  files. Dry-run by default (preview added/removed per file, writes nothing);
  `--apply` reconciles and writes atomically (`fsx.AtomicWriteBytes`). Iterates
  the module registry. No version numbers — the structural diff *is* the
  migration.
- **`lyx init` stops special-casing modules.** Both `init` and `update` reconcile
  each registry module from its template (init = "reconcile against an absent
  file" → all keys added). Each module owns its own template; init/update just
  iterate the shared registry. Consequence: `init` now also materializes
  `weft.yaml` (today it only writes board.yaml + worktree.yaml).
- **Centralize the config layout in `internal/paths`** — a `LyxDirName` constant
  plus helpers (`ConfigDir(baseDir)`, `ConfigFile(baseDir, module)`,
  `DotEnv(baseDir)`); refactor every hardcoded `"_lyx"` / `"config"` / `".env"`
  literal (in `config.go`, `edit.go`, `board/init.go`, `configcli`, `menu.go`,
  `ide/menu.go`, and `paths.go`'s own bodies) to use them.

**Out:**

- **Module activation / enable-disable** (e.g. making codeguide optional). Deferred
  to the codeguide-port task. All current registry modules (board, worktree, weft)
  are treated as **always-active**. See *Technical context → Forward-looking* for
  the recorded design so nothing precludes it.
- **Rename / value-transform migrations.** The structural diff covers add/remove
  perfectly; renames manifest as drop-old + add-new (losing the old value), which
  is accepted. No "carry value across rename."
- **Changed-default propagation.** Reconcile syncs the key-SET, not values: a new
  default for an *existing* key does NOT overwrite the user's value.
- **User-authored comments in config files** — regenerated from the template on
  every reconcile (acceptable for a generated file).
- **`lyx <module> init` per-module subcommands** — not added; init iterating the
  shared registry already honors module ownership of templates.
- **Escaping a literal `${env:...}`** and a literal `}` inside a default — no
  escape mechanism (YAGNI).
- **Non-flat substitution semantics beyond scalar leaves** — lists/maps are
  preserved structurally; only scalar string leaves are scanned for `${env:...}`.

## Decisions

### env-marker grammar

- Decision: **`${env:NAME}` (required) / `${env:NAME:-default}` (optional)**,
  brace-delimited, POSIX-parameter-expansion style, namespaced with `env:`.
  - Required `${env:NAME}` → error if the var is absent.
  - `:-` → use the default when the var is **absent or empty** (shell-faithful).
  - The default is the **literal text** between `:-` and the closing `}` —
    spaces preserved, **no quote-stripping, no trimming**. `${env:VAR:-}` is an
    empty-string default. Quotes, if written, are literal characters.
  - **Interpolation is supported**: markers may appear inside a larger string,
    e.g. `path: ${env:LYX_BOARD_PATH:-../_board}/sub`. The braces make this
    unambiguous.
  - A value with no `${...}` is a literal (the YAML-decoded scalar, verbatim).
  - YAML-special characters in a value/default are handled by quoting the **whole
    YAML value** (`key: "${env:X:-a: b}"`) — that's YAML's job, not the engine's.
- Rationale: brace-delimiting is the generic, recognized convention (shell,
  docker-compose, `envsubst`) and is the only clean way to allow interpolation
  once defaults exist; `env:` keeps the "this is a placeholder, not a literal"
  signal the user wanted.
- Rejected:
  - `$env:NAME ? "default"` (the proposal's original, current code's flavor) —
    unbraced, can't interpolate cleanly, needed mandatory quoting to delimit the
    default.
  - Plain POSIX `${NAME}` / `${NAME:-default}` — drops the `env:` namespace.
  - Mandatory quoted defaults — unnecessary once braces delimit (relaxes an
    earlier interim decision to require quotes).
  - millhouse's `.`-dot flattened keys (`code-reviewer.provider`) — replaced by
    real nested YAML via `yaml.Node`.

### nested config support

- Decision: **Support nested YAML now**, via `gopkg.in/yaml.v3` `yaml.Node`.
  `Resolve` returns the resolved YAML (as a Node or bytes — exact return type is a
  mill-plan detail), and each typed wrapper (`board.Config`, `worktree.Config`,
  `weft.Config`) unmarshals it into its own struct. **Drop `map[string]string`**
  and the manual `raw["path"]` field-mapping in every wrapper.
- Rationale: loom will have many reviewers with separate sub-configs; nesting is a
  near-term need, not hypothetical. Node-based handling makes nesting "just work"
  for both Resolve and Reconcile and simplifies the wrappers. The file is never
  read manually at runtime, so the library controlling exact formatting is fine.
- Rejected: flat `map[string]string` (the proposal's original "flat string only"
  scope) — would force ugly key-flattening for nested config.

### Reconcile implementation = yaml.Node round-trip

- Decision: Parse the template into a `yaml.Node` (preserves key order + head/line
  comments), parse the existing file into a node/map, **walk the template tree**
  and overwrite each scalar value where the same key-path exists in the existing
  file, then marshal the template node back. Added = template key-paths absent
  from existing; removed = existing key-paths absent from template.
- Rationale: comments + order "cannot rot" because they always come from the
  template; robust against `#`/quotes/colons inside values (which a line-based
  text renderer parses fragilely).
- Rejected: line-based template renderer (byte-exact but brittle value-token
  parsing); accepted that yaml.v3 controls exact re-formatting.

### strict Load + where the policy lives

- Decision: `yamlengine` stays **pure** (Resolve + Reconcile + a recursive
  key-path-set helper). The **strict-load policy lives in `internal/config.Load`**,
  whose signature changes to take the template bytes:
  roughly `Load(baseDir, module string, template []byte) (resolved, error)`.
  Flow: check `_lyx` exists → read on-disk file (absent ⇒ strict error) →
  recursive key-path diff template-vs-file (any missing ⇒ error naming the file +
  missing paths + "run `lyx update`") → `env := envsource.Build(baseDir)` →
  `yamlengine.Resolve(fileBytes, env)`. Load errors **only on missing** template
  keys; extra/stale keys are tolerated by Load and cleaned by `lyx update`. The
  diff is **presence-based on key-paths, not values** — a key present with an empty
  resolved value (e.g. `branch_prefix` from `${env:LYX_BRANCH_PREFIX:-}`) counts as
  present and must NOT be flagged missing.
- Rationale: keeps the engine I/O-free and reusable; policy (file layout, env
  sourcing, error wording) belongs in the consumer.
- Rejected: a strict `ResolveStrict(template, existing, env)` inside `yamlengine`
  (would pull file/policy concerns into the pure engine).

### remove DefaultConfig, no helper

- Decision: Delete `DefaultConfig()` and `DefaultOutputs()` from board, worktree,
  weft. The only production consumer (`internal/board/cli.go`, the `--board-path`
  detached sync child) becomes `cfg = Config{Path: *boardPathFlag}`. No
  "defaults helper" is introduced — the template `.yaml` is the sole source of
  defaults, resolved through the normal `Load` path everywhere else.
- Rationale: `Sync` only reads `cfg.Path`; the other fields are irrelevant for the
  detached child, so no default values are needed there. Single source of truth.
- Rejected: a per-module "resolve template against empty env" helper (unnecessary
  once we confirmed the child only needs `Path`); keeping `DefaultConfig` as a thin
  wrapper (perpetuates the duplication).
- Note: the detached child's behavior is unchanged under `Config{Path: ...}`.
  `DefaultConfig()` never set `SkipGit`/`SkipPush` either (they default to zero),
  and `applySkipEnv(cfg)` (cli.go:86) still runs afterward to fold in
  `BOARD_SKIP_*`. So nothing is lost — a plan-writer should not reintroduce a
  defaults helper to "restore" Skip fields.

### init/update share the registry (module ownership)

- Decision: A single shared module registry (name → embedded template, currently
  board/worktree/weft) drives both `lyx init` and `lyx update`. Each module owns
  its template (`board.ConfigTemplate` etc. live in the module). init = reconcile
  against an absent file. The registry currently lives unexported in
  `internal/configcli`; extract/export it so `lyx update` and `lyx init` share one
  source (exact location — exported from `configcli` vs a small new package — is a
  mill-plan detail).
- Rationale: honors "a module knows how it's set up" without adding per-module CLI
  surface; eliminates init's hardcoded per-module writes.
- Rejected: `lyx init` hardcoding each module's file (current anti-pattern); full
  `lyx <module> init` subcommands (larger, overlaps with `lyx update`).

### lyx update UX

- Decision: `lyx update` (dry-run default) previews `added`/`removed` key-paths per
  module and writes nothing; `lyx update --apply` reconciles and writes each file
  atomically via `fsx.AtomicWriteBytes`. Output is JSON, consistent with the rest
  of the CLI (`{"ok":true,"modules":[{"module":..,"added":[..],"removed":[..],"applied":bool}]}`).
  A wholly-absent config file is created from the template (all keys "added").
  Mirrors the `--apply` convention used elsewhere.
- Rationale: safe-by-default migration; atomic writes already available in `fsx`.
- Rejected: write-by-default; version-numbered migrations (semver would rot under
  constant small changes; structural diff is always current).

### config-path resolution (host baseDir + junction)

- Decision: `lyx update` and `lyx init` resolve each module's config file at the
  **host baseDir** (`WorktreeRoot/RelPath/_lyx/config/<module>.yaml`), the same way
  `lyx config` (configcli) does today. The host `_lyx` is a **directory junction**
  into the weft worktree's `_lyx` (`HostLyxLinkHere()` ↔ `WeftLyxDir()`), so
  board/worktree/weft config files are one physical `_lyx/config/` directory;
  `weft.LoadConfig` reads that same file via `WeftWorktree()`.
- Rationale: keeps update/init consistent with the existing `lyx config` command;
  the junction unifies host and weft `_lyx` in task worktrees.
- Rejected: resolving each module at its own distinct path (board/worktree via cwd,
  weft via `WeftWorktree()`) — redundant given the junction and inconsistent with
  configcli.

### migration of existing installs

- Decision: Accept the natural migration with no special-casing. Existing config
  files are fully **commented-out** (old format) → after this change strict `Load`
  errors until `lyx update --apply` runs. Reconcile sees the commented file as an
  empty map ⇒ all template keys "added" ⇒ writes the live template. No user values
  are lost because the old files never held live values. The strict-Load error
  names the file + missing keys + "run `lyx update`".
- Rationale: `lyx update` *is* the migration; the structural diff handles it.
- Rejected: a one-off migration shim / lenient fallback for the old format.

## Technical context

What mill-plan needs to know:

- **Current engine to extract/replace:** `internal/config/config.go` —
  `Load(baseDir, module, defaults map[string]string)`, `expandEnv`, `envOptRe`,
  `envReqRe`, `loadDotEnv`, `loadYAMLLayer`, `FindBaseDir`. `expandEnv` currently
  reads OS env directly (`os.LookupEnv`) with `.env` as fallback and does inline
  token replacement — this moves to `envsource` (env sourcing) + `yamlengine`
  (pure substitution), with the grammar changing to `${env:...}`.
- **Edit machinery stays in `internal/config`:** `internal/config/edit.go`
  (`Edit`, `EditorFunc`, `ErrAborted`, `DefaultEditor`) — only its hardcoded
  `_lyx`/`config` paths get centralized; behavior unchanged. It validates YAML
  syntax only.
- **Typed wrappers to rewrite:** `internal/board/config.go`,
  `internal/worktree/config.go`, `internal/weft/config.go` — each currently builds
  a defaults map from `DefaultConfig()`, calls `config.Load`, and maps
  `raw[...]` → struct. New: pass the embedded template, call the new strict
  `Load`, unmarshal the resolved YAML into the struct. Keep each module's
  error-wrapping ("not initialized here; run \"lyx init\"" for board/worktree;
  "weft worktree or its _lyx is missing at <dir>" for weft). **`weft.LoadConfig`
  keeps its current `weftBaseDir` argument, built at weft/cli.go:95 as
  `filepath.Join(l.WeftWorktree(), l.RelPath)` (RelPath-mirrored; the call site is
  weft/cli.go:98) — the host-baseDir-vs-weft split is unchanged; only its internal
  `config.Load` call gains the template arg.** (The host `_lyx` junction makes this the same
  physical file as the host baseDir that `update`/`init`/`config` use.)
- **Error-ignoring consumer to fix:** `internal/ide/menu.go:39` does
  `cfg, _ := board.LoadConfig(l.Cwd, "board")` and today relies on
  `DefaultConfig()` populating `cfg` even on partial failure. Under strict Load a
  missing-key/absent-file error returns a zero `Config{}` (empty `cfg.Path`), so
  `b.HealthCheck()` would run against an empty board path. The plan must make this
  call site **handle the error** (skip/flag the entry rather than HealthCheck an
  empty path) instead of swallowing it.
- **Templates to convert (commented Go string-builders → live embedded YAML):**
  - `internal/board/template.go` → keys `path`, `home`, `sidebar`,
    `proposal_prefix` with `${env:LYX_BOARD_PATH:-../_board}`,
    `${env:LYX_HOME:-Home.md}`, `${env:LYX_SIDEBAR:-_Sidebar.md}`,
    `${env:LYX_PROPOSAL_PREFIX:-proposal-}`, keeping the trailing `# ...` comments.
  - `internal/worktree/template.go` → `branch_prefix: ${env:LYX_BRANCH_PREFIX:-}`
    (empty default).
  - `internal/weft/template.go` → `pathspec: "_lyx"` — a **plain literal**
    (weft never had an env var), serving as the template's literal-value example.
  - `ConfigTemplate()` may keep its `func() string` shape returning the embedded
    file content, or the registry may hold template bytes — mill-plan's call.
- **Registry today:** `internal/configcli/configcli.go` holds the unexported
  `registry` (`{name, Template func() string}` for board/worktree/weft) plus
  `templateFor` / `moduleNames`. `lyx config` dispatches at
  `baseDir = filepath.Join(l.WorktreeRoot, l.RelPath)`. `lyx update` reuses this.
  Whether the registry entry stays `Template func() string` or becomes template
  bytes, the change ripples into `configcli`'s `templateFor` / `editOne` and the
  `config.Edit(baseDir, module, template, ...)` signature — these consumers must be
  updated in lockstep with the chosen shape.
- **board `--board-path` child:** `internal/board/cli.go:67-83` — the only
  `DefaultConfig()` production caller; `internal/board/sync.go` confirms `Sync`
  uses only `boardPath` (= `cfg.Path`).
- **Atomic write:** `internal/fsx/fsx.go` — `AtomicWriteBytes(absPath, data)`
  (temp + rename; creates parent dirs). Use for `--apply`.
- **Output convention:** `internal/output` — `output.Ok(out, map)` /
  `output.Err(out, msg)`; all CLI output is JSON on stdout, exit 1 on error.
- **Paths module:** `internal/paths/paths.go` — `Getwd()`, `Resolve(cwd) Layout`,
  `LyxDir()` (cwd/_lyx), `HostLyxLinkHere()`, `WeftLyxDir()`, `WeftWorktree()`,
  `RelPath`, `WorktreeRoot`, `Hub`. New constants/helpers land here.
- **CLI dispatch:** `cmd/lyx/main.go` — add `case "update": return update.RunCLI(...)`
  (module name TBD), and update the doc comment listing modules.
- **init today:** `internal/board/init.go` (`RunInit`) writes board.yaml +
  worktree.yaml (not weft.yaml) and maintains `.gitignore` (`.lyx/` via
  `internal/gitignore.Ensure`). It must move to the shared-registry reconcile and
  now also create weft.yaml.
- **Docs to update:** `docs/shared-libs/config.md`, `docs/shared-libs/README.md`,
  `docs/shared-libs/paths.md`, `docs/overview.md` (config layout / module list),
  and add a yamlengine entry. Follow the doc-lifecycle convention referenced in
  CONSTRAINTS.md (`docs/overview.md#documentation-lifecycle`).

**Forward-looking (NOT implemented this task — recorded so the design isn't
precluded):** module activation will arrive with the codeguide port. The
activation check in millhouse is simply **presence of `_codeguide/Overview.md` in
cwd** (file present ⇒ codeguide active ⇒ update it; else not). codeguide is special:
it works **recursively down into subdirectories** — activating it at the repo root
covers submaps, no re-activation needed below. By contrast, lyx/loom must be
activated in a submap explicitly to be used as a base there, and **activating
loom in a submap uses the SAME board as every other loom instance in the same
repo**. So activation is expected to be **presence-based**, not a top-level
manifest (an earlier manifest idea is superseded). board/worktree/weft are
essential and always-active.

## Constraints

- **Path invariant (CONSTRAINTS.md):** all cwd/worktree-root queries go through
  `internal/paths.Getwd()` / `internal/paths.Resolve()`. Raw `os.Getwd` and
  `git rev-parse --show-toplevel` are banned outside `internal/paths` and
  `cmd/lyx/main.go`, enforced by `internal/paths/enforcement_test.go`. The new
  `update` module and any new path logic must use `paths`.
- **fslink / junctions (CLAUDE.md):** cross-OS dir links go through
  `internal/fslink` (junctions on Windows). The host `_lyx` ↔ weft `_lyx` junction
  is why a single host baseDir reaches all module config files.
- **Atomic writes:** config-file writes in `--apply` must be atomic
  (`fsx.AtomicWriteBytes`).
- **JSON output:** every CLI command emits JSON on stdout, exit 1 on error.
- **Documentation lifecycle:** keep durable design docs; update the shared-libs
  docs that describe the config engine and paths.

## Testing

`yamlengine` is pure → strong **TDD candidate**; `envsource` and `Reconcile` too.

- **`internal/yamlengine` — `Resolve` (table-driven grammar matrix):**
  required `${env:NAME}` present vs absent (error); optional
  `${env:NAME:-default}` unset/empty → default, set → value; empty default
  `${env:VAR:-}`; default with spaces (no trimming, no quote-stripping); literal
  quotes inside default; **interpolation** (`${env:X:-d}/sub`, multiple markers in
  one value); literal value (no marker) untouched; **nested** maps (leaves at
  depth); lists preserved; no recursive expansion (a resolved value containing
  `${env:...}` text is not re-expanded); value that merely contains `}` text.
- **`internal/yamlengine` — `Reconcile`:** add missing keys (with template
  default), remove stale keys, preserve surviving user values verbatim, preserve
  template comments + key order, nested add/remove/preserve, empty existing
  (all-added) = the init/migration case, commented-only existing → empty map →
  all-added, idempotence (reconcile twice = no change), correct `added`/`removed`
  key-path reporting.
- **`internal/envsource` — `Build`:** `.env` parsing (comments, blank lines, no-`=`
  lines, `=` in value); absent `.env` → OS-only; OS overlay precedence (OS wins);
  eager single build.
- **`internal/config` — strict `Load`:** happy path (all keys present → resolved
  struct); missing template key → error naming file + missing key-paths + "run
  `lyx update`"; absent file → error; extra/stale key tolerated; `_lyx` absent →
  existing "not initialized" error; nested config round-trips into the struct.
- **board/worktree/weft `LoadConfig`:** module-specific error wrapping preserved;
  relative `path` resolution still applied (board); `DefaultConfig`/`DefaultOutputs`
  removed (update/replace their tests, including `render_test.go`,
  `board_test.go`, `boardtest/*`, `config_test.go`).
- **`lyx update`:** dry-run prints added/removed and writes nothing; `--apply`
  writes atomically and is idempotent; absent file created from template; JSON
  shape; migrates a commented-old-format file to live template.
- **`lyx init`:** materializes all three live templates (incl. weft.yaml) via the
  shared registry; idempotent; `.gitignore` managed block unchanged; a fresh init
  immediately passes strict `Load`.
- **`internal/paths` centralization:** helpers return expected paths; the
  enforcement test still passes; consumers compile against the new helpers.
- **CLI dispatch:** `cmd/lyx/main_test.go` covers the new `update` route.
- Existing grammar tests in `internal/config/config_test.go` and template tests
  (`*/template_test.go`) must be rewritten for the `${env:...}` grammar and live
  templates.

## Q&A log

- **Q:** Keep `$env:` or switch to the proposal's `ENV:`? **A:** Neither verbatim — settled on brace-delimited `${env:NAME}` / `${env:NAME:-default}` (keeps the `env:` placeholder signal, adds POSIX-style braces for interpolation).
- **Q:** Require quoted defaults? **A:** No — braces delimit; default is literal text between `:-` and `}` (spaces preserved, no quote/trim), generic POSIX/envsubst practice. `${env:VAR:-}` = empty default.
- **Q:** Support interpolation (`${env:X}/sub`) now? **A:** Yes — braces make it unambiguous; it's the common approach.
- **Q:** `Resolve` env handling? **A:** Engine is pure, caller supplies the env map; a new `internal/envsource.Build` reads `.env` + overlays OS env (OS wins), eager, built in this task.
- **Q:** Where does strict-load policy live? **A:** In `internal/config.Load` (takes template bytes); `yamlengine` stays pure.
- **Q:** Behavior when a config file is missing/incomplete? **A:** Error; the message must clearly state *what* is wrong (file + missing key-paths) and point to `lyx update`.
- **Q:** Flat or nested config? **A:** Nested, via `yaml.Node`; drop `map[string]string`; wrappers unmarshal into typed structs. Motivated by future multi-reviewer sub-configs.
- **Q:** Reconcile implementation? **A:** `yaml.Node` round-trip (comments/order from template, user values overlaid), not line-based.
- **Q:** Keep a defaults helper after removing `DefaultConfig`? **A:** No helper — the `--board-path` child only needs `Path`, so `Config{Path: ...}`; all other defaults come from the template file.
- **Q:** Should `lyx init` hardcode each module's config write? **A:** No — init and update share the module registry and reconcile each module from its template; init now also creates weft.yaml.
- **Q:** Include module activation (codeguide opt-in) in this task? **A:** No — deferred to the codeguide-port task; board/worktree/weft always-active. Recorded that activation will be presence-based (`_codeguide/Overview.md`), recursive into subdirs; loom in a submap shares the same board.
- **Q:** Escape for a literal `${env:...}` / literal `}` in a default? **A:** None (YAGNI).
- **Q:** Hardcoded `_lyx`/config paths? **A:** Centralize in `internal/paths` (constant + helpers) and refactor all consumers, including paths.go's own literals.
- **Q:** How does `lyx update` find weft's config given weft reads from the weft worktree? **A:** Use the host baseDir like `lyx config`; the host `_lyx` is a junction into the weft `_lyx`, so it's one physical file.
- **Q:** Migration for existing commented-out config files? **A:** No special-casing — strict Load errors, `lyx update --apply` rewrites them to live templates (empty existing → all-added); no values lost.
- **Q (review r1 gap):** What about `internal/ide/menu.go:39`'s error-ignoring `cfg, _ := board.LoadConfig(...)` under strict Load? **A:** Plan must make that call site handle the error (skip/flag the entry) rather than HealthCheck an empty `cfg.Path`.
- **Q (review r1 gap):** Does `weft.LoadConfig`'s baseDir change? **A:** No — it keeps `weftBaseDir` from `WeftWorktree()`; only its internal `config.Load` call gains the template arg (host junction makes it the same physical file).
- **Q (review r1 note):** Could an empty resolved value be treated as a missing key? **A:** No — the strict diff is presence-based on key-paths; an empty value (e.g. `branch_prefix`) counts as present.
- **Q (review r1 note):** Registry template shape? **A:** `func() string` vs bytes ripples into `configcli` (`templateFor`/`editOne`/`config.Edit`); those must change in lockstep with the chosen shape.
