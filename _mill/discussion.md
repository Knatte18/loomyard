# Discussion: board-modul (rename fra wiki) + _mhgo-konfigurasjon

```yaml
task: board-modul (rename fra wiki) + _mhgo-konfigurasjon
slug: config-layer
status: discussing
parent: main
```

## Problem

`mhgo` is the in-progress Go reimplementation of Millhouse: **one binary
(`mhgo.exe`) that will eventually cover everything Millhouse does today**, with
each capability as a submodule dispatched by the first CLI argument
(`mhgo <module> ...`). Today it has exactly one module, and it is named `wiki`.

That name is dangerously ambiguous — it collides both with millpy/millhouse's
own Python "wiki" **and** with GitHub's per-repo wiki. The ambiguity is not
theoretical: it already let the Go port write its flat `tasks.json` into the
Python-owned wiki directory and break the Python daemon. The module is really a
per-repo collection of tasks, shared across worktrees, rendered to readable
markdown (layers A/B/C + dependencies — half a kanban board already). The right
name is **`board`**.

Separately, the module hardcodes its output filenames (`Home.md`,
`_Sidebar.md`, `proposal-` prefix) and resolves its target directory through a
brittle precedence — `--wiki-path` flag → `MHGO_WIKI_PATH` env → hardcoded
`../gowiki` — with no git-tracked, per-repo source of truth. There is no way to
render the board onto a normal repo's `README.md` landing page, and the path
resolution silently assumes things about the working directory.

**Why now:** the broken-daemon incident forces the rename, and `mhgo` is about
to grow more modules — the naming and configuration foundation must be correct
before anything else is built on top of it.

## Scope

**In:**

- **Rename `wiki` → `board`, in full:** `internal/wiki` → `internal/board`;
  `package wiki` → `package board`; the `Wiki` struct → `Board` (and `New`,
  `RunCLI`, all identifiers carrying the `wiki` name); CLI `mhgo wiki ...` →
  `mhgo board ...`; the `wikitest` package → `boardtest`; control env vars
  `WIKI_SKIP_GIT`/`WIKI_SKIP_PUSH` → `BOARD_SKIP_GIT`/`BOARD_SKIP_PUSH`; the
  background commit message `"wiki sync"` → `"board sync"`; docs `wiki.md` →
  `board.md` plus all references in `overview.md`/`benchmarks.md`.
- **A layered configuration system** for the `board` module, read per
  invocation from the current working directory:
  built-in defaults **<** `_mhgo/board.yaml` (git-tracked) **<**
  `.mhgo/board.yaml` (gitignored, machine-local), deep-merged per key, with
  `$env:NAME` interpolation applied to the merged values.
- **Make configurable** via that system: the board directory path, the home
  filename (`Home.md` *or* `README.md` etc.), the sidebar filename, and the
  proposal-file prefix. This **replaces and removes** the `--wiki-path` flag,
  the `MHGO_WIKI_PATH` env precedence, the hardcoded `../gowiki` default, and
  the hardcoded `Home.md`/`_Sidebar.md`/`proposal-` names.
- **A new top-level `mhgo init`** command that scaffolds `_mhgo/board.yaml`
  (config only — see Decisions).
- **A new `docs/roadmap.md`** capturing the long-term direction and all
  deferred work (see Decisions → roadmap-split).
- **Update every test and doc** affected by the rename and the config change.

**Out (recorded in `docs/roadmap.md`, not implemented here):**

- Creating or cloning the board repo itself (the future "real" growth of
  `init`, analogous to mill-setup Phases 1–3). For now the board directory is
  auto-created on first write; the user makes it a git repo for push to work.
- Seeding a `.mhgo/board.yaml` override stub or persisting machine-local
  overrides (analogous to mill-setup 3.2/5).
- A `verify`/doctor subcommand (analogous to mill-setup Phase 8).
- Any second module (e.g. a `mill` orchestrator), Claude-Code-plugin packaging,
  and all millpy plumbing that does not apply to a Go binary — junctions,
  hardlinks, portals, `PYTHONPATH`/venv/`MILL_PYTHON`, the wiki daemon,
  VS Code colours, `Home.md` content-shape heuristics, wiki-URL derivation.
- Renaming the on-disk `tasks.json` and `*.lock`/`*.swaplock` files (internal
  to the board directory; renaming costs a data migration for no benefit).
- Migrating existing `../gowiki` data to `../_board` (operational, not code).

## Decisions

### domain-model (rationale for the structure below)

- Decision: `mhgo` is treated as **one program = the Go reimplementation of all
  of Millhouse**; modules are submodules of that one binary, dispatched by the
  first argument. `board` is one module; future capabilities are future
  modules. This is *why* config is keyed per module and `init` is top-level.
- Rationale: matches the existing `main.go` module switch and the user's intent
  that mhgo grows to cover everything Millhouse does.
- Rejected: treating mhgo as a single-purpose board tool (would have justified
  hardcoding `board.yaml` and a `board init` subcommand).

### rename-depth

- Decision: **full rename** including env knobs, the sync commit message, and
  all Go identifiers; **keep** the on-disk `tasks.json` and lock filenames.
- Rationale: the whole point is to erase the ambiguous "wiki" name from every
  user- and developer-facing surface; on-disk data names are internal to the
  board directory and renaming them forces a migration with no upside.
- Rejected: minimal rename (leaves "wiki" in env/API); total rename including
  `tasks.json`→`board.json` (forces data migration).

### config-location (cwd is authoritative)

- Decision: configuration lives at **`<cwd>/_mhgo/`**, read fresh each
  invocation. No walk-up, no `git rev-parse`, no discovery. If `<cwd>/_mhgo/`
  does not exist, the command **errors** with a clear "not initialized here;
  run `mhgo init`" message. The two layers are `<cwd>/_mhgo/board.yaml`
  (tracked) and `<cwd>/.mhgo/board.yaml` (gitignored).
- Rationale: "CWD need not equal the git repo root" means mhgo must not assume a
  git root — the cwd is the unit of activation. Using mhgo in a directory where
  it was never initialized is a usage error, not something to paper over.
- Rejected: walk-up to nearest `_mhgo/`; resolve via git toplevel (both tie
  resolution to assumptions the cwd model deliberately drops).

### env-interpolation (not a precedence layer)

- Decision: there is **no implicit env-var override layer**. Env vars are only
  consulted when a config value explicitly references one via the token
  `$env:NAME`, which is expanded from the **process environment** at resolution
  time. The token may appear anywhere within a value (e.g.
  `path: $env:MHGO_BOARD_PATH/sub`); the variable name matches
  `[A-Za-z_][A-Za-z0-9_]*`. A referenced-but-unset variable is a **hard error**.
- Rationale: the user wants env input to be opt-in and *local* (PowerShell
  `$env:X` is session-scoped, not machine-global). An unreferenced env var is
  never used; failing loudly on an unset reference beats resolving to an empty
  path.
- Rejected: env var as an implicit precedence layer (the old `MHGO_WIKI_PATH`
  model the user explicitly dislikes); unset → empty string (silent later
  failure); `${NAME}` POSIX syntax (doesn't match the user's mental model).

### config-schema

- Decision: one flat YAML file per module, `_mhgo/<module>.yaml` (module =
  filename). For `board`, the keys and built-in defaults are:

  ```yaml
  path: ../_board       # board dir (tasks.json + rendered output); relative to
                        # cwd; may contain $env:... ; "_board" = system folder
  home: Home.md         # set to README.md to render on a repo landing page
  sidebar: _Sidebar.md
  proposal_prefix: proposal-
  ```

  Add `gopkg.in/yaml.v3` to parse it.
- Rationale: filename-as-module-key matches "one YAML per module" and makes a
  future module just a new file; flat keys are simplest; `../_board` underscores
  that the board dir is a system/managed folder like `_mhgo`.
- Rejected: nesting under a `board:` key (redundant — the filename already
  scopes it); TOML; hand-rolled parser (fragile).

### merge-and-defaults

- Decision: deep-merge **per key**: built-in defaults **<** `_mhgo/board.yaml`
  **<** `.mhgo/board.yaml`. A key absent from a higher layer falls through to
  the layer below, then to the built-in default. `$env:` expansion runs on the
  merged result. A relative `path` resolves against cwd (the dir holding
  `_mhgo/`); an absolute path (after expansion) is used as-is. If `_mhgo/`
  exists but `_mhgo/board.yaml` is absent, built-in defaults apply (no error) —
  `_mhgo/` presence is the activation signal, the file is optional sugar.
- Rationale: matches §3's three layers and keeps a freshly-`init`'d repo usable
  before the user edits anything.
- Rejected: requiring `board.yaml` to exist (needless friction).

### config-seed-style (init writes commented defaults)

- Decision: `mhgo init` seeds `_mhgo/board.yaml` as **all-commented
  documentation** — every key shown with its default value, commented out, with
  an explanatory comment. No active values. Never clobber an existing file.
- Rationale: the tracked file documents the schema, but the **code defaults
  always apply** until the user uncomments a key. Writing active values equal
  to today's defaults would freeze them into every repo and silently override a
  future change to a built-in default.
- Rejected: seeding active values (causes default-drift).

### init-scope (config scaffold only, for now)

- Decision: `mhgo init` is a **top-level** command (a `case "init"` in
  `main.go`, beside `case "board"`). For this task it does exactly:
  (1) create `<cwd>/_mhgo/` if missing; (2) write the commented
  `_mhgo/board.yaml` if missing; (3) maintain a `.gitignore` managed block (see
  gitignore-block) containing `.mhgo/`; (4) print a JSON action summary
  (created vs skipped per item); fully idempotent / re-run safe. It does **not**
  create or clone the board directory.
- Rationale: `init` will eventually grow into a mill-setup-equivalent, but most
  of mill-setup is millpy plumbing irrelevant to a Go binary or needs LLM
  judgment. Keep the mechanical, in-domain minimum now; defer the rest to the
  roadmap.
- Rejected: creating/cloning the board dir now; doing the full mill-setup
  surface.

### gitignore-block

- Decision: `init` maintains an `# === mhgo-managed === ... # === end
  mhgo-managed ===` marker block in `<cwd>/.gitignore` (create the file if
  absent), containing `.mhgo/`. Idempotent: only rewrites when the block's
  contents differ.
- Rationale: mirrors mill-setup Phase 4.5b — a managed block is idempotent,
  cleanly removable, and extensible by future `init` features; a bare append is
  not.
- Rejected: bare idempotent append (no clean removal/extension story).

### board-dir-autocreate

- Decision: when the configured board directory does not exist at write time,
  create it with `MkdirAll`. File writes always succeed and are durable on local
  disk. The detached `board sync` push **silently no-ops** if the board dir is
  not a git repo (sync runs detached; its errors are already not surfaced).
- Rationale: convenient first run; backup is a best-effort background concern
  that already tolerates failure.
- Rejected: erroring until the board dir exists (needless friction for the
  common first-write case).

### spawn-sync-path

- Decision: `spawnSync` passes the **resolved absolute board path** to the
  detached `mhgo board sync` child via an internal `--board-path` flag (the term
  "wiki-path" is retired). The child does **not** re-resolve config: when
  `--board-path` is present, `RunCLI` skips `LoadConfig` **and** the
  `<cwd>/_mhgo/` existence check entirely (the path is injected, not resolved),
  so the detached child never spuriously errors from an inherited cwd that lacks
  `_mhgo/`. Output names are not passed to the child — `sync` touches only git
  and `tasks.json`, never rendering.
- Rationale: the public CLI is flag-free, but the detached background process
  must get an unambiguous path that is immune to any cwd/env differences in the
  child.
- Rejected: child re-resolves from its inherited cwd (re-reads config and
  re-expands env in a background process — fragile).

### testability (no process-global chdir)

- Decision: the config resolver takes an **explicit base-dir parameter**
  (`LoadConfig(baseDir, module)`); the CLI calls `os.Getwd()` once at the top of
  `RunCLI` and threads the directory down. Unit tests pass a temp dir — no
  `os.Chdir`. Keep a `New(boardPath, outputs)`-style facade constructor so
  facade/store/render tests bypass config resolution entirely.
- Rationale: `os.Chdir` is process-global and unsafe with parallel tests;
  passing the dir keeps resolution pure and parallel-safe.
- Rejected: `t.Chdir` per test (forces those tests serial).

### roadmap-split

- Decision: `discussion.md` stays scoped to **this** task. All future/deferred
  work goes into a new **`docs/roadmap.md`** (created by this task), which later
  tasks update continuously. Do not enumerate future tasks inside
  `discussion.md`, and do not create task entries now.
- Rationale: future-task content in a per-task discussion rots; a living roadmap
  doc is the right home and stays current.
- Rejected: listing future tasks in `discussion.md`; pre-creating task entries.

## Technical context

The repo is a small single-package Go module (`github.com/Knatte18/mhgo`,
go 1.26), today only depending on `github.com/gofrs/flock`. Layout:

- `cmd/mhgo/main.go` — thin module dispatcher: `run(args, out)` switches on the
  first arg. Today `case "wiki": return wiki.RunCLI(...)`. Add `case "board"`
  and a new top-level `case "init"`; drop `case "wiki"`. `main()` stays
  `os.Exit(run(os.Args[1:], os.Stdout))`.
- `internal/wiki/` (→ `internal/board/`), `package wiki` (→ `package board`):
  - `cli.go` — `RunCLI(out, args)` parses `[--wiki-path] <subcommand>
    [json-payload]`, calls `resolveWikiPath` (flag → `MHGO_WIKI_PATH` →
    `../gowiki`), constructs `New(wikiPath)`, dispatches subcommands (`upsert`,
    `upsert-batch`, `set-phase`, `remove`, `get`, `list`, `list-full`, `merge`,
    `set-deps`, `rerender`, `sync`). **Changes:** delete the `--wiki-path` flag
    and `resolveWikiPath`; at the top call `os.Getwd()` + `LoadConfig(cwd,
    "board")`; build the `Board` from the resolved path + output names; keep the
    JSON output helpers. The internal `--board-path` flag (spawn-sync-path) is
    parsed here for the detached `sync` child only.
  - `wiki.go` (→ `board.go`) — the `Wiki` (→ `Board`) facade and `writeOp`
    (lock → load → mutate → save `tasks.json` → `RenderToDisk` → spawn detached
    sync unless `*_SKIP_GIT`). The struct currently holds only `wikiPath`; it
    must also carry the resolved output names (home/sidebar/proposal_prefix) to
    pass into rendering. `New(...)` signature changes accordingly.
  - `render.go` — `Render(tasks)` returns `map[relPath]content` with **hardcoded**
    `"Home.md"`, `"_Sidebar.md"`, and `proposal-<slug>.md`; `RenderToDisk`
    writes them; `removeOrphanProposals` globs `proposal-*.md`. **Changes:**
    thread the configured home/sidebar filenames and proposal prefix through
    `Render`/`RenderToDisk`. The proposal prefix appears at **four** sites, all
    of which must use the configured value: filename generation in
    `renderProposals`, the orphan glob in `removeOrphanProposals`, and the
    in-content proposal links hardcoded as `proposal-%s.md` in `renderHome`
    (line 97) and `renderSidebar` (line 146) — miss the last two and the links
    point at files that no longer exist under a custom prefix.
  - `store.go` — in-memory store over `tasks.json` (CRUD + validation + atomic
    save under swap lock). `tasks.json` and `*.swaplock`/`*.lock` names stay.
  - `sync.go` — background pusher; constants `writeLockFile`, `pushLockFile`;
    reads `WIKI_SKIP_GIT`/`WIKI_SKIP_PUSH` (→ `BOARD_SKIP_*`); commit message
    `"wiki sync"` (→ `"board sync"`); `ensureLockfilesIgnored` writes the
    board dir's own `.gitignore`.
  - `spawn_windows.go` / `spawn_other.go` — `spawnSync` runs
    `exec.Command(exe, "wiki", "--wiki-path", wikiPath, "sync")`. **Changes:**
    `"wiki"` → `"board"`, `--wiki-path` → internal `--board-path` carrying the
    resolved absolute path.
  - `layer.go`, `git.go`, `lock.go`, `task.go` — touched only by the package
    rename and any identifier references.
  - `wikitest/` (→ `boardtest/`) — black-box benchmarks, concurrency, and
    git-backed integration suites; `package wikitest` → `boardtest`; imports and
    `New`/env/filename usages updated.
- New file(s) in `internal/board/`, e.g. `config.go` (+ `config_test.go`):
  `Config` struct (`Path, Home, Sidebar, ProposalPrefix`), `DefaultConfig()`,
  `LoadConfig(baseDir, module string) (Config, error)` implementing
  config-location + merge-and-defaults + env-interpolation, and the
  `$env:` token expander.
- Docs: `docs/overview.md` (structure tree, module list, dispatch example all
  say `wiki`), `docs/wiki.md` → `docs/board.md` (the deep module doc — add a
  config + `init` section), `docs/benchmarks.md` (a few `wiki` references). New
  `docs/roadmap.md`.
- Repo `.gitignore` already ignores the compiled `/mhgo`/`mhgo.exe`; the
  mill-managed block is separate and untouched.

Gotchas:

- The detached sync child must receive an explicit resolved path
  (spawn-sync-path) — do not let it depend on cwd/config.
- `removeOrphanProposals` must use the **configured** prefix, or renaming the
  prefix would orphan old files and/or miss cleanup.
- `LoadConfig` must take `baseDir` explicitly (testability) — never call
  `os.Getwd()` inside the resolver.
- Empty/missing board dir on a **read** path is already fine (`store.Load`
  treats a missing `tasks.json` as an empty list); only writes need `MkdirAll`.

## Constraints

- No `CONSTRAINTS.md` at the hub root.
- Go 1.26; keep the dependency set minimal (only add `gopkg.in/yaml.v3`).
- Cross-platform: Windows + non-Windows build tags already exist
  (`spawn_windows.go` / `spawn_other.go`); env-var reading is via `os.Getenv`,
  so `$env:` interpolation is just a YAML-value convention, not OS-specific.
- All CLI output remains single-line JSON (`{"ok":true,...}` / `{"ok":false,
  "error":"..."}`, exit 1 on error), including `mhgo init`.
- Resolution is per-invocation and cwd-authoritative; mhgo must not require
  being run from a git repo root.

## Testing

Follow the existing per-file unit-test layout (`store.go` ↔ `store_test.go`)
and the black-box cross-cutting suite in `boardtest/`.

- **`config.go` (TDD candidate)** — new `config_test.go`, table-driven, passing
  an explicit temp base-dir:
  - defaults when `_mhgo/` present but `board.yaml` absent;
  - error when `_mhgo/` absent;
  - per-key deep-merge across defaults / `_mhgo/board.yaml` / `.mhgo/board.yaml`;
  - `$env:NAME` expansion (whole value and embedded `$env:NAME/sub`), including
    the hard error on an unset referenced variable;
  - relative `path` resolved against base-dir vs absolute path passed through;
  - malformed YAML surfaces an error.
- **`mhgo init` (TDD candidate)** — a test exercising: creates `_mhgo/` +
  commented `board.yaml`; idempotent re-run (no clobber); `.gitignore` managed
  block created and not duplicated on re-run; JSON summary shape.
- **`render_test.go`** — update for configurable filenames/prefix: rendering to
  `README.md` as home, a non-default proposal prefix, and orphan cleanup keyed
  to the configured prefix.
- **`cli_test.go`** — drop `--wiki-path`/`MHGO_WIKI_PATH` cases; drive the CLI
  with a temp cwd containing `_mhgo/board.yaml` (or via the facade constructor);
  cover the `board` subcommands and the new `init` path.
- **`boardtest` CLI benchmarks** — `bench_test.go` drives `RunCLI` via
  `--wiki-path <tempdir>` against dirs with no `_mhgo/`; with the flag gone and
  the CLI requiring `<cwd>/_mhgo/`, these benches must be **re-architected** (run
  in a temp cwd seeded with `_mhgo/board.yaml`, or moved to the facade
  constructor), not merely renamed. CLI-path benchmark numbers now include the
  added `os.Getwd()` + config-load cost — note this when comparing against
  historical figures.
- **Rename churn** — every test referencing `Home.md`/`_Sidebar.md`/`proposal-`,
  `--wiki-path`, `MHGO_WIKI_PATH`, `WIKI_SKIP_GIT`/`WIKI_SKIP_PUSH`, the `Wiki`
  type, or `package wikitest` must be updated; `wikitest` → `boardtest`.
- **Whole suite green** — `go build ./...` and `go test ./...` pass on Windows;
  the `boardtest` git-backed integration/concurrency/bench suites still run
  against a local bare repo (no network) with `BOARD_SKIP_*` honored.

## Q&A log

- **Q:** Is the Python/millpy side or plugin packaging in scope? **A:** No —
  pure Go `mhgo` repo; plugin packaging is design-only, on the roadmap.
- **Q:** Do `--wiki-path`/`MHGO_WIKI_PATH` survive as an override layer? **A:**
  No. Env is never an implicit precedence layer; config is the mechanism, with
  opt-in `$env:NAME` interpolation expanded from the (session-local) process
  environment.
- **Q:** How is `_mhgo/` located when cwd ≠ repo root? **A:** It is always
  exactly `<cwd>/_mhgo/`; absent → error ("run `mhgo init`"). cwd is the unit of
  activation; mhgo need not be at a git root.
- **Q:** How deep is the rename? **A:** Full (env knobs, commit message,
  identifiers, docs); keep on-disk `tasks.json`/lock filenames.
- **Q:** YAML dependency OK? **A:** Yes — `gopkg.in/yaml.v3`.
- **Q:** What does `init` do for now? **A:** Config scaffold only (create
  `_mhgo/` + commented `board.yaml`, maintain `.gitignore` block, JSON summary);
  it grows into a mill-setup-equivalent later — out of scope now.
- **Q:** Unset `$env:` reference? **A:** Hard error.
- **Q:** Default board dir name? **A:** `../_board` (underscore = system folder).
- **Q:** Keep the `.mhgo/` gitignored override layer? **A:** Yes, deep-merge
  over `_mhgo/board.yaml`.
- **Q:** `_mhgo/` present but `board.yaml` absent? **A:** Built-in defaults
  apply (no error).
- **Q:** Board dir missing at write time? **A:** `MkdirAll`; non-git board →
  detached push silently no-ops.
- **Q:** How does the detached `sync` child get the path? **A:** Internal
  `--board-path` flag with the resolved absolute path; child does not
  re-resolve.
- **Q:** Loader keyed per module? **A:** Yes — `_mhgo/<module>.yaml` (today only
  `board`).
- **Q:** `init` seed style? **A:** All-commented defaults, so code defaults
  always apply until the user opts in.
- **Q:** `.gitignore` handling? **A:** `# === mhgo-managed ===` marker block.
- **Q:** Where do future-task notes go? **A:** A new `docs/roadmap.md`
  maintained by later tasks — not in `discussion.md`, and no task entries
  created now.
- **Q:** What is mhgo's domain vs mill/millpy? **A:** mhgo = one binary
  reimplementing all of Millhouse; modules are submodules (`mhgo board ...`);
  `board` is today's only module. This justifies module-keyed config and a
  top-level `init`.
- **Q:** (review r2 GAP) Detached `sync` child vs the cwd-required `_mhgo/`
  check? **A:** `--board-path` fully bypasses `LoadConfig` and the `_mhgo/`
  existence check; the path is injected, not resolved, so the child never
  spuriously errors from a cwd without `_mhgo/`. Output names are not passed —
  `sync` never renders.
- **Q:** (review r2 NOTE) `boardtest` CLI benchmarks under the new cwd model?
  **A:** They drive `RunCLI` via `--wiki-path`; with the flag gone they must be
  re-architected (temp cwd with `_mhgo/board.yaml`, or the facade constructor),
  and CLI-bench numbers now include config-load overhead.
- **Q:** (review r2 NOTE) Where does the proposal prefix apply? **A:** Four
  sites — `renderProposals` filenames, the `removeOrphanProposals` glob, and the
  in-content links in `renderHome` (line 97) and `renderSidebar` (line 146).
