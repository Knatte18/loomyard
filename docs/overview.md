# Overview: Loomyard

Loomyard is a Go toolkit of one-shot CLI modules. Each invocation starts a process,
runs one command, writes JSON to stdout, and exits — there is no daemon and no
shared memory. State lives on disk per module and is coordinated with file locks,
so concurrent `lyx` processes on a machine cooperate through the filesystem. The
first module, **board** (a task tracker), is implemented; **warp** (the host↔weft
topology owner) is implemented; and **mux**, the clean tmux overlay built on what its
now-deleted proof-of-concept (`muxpoc`) proved, is implemented (see [roadmap.md](roadmap.md)).

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
(`lyx`) and every module (`loom`, `perch`, …), so no name is shared between a skill and a
script/module (the ambiguity that forced the millhouse `mill` → `millpy` rename). Internal Go feature packages follow the `<module>cli` / `<module>engine` split
(e.g. `internal/boardcli` + `internal/boardengine`, `internal/warpcli` + `internal/warpengine`) —
see the Package naming rule in [CONSTRAINTS.md](../CONSTRAINTS.md#package-naming).

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
   daemon is the one deliberate exception, for crash recovery tmux can't self-detect.)
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
7. **Go where it can be; LLM only for judgment.** Everything deterministic — verbs,
   control-flow, parsing, distillation, geometry, git — is Go. An LLM is reserved for the
   irreducible judgment a program cannot do: review verdicts, triage, batch implementation,
   an orchestrator's recovery decisions. The seam is consistent everywhere: **fat Go verbs**
   (`lyx <module> <verb>`) are the callable surface; an LLM session *drives* them and consumes
   **Go-distilled digests, never raw prose**; and any **skill is a thin human wrapper** over
   those verbs, never where logic lives. Consequences already on the ground: `perch` is a Go
   loop with an LLM judge; triage is LLM judgment with Go doing the board writes; `builder` is
   an LLM orchestrator over Go verbs with an embedded, co-versioned prompt.

## Hub Geometry Invariants

**All worktree and Hub geometry resolves through `internal/hubgeometry`.**

The `internal/hubgeometry` package is the sole owner of cwd and worktree-root geometry math. It
exposes two entry points:

- `Getwd()` — the only permitted call to `os.Getwd` outside `cmd/lyx/main.go`.
- `Resolve(cwd)` → `Layout` — one-stop geometry: cwd, repo root (from `git rev-parse
  --show-toplevel`), Hub, relative path, and Prime worktree.

The `Layout` type provides geometry methods: `LyxDir()`, `WorktreePath(slug)`,
`PortalsDir()`, `PortalLink(slug)`, `PortalTarget(slug)`, `LaunchersDir()`, `LauncherDir(slug)`, `MenuLauncherPath()`, `LauncherSpawnRel(slug)`, `MenuLauncherRel()`, `PrimeName()`, `WeftRepoRoot()`, `WeftWorktreePath(slug)`, `WeftWorktree()`, `WeftLyxDir()`, `WeftLyxDirFor(slug)`, `WeftRaddleDir()`, `HostLyxLink(slug)`, `HostLyxLinkHere()`, `HostJunctions(slug)`.

**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside `internal/hubgeometry`
and `cmd/lyx/main.go`. The ban is enforced at `go test` / CI time by
`internal/hubgeometry/enforcement_test.go`, which walks the entire source tree and fails the build
if either literal token is found in any non-test `.go` file outside the allowlist.

See [CONSTRAINTS.md](../CONSTRAINTS.md) for details.

## Documentation lifecycle

Mechanical per-module design docs (`docs/modules/<module>.md`) are deleted when their module lands; the implementation and tests become the source of truth. The durable documentation is this `overview.md` (principles, naming, the module and shared-lib map, the weft contract, and this lifecycle convention) and the not-yet-landed portion of `roadmap.md`. A module's purpose and key design rationale live in its Go package header comment, next to the code it documents.

## Weft overlay model

lyx organizes overlay artifacts (configuration, task state, raddle docs, and the board) into a **weft repo** — a companion git repository that stays separate from the host repo, keeping the host pristine.

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
| `_lyx/config/` | Weft worktree | Weft | Live YAML configuration files for all modules (board, warp, weft); reconciled via `lyx config reconcile` |
| `.env` | Weft worktree | Weft | Git-ignored per-machine environment variable overrides (KEY=value format) |
| `_raddle/` | Weft worktree | Weft | Raddle documentation (the raddle nav-doc overlay) |
| `_board/` | Hub | Board | Task board at a **configured** board-repo URL — `lyx board` accepts any URL; `ly-git-clone` defaults it to the weft repo's GitHub wiki (`<weft>.wiki.git`) |
| Host source | Host worktree | Host | Project source code |

### Durable vs ephemeral state (`_lyx/` vs `.lyx/`)

Two state roots with opposite lifecycles:

- **`_lyx/`** — **durable, synced, portable.** Lives in the weft repo (git-synced), so it
  survives a machine and transfers to another. Config, raddle, the board, and loom's
  orchestration **status** (current phase, review round, verdict history) go here — loom
  resume works across machines *because* its status is weft-synced.
- **`.lyx/`** — **ephemeral, local, machine-bound.** Untracked (listed in
  `.git/info/exclude`, never `.gitignore`), changing constantly while a run is live. The live
  tmux runtime state — `mux`'s (see the `internal/muxengine` package documentation) `.lyx/mux.json`
  (the socket/session names + the strand table: each managed process, its session, parent,
  ephemeral pane id, and display spec) — goes here, because a pane ID or the tmux socket is
  meaningless on another machine. It is rebuilt by reconciling against live tmux on startup, never
  synced.

The test: **would this state mean anything on a different machine?** Orchestration progress
yes → `_lyx/`. A pane handle no → `.lyx/`.

### Junction model

Each host worktree has a sibling weft worktree. Host worktrees use **junctions** (Windows) or symlinks to route writes into the sibling weft worktree:
- `<host>/_lyx` → `<hub>/<slug>-weft/_lyx` (config junction)
- `<host>/_raddle` → `<hub>/<slug>-weft/_raddle` (raddle junction)

Junctions are listed in `.git/info/exclude` per worktree and are never committed to `.gitignore`. From the CLI's perspective, reads and writes happen transparently — code that writes to `_lyx/config/board.yaml` writes through the junction into the weft repo without awareness of the indirection.

### Branch model

Weft branches mirror host-repo branching. When a new weft worktree is spawned, the new weft branch forks from the weft branch whose name equals the host worktree's current branch at spawn time, preserving a shared merge-base for future squash-merge-back operations (`_raddle` — see below). This guarantees that subtasks (spawned from non-main branches) inherit the correct fork point: branch isolation is **not** orphan-based (each isolated from history) but **merge-base-preserving** (each on its parent's timeline). `_lyx` is isolated by pathspec (junctions route it into weft; host `.git/info/exclude` hides it) rather than by orphan topology, so no merge-back state is lost.

### Weft suffix convention

The weft worktree for any host worktree is deterministic:
- Host: `<hub>/<slug>/` → Weft: `<hub>/<slug>-weft/`
- Host: `<prime>/` → Weft: `<prime>-weft/` (prime is the name of the main worktree)

The `-weft` suffix is fixed and non-configurable. Weft paths are computed on demand from geometry and do not require a registry.

### Status

- **Go implementation** (paths geometry, paired spawn, `lyx weft` command): ✅ task 006 complete. The weft engine (paths geometry, paired `lyx warp add` spawn, and `lyx weft status|commit|push|pull|sync`) now exists in Go. Paired `lyx warp add` hard-requires a weft repo built by the downstream hub-creator.
- **`lyx config` command**: ✅ task 008 complete. The interactive menu (`lyx config`, `lyx config <module>`) and `lyx config reconcile` shipped. (`_raddle` junction activation and a raddle config schema are **raddle** nav-doc work, not part of this task — they were only historically mis-bundled here.)
- **Portals**: unimplemented; the weft junction model is the live mechanism. (Symlink-based overlay sharing is not on the critical path.)

```
github.com/Knatte18/loomyard/
├── cmd/lyx/
│   └── main.go                   entrypoint: routes the <module> argument to a module
├── internal/boardcli/            the board CLI command
├── internal/boardengine/         the board domain kernel
├── internal/warpcli/             the warp CLI command (host↔weft topology owner)
├── internal/warpengine/          the warp domain kernel
├── internal/weftcli/             the weft CLI command
├── internal/weftengine/          the weft domain kernel
├── internal/idecli/              the ide CLI command
├── internal/ideengine/           the ide domain kernel
├── internal/muxcli/              the mux CLI command
├── internal/muxengine/           the mux domain kernel (overlay + strand bookkeeping)
├── internal/muxengine/render/    pure display-vocabulary leaf (layout = Rules(strands))
├── internal/ghissuescli/         the ghissues CLI command
├── internal/ghissuesengine/      the ghissues domain kernel
├── internal/selfreportcli/       the selfreport CLI command
├── internal/selfreportengine/    the selfreport domain kernel
├── internal/hubgeometry/         geometry resolver (the sole owner of cwd/root math)
├── internal/configengine/        shared config resolution
├── internal/gitexec/             shared git operations
├── internal/lock/                shared file locking
├── internal/output/              shared JSON output
├── internal/modelspec/           model-spec parser + models.yaml registry leaf
└── internal/shell/               provider-invariant pane-shell mechanics leaf (pwsh + posix)
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

- **init** — scaffolds the `_lyx/` directory structure and creates all module config files via reconciliation against templates (`internal/initcli` + `internal/initengine`). Idempotent: does not clobber existing config files. `lyx init --undo` reverses that scaffolding (junction, weft-side content, `.gitignore` block, `.git/info/exclude` entry) for test/sandbox cleanup. ✅ Implemented.
- **board** — the task-tracker board (`internal/boardcli` + `internal/boardengine`). ✅ Implemented.
- **config** — interactive menu for viewing and editing module configs; `lyx config reconcile` reconciles all module config files against their live templates (dry-run by default, `--apply` writes atomically) except seed-only modules (today: `models`), which are materialized once when absent and never rewritten again since the file is operator-owned; `lyx config <module> --set key=value` (repeatable) writes one or more config values directly with no editor invocation, for scripts/agents that need a non-interactive path. ✅ Implemented.
- **weft** — owns all git into the paired weft repo (`lyx weft status|commit|push|pull|sync`). ✅ Implemented.
- **warp** — **host↔weft-coordinated git topology**: clone (hub-creator), dual-worktree add/remove, coordinated checkout (switches host+weft together + re-points junctions), reconcile, status, prune, cleanup. The single owner of the mirror invariant — consolidates the former `worktree` / `git-clone` modules and `internal/git`; its CLI surface is `lyx warp clone|add|list|remove|checkout|status|reconcile|prune|cleanup`. ✅ Implemented.
- **ide** — one-shot VS Code launcher with interactive menu. ✅ Implemented.
- **selfreport** — file bugs and enhancements against `Knatte18/loomyard` via the `gh` CLI
  (`lyx selfreport create <title>`). Target repo is hardcoded; supports `--body` (or `-` for
  stdin) and `--label`; defaults to `bug`. Callable from any sandbox agent context with no
  config. ✅ Implemented.
- **mux** — **the window to the world**: tmux overlay + **strand** bookkeeping + render
  (`internal/muxcli` + `internal/muxengine` + `internal/muxengine/render`). Hosts every managed
  process as a strand, arranges them, persists to `.lyx/mux.json` (`lyx mux
  up|add|remove|status|attach|resume|down`). Built on what its proof-of-concept, `muxpoc`, proved
  first (layout checksum, bottom-dominant layout, env hygiene, native `--resume`); `muxpoc` has
  since been deleted, its job done. ✅ Implemented. See the `internal/muxengine` package
  documentation.
- **shuttle** — run **one** LLM agent as an interactive tmux strand over the file contract
  (`internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`;
  `lyx shuttle run|interrupt|send`). `Stop`-hook completion is read off an events file and
  classified into four outcomes — `done`/`asking`/`died`/`timeout` — with `asking` as the
  escalation channel back to a human or a higher-capability model; an interactive run also detects
  a live `AskUserQuestion` tool call in real time via a non-denying marker hook, classified as the
  same `asking` outcome instead of waiting for the timeout. `PreToolUse` guardrails deny
  the in-process `Agent` tool always, and `AskUserQuestion` too when the run is autonomous
  (`Interactive: false`, the default). The provider is swappable behind an **engine** seam; Claude
  is the only v1 engine (Gemini etc. later, not a current priority). Per-run `Model` and `Effort`
  knobs (`lyx shuttle run --model`/`--effort`; effort values `low|medium|high|xhigh|max`, empty =
  provider default) are engine-validated, not policed by `Spec.validate`. `Spec.Version` is a
  programmatic engine-validated version pin (claudeengine composes the pinned model id; no CLI
  flag — consumers drive it via the model-spec notation's `version=` param). ✅ Implemented. See
  the `internal/shuttleengine` package documentation.
- **builder** — LLM orchestrator + Go verbs: a long-lived orchestrator session (model
  config-chosen; Sonnet default) drives fat `lyx builder validate|run|spawn-batch|poll|
  status|pause` verbs (`internal/builderengine` + `internal/buildercli`) through a
  pinned plan-format v2 plan, batch by batch, until the plan is built; Go supplies only
  the verbs plus the distillation behind them (digest, chain rollback, pause, outcome
  parsing), never the loop itself. Input contract:
  [plan-format.md](modules/plan-format.md). Branches off `shuttle` directly; does not
  need `perch`. Ends at batches-built — the terminal holistic review is the separate
  Builder-review gate (`perch`), driven by `loom` or the operator. ✅ Implemented. See
  [modules/builder-contract.md](modules/builder-contract.md).
- **loom** — phased orchestrator: drives Setup → Discussion → Plan → Builder → Finalize, each
  gated by a perch review (`lyx loom run`, alias `lyx run`). 🚧 Design — not built. See
  [modules/loom.md](modules/loom.md).
- **perch** — generic profile-driven gate loop: runs `burler` rounds on one artifact until
  `APPROVED`/`STUCK` (milestone-capped `round_caps` ladder + a holistic progress judge), plus an
  operational `PAUSED` exit; independent of `loom` but used by it between every phase, and standalone
  (`lyx perch run|pause`). ✅ Implemented. See the `internal/perchengine` package documentation.
- **burler** — one review+fix round: A-review → B-fix, one agent, no self-grading, over the shuttle
  file contract (`internal/burlerengine` + `internal/burlercli`). Profile-driven: `{overlay, source}`
  fix-scope, tool-use, cluster-N rejected with a typed error until mux own-window anchoring lands.
  Strict frontmatter verdict parse; debug CLI `lyx burler run`. ✅ Implemented. See the
  `internal/burlerengine` package documentation.
- **hardener** — **DRAFT / concept.** Behavior-based reviewer that *runs* a live-substrate module
  (needs a sandbox repo) to harden it before merge; on-demand, post-loom, **off the spine**, shares
  only the `burler` round discipline. See [modules/hardener.md](modules/hardener.md).

The cross-OS spawn primitive **proc** is the one remaining internal (non-CLI) layer — the base of
the stack. The [module map](modules/README.md) explains how proc / mux / shuttle fit together.
(Earlier drafts split mux into separate `shed`/`glance` modules; both folded back into mux — see
the `internal/muxengine` package documentation.)

**init** is not a module but a cross-cutting setup command (`lyx init`) that
scaffolds the shared `_lyx/` config dir for every module.

The user-facing modules sit on a thin layer of shared infrastructure
(`internal/configengine`, `internal/gitexec`, `internal/lock`, `internal/output`, `internal/hubgeometry`, `internal/state`, `internal/shell`, `internal/modelspec`) — defined in
[shared-libs/README.md](shared-libs/README.md).

## Execution stack (orchestration layers)

The orchestrator is not one module but a **layered stack**, each layer knowing only the one
below it. It exists in this shape for one reason: agents must run as **interactive tmux
sessions, never headless `claude -p`** (an economic constraint — see the `internal/shuttleengine`
package documentation), so
spawning an agent is not a plain `exec` but "place a pane, launch a provider in it, drive it,
detect completion." Full side-by-side disambiguation: the [module map](modules/README.md).

```
internal/proc     spawn any OS process (windowless / detached), cross-OS      [OS primitive]
internal/mux      the window to the world — overlay + strand bookkeeping +     [builds on proc]  ✅
                  render; hosts every managed process as a strand, arranges
                  them, persists to .lyx/mux.json
internal/shuttle  run ONE LLM agent in a strand via a swappable engine over    [builds on mux]    ✅
                  the file contract; Stop-hook completion
burler            one review+fix round: A-review (+cluster) → B-fix           [builds on shuttle] ✅
perch             run burler rounds on one artifact → APPROVED|STUCK          [builds on burler]  ✅
loom              phase machine: drive each phase through a perch gate         [builds on perch]
```

The whole stack runs **headless** (auto mode): strands exist (the interactive-session
requirement), agents run, output files are read, nobody need watch.

- **mux is three things, and it is built** — an **overlay** over tmux, **strand bookkeeping** (a
  strand = one tracked process: a metadata record with a `guid`, `name`, worktree slug, parent, and
  a *generic* display spec), and a **render** sub-package (`internal/muxengine/render`,
  `layout = Rules(strands, box)`). Callers hand mux `{cmd, name, display}` where `display` is
  generic (anchor / focus / shrinkWhenWaitingOnChild; height is derived, not caller-set) — never a
  domain `type`, so mux never learns what a "phase" or "cluster" is. Earlier drafts split the model
  and view into separate `shed`/`glance` modules; with one terminal per worktree they fold cleanly
  into `internal/muxengine` + `internal/muxengine/render`. See the `internal/muxengine` package
  documentation.
- **provider-invariant** — `shuttle` runs Claude today through an **engine**; the verdict/output
  contract is provider-invariant, so a different model can be swapped in without touching the
  review machinery. Non-Claude is not a current priority.
- **perch is independent of loom** — it is a standalone gate loop (`lyx perch`) over `burler` rounds;
  loom just uses it heavily (a perch review between every phase). perch builds on `burler` → `shuttle`,
  not on `loom`.
- **the bootstrap** — `lyx loom run` (alias `lyx run`) brings up the worktree's tmux session, adds
  the `lyx loom status` strand (a 1-line top pane), spawns the loom driver **detached** (via `proc`,
  no TTY), and attaches the terminal to the session. loom runs in the background; the mux view takes
  the foreground. A `.lyx/lyxrun.cmd` launcher makes it one click.
- `mux`, `shuttle`, `perch`, and `loom` each get a user-facing `lyx <module>` CLI
  (`lyx shuttle run|interrupt|send` lets an operator or another process drive one agent
  standalone, before loom/perch exist); `burler` is composed by `perch` (`lyx burler run` is a
  debug-only wrapper, not a product verb), and
  `proc` alone stays an internal library with no CLI of its own. See the [module map](modules/README.md).

## Tests

Per-file unit tests sit next to the source they test (`store.go` ↔
`store_test.go`). The cross-cutting suites — benchmarks, concurrency stress, and
git-backed integration — live in the black-box `internal/boardengine/boardtest` package.

## Sandbox Hub

The **sandbox Hub** is a dedicated bench for manual testing of lyx's core workflows — its purpose is dogfooding lyx against itself. It lives on disk at `C:\Code\lyx-test-HUB` and exercises the real deployed `lyx` binary: the command surface, JSON output, and topology wiring users encounter. Build it via `sandbox-build.cmd` once `lyx` is deployed and the GitHub weft wiki is initialized (`sandbox-core-suite.cmd` then runs the agent, `sandbox-mux-suite.cmd` runs the mux-specific suite (`SANDBOX-MUX-SUITE.md`, needs live tmux), and `sandbox-fetch.cmd` collects the report from either — the same fetch command for both). See [sandbox-howto.md](sandbox-howto.md) for the step-by-step runbook (deploy → clone Hub → run suite) and [sandbox-hub.md](sandbox-hub.md) for topology and design details.

## Other docs

- [modules/README.md](modules/README.md) — **the module map**: index of every module doc + how the layers stack (design).
- [modules/loom.md](modules/loom.md) — the phased orchestrator (`lyx loom` + `lyx perch`); design.
- [modules/builder-contract.md](modules/builder-contract.md) — the batch-implementation loop (`lyx builder`): verb surface, digest contract, poll classification, chain rollback, pause, outcome contract (as-built; kept as a durable contract doc, not deleted on landing).
- `internal/muxengine` package documentation — the window to the world: tmux overlay + strand bookkeeping + render (as-built; module doc deleted per the documentation lifecycle).
- `internal/shuttleengine` package documentation — run one LLM agent via a swappable engine over the file contract (as-built; module doc deleted per the documentation lifecycle).
- `internal/burlerengine` package documentation — one review+fix round: A-review → B-fix, no self-grading (as-built; module doc deleted per the documentation lifecycle).
- `internal/perchengine` package documentation — the gate loop: run `burler` rounds → `APPROVED`/`STUCK`/`PAUSED` (as-built; module doc deleted per the documentation lifecycle).
- [modules/hardener.md](modules/hardener.md) — **DRAFT/concept**: behavior-based hardening of a live-substrate module (post-loom, off-spine).
- [benchmarks/](benchmarks/board-performance.md) — board performance, tracked across revisions.
- [shared-libs/](shared-libs/README.md) — the shared infrastructure plumbing.
- [research/](research/) — design exploration (mux research logs).
- [reference/tmux_scripting.md](reference/tmux_scripting.md) — tmux command reference (vendored).
- [roadmap.md](roadmap.md) — numbered milestones and long-term direction.
- [sandbox-howto.md](sandbox-howto.md) — operator runbook: deploy `lyx`, build the Hub, run the suite agent (procedure).
- [sandbox-hub.md](sandbox-hub.md) — the sandbox Hub: a dedicated bench for manual (dogfooding) testing.
- [reviews/README.md](reviews/README.md) — the **serial review+fix loop**: a reusable method for hardening a live-substrate module before merge (orchestrator-driven, model-rotating, clean-room self-fixing rounds + independent verification). The hand-executed prototype of the `perch` (see the `internal/perchengine` package documentation) + `burler` (see the `internal/burlerengine` package documentation) round loop (and the origin of the [`hardener`](modules/hardener.md) concept); ships two paste-ready prompts — an [orchestrator prompt](reviews/orchestrator-prompt.md) (drives the loop + verifies) and a [round-agent prompt template](reviews/review-prompt-template.md) (the reviewer-fixer), to instantiate per module.
