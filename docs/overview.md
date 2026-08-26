# Overview: Loomyard

Loomyard is a Go toolkit of one-shot CLI modules.
Each invocation starts a process, runs one command, writes JSON to stdout, and exits — there is no daemon and no shared memory.
State lives on disk per module and is coordinated with file locks, so concurrent `lyx` processes on a machine cooperate through the filesystem.
The first module, **board** (a task tracker), is implemented;
**fabric** (the warp↔weft git-coordination module) is implemented;
and **reed**, the clean tmux overlay built on what its now-deleted proof-of-concept (`muxpoc`) proved, is implemented (see [manifest/roadmap.md](../manifest/roadmap.md)).

In the long term, Loomyard is intended to **replace mill/millhouse (Python)** entirely.
We get there by building these modules as self-contained toolkits first;
orchestration comes last.
See [Principles](#principles).

Module path: `github.com/Knatte18/loomyard`

## Naming: `lyx` (binary) · `loom` (orchestrator module) · `ly` (skills)

Three distinct names for three layers, deliberately non-overlapping to avoid the millhouse `mill`/`millpy` collision (where one name meant two different things):

- **`lyx`** — the binary/CLI, **L**oom**Y**ard e**X**ecutable — one binary with a namespaced subcommand tree (`lyx board`, `lyx fabric`, `lyx loom`, …).
  The analog of millhouse's `millpy` backend.
- **`loom`** — the orchestrator *module* (`lyx loom run`, `lyx loom status`): the domain that drives the phased run, a module like `board` or `fabric`.
  See [manifest/designs/loom.md](../manifest/designs/loom.md).
- **`ly`** — the skill / orchestration plugin (the analog of `mill`);
  skills are `/ly-*`.

**Never name skills `lyx-*` or `loom-*`** — skills are `ly-*`, distinct from both the binary (`lyx`) and every module (`loom`, `burler`, …), so no name is shared between a skill and a script/module (the ambiguity that forced the millhouse `mill` → `millpy` rename).
Internal Go feature packages follow the `<module>cli` / `<module>engine` split (e.g. `internal/boardcli` + `internal/boardengine`, `internal/fabriccli` + `internal/fabricengine`) — see the Package naming rule in [CONSTRAINTS.md](../CONSTRAINTS.md#package-naming).

Convenience alias: **`lyx run` → `lyx loom run`** (the everyday autonomous call).

## Principles

1. **Toolkit-first.**
   Build small, composable primitives (board, fabric, reed) before any orchestrator that ties them together. mill's Agent Dispatch orchestrates for now.
2. **Self-contained modules, deep internal tests.**
   All of a module's domain logic and its test suite live in its own package.
   What modules share is a thin layer of infrastructure plumbing — see [shared-libs/README.md](shared-libs/README.md).
3. **One-shot, daemonless, file-coordinated.**
   A command does its work, writes JSON, exits.
   Processes cooperate through files + locks, not a server. (The future reed daemon is the one deliberate exception, for crash recovery tmux can't self-detect.)
4. **cwd-authoritative;
   cwd ≠ git-repo-path.**
   Config and state resolve from the current working directory, which need *not* equal the git-repo root.
   Designed in from the start — this was repeatedly forgotten in millpy and caused constant trouble.
5. **Full control, incremental milestones.**
   Land one milestone at a time;
   refactors are behaviour-preserving with the existing test suite as guardrail.
6. **Correctness by tool-design, not by recall.**
   A `lyx` command should make the *correct* path the path of least resistance and make drift *detectable* (`status` / a future `doctor`), rather than relying on an agent or operator remembering a rule.
   No on-disk operation is truly un-bypassable when a shell is available, so the achievable bar is "right path is easiest + mistakes are detectable," **not** "wrong path impossible."
   Hard blocks (hooks, permission rules) are brittle and out of scope.
   Example: `lyx fabric` owns the overlay's git so raw `git -C` is never *needed* (it would be strictly more work), and `lyx fabric status` flags drift — but it is a friction asymmetry, not a wall.
7. **Go where it can be;
   LLM only for judgment.**
   Everything deterministic — verbs, control-flow, parsing, distillation, geometry, git — is Go.
   An LLM handles only the irreducible judgment a program can't: review verdicts, triage, batch implementation, an orchestrator's recovery decisions.
   The seam is consistent everywhere: **fat Go verbs** (`lyx <module> <verb>`) are the callable surface;
   an LLM session *drives* them and consumes **Go-distilled digests, never raw prose**;
   any **skill is a thin human wrapper** over those verbs, never where logic lives.

## Cwd Resolution Invariant

**All cwd resolution goes through `internal/lyxcwd`, and nothing else.** `lyxcwd` owns cwd resolution alone — never a weft path, a junction path, or any per-module subdirectory;
those are each owned by the module that constructs them.

`internal/lyxcwd` exposes a three-operation contract:

- `Getwd()` — the only permitted call to `os.Getwd` outside `cmd/lyx/main.go`.
- `Resolve(cwd)` → `*Location` — resolves the current cwd into a legal worktree's coordinates, applying the strict cwd gate.
- `ResolveWithAnchor(cwd, anchor)` / `ResolveWorktree(root)` — the two ungated variants, for callers that hold something other than an acting cwd (see `docs/shared-libs/lyxcwd.md`).

`Location` carries exactly four fields — `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel` — plus two derived accessors, `WorktreePath()` and `AnchorPath()`.
Every other geometry token (weft paths, junctions, `_lyx/<module>`, portals, launchers, the hub-reserved name set) is a per-module constructor, joined onto `Location`'s coordinates by the module that owns that token — see `CONSTRAINTS.md`'s Cwd Resolution Invariant for the full per-token ownership map.

`lyxcwd.Resolve` proves that cwd is the root of a git worktree and nothing more.
It succeeds in any ordinary git repository run from its root, and the `HubPath` and `RepoName` it returns are fiction in that case.
Proving a worktree is lyx-initialized and Fabric-wired is a different layer's job.
See [CONSTRAINTS.md's Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant) for the tier map.

**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside `internal/lyxcwd` and `cmd/lyx/main.go`.
The ban is enforced at `go test` / CI time by `internal/lyxcwd/enforcement_test.go`, which walks the entire source tree and fails the build if either literal token is found in any non-test `.go` file outside the allowlist.
A second scan in the same file, `TestEnforcement_GeometryLiterals`, enforces the per-token ownership map itself: no policed geometry token may be constructed as a string literal outside its registered owner directory.
A third scan, `TestEnforcement_FabricVocabulary`, enforces the separate Fabric Vocabulary Invariant: outside an owner set (`fabricengine`, `fabriccli`, `weftname`, `gitkit`, `hubforge`, `boardengine`, `configsync` string-literal-only), the tokens `weft`/`warp` may not appear in identifiers, string literals, or comments in production `.go` files, nor in the embedded agent prompt templates.
The fabric-sense phrase form of `host` (e.g. `host repo`, `hostBranch` — never the bare word) is banned everywhere this test reaches, including inside the owner set — `host` is retired, not merely scoped.
It shares this file's placement as a walk-helper convenience, not because the vocabulary rule is `lyxcwd`'s to own — see CONSTRAINTS.md's Fabric Vocabulary Invariant.

See [CONSTRAINTS.md](../CONSTRAINTS.md) for details.

## Documentation lifecycle

Two doc classes, opposite lifecycles:

- **Module-design docs** (`manifest/designs/<module>.md`) are mechanical per-module design drafts for **planned, not-yet-built** modules — deleted when their module lands;
  the implementation and tests become the source of truth.
  A module's purpose and key design rationale then live in its Go package header comment, next to the code it documents.
- **Durable Go-to-Go contract docs** (`contracts/specs/`) pin cross-module schemas a real consumer honors — they are **kept**, not deleted on landing: `loom-status-spec.md`, `webster-spec.md`, `llm-model-spec.md`. LLM-facing producer format contracts (what `Discussion-Write`/`Plan-Write` must write) live in the producer's own stencil under `contracts/stencils/`, not as a separate doc — see the Documentation Lifecycle's stencil-vs-doc split.

The other durable documentation is this `overview.md` (principles, naming, the module and shared-lib map, the weft contract,
and this lifecycle convention).
Planned-but-not-built work lives under the separate top-level `manifest/` (`manifest/roadmap.md` + `manifest/designs/`) — see its own maintenance note there.

## Weft overlay model

lyx organizes overlay artifacts (configuration, task state, raddle docs, and the board) into a **weft repo** — a companion git repository that stays separate from the Fabric repo, keeping it pristine.

### Topology

```
<hub>/                              (top-level Hub, NOT a git repo)
  ├── <prime>/                      (warp worktree, main branch; git repo root)
  ├── <prime>-weft/                 (weft Prime worktree; git repo root)
  ├── <slug>/                       (additional warp worktree; git repo root)
  ├── <slug>-weft/                  (weft worktree for <slug>; git repo root)
  ├── _board/                       (weft:main worktree; the task store)
  │     └── .lyx/                   (hub-wide machine-local scratch; a real dir, never a junction)
  ├── _portals/<anchor>/<slug>      (junction into <slug>'s _lyx; anchor-mirrored)
  └── _launchers/<anchor>/<slug>    (per-worktree launcher scripts; anchor-mirrored)
```

`_board`, `_portals`, and `_launchers` are hub geometry, so none of them can be claimed as a worktree slug
(`fabricengine.IsReservedHubName`); `.lyx` stays slug-reserved too, but via `structuralNeverCommittedDirs`
rather than as hub geometry, since it no longer has a hub-level presence of its own.

### Git ownership

The **Fabric repo** is the project's source of truth, maintained by developers.
All lyx-specific artifacts live in the **weft repo**, a separate git repository that lyx controls.
This separation keeps Fabric commits focused on project code and delegates lyx infrastructure to the weft.

### Artifacts location

| Artifact | Location | Repo | Purpose |
|----------|----------|------|---------|
| `_lyx/config/` | Weft worktree | Weft | Live YAML configuration files for all modules (board, fabric); reconciled via `lyx config reconcile` |
| `.env` | Weft worktree | Weft | Git-ignored per-machine environment variable overrides (KEY=value format) |
| `_lyx/raddle/` | Weft worktree | Weft | Raddle documentation (the raddle nav-doc overlay), reached through the `_lyx` junction like every other `_lyx` subtree |
| `_board/` | Hub | Board | A second weft worktree, checked out on the warp's own unsuffixed default branch (`weft:main` in the common case) — never a separate clone, never `<branch>-weft` |
| Warp source | Warp worktree | Warp | Project source code |

### Durable vs ephemeral state (`_lyx/` vs `.lyx/`)

Two state roots with opposite lifecycles:

- **`_lyx/`** — **durable, synced, portable.**
  Lives in the weft repo (git-synced), so it survives a machine and transfers to another.
  Config, raddle, the board, and loom's orchestration **status** (current producer, run state, per-producer-call history) go here — loom resume works across machines *because* its status is fabric-synced.
- **`.lyx/`** — **ephemeral, local, machine-bound.**
  Untracked in both the warp and the weft repo (listed in each repo's own `.git/info/exclude`, never a committed `.gitignore` in either), changing constantly while a run is live.
  The live tmux runtime state — `reed`'s (see the `internal/reedengine` package documentation) `.lyx/reed.json` (the socket/session names + the strand table: each managed process, its session, parent, ephemeral pane id, and display spec) — goes here, because a pane ID or the tmux socket is meaningless on another machine.
  It is rebuilt by reconciling against live tmux on startup, never synced.
  A pane id is meaningless even on the SAME machine once the tmux server has restarted — ids are server-global and restart at `%0` — so `reed.json` also records the *pane generation*, the identity of the session incarnation its pane ids were bound against, and discards every binding minted against a different one.

The test: **would this state mean anything on a different machine?**
Orchestration progress yes → `_lyx/`.
A pane handle no → `.lyx/`.

### Junction model

Each warp worktree has a sibling weft worktree.
Warp worktrees use **junctions** (Windows) or symlinks to route writes into the sibling weft worktree.
Worktrees are wired eagerly at `lyx fabric clone`/`lyx fabric add` time — there is no separate setup step: clone and worktree-add each materialize junctions, `_lyx`, and config in one call.

The wired junction set is not hardcoded,
and it is not purely the repo-wide `pathspec` list either: it is `structuralCommittedDirs` ∪ `structuralNeverCommittedDirs` ∪ the hub-reserved-filtered config names, deduplicated.
The two structural sets — `_lyx` and `.lyx` — are injected in code, never read from `fabric.yaml`;
only the third piece comes from the **repo-wide** `pathspec` list recorded once at `<BoardDir>/_lyx/config/fabric.yaml` (read from `weft:main`, via `fabricengine.BoardDir`), filtered against `fabricengine.HubReservedNames()` (the hub-structural tokens — `_board`, `_portals`, `_launchers` — that can never be a per-worktree junction).
Because the pathspec is repo-wide, `lyx fabric reconcile` declaratively converges **every** worktree to the same recorded set — adding a junction missing on disk, removing one absent from the wired set,
and no-op'ing one already correct — rather than each worktree carrying its own drift-prone copy. `lyxcwd` itself stays config-blind;
it only resolves the cwd coordinates that `fabricengine` builds the junction records onto.
This produces the two concrete junctions this repo ships with today, both placed at the repo's lyx-anchor (`<warp>/<anchor>/…`, which is `<warp>/` itself at the default `.` anchor):
- `<anchor>/_lyx` → `<hub>/<slug>-weft/<anchor>/_lyx` (config junction, structural)
- `<anchor>/.lyx` → `<hub>/<slug>-weft/<anchor>/.lyx` (machine-local scratch junction, structural)

The Hub Containment Invariant (`CONSTRAINTS.md`) is the rule that forbids re-adding a junction into `<hub>/_board`: no hub-level container is ever junctioned into a worktree.

The optional `pathspec` default is empty today.
A future weft-backed module is wired by appending its directory name to `pathspec`'s template default — no `fabric`/`lyxcwd` code change needed — but that mechanism now applies to *optional* directories only;
a structural directory is never sourced from `pathspec`.

Raddle content is anchor-level by design — it lives at `_lyx/raddle/`, reached through the existing `_lyx` junction, with no `_raddle` junction of its own now or ever;
see `manifest/designs/raddle.md`.

Every junction is listed in the warp worktree's own `.git/info/exclude` and is never committed to a `.gitignore` in the user's repo — a tracked entry would advertise that LYX is in use.
The entry is the junction's own anchored path (`/backend/_lyx`, or `/_lyx` at a root anchor), never a bare name: a slash-free gitignore pattern matches at any depth, which on a subpath-anchored monorepo would silently untrack same-named directories lyx never wired.
`.lyx` additionally seeds `.lyx/` into the **weft** repo's own `.git/info/exclude` at wiring time, so weft-side scratch never shows as untracked dirt either.
From the CLI's perspective, reads and writes happen transparently — code that writes to `_lyx/config/board.yaml` writes through the junction into the weft repo without awareness of the indirection.

A pre-existing real `.lyx` directory — every worktree that predates this junction, since several of lyx's own subsystems write `.lyx` unconditionally — is adopted rather than refused: its content is moved into the weft-side target and replaced with the junction, one time, on the first `lyx fabric reconcile` after upgrade.
`_lyx` keeps the hard refusal (fabric never moves or deletes what might be the user's hand-authored content); `.lyx` is the one exception because its content is always lyx's own machine-local scratch.

### Branch model

Weft branches mirror warp-repo branching: when a new weft worktree is spawned, its branch forks from the weft branch whose name equals the warp worktree's current branch at spawn time, preserving a shared merge-base for future squash-merge-back operations.
This guarantees subtasks (spawned from non-main branches) inherit the correct fork point: branch isolation is **not** orphan-based but **merge-base-preserving** (each on its parent's timeline). `_lyx` is isolated by pathspec (junctions route it into weft;
warp `.git/info/exclude` hides it) rather than by orphan topology, so no merge-back state is lost.

### Weft suffix convention

The weft worktree for any warp worktree is deterministic:
- Warp: `<hub>/<slug>/` → Weft: `<hub>/<slug>-weft/`
- Warp: `<prime>/` → Weft: `<prime>-weft/` (prime is the name of the main worktree)

The `-weft` suffix is fixed and non-configurable.
Weft paths are computed on demand from geometry and do not require a registry.

### Status

- **Go implementation** (paths geometry, paired spawn, `lyx fabric` command): ✅ Implemented. `fabric` (paths geometry, paired `lyx fabric add` spawn, and `lyx fabric status|commit|push|pull|sync|diff|merge-in|merge|merge-stage`) is the sole git-coordination module now. `status` is the unified both-sides uncommitted-change view. Paired `lyx fabric add` hard-requires a weft repo, which `lyx fabric clone` builds — there is no separate hub-creator tool.
- **`lyx config` command**: ✅ task 008 complete.
  The interactive menu (`lyx config`, `lyx config <module>`) and `lyx config reconcile` shipped. (A raddle config schema is **raddle** nav-doc work, not part of this task — it was only historically mis-bundled here; there is no `_raddle` junction to activate.)
- **Portals**: unimplemented;
  the weft junction model is the live mechanism. (Symlink-based overlay sharing is not on the critical path.)

```
github.com/Knatte18/loomyard/
├── cmd/lyx/
│   └── main.go                   entrypoint: routes the <module> argument to a module
├── internal/boardcli/            the board CLI command
├── internal/boardengine/         the board domain kernel
├── internal/fabriccli/           the fabric CLI command (warp↔weft git coordination)
├── internal/fabricengine/        the fabric domain kernel
├── internal/idecli/              the ide CLI command
├── internal/ideengine/           the ide domain kernel
├── internal/reedcli/             the reed CLI command
├── internal/reedengine/          the reed domain kernel (overlay + strand bookkeeping)
├── internal/reedengine/render/   pure display-vocabulary leaf (layout = Rules(strands))
├── internal/ghissuescli/         the ghissues CLI command
├── internal/ghissuesengine/      the ghissues domain kernel
├── internal/selfreportcli/       the selfreport CLI command
├── internal/selfreportengine/    the selfreport domain kernel
├── internal/treadleengine/       generalized round-loop engine (judge/gate/round-spawn/cap/pause/lock)
├── internal/shedengine/          generic outer phase-FSM: walks one flat producer list, honoring resume, crash-recovery, and pause at producer granularity
├── internal/shedadapters/        the three Shed engine adapters (SingleLLMProducer, Webster, the burler round producer) over shuttle/websterengine/burlerengine, plus the Bouncer adapter
├── internal/shedcheck/           authoring-time structural checker over an assembled OnDone/OnStuck producer graph
├── internal/loomcli/             loom's cobra module: the session bootstrap plus the driver, status, and pause verbs
├── internal/loomshed/            loom's own row-name constants and producer constructors over `shedengine`
├── internal/loomrecipe/          assembles loom's `*shedengine.Shed` from the embedded recipe
├── internal/shedrecipe/          the engine registry — the name to `ShedProducer`-constructor mapping a recipe loader resolves each row's `Engine` against
├── internal/shedbuild/           the recipe file format's loader and builder — decodes a recipe document and assembles the producer-definition list the shed engine already consumes
├── internal/landingshed/         landing's two general ShedProducers, Publish and Finalize, shared by reference across producer lists
├── internal/mergeresolve/        the merge-in + LLM conflict-resolution engine internal/landingshed's two producers each call
├── internal/hubgeom/             the hub-mode told-geometry teller that converts a resolved `lyxcwd.Location` into each engine's geometry struct
├── internal/standalonegeom/      the told-mode geometry teller that builds each engine's geometry struct from told absolute path strings
├── internal/preflightshed/       the general `Preflight` `ShedProducer` over `internal/preflight`'s tier-1/tier-2 checks, shared by reference across producer lists
├── internal/preflight/           orchestrator-agnostic tier-1/tier-2 precondition checks (geometry, worktree-pair cleanliness, Fabric readiness/sync) + the shared Report result type
├── internal/lyxcwd/              cwd resolution entry gate (the sole owner of cwd resolution, nothing else)
├── internal/lyxdirs/             the two directory-name tokens (`_lyx` durable, `.lyx` ephemeral), a zero-import leaf
├── internal/buildinfo/           the ldflags-stamped build channel, a zero-import leaf
├── internal/standalonestate/     target-path-to-hash8-and-state-directory derivation, a stdlib-only leaf
├── internal/configengine/        shared config resolution
├── internal/gitexec/             shared git operations
├── internal/gitrepo/             typed Repo over one local git checkout: go-git for local reads, gitexec for remote-auth/mutation
├── internal/githubclient/        GitHub token resolution, caching, and authenticated *github.Client construction — auth only, no per-operation wrappers
├── internal/lock/                shared file locking
├── internal/output/              shared JSON output
├── internal/modelspec/           model-spec parser + models.yaml registry leaf
├── internal/tokenvocab/          shared token vocabulary (repo, hub) + Render compose over stencil, a leaf
├── internal/pattern/             PATTERN active check + role directive leaf, consumed by webster/burler/loom
└── internal/shell/               provider-invariant pane-shell mechanics leaf (pwsh + posix)
```

`cmd/lyx` is `package main`;
everything else is in `internal/`. `main` is the only thing that imports a module.

## Module dispatch

`cmd/lyx/main.go` assembles all modules into a single cobra root via `newRoot()`.
Each module contributes a `Command() *cobra.Command` that is passed to `root.AddCommand(...)`, so every module and subcommand is discoverable via `lyx --help` without any central dispatch table.
Adding a module is three steps: import the package, add `<module>.Command()` to `root.AddCommand(...)` in `newRoot()`, and append the module name to `root.Long`.

`run(args, out)` is the testable seam: it builds a fresh root, merges stdout and stderr into `out`, and calls `root.ExecuteContext`, returning the process exit code without spawning a binary or trapping `os.Exit`.
Each module also exposes `RunCLI(out io.Writer, args []string) int` — exactly `return RunCLIIn("", out, args)` — as an in-process test seam that drives a module in isolation without involving the cobra root.
Ten of the eleven modules also expose `RunCLIIn(cwd string, out io.Writer, args []string) int`: `cwd == ""` delegates to `clihelp.Execute(Command(), out, args)` exactly as `RunCLI` always has, and any other value delegates to `clihelp.ExecuteIn(Command(), cwd, out, args)`, seeding `cwd` into the execution context so the module's handlers read it back via `lyxcwd.CwdFrom(cmd.Context())` instead of the process working directory.
`internal/selfreportcli` is the one seam module without `RunCLIIn`, since it references `lyxcwd` nowhere.
`clihelp.ExecuteIn`, `clihelp.RunRootCtx`, and `clihelp.WrapRunCtx` are `Execute`/`RunRoot`/`WrapRun`'s context-carrying siblings: they seed an explicit cwd or propagate an existing context into a command's execution instead of relying on the process working directory, letting a handler read it back via `lyxcwd.CwdFrom(cmd.Context())`.
`cmd/lyx/main.go` uses `RunRoot` unchanged.

All commands print JSON: `{"ok":true, ...}` on success, `{"ok":false,"error":"..."}` on failure (exit code 1).

## Modules

User-facing modules each get one `lyx <module>` namespace:

- **board** — the task-tracker board (`internal/boardcli` + `internal/boardengine`). ✅ Implemented.
- **config** — interactive menu for viewing and editing module configs;
  `lyx config reconcile` reconciles all module config files against their live templates (dry-run by default, `--apply` writes atomically) except seed-only modules (today: `models`), which are materialized once when absent and never rewritten again since the file is operator-owned;
  `lyx config <module> --set key=value` (repeatable) writes one or more config values directly with no editor invocation, for scripts/agents that need a non-interactive path. ✅ Implemented.
- **fabric** — the sole warp↔weft git-coordination module, unified over two `internal/gitrepo.Repo` instances: clone (the hub creator), dual-worktree add/remove, coordinated checkout (switches warp+weft together + re-points junctions), reconcile, status, prune, cleanup, weft content-sync (commit/push/pull/sync/diff), and a merge/conflict lifecycle (`merge-in`/`merge`/`merge-stage`/`merge --continue`/`merge --abort`, mirroring git's own exit codes and surfacing conflicts as unified, worktree-relative paths; `merge-stage` marks resolved paths so `--continue`'s index gate can pass, and is the only route for a conflict under a wired junction name, which git refuses to stage through), all in one command tree (`internal/fabriccli` + `internal/fabricengine`); CLI surface is `lyx fabric clone|add|list|remove|checkout|pairs|reconcile|prune|cleanup|unwire|status|commit|push|pull|sync|diff|merge-in|merge|merge-stage`. `clone` takes the weft URL first with the warp URL optional, derived from the warp binding recorded on the weft's main branch when omitted; `reconcile` backfills that binding for hubs whose weft predates it. `status` is the unified both-sides uncommitted-change view (`Fabric.Status`); `diff` reports the side-labelled changes since a given warp SHA (`Fabric.Diff`). `pull` is now unified across warp+weft, not weft-only: it fast-forwards weft first, then fetches and inspects warp, detecting a rebased/force-pushed warp remote via ancestry and safely re-anchoring weft's correspondence to it when it is safe to do so. ✅ Implemented; see the `internal/fabricengine` package documentation for rationale.
- **ide** — one-shot VS Code launcher with interactive menu. ✅ Implemented.
- **selfreport** — file bugs and enhancements against `Knatte18/loomyard` via go-github through `internal/githubclient` (`lyx selfreport create <title>`).
  Credentials resolve from `GH_TOKEN`/`GITHUB_TOKEN` first, with the `gh` CLI (`gh auth token`) as a bounded, non-blocking fallback token source — not a hard prerequisite.
  Target repo is hardcoded;
  supports `--body` (or `-` for stdin) and `--label`;
  defaults to `bug`.
  Callable from any sandbox agent context with no config. ✅ Implemented.
- **reed** — **the window to the world**: tmux overlay + **strand** bookkeeping + render (`internal/reedcli` + `internal/reedengine` + `internal/reedengine/render`). Hosts every managed process as a strand, arranges them, persists to `.lyx/reed.json` (`lyx reed up|add|remove|status|attach|resume|header|down`). Built on what its proof-of-concept, `muxpoc`, proved first (layout checksum, bottom-dominant layout, env hygiene, native `--resume`); `muxpoc` has since been deleted, its job done. `reed attach` and `reed header --blocking` are this module's two registered interactive-handoff exceptions (CONSTRAINTS.md CLI/Cobra Invariant): `attach` hands the operator's stdio to a `tmux attach-session` child in place, and `header --blocking` prints the rendered header-pane text (`Engine.HeaderText`, over `internal/tokenvocab`) then blocks forever as the header pane's own keepalive — in both cases every fallible step runs pre-flight, on the envelope, and only the terminal-handover/keepalive tail itself is exempt from emitting JSON. ✅ Implemented. See the `internal/reedengine` package documentation.
- **shuttle** — run **one** LLM agent as an interactive tmux strand over the file contract (`internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`; `lyx shuttle run|interrupt|send`). `Stop`-hook completion is read off an events file and classified into four outcomes — `done`/`asking`/`died`/`timeout` — with `asking` as the escalation channel back to a human or a higher-capability model. `asking` means the agent ended its turn WITHOUT satisfying the file contract, whatever its reason: shuttle never inspects the message, so a blocked agent, a question, and a mid-task status report all classify identically, and `lastAssistantMessage` carries whatever it last said rather than a guaranteed question. An interactive run also detects a live `AskUserQuestion` tool call in real time via a non-denying marker hook, classified the same way instead of waiting for the timeout. `died` is correspondingly narrow: it means a strand reed STILL TRACKS has a pane that is not alive (or the provider never came up inside the startup window). Reed's strand table losing the strand entirely — a `reed down`/`remove`, a deleted or rebuilt `reed.json`, a worktree renamed under an in-flight run — is reported as a mechanism failure carrying the run's identity, never as `died`, because reed's bookkeeping going away says nothing about the agent, whose process is often still working in its pane. `PreToolUse` guardrails deny the in-process `Agent` tool in both interactive and autonomous runs (on by default, switchable off via `shuttle.yaml`'s `claude_deny_agent_tool`, and narrowed in a `ForkSubagents` run to permit fork subagents while still refusing every other subagent type), and `AskUserQuestion` too when the run is autonomous (`Interactive: false`, the default). The provider is swappable behind an **engine** seam; Claude is the only v1 engine. Per-run `Model`/`Effort` knobs (`lyx shuttle run --model`/`--effort`; effort values `low|medium|high|xhigh|max`, empty = provider default) are engine-validated, not policed by `Spec.validate`. `Spec.Version` is a programmatic engine-validated version pin (claudeengine composes the pinned model id; no CLI flag — consumers drive it via the model-spec notation's `version=` param). ✅ Implemented. See the `internal/shuttleengine` package documentation.
- **webster** — the implementer module: one long-lived Master session that reads the codebase and the whole plan once, then forks one implementer per execution batch in-session (Claude Code's Agent tool) instead of spawning a fresh reed/tmux strand per batch;
  bracket verbs (`begin-batch`/`await-batch`/`record-batch`) drive each in-session fork (Master long-polls `await-batch` for each batch's report instead of relying on a synchronous fork return), and a genuine model escalation (recovery after a stuck/report-less fork) spawns a cold strand.
  Consumes the flat card-list plan (pinned in `contracts/stencils/loom/loom-template-plan.md`, the Plan producer's own stencil) via its own sole parser, `internal/planparser`, groups cards into execution batches via `internal/batcher`, a separate config-selected registry module webster consumes (identity batcher — one card, one batch — by default), then sequences those batches into a topological execution order derived from the cards' own `Targets`/`Uses` refs, condensing and reporting dependency cycles rather than refusing the run, and runs a dedicated integration-suite fork with in-process SHA-bisect on failure once every batch has landed (`internal/websterengine` + `internal/webstercli`). ✅ Implemented.
  See [webster-spec.md](../contracts/specs/webster-spec.md).
- **planparser** — the sole parser of the on-disk flat card-list plan format (`_lyx/plan/`, see [loom-plan-spec.md](../contracts/specs/loom-plan-spec.md));
  no other package reads that tree directly, and it also declares where that tree *is* — the worktree-relative form (`PlanDirName`/`PlanDirRel`) and the absolute told-anchor form (`PlanDir`/`PlanOverview`), with the caller supplying the anchor path (`internal/planparser`). ✅ Implemented.
- **discussionparser** — the sole reader of `_lyx/discussion/`'s on-disk format (the decision record's required sections and the support log's existence);
  it takes told absolute paths and declares no location of its own — deliberately unlike `planparser`, because `loomengine`'s accessors take a `*lyxcwd.Location`, which this stdlib-only leaf may not import.
  Consumed by `loomshed.discussionValidate` and by the `lyx loom validate-discussion` verb (`internal/discussionparser`). ✅ Implemented.
- **batcher** — the name-keyed batchifier registry that groups a plan's flat card list into webster's execution batches, selected by `batcher.yaml`'s `active:` config key (default: identity, one card per batch); its own standalone configreg module, separate from webster's (`internal/batcher`). ✅ Implemented.
- **stencil** — the operator surface over the hub's producer-prompt stencils (`internal/stencilcli` + `internal/stencilstore`; `lyx stencil list|validate|diff|sync|promote`): `list` reports every registered stencil's board-copy path and edit state, `validate` reports marker mismatches between a board copy and its shipped default, `diff` shows upstream changes not yet taken or (`--all`/`--exit-code`) board edits not yet ported back, `sync` force-refreshes every stencil against the shipped registry even from a `-dev` build, and `promote` copies a board-copy edit back into the worktree's `contracts/stencils/` source tree. ✅ Implemented.
- **loom** — phased orchestrator: drives its flat, ordered [producer list](../manifest/designs/loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots), each gated by a `Bouncer` review segment (`internal/loomcli` + `internal/loomengine` + `internal/loomshed` + `internal/loomrecipe`; `lyx loom run|drive|status|pause|validate-discussion|validate-plan`, plus the `run` verb registered a second time as the bare root alias `lyx run`).
  `run` is the session bootstrap, performing four steps in order: resolve the recorded parent branch and seed+commit the status file weft-side when it is absent; ensure the worktree's tmux session is up and its status strand exists; spawn the detached loom driver unless one is already alive; and hand the terminal to the tmux session.
  `drive` is the no-tmux escape hatch that runs the phase machine in the foreground, for debugging and CI.
  `status` reports the current phase as a single JSON envelope and, with `--watch`, tails it, printing a line only when the composed activity changes rather than once per poll.
  `pause` requests a pause at the next producer boundary.
  `validate-discussion` runs the Discussion-Validate gate's checks standalone, exiting 0 on a clean gate and 1 otherwise, with findings in the failure envelope so a writer agent can self-check before handing off.
  `validate-plan` runs the Plan-Validate gate's checks standalone, with the same exit-code and findings-envelope contract, over the current worktree's plan instead of its discussion.
  `lyx loom status --watch` and `lyx loom run` (alias `lyx run`) are this module's two registered interactive-handoff exceptions (CONSTRAINTS.md CLI/Cobra Invariant): `status --watch` self-displays the polled status line then blocks forever as its own keepalive tail, and `run` hands the operator's stdio to a `tmux attach-session` child as its own terminal-handover tail — in both cases every fallible step runs pre-flight, on the envelope, and only the named tail itself is exempt from emitting JSON.
  ✅ Implemented. loom's config module (`loom.yaml`, holding the `discussion`/`plan`/`review` role model-specs, `discussion_timeout_min`/`plan_timeout_min`/`review_timeout_min`, and `discussion_interactive`) exists and reconciles via `lyx config reconcile --apply` (the bare verb is a dry run that only reports added and removed keys and writes nothing).
  The `review` pair is the review segments' own model and timeout, and lives here rather than in the recipe because the recipe is embedded in the binary and a recipe-literal model would be untunable without a rebuild — see [manifest/designs/loom.md](../manifest/designs/loom.md)'s "The review model's home is `loom.yaml`, not the recipe" section.
  The Discussion producer: a prompt/profile fed to `shuttle.Run`, its prompt shipped as an embedded default in the top-level `contracts/stencils` package and read at call time from the hub's stencils directory (`contracts/stencils/loom/loom-template-discussion.md`), composed by `internal/loomengine`'s `prompt.go` + `discussion.go`.
  The producer runs in one of two modes, selected by `discussion_interactive`: autonomous by default, or interactive when the key is set, so an operator can interview the agent from its pane instead of it self-judging every answer.
  Both prompt renderings ship in the same stencil, selected by the `{{.mode_rules}}` marker.
  The Planner producer, the same way (`contracts/stencils/loom/loom-template-plan.md`), composed by `internal/loomengine`'s `prompt.go` + `plan.go`.
  See [manifest/designs/loom.md](../manifest/designs/loom.md).
- **shed** — the generic outer phase-FSM `loom` and the eventual `Hardener` are each built on: a Go engine that walks one flat, ordered producer list, honoring resume, crash-recovery, and pause uniformly at producer granularity, with no predefined slots (`internal/shedengine`).
  The four shipped engine adapters — `SingleLLMProducer` over `shuttle`, the `Webster` adapter, the burler round producer, and the Bouncer (the generic review-gate producer rather than a wrapper over an engine) — live in one package, `internal/shedadapters`, alongside their shared context and archive helpers.
  No `lyx shed` verb of its own by design — a product's own CLI constructs a `Shed` with its own producer list and calls `Run`, and a bare verb would be a command with no list to walk.
  The skeleton (the loop, the status file, the `ShedProducer` interface) is ✅ **implemented**; the four engine adapters (`SingleLLMProducer`, the `Webster` adapter, the burler round producer, and the Bouncer) are ✅ **implemented** too, shipped as `internal/shedadapters`.
  `internal/shedcheck` is the shipped structural checker over an assembled producer list, enforced by a `go test` invariant over loom's own list rather than called from any production constructor — see [manifest/designs/shed.md](../manifest/designs/shed.md#checking-an-assembled-producer-list) for the eight finding kinds it reports.
  The Shed recipe group's engine registry (piece 1 of that group) is ✅ **implemented** too, as `internal/shedrecipe`; it registers twelve engine names.
  The recipe file format and the loader/builder shipped too, as `internal/shedbuild`, and loom's own conversion to a recipe file has now shipped as well: `contracts/recipes/loom-recipe.yaml` plus `internal/loomrecipe`, which assembles it into the `*shedengine.Shed` `internal/loomcli` runs.
  See the `internal/shedengine`, `internal/shedadapters`, `internal/shedcheck`, `internal/shedrecipe`, `internal/shedbuild`, and `internal/loomrecipe` package documentation and [manifest/designs/shed.md](../manifest/designs/shed.md).
- **burler** — one review+fix round: A-review → B-fix, one agent, no self-grading, over the shuttle file contract (`internal/burlerengine` + `internal/burlercli`).
  Profile-driven: `{overlay, source}` fix-scope, tool-use.
  Cluster review fans job A out into N fork-subagent reviewers by naming a fan (`cluster-fan`) from the seed-only `burler.yaml` lens/fan library — never on by default.
  Strict frontmatter verdict parse;
  debug CLI `lyx burler run`. ✅ Implemented.
  See the `internal/burlerengine` package documentation.
- **hardener** — **DRAFT / concept.**
  Behavior-based reviewer that *runs* a live-substrate module (needs a sandbox repo) to harden it before merge;
  on-demand, post-loom, **off the spine**, shares only the `burler` round discipline.
  See [manifest/designs/hardener.md](../manifest/designs/hardener.md).

The cross-OS spawn primitive **proc**, and the generic outer phase-FSM **shed**, are the two remaining internal (non-CLI) layers — proc the base of the stack, shed the generic engine `loom` configures rather than a stack layer of its own;
see the [Execution stack](#execution-stack-orchestration-layers) section below for how proc / reed / shuttle fit together. (Earlier drafts split reed into separate `shed`/`glance` modules;
both folded back into reed — see the `internal/reedengine` package documentation. This `shed` is an abandoned earlier `reed` model/view draft, unrelated to [`Shed`](../manifest/designs/shed.md) the outer phase-FSM.)

The user-facing modules sit on a thin layer of shared infrastructure (`internal/configengine`, `internal/gitexec`, `internal/gitrepo`, `internal/lock`, `internal/logger`, `internal/output`, `internal/lyxcwd`, `internal/lyxdirs`, `internal/state`, `internal/shell`, `internal/modelspec`, `internal/tokenvocab`, `internal/pattern`, `internal/buildinfo`, `internal/standalonestate`) — defined in [shared-libs/README.md](shared-libs/README.md). `internal/pattern` is the leaf that computes whether `_lyx/PATTERN.md` is present and returns the role-appropriate constraints directive injected into every code-touching agent prompt (webster fork/Master, burler review+fix, loom plan).
Above the engines sits a separate precondition-and-geometry layer, not the shared-infrastructure layer above: `internal/preflight` is the tier-1/tier-2 precondition layer (worktree geometry, worktree-pair cleanliness, Fabric readiness/sync), and `internal/hubgeom` and `internal/standalonegeom` are its hub-mode and told-mode constructors of the `Geometry` struct each engine is handed — see the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant).
`internal/preflightshed` sits alongside these as the producer-shaped wrapper around that same layer, letting a `Shed` producer list name it as a single row rather than each caller composing `internal/preflight.Check` for itself.

## Execution stack (orchestration layers)

The orchestrator is not one module but a **layered stack**, each layer knowing only the one below it.
It exists in this shape for one reason: agents must run as **interactive tmux sessions, never headless `claude -p`** (an economic constraint — see the `internal/shuttleengine` package documentation), so spawning an agent is not a plain `exec` but "place a pane, launch a provider in it, drive it, detect completion."

```
internal/proc     spawn any OS process (windowless / detached), cross-OS      [OS primitive]
internal/reed     the window to the world — overlay + strand bookkeeping +     [builds on proc]  ✅
                  render; hosts every managed process as a strand, arranges
                  them, persists to .lyx/reed.json
internal/shuttle  run ONE LLM agent in a strand via a swappable engine over    [builds on reed]    ✅
                  the file contract; Stop-hook completion
burler            one review+fix round: A-review (+cluster) → B-fix           [builds on shuttle] ✅
shed              generic outer phase-FSM: walk one flat producer list,        [stdlib +           ✅
                  honoring resume/crash-recovery/pause at producer granularity  internal/state,lock
                                                                                 only -- skeleton]
loom              phase machine: drive each phase through a Bouncer gate       [builds on shed,
                                                                                 burler]
```

The whole stack runs **headless** (auto mode): strands exist (the interactive-session requirement), agents run, output files are read, nobody need watch.

The stack now has two entry modes, not one: every layer from `reed` up is **told** its geometry rather than deriving it.
`internal/hubgeom` and `internal/standalonegeom` are the two constructors that tell it — hub mode and told mode respectively — with `preflight.ResolveMode` selecting between them at a standalone-capable CLI's pre-run.
The consequence a reader needs: a producer verb therefore runs in a directory that is not a git repository, with no hub, no fabric, and no orchestrator status seed.
See the [Told-Geometry Invariant](../CONSTRAINTS.md#told-geometry-invariant) for the rule.

- **reed is three things, and it is built** — an **overlay** over tmux, **strand bookkeeping** (a strand = one tracked process: a metadata record with a `guid`, `name`, worktree slug, parent, and a *generic* display spec),
  and a **render** sub-package (`internal/reedengine/render`, `layout = Rules(strands, box)`).
  Callers hand reed `{cmd, name, display}` where `display` is generic (anchor / focus / shrinkWhenWaitingOnChild;
  height is derived, not caller-set) — never a domain `type`, so reed never learns what a "phase" or "cluster" is.
  Earlier drafts split the model and view into separate `shed`/`glance` modules;
  with one terminal per worktree they fold cleanly into `internal/reedengine` + `internal/reedengine/render`.
  See the `internal/reedengine` package documentation.
- **provider-invariant** — `shuttle` runs Claude today through an **engine**;
  the verdict/output contract is provider-invariant, so a different model can be swapped in without touching the review machinery.
  Non-Claude is not a current priority.
- **`tokenvocab` is a shared leaf, not a stack layer** — `internal/tokenvocab` (the `repo`/`hub` token registry + the `Render` compose over `internal/stencil`) sits beside `stencil` and `modelspec` as a general-purpose leaf the stack's modules consume, not a stage of the proc→reed→shuttle→burler→shed→loom chain itself. reed's header text pipeline consumes it today;
  loom's prompt templates are expected to reuse the same `Render` compose later.
  See the `internal/tokenvocab` package documentation.
- **the bootstrap** — `lyx loom run` (alias `lyx run`) brings up the worktree's tmux session, adds the `lyx loom status` strand (a 1-line top pane), spawns the loom driver **detached** (via `proc`, no TTY), and attaches the terminal to the session. loom runs in the background;
  the reed view takes the foreground.
  A `.lyx/lyxrun.cmd` launcher makes it one click.
- `reed`, `shuttle`, and `loom` each get a user-facing `lyx <module>` CLI (`lyx shuttle run|interrupt|send` lets an operator or another process drive one agent standalone, before loom exists); `burler` is composed by loom's own review segments (`lyx burler run` is a debug-only wrapper, not a product verb), and `proc` alone stays an internal library with no CLI of its own.

### Following one spawn down the stack

loom wants a plan-reviewer for worktree `feature-x`:

1. `loom` → its Plan-Review segment's `Bouncer` — "review this plan against the discussion until clean."
2. the segment's `Burler`-round producer → `burler.Run(profile, priorFiles)` — "run one review+fix round."
3. `burler` → `shuttle.Run(prompt, engine)` — "run one handler agent."
4. `shuttle` → `reed.AddStrand{ cmd:"claude …", worktree:"feature-x", display:{anchor:below-parent, focus:true} }`.
5. `reed` records the strand in `.lyx/reed.json`, runs the command via `proc` in a pane, re-renders the layout (`layout = rules(strands)`), and applies it.
6. The `Stop` hook fires → reed notes the edge → shuttle reads the output file → returns to burler → burler writes review/fixer-report + verdict → the segment's `Bouncer` reads it, decides another round or exit → on an APPROVED verdict returns `Done` → loom advances.

### The disambiguating test

- About **the OS**? → `proc`.
- About **a tmux mechanic, a strand, or how it's laid out**? → `reed`.
- About **running an LLM and getting its answer**? → `shuttle`.
- About **one review+fix round**? → `burler`.
- About **whether an artifact passes (loop rounds until clean/stuck)**? → a `Bouncer` review segment.
- About **hardening a live-substrate module by running it** (post-loom, off-spine)? → `hardener` (DRAFT).
- About **what to run next**? → `loom`.

## Tests

Per-file unit tests sit next to the source they test (`store.go` ↔ `store_test.go`).
The cross-cutting suites — benchmarks, concurrency stress, and git-backed integration — live in the black-box `internal/boardengine/boardtest` package.

Fabric's own cross-cutting suite follows the same black-box convention but keeps its own name: a set of `package fabricengine_test` files inside `internal/fabricengine/`, `//go:build integration`-tagged.
It is the **live-state integration harness**: it drives real cloned hubs, built by really cloning rather than hand-assembling a fixture, into dirty and hostile on-disk states and asserts what a destructive verb is and is not permitted to touch.
See `internal/fabricengine`'s own package doc for the state matrix, the verb table, and the sabotage-proof table recording that each of the crucible campaign's eight data-loss defects still fails on demand when its guarding check is neutered.

`internal/gitkit` is the below-fabric leaf holding git primitives — `MustRun`, `SeedConfig`, `HermeticGitEnv`, `GitStatusPorcelain`, and the primitive repo fixture `CopyRepo` — and asserts nothing itself; it never imports fabric.
`internal/hubforge` is the repo-wide real-hub fixture factory: it builds every hub fixture in the repo through `fabriccli.CloneAndWire`, never a hand-assembled stand-in, and asserts nothing about fabric either.

## Sandbox Hub

The **sandbox Hub** is a dedicated bench for manual testing of lyx's core workflows — dogfooding lyx against itself.
It lives on disk at `C:\Code\lyx-test-HUB` and exercises the resolved `lyx` binary under test: the dev binary via `deploy-dev` into `.dev-bin` when present, else the production binary on PATH via `deploy.cmd`.
Build it via `sandbox/win/build.cmd` (`sandbox/posix/build.sh` on Linux/macOS), run the core suite via `sandbox/win/core-suite.cmd` (`sandbox/posix/core-suite.sh`, or the `reed-suite`/`reed-suite.sh` pair for the reed-specific suite, which needs live tmux), and collect the report via `sandbox/win/fetch.cmd` (`sandbox/posix/fetch.sh`) for either.
See [sandbox-howto.md](sandbox-howto.md) for the step-by-step runbook and [sandbox-hub.md](sandbox-hub.md) for topology and design details.

## Other docs

- [manifest/designs/loom.md](../manifest/designs/loom.md) — the phased orchestrator (`lyx loom`);
  design.
- `internal/tokenvocab` package documentation — the shared token vocabulary (`repo`/`hub` + `Render` over `internal/stencil`), consumed by reed's header pipeline and, later, loom's prompt templates;
  a leaf, not a phased module (as-built;
  module doc deleted per the documentation lifecycle).
- [webster-spec.md](../contracts/specs/webster-spec.md) — webster's cross-module contract: the `_lyx/webster/` boundary, `outcome.yaml`, and the `summary.md` artifact Finalize consumes (as-built;
  kept as a durable contract doc, not deleted on landing).
- `internal/reedengine` package documentation — the window to the world: tmux overlay + strand bookkeeping + render (as-built;
  module doc deleted per the documentation lifecycle).
- `internal/shuttleengine` package documentation — run one LLM agent via a swappable engine over the file contract (as-built;
  module doc deleted per the documentation lifecycle).
- `internal/burlerengine` package documentation — one review+fix round: A-review → B-fix, no self-grading (as-built;
  module doc deleted per the documentation lifecycle).
- `internal/treadleengine` package documentation — the generalized round-loop engine (judge, gate, round-spawn, milestone cap ladder, judge-maintained handoff, pause, run-dir lock), with a pluggable `RoundRunner` seam a future consumer (Tenter) can drive (as-built;
  module doc deleted per the documentation lifecycle).
- [manifest/designs/hardener.md](../manifest/designs/hardener.md) — **DRAFT/concept**: behavior-based hardening of a live-substrate module (post-loom, off-spine).
- [benchmarks/](benchmarks/board-performance.md) — board performance, tracked across revisions.
- [shared-libs/](shared-libs/README.md) — the shared infrastructure plumbing.
- [research/](research/) — design exploration (reed research logs).
- [reference/tmux_scripting.md](reference/tmux_scripting.md) — tmux command reference (vendored).
- [manifest/roadmap.md](../manifest/roadmap.md) — planned, someday, and shipped modules — the single home for unscheduled ideas too (no separate long-term-ideas file).
- [sandbox-howto.md](sandbox-howto.md) — operator runbook: deploy `lyx`, build the Hub, run the suite agent (procedure).
- [sandbox-hub.md](sandbox-hub.md) — the sandbox Hub: a dedicated bench for manual (dogfooding) testing.
- [crucible/README.md](../crucible/README.md) — **`crucible`**, the **serial review+fix loop**: a reusable method for hardening a live-substrate module before merge (orchestrator-driven, model-rotating, clean-room self-fixing rounds + independent verification).
  The hand-executed prototype of the review-gate + `burler` (see the `internal/burlerengine` package documentation) round loop (and the origin of the [`hardener`](../manifest/designs/hardener.md) concept, named separately to avoid colliding with it);
  ships two paste-ready prompts — an [orchestrator prompt](../crucible/orchestrator-prompt.md) (drives the loop + verifies) and a [round-agent prompt template](../crucible/review-prompt-template.md) (the reviewer-fixer), to instantiate per module.
  Lives at the repo root, not under `docs/`, since it's a working method/prompt set, not documentation of shipped code.
