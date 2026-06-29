# Overview: Loomyard

Loomyard is a Go toolkit of one-shot CLI modules. Each invocation starts a process,
runs one command, writes JSON to stdout, and exits — there is no daemon and no
shared memory. State lives on disk per module and is coordinated with file locks,
so concurrent `lyx` processes on a machine cooperate through the filesystem. The
first module, **board** (a task tracker), is implemented; **warp** (the host↔weft
topology owner) is implemented; **muxpoc**, a proof-of-concept orchestrator, is
shipped; and the planned clean `internal/mux` remains design (see
[roadmap.md](roadmap.md)).

In the long term, Loomyard is intended to **replace mill/millhouse (Python)** entirely.
We get there by building these modules as self-contained toolkits first;
orchestration comes last. See [Principles](#principles).

Module path: `github.com/Knatte18/loomyard`

## Naming: `lyx` (binary) · `loom` (orchestrator module) · `ly` (skills)

Three distinct names for three layers, deliberately non-overlapping to avoid the millhouse
`mill`/`millpy` collision (where one name meant two different things):

- **`lyx`** — the binary/CLI, **L**oom**Y**ard e**X**ecutable — one binary with a namespaced
  subcommand tree (`lyx board`, `lyx weft`, `lyx loom`, …). The analog of millhouse's `millpy`
  backend.
- **`loom`** — the orchestrator *module* (`lyx loom run`, `lyx loom status`): the domain that
  drives the phased run, a module like `board` or `weft`. See [modules/loom.md](modules/loom.md).
- **`ly`** — the skill / orchestration plugin (the analog of `mill`); skills are `/ly-*`.

**Never name skills `lyx-*` or `loom-*`** — skills are `ly-*`, distinct from both the binary
(`lyx`) and every module (`loom`, `review`, …), so no name is shared between a skill and a
script/module (the ambiguity that forced the millhouse `mill` → `millpy` rename). Internal Go
packages (`internal/board`, `internal/weft`, …) keep their own names and are not user-facing.

Convenience alias: **`lyx run` → `lyx loom run`** (the everyday autonomous call).

## Principles

1. **Toolkit-first.** Build small, composable primitives (board, warp, mux)
   before any orchestrator that ties them together. mill's Agent Dispatch
   orchestrates for now.
2. **Self-contained modules, deep internal tests.** All of a module's domain logic
   and its test suite live in its own package. What modules share is a thin layer of
   infrastructure plumbing — see [shared-libs/README.md](shared-libs/README.md).
3. **One-shot, daemonless, file-coordinated.** A command does its work, writes JSON,
   exits. Processes cooperate through files + locks, not a server. (The future mux
   daemon is the one deliberate exception, for crash recovery psmux can't self-detect.)
4. **cwd-authoritative; cwd ≠ git-repo-path.** Config and state resolve from the
   current working directory, which need *not* equal the git-repo root. Designed in
   from the start — this was repeatedly forgotten in millpy and caused constant
   trouble.
5. **Full control, incremental milestones.** Land one milestone at a time;
   refactors are behaviour-preserving with the existing test suite as guardrail.
6. **Correctness by tool-design, not by recall.** A `lyx` command should make the *correct* path
   the path of least resistance and make drift *detectable* (`status` / a future `doctor`), rather
   than relying on an agent or operator remembering a rule. No on-disk operation is truly
   un-bypassable when a shell is available, so the achievable bar is "right path is easiest +
   mistakes are detectable," **not** "wrong path impossible." Hard blocks (hooks, permission rules)
   are brittle and out of scope. Example: `lyx weft` owns the overlay's git so raw `git -C` is
   never *needed* (it would be strictly more work), and `lyx weft status` flags drift — but it is a
   friction asymmetry, not a wall.

## Path Invariants

**All worktree and Hub geometry resolves through `internal/paths`.**

The `internal/paths` package is the sole owner of cwd and worktree-root geometry math. It
exposes two entry points:

- `Getwd()` — the only permitted call to `os.Getwd` outside `cmd/lyx/main.go`.
- `Resolve(cwd)` → `Layout` — one-stop geometry: cwd, repo root (from `git rev-parse
  --show-toplevel`), Hub, relative path, and Prime worktree.

The `Layout` type provides geometry methods: `LyxDir()`, `WorktreePath(slug)`,
`PortalsDir()`, `PortalLink(slug)`, `PortalTarget(slug)`, `LaunchersDir()`, `LauncherDir(slug)`, `MenuLauncherPath()`, `LauncherSpawnRel(slug)`, `MenuLauncherRel()`, `PrimeName()`, `WeftRepoRoot()`, `WeftWorktreePath(slug)`, `WeftWorktree()`, `WeftLyxDir()`, `WeftLyxDirFor(slug)`, `WeftCodeguideDir()`, `HostLyxLink(slug)`, `HostLyxLinkHere()`, `HostJunctions(slug)`.

**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside `internal/paths`
and `cmd/lyx/main.go`. The ban is enforced at `go test` / CI time by
`internal/paths/enforcement_test.go`, which walks the entire source tree and fails the build
if either literal token is found in any non-test `.go` file outside the allowlist.

See [CONSTRAINTS.md](../CONSTRAINTS.md) for details.

## Documentation lifecycle

Mechanical per-module design docs (`docs/modules/<module>.md`) are deleted when their module lands; the implementation and tests become the source of truth. The durable documentation is this `overview.md` (principles, naming, the module and shared-lib map, the weft contract, and this lifecycle convention) and the not-yet-landed portion of `roadmap.md`. A module's purpose and key design rationale live in its Go package header comment, next to the code it documents.

## Weft overlay model

lyx organizes overlay artifacts (configuration, task state, codeguide docs, and the board) into a **weft repo** — a companion git repository that stays separate from the host repo, keeping the host pristine.

### Topology

```
<hub>/                              (top-level Hub, NOT a git repo)
  ├── <prime>/                      (host worktree, main branch; git repo root)
  ├── <prime>-weft/                 (weft Prime worktree; git repo root)
  ├── <slug>/                       (additional host worktree; git repo root)
  ├── <slug>-weft/                  (weft worktree for <slug>; git repo root)
  └── _board/                       (board repo; the task store)
```

### Git ownership

The **host repo** is the project's source of truth, maintained by developers. All lyx-specific artifacts live in the **weft repo**, a separate git repository that lyx controls. This separation keeps host commits focused on project code and delegates lyx infrastructure to the weft.

### Artifacts location

| Artifact | Location | Repo | Purpose |
|----------|----------|------|---------|
| `_lyx/config/` | Weft worktree | Weft | Live YAML configuration files for all modules (board, warp, weft); reconciled via `lyx update` |
| `.env` | Weft worktree | Weft | Git-ignored per-machine environment variable overrides (KEY=value format) |
| `_codeguide/` | Weft worktree | Weft | Codeguide documentation (task 008) |
| `_board/` | Hub | Board | Task board at a **configured** board-repo URL — `lyx board` accepts any URL; `ly-git-clone` defaults it to the weft repo's GitHub wiki (`<weft>.wiki.git`) |
| Host source | Host worktree | Host | Project source code |

### Durable vs ephemeral state (`_lyx/` vs `.lyx/`)

Two state roots with opposite lifecycles:

- **`_lyx/`** — **durable, synced, portable.** Lives in the weft repo (git-synced), so it
  survives a machine and transfers to another. Config, codeguide, the board, and loom's
  orchestration **status** (current phase, review round, verdict history) go here — loom
  resume works across machines *because* its status is weft-synced.
- **`.lyx/`** — **ephemeral, local, machine-bound.** Untracked (listed in
  `.git/info/exclude`, never `.gitignore`), changing constantly while a run is live. The live
  psmux runtime state — [`mux`](modules/mux.md)'s `.lyx/mux.json` (server PID + the
  [strand](modules/mux.md#the-strand-model) table: each managed process, its session, parent, and
  display spec) — goes here, because a pane ID or a psmux server PID is meaningless on another
  machine. It is rebuilt by reconciling against live psmux on startup, never synced.

The test: **would this state mean anything on a different machine?** Orchestration progress
yes → `_lyx/`. A pane handle no → `.lyx/`.

### Junction model

Each host worktree has a sibling weft worktree. Host worktrees use **junctions** (Windows) or symlinks to route writes into the sibling weft worktree:
- `<host>/_lyx` → `<hub>/<slug>-weft/_lyx` (config junction)
- `<host>/_codeguide` → `<hub>/<slug>-weft/_codeguide` (codeguide junction, task 008)

Junctions are listed in `.git/info/exclude` per worktree and are never committed to `.gitignore`. From the CLI's perspective, reads and writes happen transparently — code that writes to `_lyx/config/board.yaml` writes through the junction into the weft repo without awareness of the indirection.

### Branch model

Weft branches mirror host-repo branching. When a new weft worktree is spawned, the new weft branch forks from the weft branch whose name equals the host worktree's current branch at spawn time, preserving a shared merge-base for future squash-merge-back operations (`_codeguide` — see below). This guarantees that subtasks (spawned from non-main branches) inherit the correct fork point: branch isolation is **not** orphan-based (each isolated from history) but **merge-base-preserving** (each on its parent's timeline). `_lyx` is isolated by pathspec (junctions route it into weft; host `.git/info/exclude` hides it) rather than by orphan topology, so no merge-back state is lost.

### Weft suffix convention

The weft worktree for any host worktree is deterministic:
- Host: `<hub>/<slug>/` → Weft: `<hub>/<slug>-weft/`
- Host: `<prime>/` → Weft: `<prime>-weft/` (prime is the name of the main worktree)

The `-weft` suffix is fixed and non-configurable. Weft paths are computed on demand from geometry and do not require a registry.

### Status

- **Go implementation** (paths geometry, paired spawn, `lyx weft` command): ✅ task 006 complete. The weft engine (paths geometry, paired `lyx warp add` spawn, and `lyx weft status|commit|push|pull|sync`) now exists in Go. Paired `lyx warp add` hard-requires a weft repo built by the downstream hub-creator.
- **`lyx config` command**: ✅ task 008 partial complete. The interactive menu interface (`lyx config` and `lyx config <module>`) shipped. `_codeguide` junction activation and codeguide config schema remain deferred.
- **Portals**: on hold pending `_codeguide` junction activation. Portals (symlink-based overlay sharing) remain unimplemented; the weft junction model is the live mechanism.

```
github.com/Knatte18/loomyard/
├── cmd/lyx/
│   └── main.go                   entrypoint: routes the <module> argument to a module
├── internal/board/               the board module
├── internal/warp/                the warp module (host↔weft topology owner)
├── internal/weft/                the weft module
├── internal/ide/                 the ide module
├── internal/muxpoc/              the muxpoc POC module
├── internal/ghissues/            the ghissues module (file bugs to GitHub via gh)
├── internal/paths/               geometry resolver (the sole owner of cwd/root math)
├── internal/configengine/        shared config resolution
├── internal/gitexec/             shared git operations
├── internal/lock/                shared file locking
└── internal/output/              shared JSON output
```

`cmd/lyx` is `package main`; everything else is in `internal/`. `main` is the
only thing that imports a module.

## Module dispatch

`cmd/lyx/main.go` assembles all modules into a single cobra root via `newRoot()`.
Each module contributes a `Command() *cobra.Command` that is passed to
`root.AddCommand(...)`, so every module and subcommand is discoverable via
`lyx --help` without any central dispatch table. Adding a module is three steps:
import the package, add `<module>.Command()` to `root.AddCommand(...)` in
`newRoot()`, and append the module name to `root.Long`.

`run(args, out)` is the testable seam: it builds a fresh root, merges stdout and
stderr into `out`, and calls `root.ExecuteContext`, returning the process exit code
without spawning a binary or trapping `os.Exit`. Each module also exposes
`RunCLI(out io.Writer, args []string) int` — exactly
`return clihelp.Execute(Command(), out, args)` — as an in-process test seam that
drives a module in isolation without involving the cobra root.

All commands print JSON: `{"ok":true, ...}` on success,
`{"ok":false,"error":"..."}` on failure (exit code 1).

## Modules

User-facing modules each get one `lyx <module>` namespace:

- **init** — scaffolds the `_lyx/` directory structure and creates all module config files via reconciliation against templates (`internal/initcli`). Idempotent: does not clobber existing config files. ✅ Implemented.
- **update** — reconciles all module config files against their live templates, reporting added/removed keys and optionally writing changes (`internal/update`). Dry-run by default; `--apply` writes atomically. ✅ Implemented.
- **board** — the task-tracker board (`internal/board`). ✅ Implemented.
- **config** — interactive menu for viewing and editing module configs. ✅ Implemented.
- **weft** — owns all git into the paired weft repo (`lyx weft status|commit|push|pull|sync`). ✅ Implemented.
- **warp** — **host↔weft-coordinated git topology**: clone (hub-creator), dual-worktree add/remove, coordinated checkout (switches host+weft together + re-points junctions), reconcile, status, prune, cleanup. The single owner of the mirror invariant — consolidates the former `worktree` / `git-clone` modules and `internal/git`; its CLI surface is `lyx warp clone|add|list|remove|checkout|status|reconcile|prune|cleanup`. ✅ Implemented.
- **ide** — one-shot VS Code launcher with interactive menu. ✅ Implemented.
- **muxpoc** — shipped proof-of-concept psmux orchestrator proving the risky parts of the
  planned mux module. ✅ Implemented.
- **ghissues** — file bugs and enhancements against `Knatte18/loomyard` via the `gh` CLI
  (`lyx ghissues create <title>`). Target repo is hardcoded; supports `--body` (or `-` for
  stdin) and `--label`; defaults to `bug`. Callable from any sandbox agent context with no
  config. ✅ Implemented.
- **mux** — **the window to the world**: psmux overlay + **strand** bookkeeping + render. Hosts
  every managed process as a strand, arranges them, persists to `.lyx/mux.json` (`lyx mux`). 🚧
  Design — not built. See [modules/mux.md](modules/mux.md).
- **loom** — phased orchestrator: drives Setup → Discussion → Plan → Builder → Finalize, each
  gated by a review (`lyx loom run`, alias `lyx run`). 🚧 Design — not built. See
  [modules/loom.md](modules/loom.md).
- **review** — generic profile-driven gate engine (handler+fixer, optional cluster, stuck judge);
  independent of `loom` but used by it between every phase, and standalone (`lyx review`). 🚧
  Design — not built. See [modules/review.md](modules/review.md).

One further design module is internal: **shuttle** (run one LLM agent via a swappable engine;
[modules/shuttle.md](modules/shuttle.md)). The cross-OS spawn primitive **proc** is likewise
internal — the base of the stack. The [module map](modules/README.md) explains how proc / mux /
shuttle fit together. (Earlier drafts split mux into separate `shed`/`glance` modules; both folded
back into mux — see [modules/mux.md](modules/mux.md#naming).)

**init** is not a module but a cross-cutting setup command (`lyx init`) that
scaffolds the shared `_lyx/` config dir for every module.

The user-facing modules sit on a thin layer of shared infrastructure
(`internal/configengine`, `internal/gitexec`, `internal/lock`, `internal/output`, `internal/paths`, `internal/state`) — defined in
[shared-libs/README.md](shared-libs/README.md).

## Execution stack (orchestration layers)

The orchestrator is not one module but a **layered stack**, each layer knowing only the one
below it. It exists in this shape for one reason: agents must run as **interactive psmux
sessions, never headless `claude -p`** (an economic constraint — see
[modules/shuttle.md](modules/shuttle.md#interactive-never-headless--the-economic-constraint)), so
spawning an agent is not a plain `exec` but "place a pane, launch a provider in it, drive it,
detect completion." Full side-by-side disambiguation: the [module map](modules/README.md).

```
internal/proc     spawn any OS process (windowless / detached), cross-OS      [OS primitive]
internal/mux      the window to the world — overlay + strand bookkeeping +     [builds on proc]
                  render; hosts every managed process as a strand, arranges
                  them, persists to .lyx/mux.json
internal/shuttle  run ONE LLM agent in a strand via a swappable engine over    [builds on mux]
                  the file contract; Stop-hook completion
review            generic gate engine: handler/fixer + cluster + stuck judge   [builds on shuttle]
loom              phase machine: drive each phase through a review gate         [builds on review]
```

The whole stack runs **headless** (auto mode): strands exist (the interactive-session
requirement), agents run, output files are read, nobody need watch.

- **mux is three things** — an **overlay** over psmux, **strand bookkeeping** (a strand = one
  tracked process: a metadata record with a name, worktree slug, parent, and a *generic* display
  spec), and a **render** sub-package (`layout = rules(strands)`). Callers hand mux `{cmd, name,
  display}` where `display` is generic (anchor / height / focus) — never a domain `type`, so mux
  never learns what a "phase" or "cluster" is. Earlier drafts split the model and view into separate
  `shed`/`glance` modules; with one terminal per worktree they fold cleanly into mux.
- **provider-invariant** — `shuttle` runs Claude today through an **engine**; the verdict/output
  contract is provider-invariant, so a different model can be swapped in without touching the
  review machinery. Non-Claude is not a current priority.
- **review is independent of loom** — it is a standalone gate engine (`lyx review`); loom just
  uses it heavily (a review between every phase). review builds on `shuttle`, not on `loom`.
- **the bootstrap** — `lyx loom run` (alias `lyx run`) brings up the worktree's psmux session, adds
  the `lyx loom status` strand (a 1-line top pane), spawns the loom driver **detached** (via `proc`,
  no TTY), and attaches the terminal to the session. loom runs in the background; the mux view takes
  the foreground. A `.lyx/lyxrun.cmd` launcher makes it one click.
- Only `mux`, `loom`, and `review` get a user-facing `lyx <module>` CLI; `proc` and `shuttle` are
  internal libraries. See the [module map](modules/README.md).

## Tests

Per-file unit tests sit next to the source they test (`store.go` ↔
`store_test.go`). The cross-cutting suites — benchmarks, concurrency stress, and
git-backed integration — live in the black-box `internal/board/boardtest` package.

## Sandbox Hub

The **sandbox Hub** is a dedicated bench for manual testing of lyx's core workflows — its purpose is dogfooding lyx against itself. It lives on disk at `C:\Code\lyx-test-HUB` and exercises the real deployed `lyx` binary: the command surface, JSON output, and topology wiring users encounter. Build it via `sandbox.cmd` once `lyx` is deployed and the GitHub weft wiki is initialized. See [sandbox-hub.md](sandbox-hub.md) for details.

## Other docs

- [modules/README.md](modules/README.md) — **the module map**: index of every module doc + how the layers stack (design).
- [modules/loom.md](modules/loom.md) — the phased orchestrator (`lyx loom` + `lyx review`); design.
- [modules/mux.md](modules/mux.md) — the window to the world: psmux overlay + strand bookkeeping + render (design).
- [modules/shuttle.md](modules/shuttle.md) — run one LLM agent via a swappable engine over the file contract (design).
- [modules/review.md](modules/review.md) — the generic gate engine (handler/fixer + cluster + stuck judge); design.
- [benchmarks/](benchmarks/board-performance.md) — board performance, tracked across revisions.
- [shared-libs/](shared-libs/README.md) — the shared infrastructure plumbing.
- [research/](research/) — design exploration (mux research logs).
- [reference/psmux_scripting.md](reference/psmux_scripting.md) — upstream psmux command reference (vendored).
- [roadmap.md](roadmap.md) — numbered milestones and long-term direction.
- [sandbox-hub.md](sandbox-hub.md) — the sandbox Hub: a dedicated bench for manual (dogfooding) testing.
