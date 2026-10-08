# Overview: Loomyard

Loomyard is a Go toolkit of one-shot CLI modules.
Each invocation starts a process, runs one command, writes JSON to stdout, and exits — there is no daemon and no shared memory.
State lives on disk per module and is coordinated with file locks, so concurrent `lyx` processes on a machine cooperate through the filesystem.
The first module, **board** (a task tracker), is implemented;
**fabric** (the git-coordination module over the code and its records) is implemented;
and **reed**, the clean tmux overlay built on what its now-deleted proof-of-concept (`muxpoc`) proved, is implemented.

In the long term, Loomyard is intended to **replace mill/millhouse (Python)** entirely.
We get there by building these modules as self-contained toolkits first;
orchestration comes last.
See [Principles](#principles).

Module path: `github.com/Knatte18/loomyard`

## Naming: `lyx` (binary) · `loom` (orchestrator module) · `ly` (skills)

Three distinct names for three layers, deliberately non-overlapping to avoid the millhouse `mill`/`millpy` collision (where one name meant two different things):

- **`lyx`** — the binary/CLI, **L**oom**Y**ard e**X**ecutable — one binary with a namespaced subcommand tree (`lyx board`, `lyx fabric`, `lyx loom`, …).
  The analog of millhouse's `millpy` backend.
- **`loom`** — the orchestrator *module* (`lyx loom start`, `lyx loom status`): the domain that drives the phased run, a module like `board` or `fabric`.
  See the `internal/loomengine` and `internal/loomcli` package documentation.
- **`ly`** — the skill / orchestration plugin (the analog of `mill`);
  skills are `/ly-*`.

**Never name skills `lyx-*` or `loom-*`** — skills are `ly-*`, distinct from both the binary (`lyx`) and every module (`loom`, `burler`, …), so no name is shared between a skill and a script/module (the ambiguity that forced the millhouse `mill` → `millpy` rename).
Internal Go feature packages follow the `<module>cli` / `<module>engine` split (e.g. `internal/boardcli` + `internal/boardengine`, `internal/fabriccli` + `internal/fabricengine`) — see the package-naming rule in [PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md).

Convenience alias: **`lyx start` → `lyx loom start`** (the everyday autonomous call).

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

**All cwd resolution goes through `internal/lyxcwd`, and nothing else.** `lyxcwd` owns cwd resolution alone — never a records path, a junction path, or any per-module subdirectory;
those are each owned by the module that constructs them.

`internal/lyxcwd` exposes a three-operation contract:

- `Getwd()` — the only permitted call to `os.Getwd` outside `cmd/lyx/main.go`.
- `Resolve(cwd)` → `*Location` — resolves the current cwd into a legal worktree's coordinates, applying the strict cwd gate.
- `ResolveWithAnchor(cwd, anchor)` / `ResolveWorktree(root)` — the two ungated variants, for callers that hold something other than an acting cwd (see `docs/shared-libs/lyxcwd.md`).

`Location` carries exactly four fields — `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel` — plus two derived accessors, `WorktreePath()` and `AnchorPath()`.
Every other geometry token (records paths, junctions, `_lyx/<module>`, portals, launchers, the hub-reserved name set) is a per-module constructor, joined onto `Location`'s coordinates by the module that owns that token — see [PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md) for the full per-token ownership map.

`lyxcwd.Resolve` proves that cwd is the root of a git worktree and nothing more.
It succeeds in any ordinary git repository run from its root, and the `HubPath` and `RepoName` it returns are fiction in that case.
Proving a worktree is lyx-initialized and Fabric-wired is a different layer's job.
See [PATTERN-told-geometry](../pattern/PATTERN-told-geometry.md) for the tier map.

**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside `internal/lyxcwd` and `cmd/lyx/main.go`.
The ban is enforced at `go test` / CI time by `internal/lyxcwd/enforcement_test.go`, which walks the entire source tree and fails the build if either literal token is found in any non-test `.go` file outside the allowlist.
A second scan in the same file, `TestEnforcement_GeometryLiterals`, enforces the per-token ownership map itself: no policed geometry token may be constructed as a string literal outside its registered owner directory.
A third scan, `TestEnforcement_FabricVocabulary`, enforces the separate Fabric Vocabulary Invariant: outside the owner set it names, the two side names may not appear in identifiers, string literals, or comments in production `.go` files, nor in the embedded agent prompt templates.
The fabric-sense phrase form of `host` (e.g. `host repo`, `hostBranch` — never the bare word) is banned everywhere this test reaches, including inside the owner set — `host` is retired, not merely scoped.
It shares this file's placement as a walk-helper convenience, not because the vocabulary rule is `lyxcwd`'s to own — see [PATTERN-fabric-vocabulary](../pattern/PATTERN-fabric-vocabulary.md).

See [PATTERN.md](../PATTERN.md) for details.

## Documentation lifecycle

Two doc classes, opposite lifecycles:

- **Module designs** for **planned, not-yet-built** modules live in their board entry's body;
  when the module lands, the entry's design moves into the package's `doc.go` and the implementation and tests become the source of truth.
  A module's purpose and key design rationale then live in its Go package header comment, next to the code it documents.
- **Durable Go-to-Go contract docs** (`contracts/specs/`) pin cross-module schemas a real consumer honors — they are **kept**, not deleted on landing: `loom-status-spec.md`, `webster-spec.md`, `llm-model-spec.md`, `final-summary-spec.md`, `loom-plan-spec.md`. LLM-facing producer format contracts (what `Discussion-Write`/`Plan-Write` must write) live in the producer's own stencil under `contracts/stencils/`, not as a separate doc — see the Documentation Lifecycle's stencil-vs-doc split.

The other durable documentation is this `overview.md` (principles, naming, the module and shared-lib map, the records contract,
and this lifecycle convention).
Planned-but-not-built work lives on the board, one entry per item, with an unbuilt design in the entry's body.

## Records overlay model

lyx organizes overlay artifacts (configuration, task state, raddle docs, and the board) into a **records repo** — a companion git repository that stays separate from the project's own repo, keeping it pristine.
The two-sided design is documented in `internal/fabricengine`'s package doc.

### Topology

```
<hub>/                              (top-level Hub, NOT a git repo)
  ├── <prime>/                      (code worktree, main branch; git repo root)
  ├── <prime>-<records>/            (records Prime worktree; git repo root)
  ├── <slug>/                       (additional code worktree; git repo root)
  ├── <slug>-<records>/             (records worktree for <slug>; git repo root)
  ├── _board/                       (records main-branch worktree; holds board.json)
  │     └── .lyx/                   (hub-wide machine-local scratch; a real dir, never a junction)
  ├── _portals/<anchor>/<slug>      (junction into <slug>'s _lyx; anchor-mirrored)
  └── _launchers/<anchor>/          (anchor-mirrored)
        ├── <slug>                  (per-worktree launcher scripts)
        └── <prime>.code-workspace  (the prime's hub workspace; lyx-owned)
```

`_board`, `_portals`, and `_launchers` are hub geometry, so none of them can be claimed as a worktree slug
(`fabricengine.IsReservedHubName`); `.lyx` stays slug-reserved too, but via `structuralNeverCommittedDirs`
rather than as hub geometry, since it no longer has a hub-level presence of its own.

### Git ownership

The **code repo** is the project's source of truth, maintained by developers.
All lyx-specific artifacts live in the **records repo**, a separate git repository that lyx controls.
This separation keeps code commits focused on project code and delegates lyx infrastructure to the records.

### Artifacts location

| Artifact | Location | Repo | Purpose |
|----------|----------|------|---------|
| `_lyx/config/` | Records worktree | Records | Live YAML configuration files for all modules (board, fabric); reconciled via `lyx config reconcile` |
| `.env` | Records worktree | Records | Git-ignored per-machine environment variable overrides (KEY=value format) |
| `_lyx/raddle/` | Records worktree | Records | Raddle documentation (the raddle nav-doc overlay), reached through the `_lyx` junction like every other `_lyx` subtree |
| `_board/` | Hub | Board | A second records worktree, checked out on the code repo's own unsuffixed default branch (the records main branch in the common case) — never a separate clone, never a per-task records branch |
| Source | Code worktree | Code | Project source code |

### Durable vs ephemeral state (`_lyx/` vs `.lyx/`)

Two state roots with opposite lifecycles:

- **`_lyx/`** — **durable, synced, portable.**
  Lives in the records repo (git-synced), so it survives a machine and transfers to another.
  Config, raddle, the board, and loom's orchestration **status** (current producer, run state, per-producer-call history) go here — loom resume works across machines *because* its status is fabric-synced.
- **`.lyx/`** — **ephemeral, local, machine-bound.**
  Untracked in both the code and the records repo (listed in each repo's own `.git/info/exclude`, never a committed `.gitignore` in either), changing constantly while a run is live.
  The live tmux runtime state — `reed`'s (see the `internal/reedengine` package documentation) `.lyx/reed.json` (the socket/session names + the strand table: each managed process, its session, parent, ephemeral pane id, and display spec) — goes here, because a pane ID or the tmux socket is meaningless on another machine.
  It is rebuilt by reconciling against live tmux on startup, never synced.
  A pane id is meaningless even on the SAME machine once the tmux server has restarted — ids are server-global and restart at `%0` — so `reed.json` also records the *pane generation*, the identity of the session incarnation its pane ids were bound against, and discards every binding minted against a different one.

The test: **would this state mean anything on a different machine?**
Orchestration progress yes → `_lyx/`.
A pane handle no → `.lyx/`.

### Junction model

Each code worktree has a sibling records worktree.
Code worktrees use **junctions** (Windows) or symlinks to route writes into the sibling records worktree.
Worktrees are wired eagerly at `lyx fabric clone`/`lyx fabric add` time — there is no separate setup step: clone and worktree-add each materialize junctions, `_lyx`, and config in one call.

The wired junction set is not hardcoded,
and it is not purely the repo-wide `pathspec` list either: it is `structuralCommittedDirs` ∪ `structuralNeverCommittedDirs` ∪ the hub-reserved-filtered config names, deduplicated.
The two structural sets — `_lyx` and `.lyx` — are injected in code, never read from `fabric.yaml`;
only the third piece comes from the **repo-wide** `pathspec` list recorded once at `<BoardDir>/_lyx/config/fabric.yaml` (read from the records main branch, via `fabricengine.BoardDir`), filtered against `fabricengine.HubReservedNames()` (the hub-structural tokens — `_board`, `_portals`, `_launchers` — that can never be a per-worktree junction).
`board.yaml` is hub-wide like it: read and written at `<BoardDir>/_lyx/config/board.yaml` and never in a worktree's `_lyx/config/`.
Because the pathspec is repo-wide, `lyx fabric reconcile` declaratively converges **every** worktree to the same recorded set — adding a junction missing on disk, removing one absent from the wired set,
and no-op'ing one already correct — rather than each worktree carrying its own drift-prone copy. `lyxcwd` itself stays config-blind;
it only resolves the cwd coordinates that `fabricengine` builds the junction records onto.
This produces the two concrete junctions this repo ships with today, both placed at the repo's lyx-anchor (`<code>/<anchor>/…`, which is `<code>/` itself at the default `.` anchor):
- `<anchor>/_lyx` → `<hub>/<slug>-<records>/<anchor>/_lyx` (config junction, structural)
- `<anchor>/.lyx` → `<hub>/<slug>-<records>/<anchor>/.lyx` (machine-local scratch junction, structural)

[PATTERN-hub-containment](../pattern/PATTERN-hub-containment.md) is the rule that forbids re-adding a junction into `<hub>/_board`: no hub-level container is ever junctioned into a worktree.

The optional `pathspec` default is empty today.
A future records-backed module is wired by appending its directory name to `pathspec`'s template default — no `fabric`/`lyxcwd` code change needed — but that mechanism now applies to *optional* directories only;
a structural directory is never sourced from `pathspec`.

Raddle content is anchor-level by design — it lives at `_lyx/raddle/`, reached through the existing `_lyx` junction, with no `_raddle` junction of its own now or ever;
see the board's `raddle` note.

Every junction is listed in the code worktree's own `.git/info/exclude` and is never committed to a `.gitignore` in the user's repo — a tracked entry would advertise that LYX is in use.
The entry is the junction's own anchored path (`/backend/_lyx`, or `/_lyx` at a root anchor), never a bare name: a slash-free gitignore pattern matches at any depth, which on a subpath-anchored monorepo would silently untrack same-named directories lyx never wired.
`.lyx` additionally seeds `.lyx/` into the **records** repo's own `.git/info/exclude` at wiring time, so records-side scratch never shows as untracked dirt either.
From the CLI's perspective, reads and writes happen transparently — code that writes to `_lyx/config/loom.yaml` writes through the junction into the records repo without awareness of the indirection.

A pre-existing real `.lyx` directory — every worktree that predates this junction, since several of lyx's own subsystems write `.lyx` unconditionally — is adopted rather than refused: its content is moved into the records-side target and replaced with the junction, one time, on the first `lyx fabric reconcile` after upgrade.
`_lyx` keeps the hard refusal (fabric never moves or deletes what might be the user's hand-authored content); `.lyx` is the one exception because its content is always lyx's own machine-local scratch.

### Branch model

Records branches mirror code-repo branching: when a new records worktree is spawned, its branch forks from the records branch whose name equals the code worktree's current branch at spawn time, preserving a shared merge-base for future squash-merge-back operations.
This guarantees subtasks (spawned from non-main branches) inherit the correct fork point: branch isolation is **not** orphan-based but **merge-base-preserving** (each on its parent's timeline). `_lyx` is isolated by pathspec (junctions route it into the records;
the code repo's `.git/info/exclude` hides it) rather than by orphan topology, so no merge-back state is lost.

### Records suffix convention

The records worktree for any code worktree is deterministic:
- Code: `<hub>/<slug>/` → Records: `<hub>/<slug>-<records>/`
- Code: `<prime>/` → Records: `<prime>-<records>/` (prime is the name of the main worktree)

The suffix is fixed and non-configurable, and `internal/fabricengine`'s package doc names it.
Records paths are computed on demand from geometry and do not require a registry.

### Status

- **Go implementation** (paths geometry, paired spawn, `lyx fabric` command): ✅ Implemented. `fabric` (paths geometry, paired `lyx fabric add` spawn, and `lyx fabric status|commit|push|pull|sync|diff|merge-in|merge|merge-stage`) is the sole git-coordination module now. `status` is the unified both-sides uncommitted-change view, also reporting `merge_in_progress`, whether THIS pair has a fabric merge parked. Paired `lyx fabric add` hard-requires a records repo, which `lyx fabric clone` builds — there is no separate hub-creator tool.
- **`lyx config` command**: ✅ task 008 complete.
  Bare `lyx config` lists modules and verbs, `lyx config menu` is the interactive picker, `lyx config <module>` edits, and `lyx config reconcile` shipped. (A raddle config schema is **raddle** nav-doc work, not part of this task — it was only historically mis-bundled here; there is no `_raddle` junction to activate.)
- **Portals**: unimplemented;
  the records junction model is the live mechanism. (Symlink-based overlay sharing is not on the critical path.)

```
github.com/Knatte18/loomyard/
├── cmd/lyx/
│   └── main.go                   entrypoint: routes the <module> argument to a module
├── internal/agentname/           the sole former, parser and validator of agent names (`<shortname>:<slug>:<role>`), a stdlib-only leaf
├── internal/boardcli/            the board CLI command
├── internal/boardengine/         the board domain kernel
├── internal/fabriccli/           the fabric CLI command (git coordination of code and records)
├── internal/fabricengine/        the fabric domain kernel
├── internal/idecli/              the ide CLI command
├── internal/ideengine/           the ide domain kernel
├── internal/pairteardown/        the one sequence that ends a pair: quiet wait, refusal probe, reed session end, then fabric removal
├── internal/reedcli/             the reed CLI command
├── internal/reedengine/          the reed domain kernel (overlay + strand bookkeeping)
├── internal/reedengine/render/   pure display-vocabulary leaf (layout = Rules(strands))
├── internal/orchcli/             the orch CLI command
├── internal/orchengine/          the orch domain kernel (config, stencil renders, persisted state, cycle state machine)
├── internal/ghissuescli/         the ghissues CLI command
├── internal/ghissuesengine/      the ghissues domain kernel
├── internal/selfreportcli/       the selfreport CLI command
├── internal/selfreportengine/    the selfreport domain kernel
├── internal/treadleengine/       generalized round-loop engine (judge/gate/round-spawn/cap/pause/lock)
├── internal/shedengine/          generic outer phase-FSM: walks one flat producer list, honoring resume, crash-recovery, and pause at producer granularity
├── internal/shedtransient/       the one translation from lower-level failure classifications into the shed engine's transient mark
├── internal/burlermarker/        the one derivation of a burler round's machine-local ready marker path from its review path
├── internal/shedadapters/        the three Shed engine adapters (SingleLLMProducer, Webster, the burler round producer) over shuttle/websterengine/burlerengine, plus the Bouncer adapter
├── internal/shedcheck/           authoring-time structural checker over an assembled OnDone/OnStuck producer graph
├── internal/loomcli/             loom's cobra module: the session bootstrap, arming `internal/shedverbs`' generic driver, status, and pause verb bodies
├── internal/parentreview/        the parent-review round store and gate closures behind `lyx loom review`, over told directories
├── internal/loomshed/            loom's own row-name constants and producer constructors over `shedengine`
├── internal/loomrecipe/          assembles loom's `*shedengine.Shed` from the embedded recipe
├── internal/shedrecipe/          the engine registry — the name to `ShedProducer`-constructor mapping a recipe loader resolves each row's `Engine` against
├── internal/shedbuild/           the recipe file format's loader and builder — decodes a recipe document and assembles the producer-definition list the shed engine already consumes
├── internal/shedverbs/           the generic run/step/status/pause/goto cobra verb bodies shared by every module that arms a `*shedengine.Shed` onto a CLI subtree
├── internal/shedcli/             the `lyx shed` subtree: a named-recipe arming table plus the three CLI seams that register it under the lyx root
├── internal/statuscommit/        the shared per-transition status commit core (skip while mid-merge, commit hard-errors, push warns) that `loomcli` and `battencli` wrap
├── internal/landingshed/         landing's three general ShedProducers, Publish, PR-Gate and Finalize, shared by reference across producer lists
├── internal/mergeresolve/        the merge-in + LLM conflict-resolution engine internal/landingshed's two producers each call
├── internal/frictionengine/      the aggregation-and-reflection step loom's terminal Friction-Reflect row runs and loom runs after a `blocked` or `failed` halt, under `run` and `step`, except that an escalation halt writes its note and runs no reflection; it is re-entrant across a killed driving process
├── internal/hubgeom/             the hub-mode told-geometry teller that converts a resolved `lyxcwd.Location` into each engine's geometry struct
├── internal/standalonegeom/      the told-mode geometry teller that builds each engine's geometry struct from told absolute path strings
├── internal/cliwire/             the shared standalone/hub wiring resolver for the standalone-capable CLIs, the layer that runs after `preflight.ResolveMode` has chosen a mode
├── internal/hubreconcile/        the hub config walk a start verb runs once after a binary change: build stamp, hub lock, reconcile and commit of every worktree's config
├── internal/preflightshed/       the general `Preflight` `ShedProducer` over `internal/preflight`'s tier-1/tier-2 checks, shared by reference across producer lists
├── internal/preflight/           orchestrator-agnostic tier-1/tier-2 precondition checks (geometry, worktree-pair cleanliness, Fabric readiness/sync) + the shared Report result type
├── internal/lyxcwd/              cwd resolution entry gate (the sole owner of cwd resolution, nothing else)
├── internal/lyxdirs/             the two directory-name tokens (`_lyx` durable, `.lyx` ephemeral), a zero-import leaf
├── internal/buildinfo/           the ldflags-stamped build channel, a zero-import leaf
├── internal/buildvcs/            the running binary's VCS identity, a stdlib-only leaf
├── internal/standalonestate/     target-path-to-hash8-and-state-directory derivation, a stdlib-only leaf
├── internal/segmentcolor/        the loom segments, lyx's color palette and its tmux colors, a stdlib-only leaf
├── internal/configengine/        shared config resolution
├── internal/gitexec/             shared git operations
├── internal/gitrepo/             typed Repo over one local git checkout: go-git for local reads, gitexec for remote-auth/mutation
├── internal/githubclient/        GitHub token resolution, caching, and authenticated *github.Client construction — auth only, no per-operation wrappers
├── internal/lock/                shared file locking
├── internal/output/              shared JSON output
├── internal/modelspec/           model-spec parser + models.yaml registry leaf
├── internal/pattern/             PATTERN directive leaf (root PATTERN.md inlined per role) + format checker
├── internal/friction/            the Tier 2 friction-note directive leaf, consumed by webster, burler, and loom
├── internal/shell/               provider-invariant pane-shell mechanics leaf (pwsh + posix)
├── internal/commentlint/        the comment line-break lint: fixed-column wraps in the `//` comment blocks a diff creates, over a told worktree and base
├── internal/impactset/          the round gate's impacted-set command: the packages a diff reaches over the import graph, plus the `//lyx:guard` tests, or a fallback reason to run the full verify
├── internal/verifytree/         the one plan-verify function: clean-tree check, verified-tree record and running marker, used by landing, both webster gates and `lyx webster verify`
└── internal/verifyrun/          in-process shell-command runner behind `internal/verifytree` and webster's card-verify rerun
```

`cmd/lyx` is `package main`;
everything else is in `internal/`. `main` is the only thing that imports a module.

## Module dispatch

`cmd/lyx/main.go` assembles all modules into a single cobra root via `newRoot()`.
Each module contributes a `Command() *cobra.Command` that is passed to `root.AddCommand(...)`, so every module and subcommand is discoverable via `lyx --help` without any central dispatch table.
Adding a module is three steps: import the package, add `<module>.Command()` to `root.AddCommand(...)` in `newRoot()`, and append the module name to `root.Long`.

`run(args, out)` is the testable seam: it builds a fresh root, merges stdout and stderr into `out`, and calls `root.ExecuteContext`, returning the process exit code without spawning a binary or trapping `os.Exit`.
Each module also exposes `RunCLI(out io.Writer, args []string) int` — exactly `return RunCLIIn("", out, args)` — as an in-process test seam that drives a module in isolation without involving the cobra root.
Every module but one also exposes `RunCLIIn(cwd string, out io.Writer, args []string) int`: `cwd == ""` delegates to `clihelp.Execute(Command(), out, args)` exactly as `RunCLI` always has, and any other value delegates to `clihelp.ExecuteIn(Command(), cwd, out, args)`, seeding `cwd` into the execution context so the module's handlers read it back via `lyxcwd.CwdFrom(cmd.Context())` instead of the process working directory.
`internal/selfreportcli` is the one seam module without `RunCLIIn`, since it references `lyxcwd` nowhere.
`clihelp.ExecuteIn`, `clihelp.RunRootCtx`, and `clihelp.WrapRunCtx` are `Execute`/`RunRoot`/`WrapRun`'s context-carrying siblings: they seed an explicit cwd or propagate an existing context into a command's execution instead of relying on the process working directory, letting a handler read it back via `lyxcwd.CwdFrom(cmd.Context())`.
`cmd/lyx/main.go` uses `RunRoot` unchanged.

All commands print JSON: `{"ok":true, ...}` on success, `{"ok":false,"error":"..."}` on failure (exit code 1).

## Modules

User-facing modules each get one `lyx <module>` namespace:

- **board** — the task-tracker board, which is also the roadmap (`internal/boardcli` + `internal/boardengine`).
  One `board.json` store holds every entry, and each entry is a task or a note with labels; only a task runs.
  `types` and `labels` in `board.yaml` are maps from label to description, and `lyx board labels` prints both in file order.
  `lyx board intake` lists open inbox issues not yet on the board, imports one as a note or folds it into an entry, and closes noise with a stated reason.
  The README renders Tasks split into dependency layers whose entries can run in parallel, then Notes grouped by type label, and links each slug to its design doc.
  Agents use the board through the `ly:board` skill.
  ✅ Implemented.
- **config** — bare `lyx config` lists modules and verbs, `lyx config menu` picks a module to edit interactively, and `lyx config <module>` edits that module's config;
  `lyx config reconcile` reconciles all module config files against their live templates (dry-run by default, `--apply` writes atomically) except seed-only modules (today: `models`), which are materialized once when absent and never rewritten again since the file is operator-owned;
  `lyx config <module> --set key=value` (repeatable) writes one or more config values directly with no editor invocation, for scripts/agents that need a non-interactive path.
  A key under a module's declared open map adds or rewrites one entry, e.g. `lyx config board --set labels.quarry="glyphs and the quarry index"`. ✅ Implemented.
- **fabric** — the sole git-coordination module over the code and its records, unified over two `internal/gitrepo.Repo` instances: clone (the hub creator), dual-worktree add/remove, coordinated checkout (switches code and records together + re-points junctions), reconcile, status, prune, cleanup, records content-sync (commit/push/pull/sync/diff; in the prime, push, commit and sync push the records side only and `push` reports an unpushed code branch as `code_push_skipped`), and a merge/conflict lifecycle (`merge-in`/`merge`/`merge-stage`/`merge --continue`/`merge --abort`, mirroring git's own exit codes and surfacing conflicts as unified, worktree-relative paths; `merge-stage` marks resolved paths so `--continue`'s index gate can pass, and is the only route for a conflict under a wired junction name, which git refuses to stage through), all in one command tree (`internal/fabriccli` + `internal/fabricengine`);
  CLI surface is `lyx fabric clone|add|list|remove|checkout|pairs|reconcile|prune|cleanup|unwire|shortname|status|commit|push|pull|sync|diff|merge-in|merge|merge-stage`.
  `clone` takes the records URL first with the code URL optional, derived from the code binding recorded on the records main branch when omitted;
  `reconcile` backfills that binding for hubs whose records predate it.
  `clone` also takes `--shortname` to record the repo's shortname as `.lyx-shortname` on the records main branch,
  and `shortname [<shortname>]` prints the recorded shortname or records one on a hub that has none.
  `status` is the unified both-sides uncommitted-change view (`Fabric.Status`);
  `diff` reports the side-labelled changes since a given code SHA (`Fabric.Diff`).
  `pull` is now unified across code and records, not records-only: it fast-forwards the records first, then fetches and inspects the code, detecting a rebased/force-pushed code remote via ancestry and safely re-anchoring the records' correspondence to it when it is safe to do so.
  `remove` also deletes the pair's local task branch once both worktrees are gone, when its work is pushed or landed on the recorded parent, and keeps the branch with a reason in the result otherwise, reporting both as `code_branch_deleted` and `code_branch_kept_reason` on the envelope;
  `--force` never overrides that check.
  ✅ Implemented; see the `internal/fabricengine` package documentation for rationale.
- **ide** — one-shot VS Code launcher with interactive menu.
  `lyx ide spawn <prime>` opens a hub workspace (prime, `_board`, `_portals`) whose `settings` carry the prime's `.vscode/settings.json`.
  The file is lyx-owned and regenerated on each prime spawn, overwriting any edit made in it;
  a task slug still opens its bare folder.
  The generated `folderOpen` task is now the sequenced reed launch chain (`reed up` → `reed add --if-absent --unless-name orch` → `reed attach`) rather than a bare `claude`, with both binary paths (`lyx`, `claude`) stamped absolute at generation time.
  The add row's `--unless-name orch` keeps a folder-open on a prime whose state holds an orch strand, live, dormant or hidden, from stacking `claude` below it.
  `lyx ide spawn` regenerates `tasks.json` on every spawn, overwriting an untracked one, and keeps `settings.json` when present.
  It keeps `.vscode/` out of git through the repository's shared `info/exclude` at the anchor subpath rather than `.gitignore`, and leaves a tracked `tasks.json` alone with a warning.
  `lyx ide spawn` also seeds a `// lyx:begin` … `// lyx:end` block into the user's VS Code `keybindings.json`, forwarding the five Alt keys of reed's tmux bindings to the integrated terminal, and reports the outcome in its JSON result.
  It writes only between its own markers, and a file it cannot merge safely is skipped with a reason and never fails the spawn.
  The driven-pair variant batten uses writes an attach-only `tasks.json` (no `reed up`, no `reed add claude`) under the same rules. ✅ Implemented.
- **selfreport** — file bugs and enhancements against `Knatte18/loomyard` via go-github through `internal/githubclient`, filed by `lyx selfreport create <title>`, run by the operator or by an agent; the friction reflection is its only automatic caller, and no Go code files an issue itself.
  Credentials resolve from `GH_TOKEN`/`GITHUB_TOKEN` first, with the `gh` CLI (`gh auth token`) as a bounded, non-blocking fallback token source — not a hard prerequisite.
  Target repo is hardcoded;
  supports `--body` (or `-` for stdin) and `--label`;
  defaults to `bug`.
  Callable from any sandbox agent context with no config. ✅ Implemented.
- **reed** — **the window to the world**: tmux overlay + **strand** bookkeeping + render (`internal/reedcli` + `internal/reedengine` + `internal/reedengine/render`). Hosts every managed process as a strand, arranges them, persists to `.lyx/reed.json` (`lyx reed up|add|remove|status|list|attach|switch|resume|watchdog|down`). Every strand carries a hierarchical full name ([PATTERN-agent-name](../pattern/PATTERN-agent-name.md)), mirrored into its pane title and Claude session name and exported to its process as `LYX_STRAND_NAME` and `LYX_PARENT`, with the watchdog repairing drift; `list` is the hub-wide name directory. Built on what its proof-of-concept, `muxpoc`, proved first (layout checksum, bottom-dominant layout, env hygiene, native `--resume`); `muxpoc` has since been deleted, its job done. `reed attach` and `reed watchdog` are this module's two registered interactive-handoff exceptions ([PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md)): `attach` hands the operator's stdio to a `tmux attach-session` child in place, pre-flighting through booting the session if cold and checking server/session liveness, and `watchdog` is the detached per-hub resize daemon that blocks for its process lifetime — in both cases every fallible step runs pre-flight, on the envelope, and only the terminal-handover/daemon tail itself is exempt from emitting JSON. `add` and `attach` boot this worktree's session when none is up, with `up`'s own semantics — a bare substrate, with no persisted strand relaunched — instead of refusing; every other reed verb still refuses on a cold worktree. ✅ Implemented. See the `internal/reedengine` package documentation.
- **shuttle** — run **one** LLM agent as an interactive tmux strand over the file contract (`internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`; `lyx shuttle run|interrupt|send|state`). `lyx shuttle state` is read-only: it lists the session state of each running run (busy, idle-done, idle-stalled, asking, dead or unknown, with its cause, since and history), read from the run's files, in shadow mode where no stop or notice depends on it. `Stop`-hook completion is read off an events file and classified into three outcomes — `done`/`died`/`timeout`. A turn end without every output file never ends a run: `Wait` holds it and keeps polling the same agent until the outputs exist, the pane dies or the deadline passes, whatever the agent's reason (a question, a status report, a live `AskUserQuestion` call). An autonomous run notifies its parent once per held turn end through the orch notice queue, and an interactive run is answered in its pane; `lyx shuttle send` ends every message with a fixed tail. Background work is read off the Stop payload's `background_tasks` first, with the transcript as the fallback when the payload carries no list; a turn end with outstanding work is waiting, not held. `Attach` waits on a held run, and on a legacy `asking` record whose strand is still live, instead of respawning beside it. `died` is narrow: it means a strand reed STILL TRACKS has a pane that is not alive (or the provider never came up inside the startup window). Reed's strand table losing the strand entirely — a `reed down`/`remove`, a deleted or rebuilt `reed.json`, a worktree renamed under an in-flight run — is reported as a mechanism failure carrying the run's identity, never as `died`, because reed's bookkeeping going away says nothing about the agent, whose process is often still working in its pane. `PreToolUse` guardrails deny the in-process `Agent` tool in both interactive and autonomous runs (on by default, switchable off via `shuttle.yaml`'s `claude_deny_agent_tool`, and narrowed in a `ForkSubagents` run to permit fork subagents while still refusing every other subagent type), and `AskUserQuestion` too when the run is autonomous (`Interactive: false`, the default); a `PreToolUse(Bash)` guardrail denies `python`, `python3` and `python3.<minor>` in command position in every run mode (on by default, switchable off via `shuttle.yaml`'s `claude_deny_python`). The engine announces each deny it installs to the session's system prompt (`--append-system-prompt`, on launch and resume), so a session knows the tool is unavailable before it tries one. The engine sets the prompt-cache TTL per role on launch and resume from `shuttle.yaml`'s `claude_prompt_cache_ttl` and `claude_prompt_cache_ttl_roles` (template: 1 hour for `driver` and `webster`, 5 minutes for every other role). The provider is swappable behind an **engine** seam; Claude is the only v1 engine. Per-run `Model`/`Effort` knobs (`lyx shuttle run --model`/`--effort`; effort values `low|medium|high|xhigh|max`, empty = provider default) are engine-validated, not policed by `Spec.validate`. `Spec.Version` is a programmatic engine-validated version pin (claudeengine composes the pinned model id; no CLI flag — consumers drive it via the model-spec notation's `v=` param). Starting a run returns only once its provider is past its startup gates, and a provider that never comes up is torn down (run dir and last pane capture kept) and reported as an error from `Start`, or as `died`/`timeout` from `Run`. ✅ Implemented. See the `internal/shuttleengine` package documentation.
- **orch** — the hub orchestrator hosted as an interactive Claude session in a reed strand of the prime worktree, cycled before its context fills (`internal/orchcli` + `internal/orchengine`; `lyx orch start|status|refresh|distill|stop`, prime only). A detached watcher has the session write a note, then clears it and resumes it from that note (`refresh`) or compacts it in place (`distill`), never clearing or compacting without one. ✅ Implemented. See the `internal/orchengine` package documentation.
- **webster** — the implementer module: one long-lived Master session that reads the codebase and the whole plan once, then forks one implementer per execution batch in-session (Claude Code's Agent tool) instead of spawning a fresh reed/tmux strand per batch;
  bracket verbs (`begin-batch`/`record-batch`) drive each in-session fork (Master ends its turn while a backgrounded fork works and records the batch on the fork's completion notification, shuttle reading that turn end as waiting rather than done), and a genuine model escalation (recovery after a stuck/report-less fork) spawns a cold strand.
  Consumes the flat card-list plan (pinned in `contracts/stencils/loom/loom-template-plan.md`, the Plan producer's own stencil) via its own sole parser, `internal/planparser`, groups cards into execution batches via `internal/batcher`, a separate config-selected registry module webster consumes (profiles in `batcher.yaml`, the `cautious` cost profile by default), then runs those batches in card order, refusing a backward dependency derived from the cards' own `Targets`/`Uses` refs, records the partition in `state.json` at first init, and carries the plan's `## verify:` command as a must-pass gate on Merriam's strand: a failure is rerun once (a pass on the rerun records the identities as flaky;
  a run that exceeds the verify timeout is not rerun, since a hang is not flakiness, and fails the gate at once with the timeout, the log path and the log tail as findings),
  and a failure that survives goes back to Merriam, who fixes it through one fixer fork, up to `verify_gate_attempts` attempts (`internal/websterengine` + `internal/webstercli`). ✅ Implemented.
  `lyx webster verify` runs the same plan-level verify on demand through `internal/verifytree`;
  like the other read-only verbs it boots no reed session.
  `run` refuses a `--plan-dir` override that moves the plan off the mode's own default, in BOTH modes: Master's in-pane verb invocations are typed flagless from its stencil, so they resolve the default and would refuse against a plan they cannot see. Every other verb keeps honouring the override.
  In standalone mode (`lyx webster run --target-dir <repo>`, a plain git checkout with no hub), `run` boots its own private reed session in-process (`lyx reed up` is hub-only and cannot reach standalone's derived geometry), requires the plan at the derived state directory's own `_lyx/plan`, and renders every prompt's plan-directory references as the absolute state-directory paths the panes can actually read.
  The target is normalized to its repository root before anything derives from it, so running from a subdirectory drives the same state directory, reed session and plan as running from the root — the derivation hashes one spelling of the repository, never the directory the operator happened to stand in.
  `recover-batch` boots the same private reed session `run` does, because it spawns a cold recovery strand of its own; the read-only verbs still boot nothing.
  See [webster-spec.md](../contracts/specs/webster-spec.md).
- **planparser** — the sole parser of the on-disk flat card-list plan format (`_lyx/plan/`, see [loom-plan-spec.md](../contracts/specs/loom-plan-spec.md));
  no other package reads that tree directly, and it also declares where that tree *is* — the worktree-relative form (`PlanDirName`/`PlanDirRel`) and the absolute told-anchor form (`PlanDir`/`PlanOverview`), with the caller supplying the anchor path (`internal/planparser`). ✅ Implemented.
- **planglyph** — the sole owner of every `quarry.Repo` call (`Open`, `Resolve`, `DeltaGit`) and of the package-level `quarry.Name`, plus the resolve-backed validation pass layered on top of `planparser`'s pure checks: `planglyph.ValidateFormat`/`Validate` call `planparser.ValidateFormat`/`Validate` and append only resolve findings, composing rather than reimplementing (`internal/planglyph`). ✅ Implemented.
- **planindex** — the cgo-free seam over `planglyph`: the plan gates' finding types, `ErrQuarryUnavailable` and the `Index` and `Delta` interfaces a package receives instead of importing the package that links tree-sitter (`internal/planindex`). ✅ Implemented.
- **quarry** — the planner's only source of glyph spellings: four read-only repository queries over the current worktree's own glyph alphabet, each delegating to a `planglyph` wrapper and emitting quarry's own rendering verbatim, with no repository-path flag on any of them (`internal/quarrycli`; `lyx quarry toc|glyphs|resolve|expand`).
  `toc` answers a table-of-contents query for a path;
  `glyphs` answers the same query under quarry's frozen depth-all/symbols-on preset, projected into a flat, depth-first index — the spelling the planner copies verbatim, and with `--text` quarry's own one-line-per-symbol view, which a line filter can cut whole symbols from;
  `resolve` checks that one or more copied spellings, given positionally in one call, name something real, answering each glyph separately: it prints every answer and exits non-zero when any glyph is negative;
  `expand` reports a type glyph's own head plus every member whose owner chain begins with it.
  `delta` and `name` are deliberately absent — both are pipeline-internal, and `name` in an agent's hands is a glyph-spelling machine, the one thing the copied-verbatim rule exists to prevent.
  `quarrycli` is a named CLI/Cobra Invariant package-naming deviation: it imports `internal/planglyph`, not a `quarryengine`. ✅ Implemented.
- **discussionparser** — the sole reader of `_lyx/discussion/`'s on-disk format (the decision record's required sections and the support log's existence);
  it takes told absolute paths and declares no location of its own — deliberately unlike `planparser`, because `loomengine`'s accessors take a `*lyxcwd.Location`, which this stdlib-only leaf may not import.
  Consumed by `loomshed.NewDiscussionGate` (the Discussion-Write and Discussion-Burler rows' own gates) and by the `lyx loom validate-discussion` verb (`internal/discussionparser`). ✅ Implemented.
- **summaryparser** — the sole declarer of the final-summary artifact's filename and the sole parser of its format (see [final-summary-spec.md](../contracts/specs/final-summary-spec.md));
  it takes told paths and declares no directory of its own, and is stdlib-only so neither consumer depends on a producer.
  Consumed by `internal/landingshed`'s `Publish` and `Finalize`, by `internal/websterengine`, by the `description` gate on loom's `Describe` row, and by the `lyx loom validate-description` verb (`internal/summaryparser`). ✅ Implemented.
- **batcher** — the batchifier registry that groups a plan's flat card list into webster's execution batches, through named profiles in `batcher.yaml` picked by its `active:` key (template default: the `cautious` profile, the cost batchifier; an empty value resolves to identity, one card per batch); its own standalone configreg module, separate from webster's (`internal/batcher`). ✅ Implemented.
- **stencil** — the operator surface over the hub's producer-prompt stencils (`internal/stencilcli` + `internal/stencilstore`; `lyx stencil list|validate|diff|sync|promote`): `list` reports every registered stencil's board-copy path and edit state, `validate` reports marker mismatches between a board copy and its shipped default, `diff` shows upstream changes not yet taken or (`--all`/`--exit-code`) board edits not yet ported back, `sync` force-refreshes every stencil against the shipped registry even from a `-dev` build, and `promote` copies a board-copy edit back into the worktree's `contracts/stencils/` source tree. The port-back drift warning classifies a differing board copy five ways (hand-edited, source-ahead, both, neither, behind) and names the matching remedy: `promote` for a hand edit, a production deploy for a source-ahead copy, a manual reconcile for both, `lyx stencil sync` for neither, and syncing the worktree with main for behind, a worktree whose HEAD lacks the binary's build commit. `list` and `sync` also cover the deployed `contracts/specs` registry; `validate`, `diff`, and `promote` do not — a spec declares no markers for `validate` to compare, and `diff`/`promote` both need the worktree source directory a deployed spec deliberately does not have. ✅ Implemented.
- **loom** — phased orchestrator: drives its flat, ordered producer list (`contracts/recipes/loom-recipe.yaml`, assembled by `internal/loomrecipe`), each gated by a `Bouncer` review segment (`internal/loomcli` + `internal/loomengine` + `internal/loomshed` + `internal/loomrecipe` + `internal/shedverbs` + `internal/parentreview`; `lyx loom start|resume|run|step|status|pause|approve|reject|commit-records|validate-discussion|validate-plan|validate-description|lint-comments|review notify|delivered|approve|reject|circling accept|continue|decision add`, plus the `start` verb registered a second time as the bare root alias `lyx start`).
  `internal/shedverbs` owns the generic `run`/`step`/`status`/`pause` verb bodies loomcli arms; `internal/loomcli` hosts only `start` and the arming glue.
  `start` is the session bootstrap, performing four steps in order: resolve the recorded parent branch and seed+commit the status file with the records when it is absent; ensure the worktree's tmux session is up, keep or add the status strand on a go-driven run and remove any status strand on an llm-driven run, then spawn the per-hub watchdog daemon, best-effort; read this run's seed and, unless a driver is already alive, spawn the driver its recorded choice selects — the detached Go runner, or a Claude strand running the loom driver in this worktree's own reed session; and print the success envelope, never attaching a tmux client (`lyx reed attach` shows the session) — the operator's terminal is Selvage, not a strand. `step` brings the tmux session up and never adds or removes the status strand.
  `start` is also the one site that reads a recorded seed's driver value, per [PATTERN-driver-choice-single-site](../pattern/PATTERN-driver-choice-single-site.md).
  `start` resumes a parked loom driver by typing one line into its pane (returning once the line's delivery is verified), and refuses after a bounded wait when the pane is not ready, since the driver may have resumed on its own.
  A live driver over a run halted at a hand-back with no park marker yet is still writing its stop report, so `start` refuses with the retryable kind `driver_not_parked`;
  batten's Inner-Run retries it, at most once per `notice_probe_s`, without recording the approval as acted on.
  When `start` would spawn or resume a driver over a pair with an unfinished merge it refuses with the non-retryable kind `merge_in_progress`, listing the conflicted paths under `conflicts` and naming the remedy.
  `lyx loom resume` wakes a halted run's live, parked driver through the same branch, and a run awaiting at a review segment's Bouncer row with a pending circling decision takes that branch too, since its driver's step re-calls the Bouncer, which acts on the decision;
  it never spawns a driver, adds a strand or brings reed up, while `start` still resumes as before;
  it reads the driver strand from reed's sessionless directory, and refuses every other state with its way forward, as the loom section of [refusal-spec.md](../contracts/specs/refusal-spec.md) lists.
  `run` is the no-tmux escape hatch that runs the phase machine in the foreground, for debugging and CI.
  `step` bootstraps idempotently, exactly as `start` does, and drives exactly one producer through `shedengine.Shed`'s own `Step`, emitting a JSON envelope; it spawns no detached driver, making it the single-producer primitive an external supervisor drives.
  `status` reports the current phase as a single JSON envelope and, with `--watch`, tails it, printing a line only when the composed activity changes rather than once per poll.
  On a terminal `status` renders a human view instead, and `--json` forces the envelope there; `--watch --json` is refused.
  While a run waits on a verify, a shuttle wait or Discussion-Write's parent-review gate, `status` also carries a waiting note naming the producer, what it waits on and for how long.
  The envelope carries `run_id`, `progress` (the main-line position and bounce count over the recipe's producer graph) and `last_step` (the record `lyx shed step` keeps under `.lyx`).
  It also carries the build identity (`vcs_revision`, `vcs_modified`) of the `lyx` that ran that step, and `binary_changed`, true when the running `lyx` is a different known build.
  A routed `stuck` reads as `bounced to <row>` in `activity.last`.
  The run directory is named by the worktree slug (`_lyx/shed/<slug>/`), with `self` kept as an alias that resolves to it,
  and a legacy `_lyx/shed/self/` still resolves.
  `pause` requests a pause at the next producer boundary.
  `validate-discussion` runs the same checks Discussion-Write's and Discussion-Burler's own gates run, standalone, exiting 0 on a clean gate and 1 otherwise, with findings in the failure envelope so a writer agent can self-check before handing off.
  `decision add [<slug>] --by parent|operator --title <t> --decision <d> --rationale <r>` appends one design call made after the Discussion to the decision record under an `Added after Discussion` heading, re-runs the discussion check (restoring the record on a finding) and commits the record.
  It is refused while Discussion-Write is running, checks no caller identity and resumes nothing.
  `validate-plan` runs the same checks Plan-Write's and Plan-Burler's own gates run, standalone, with the same exit-code and findings-envelope contract, over the current worktree's plan instead of its discussion.
  `validate-plan --rework` runs the check `PR-Rework`'s own gate runs: the format-only checks over the whole new plan, plus a check that its `first_card` equals the card number the session was told.
  `validate-description` runs the same checks the `Describe` row's `description` gate runs over `_lyx/landing/summary.md`, standalone, with the same exit-code and findings-envelope contract.
  `lint-comments [--base <commit>]` runs `internal/commentlint` over the worktree: the fixed-column-wrapped breaks the working tree, or the range from `--base` to HEAD, creates in `//` comment blocks.
  Every card gate ends with it, and it only reads git and files.
  A PR-review wait at `PR-Gate` halts the run `awaiting`, the planned hand-off state: it behaves like `blocked` for resume but spends no bounce budget, triggers no friction reflection and raises no anomaly; a `blocked` or `failed` halt writes a Go halt note and reflects.
  A review segment escalates to the run's parent, also `awaiting`, for one of two causes: the judge's `CIRCLING` verdict, or a bounce budget spent without convergence.
  The Bouncer writes a brief and a one-line parent notice into its run directory and returns the notice as `parent_notice`; the loom driver relays it to the parent, whose one-shot fork reads the brief and decides.
  The fork settles with `lyx loom circling accept|continue [<slug>]`, which records the decision and resumes nothing, and resumes the run with `lyx loom resume`.
  A `continue` after a budget escalation runs exactly one more round without spending budget; the next round past the budget needs its own decision.
  A batten-driven run is not resumed by batten on such a decision, since batten's wait reads only PR-Gate approval and rejection records, so it is resumed the same way with `lyx loom resume` in the task worktree.
  `approve` records an operator approval of the open pull request when the run is awaiting (or blocked) at the gate and the local HEAD equals the PR's head, writing `.lyx/loom/approval.json` and removing a pending rejection; resuming with `lyx loom start` then lets `PR-Gate` return Done without a GitHub merge.
  `reject <review-file>` records the operator's findings (removing a pending approval), and refuses once the `PR-Review` segment's five rejection rounds are spent.
  `lyx loom start` then routes the run through `PR-Rework`, which starts a new plan generation and re-runs `Plan-Review`, `Webster`, `Webster-Review`, `Describe`, `Publish` and the gate.
  `Plan-Bouncer` skips its judge only for an exempt generation, one whose live cards are all `Prosa` on non-source files.
  Before its session runs, Go archives the live generation into the round's `prior-generation/` directory: the plan (cards, overview, amendments and any `archive-*/` rotation), Webster's run record, and the Plan-Review and Webster-Review run directories.
  The session then writes a whole new plan into the emptied plan directory, numbered on from the retired generation through the overview's `first_card` key, and reads the archived plan for context.
  Go records on the round whether the new generation is exempt from Plan-Review (every card Prosa on a non-source file) or required, commits the round in one records commit, and removes the pending rejection.
  Each rejection is its own rework round, keyed by the rejected head and the rejection time, so a second rejection at a head the previous round left unchanged still gets a round.
  A round counts as committed only when its `record.json` carries the class, so the archive's classless completion marker never skips the session.
  The `Webster` row commits the plan directory alongside its run record, so the generation `PR-Rework` archives is the one Webster built.
  A run halted at the gate re-runs only the gate on `lyx loom start`,
  so a fix committed by hand outside loom is pushed by the operator before `approve`, or goes through `reject` instead.
  `commit-records` commits and pushes the run's records through fabric, pushing the task branch along with them — the status file, the review round record, friction notes and drive reports — and is what the loom driver's end-of-session command runs after the driver writes its stop report, and what a loom-launched driver runs at a hand-back before it parks until `lyx loom start` resumes it; a tree with nothing to commit succeeds without a commit.
  Friction notes live under `_lyx/loom/friction/` and drive reports under `_lyx/shed/<slug>/drive-reports/`, both committed with the run.
  Besides the agents' notes, the friction directory holds the Go-written halt notes, which name their anomaly kind, the crash-resume notes written at a `run` or `step` entry and `lyx webster` refusal notes, and the driver's repair records.
  `lyx loom start` writes a handoff voucher right before it spawns a driver, so a deliberate resume never reads as a crash.
  The landing tail runs `Webster-Review`, then `Describe`, then `Publish`, then `PR-Gate`, then `Finalize`;
  `Publish` returns Done once the PR is open, refreshing its title and body from the description.
  `Describe` writes the change description at `_lyx/landing/summary.md`, the single source for the PR title and body and for the landing commit, whose message carries exactly one `Co-Authored-By` trailer that Go appends from `landing.yaml`'s `co_authored_by`.
  `PR-Gate` returns Done on a valid approval (matching the branch's current HEAD), and `Finalize` closes the open PR with a comment naming the landing commit, tolerates an already-landed parent, and marks the board task `done` after the parent merge.
  Every run transition before that writes the board task's status as `<state> · <producer>` (for example `awaiting · PR-Gate`), so the board README shows where each run stands; a failed board write only warns.
  `Publish` and `Finalize` each run the plan's verify command after a parent merge-in that changed the task tree, and halt Stuck on a failure, with the command's output in the loom verify-output log;
  a pending-verify marker keeps the gate armed across a resume until a verify passes.
  `landing.yaml` gains `describe` (the row's model), `describe_timeout_min` and `co_authored_by`; an existing hub takes their template defaults until `lyx config reconcile --apply` writes them, and an in-flight run parked past `Webster-Bouncer` is restarted rather than migrated.
  `lyx loom status --watch` is this module's registered interactive-handoff exception ([PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md)): it self-displays the polled status line then blocks forever as its own keepalive tail, with every fallible step running pre-flight, on the envelope, and only that tail exempt from emitting JSON.
  ✅ Implemented. loom's config module (`loom.yaml`, holding the `discussion`/`plan`/`review`/`fix`/`judge` role model-specs, where `review` and `fix` each take one model-spec or a per-round list, and `discussion_review`/`discussion_fix`/`plan_review`/`plan_fix`/`webster_review`/`webster_fix` override them per review segment, `discussion_timeout_min`/`plan_timeout_min`/`review_timeout_min`, `discussion_interactive`, `parent_review_wait_min`, `review_circling_checkpoint`, `review_max_bounces`, and `fix_start`, which is `parallel` or `after-review` and chooses when a review round's fixer starts) exists and reconciles via `lyx config reconcile --apply` (the bare verb is a dry run that only reports added and removed keys and writes nothing).
  The `review` pair is the review segments' own model and timeout, and lives here rather than in the recipe because the recipe is embedded in the binary and a recipe-literal model would be untunable without a rebuild.
  The Discussion producer: a prompt/profile fed to `shuttle.Run`, its prompt shipped as an embedded default in the top-level `contracts/stencils` package and read at call time from the hub's stencils directory (`contracts/stencils/loom/loom-template-discussion.md`), composed by `internal/loomengine`'s `prompt.go` + `discussion.go`.
  The producer runs in one of two modes, selected by `discussion_interactive`: autonomous by default, or interactive when the key is set, so an operator can interview the agent from its pane instead of it self-judging every answer.
  Both prompt renderings ship in the same stencil, selected by the `{{.mode_rules}}` marker.
  The Planner producer, the same way (`contracts/stencils/loom/loom-template-plan.md`), composed by `internal/loomengine`'s `prompt.go` + `plan.go`.
  See the `internal/loomengine` and `internal/loomcli` package documentation.
- **shed** — the generic outer phase-FSM `loom` and the eventual `Hardener` are each built on: a Go engine that walks one flat, ordered producer list, honoring resume, crash-recovery, and pause uniformly at producer granularity, with no predefined slots (`internal/shedengine`).
  The four shipped engine adapters — `SingleLLMProducer` over `shuttle`, the `Webster` adapter, the burler round producer, and the Bouncer (the generic review-gate producer rather than a wrapper over an engine) — live in one package, `internal/shedadapters`, alongside their shared context and archive helpers.
  No `lyx shed` verb of its own by design — a product's own CLI constructs a `Shed` with its own producer list and calls `Run`, and a bare verb would be a command with no list to walk.
  The skeleton (the loop, the status file, the `ShedProducer` interface) is ✅ **implemented**; the four engine adapters (`SingleLLMProducer`, the `Webster` adapter, the burler round producer, and the Bouncer) are ✅ **implemented** too, shipped as `internal/shedadapters`.
  `internal/shedcheck` is the shipped structural checker over an assembled producer list, enforced by a `go test` invariant over loom's own list rather than called from any production constructor — see its own package documentation for the finding kinds it reports.
  The Shed recipe group's engine registry (piece 1 of that group) is ✅ **implemented** too, as `internal/shedrecipe`; its `registry` map literal declares every engine name a recipe row may use.
  The recipe file format and the loader/builder shipped too, as `internal/shedbuild`, and loom's own conversion to a recipe file has now shipped as well: `contracts/recipes/loom-recipe.yaml` plus `internal/loomrecipe`, which assembles it into the `*shedengine.Shed` `internal/loomcli` runs.
  The generic verb set is ✅ **implemented** too, as `internal/shedverbs`: the `run`/`step`/`status`/`pause` cobra bodies every arming module builds its own subtree from, owning no path of its own and deriving nothing.
  The named-recipe `lyx shed` subtree is ✅ **implemented** as `internal/shedcli`, arming either `loom` or `batten` by run-id rather than by an explicit recipe name: each verb (`lyx shed run|step|status|pause [<run-id>]`, defaulting to `self`) reads the addressed run's own `seed.json` and looks its `recipe` field up in the table, and `lyx shed seed <run-id> --recipe <name> [--driver <name>] [--param k=v]` writes that first seed, with the driver defaulting to `llm` for a recipe that has a bootstrap verb (loom, whose unseeded `start` records `llm` too) and to `go` for one that does not (batten), refusing fabric's own checkouts (the Board checkout, a pair's other side) where no run is ever driven, and refusing a recipe's seed wherever that recipe's own verbs would refuse to drive it (a batten seed anywhere but the hub's prime worktree).
  The generic `step` envelope printed on stdout names the durable `trace_file` the invocation wrote and the `envelope_path` of the full envelope, which holds the run's `friction_dir` and `scratch_dir`, plus the reflection's `friction` status when non-empty, `status` names `trace_dir`, and fabric writes every recorded mutation to that trace.
  The driver stencil (`contracts/stencils/shed/shed-template-driver.md`) launches a session that drives any seeded run through `lyx shed step` recipe-blind, repairing failures from the trace and escalating what it cannot, with an orchestrator session forking one driver loop per run.
  The driver re-steps once on its own after a transient failure or a deployed `lyx`, and a loom-launched driver parks at a hand-back until `lyx loom start` resumes it.
  `internal/shedrun` is the sole declarer of the `shed` run-directory path segment (`_lyx/shed/<run-id>/` durable, `.lyx/shed/<run-id>/` ephemeral), the run-id vocabulary (including the default literal `self`), and the `seed.json` contract — the closed `recipe`/`driver` vocabularies and the sole `ReadSeed`/`WriteSeed`/`List` reader-writer, consumed by `loomcli`, `battencli`, and `shedcli` alike. See [PATTERN-shed-run-directory](../pattern/PATTERN-shed-run-directory.md).
  See the `internal/shedengine`, `internal/shedadapters`, `internal/shedcheck`, `internal/shedrecipe`, `internal/shedbuild`, `internal/shedverbs`, `internal/shedcli`, `internal/shedrun`, and `internal/loomrecipe` package documentation, and [the recipe format](../contracts/specs/shed-recipe-spec.md).
- **burler** — one review+fix round: two agents, a reviewer then a fixer, no self-grading, over the shuttle file contract (`internal/burlerengine` + `internal/burlercli`).
  The reviewer writes the review while the fixer orients, and the fixer waits on a Go-written ready marker before it validates and fixes the findings, disputing one only with evidence that its premise is false.
  Profile-driven: `{overlay, source}` fix-scope, tool-use; `loom.yaml`'s `review` and `fix` keys pick each half's model per round.
  Cluster review fans the reviewer out into N fork-subagent reviewers by naming a fan (`cluster-fan`) from the seed-only `burler.yaml` lens/fan library — never on by default.
  Strict frontmatter verdict parse;
  debug CLI `lyx burler run`, the read-only review-gate self-check `lyx burler validate-review <review-file>`, and the read-only capped wait `lyx burler await-review <marker-path>` on a round's review-ready marker. ✅ Implemented.
  See the `internal/burlerengine` package documentation.
- **hardener** — **DRAFT / concept.**
  Behavior-based reviewer that *runs* a live-substrate module (needs a sandbox repo) to harden it before merge;
  on-demand, post-loom, **off the spine**, shares only the `burler` round discipline.
  See the board's `hardener` note.
- **batten** — drives one task worktree's whole lifecycle — create, seed the child's own inner run from the Board task's own `recipe`, run it to a terminal state, and tear down — as a single Shed run from the hub's prime worktree (`internal/battenshed` + `internal/battenrecipe` + `internal/battencli`; `lyx batten run|step|status|pause <run-id>`).
  `lyx batten run <slug> --window` starts the same run in its own tmux window of the orch's reed session and returns at once; the window lives as long as the reed session.
  The Board task's `recipe` must be `loom` or empty: `Run-Shed` starts loom's bootstrap verb inside the task worktree, the only one that exists, so `Seed-Child` refuses any other registered recipe before writing a seed rather than committing one the child's own bootstrap would refuse.
  `Seed-Child` halts `blocked` with a named `stuck_reason`, not a hard failure, for all three ways `WriteSeed` can refuse: an unknown recipe, a recipe the task worktree cannot bootstrap, and a pre-existing child seed that disagrees with the one being written (`shedrun.ErrDisagreeingSeed`/`battenshed.ErrDisagreeingChildSeed`) — the same business-judgment treatment for all three, since none of them is a path-resolution or write failure.
  `Worktree-Create` is idempotent against its own post-condition, which is the whole pair fabric's `Add` builds, not merely the task worktree directory: a process killed anywhere inside `Add`'s own multi-step sequence never runs `Add`'s in-process rollback, so a bare directory check would be satisfied by that sequence's very first step alone. The row instead checks the pair is fully materialised (the task worktree's sibling exists, its junctions are wired, and its origin record names the parent branch `Seed-Child` passes to the child's bootstrap) before reporting done; a worktree present but not yet complete halts `blocked` naming a manual cleanup from prime (`lyx fabric remove --force <slug>`, which removes whatever part of the pair `Add` reached, and deletes the local task branch itself when its work is pushed or landed) rather than silently skipping the create.
  A create refused on a leftover branch (a torn-down pair keeps its task branch on the remote, and locally too unless `remove` proved its work pushed or landed) halts `blocked` with batten's own remedy — delete the branch locally and on the remote, and any orphaned sibling branch, then resume — never fabric's `lyx fabric checkout`, which from prime switches prime's own pair onto the task's branches.
  A child that is `awaiting` (a planned PR-review hand-off) is watched, not failed: the operator runs `lyx loom approve` or `lyx loom reject` in the task worktree, batten resumes the child itself once per operator decision, approval or rejection alike, while a review segment's escalation is settled with `lyx loom circling accept` or `continue` and then `lyx loom resume` in the task worktree, and a done child's driver is waited for (up to `driver_exit_grace_s`) before teardown.
  `Run-Shed` opens VS Code on the task worktree's anchor folder once per run, after the child's bootstrap first spawns successfully, with a `folderOpen` task that runs `lyx reed attach` into the child's own session.
  A once-marker in `Run-Shed`'s scratch directory keeps a later re-spawn (an approved or rejected `awaiting` resume, a running child whose spawn this batten process did not confirm) from reopening a window the operator closed,
  and a lost marker (a resume on another machine) opens it again.
  A failed launch (no `code` on `PATH`, a headless machine) is a warning only and never changes the row's outcome.
  A child that halts (`blocked`, `failed`, `paused`) makes `Run-Shed` wait, budget-exempt and without a time limit,
  and batten never resumes a halted child (it does restart an approved or rejected `awaiting` one): the operator resumes the child from inside the task worktree (`lyx loom resume`) and the watch carries on, with one warning per halt episode;
  the reason, naming `lyx loom start` too when no driver can be woken, is what batten logs and writes to the row's stuck-reason file,
  and `lyx batten status` does not show it while the child is halted;
  `lyx batten pause <slug>` stops the wait within one check.
  When a halted or awaiting child's reed state holds a dead driver strand, batten revives the pair's strands through reed resume once per halt episode per batten process, which brings the parked driver back with its own session and leaves the run halted and the park marker in place,
  so a later `lyx loom resume` wakes that driver;
  a retiring strand is left to `lyx loom start`.
  `Run-Shed` spawns the child's bootstrap while the task worktree has no status file, and again while that status is `running` and its spawn was not confirmed by this batten process: the confirmation marker holds the pid of the process that wrote it,
  so a restarted batten spawns the bootstrap again and a bootstrap that failed or was killed after seeding is retried on resume rather than watched as running for the whole budget.
  A running child with a live or retiring driver strand in its reed state, or a held run lock, is adopted instead: batten records the confirmation for itself and spawns nothing,
  so a second driver is never stacked,
  and the bootstrap itself is idempotent against a driver already alive.
  `Worktree-Teardown` is idempotent against its own post-condition just as `Worktree-Create` is: re-entered after its own removal already succeeded (a process killed before the transition persisted), it finds the pair gone and reports done.
  That post-condition is the pair's, not the task worktree's alone: fabric removes the task worktree before its sibling, so a removal interrupted between the two halves leaves the sibling, its portal and launcher entries, and both branches behind — re-entered there, the row halts `blocked` naming the leftover and `lyx fabric prune --apply` as the remedy rather than reporting done over the debris.
  Teardown first pushes an `archive/<slug>/<tip>` tag of the pair's other-side branch's tip to its remote, so the run's committed records stay reachable after the branch is gone, then deletes that branch locally and on the remote (the task branch stays on the remote, and locally unless `remove` proved its work pushed or landed), and a re-entered row whose worktrees are already gone finishes that deletion rather than reporting done over a surviving copy, which would make a later create of the slug refuse its push; a failed remote deletion halts the row `blocked`, and resuming once the remote is reachable retries it.
  A failed archive push halts the row the same way before anything is removed, and its reason names the resume.
  Teardown is one row sequencing session shutdown before worktree removal, and never forces: anything an agent left uncommitted or untracked in the child (a build artefact, a note) blocks the row after the session is already down, with fabric's own refusal as the `stuck_reason` — the operator cleans the child and re-steps, never `--force`;
  an uncommitted change in the pair's sibling, where the run records live, instead names batten's own recovery (`lyx loom commit-records` in the task worktree, then resume the batten run), and, should the resumed teardown refuse again, committing or removing the leftover by hand, since it is then not a run record;
  uncommitted content in the task worktree itself keeps fabric's refusal, and neither is ever `--force`.
  A fresh pair's records branch starts without its parent's `_lyx/shed/` run records, which fabric's `add` drops in the pair's first records commit, so a child never sees prime's batten records or any other run's.
  `Worktree-Create` and `Worktree-Teardown` wait for a prime lock another run holds, polling every 2 s for up to 10 min, and halt `blocked` only once that wait runs out.
  `step` drives exactly one producer forward from the run's persisted current producer, seeding a fresh run first when none is persisted yet — the same single-producer primitive `lyx loom step` is.
  `run` and `step` carry `--driver` (batten's own, `go`-only: batten has no bootstrap verb, so `llm` is refused by name) and `--child-driver` (the driver the task worktree's own inner run uses, `llm` by default or `go`); both are recorded into the run's write-once seed at first seeding, and an explicitly typed flag that disagrees with an already-seeded run is refused rather than silently dropped.
  `Run-Shed` waits on its child inside its own call rather than bouncing once per poll: it checks the child's status file every `poll_interval_s` (2 s) and returns only for a state change, an arm event, a pause or a failed status stat, each a budget-exempt `Stuck` that is its own history entry.
  Anything costing a process or a multiplexer round trip runs at most once per `notice_probe_s` (30 s).
  A running child is waited on without a time limit; `lyx batten pause`, honoured within one check, cancellation and the notices bound it.
  The history writes of those returns rewrite the run's durable `status.json` without committing it — the status commit skips a transition it has already committed — so prime's own pair carries an uncommitted change at `_lyx/shed/<slug>/status.json` while a watch goes between them.
  That is deliberate (the alternative is one identical commit per bounce), and it has one operator-visible consequence worth knowing: `lyx fabric checkout` refuses hub-wide while a batten run is watching, because it requires a clean pair.
  `status` bounds the history it reports to the most recent entries, alongside the true `history_length` and a `history_truncated` flag — the history grows only on a state change, a pause or a failed status stat — and surfaces the blocked run's own producer-supplied `stuck_reason`, kept for the `Run-Shed` block through the budget arm, whose persisted `error` stays the fixed budget literal (the other rows carry no `on_stuck`,
  so their own reason is the persisted `error`), plus the teardown row's `abandonedSession` — recorded by the producer rather than only returned,
  so a `step`-driven lifecycle reports it as well as a `run`-driven one.
  ✅ Implemented. See the `internal/battenshed` and `internal/battenrecipe` package documentation.

The cross-OS spawn primitive **proc**, and the generic outer phase-FSM **shed**, are the two remaining internal (non-CLI) layers — proc the base of the stack, shed the generic engine `loom` configures rather than a stack layer of its own;
see the [Execution stack](#execution-stack-orchestration-layers) section below for how proc / reed / shuttle fit together. (Earlier drafts split reed into separate `shed`/`glance` modules;
both folded back into reed — see the `internal/reedengine` package documentation. This `shed` is an abandoned earlier `reed` model/view draft, unrelated to `Shed` (`internal/shedengine`) the outer phase-FSM.)

The user-facing modules sit on a thin layer of shared infrastructure (`internal/configengine`, `internal/gitexec`, `internal/gitrepo`, `internal/lock`, `internal/logger`, `internal/output`, `internal/lyxcwd`, `internal/lyxdirs`, `internal/state`, `internal/shell`, `internal/verifytree`, `internal/verifyrun`, `internal/modelspec`, `internal/pattern`, `internal/friction`, `internal/buildinfo`, `internal/standalonestate`, `internal/segmentcolor`) — defined in [shared-libs/README.md](shared-libs/README.md). `internal/pattern` is the leaf that inlines the root `PATTERN.md` overview into the prompts of the agents that design, plan, edit or judge, for the roles in its own doc, and ships a format checker for `PATTERN.md`.
`internal/friction` is the leaf that returns the role-appropriate friction-note directive injected into all seven agent prompts when Tier 2 is enabled, with the note path composed by its own non-clobbering `NotePath`.
Above the engines sits a separate precondition-and-geometry layer, not the shared-infrastructure layer above: `internal/preflight` is the tier-1/tier-2 precondition layer (worktree geometry, worktree-pair cleanliness, Fabric readiness/sync), and `internal/hubgeom` and `internal/standalonegeom` are its hub-mode and told-mode constructors of the `Geometry` struct each engine is handed — see [PATTERN-told-geometry](../pattern/PATTERN-told-geometry.md).
`internal/cliwire` sits between the two: it is the CLI-boundary resolver a standalone-capable CLI calls once `preflight.ResolveMode` has chosen hub or standalone mode, and it hands each CLI the told strings — the resolved target, the derived standalone state directory, the resolved stencils and plan directories — that `internal/hubgeom` and `internal/standalonegeom` then build into a `Geometry` struct.
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
burler            one review+fix round: reviewer (+cluster) → fixer           [builds on shuttle] ✅
shed              generic outer phase-FSM: walk one flat producer list,        [stdlib +           ✅
                  honoring resume/crash-recovery/pause at producer granularity  internal/state,lock
                                                                                 only -- skeleton]
loom              phase machine: drive each phase through a Bouncer gate       [builds on shed,
                                                                                 burler]
```

The batten Shed nests loom's: it is its own recipe whose `Run-Shed` row seeds and drives a task's loom run as a child process and polls that run's own persisted status for the verdict, so there are two status files by design — the task's, committed on the task branch, and batten's own, durable under prime's own `_lyx/shed/<slug>/` — each resuming independently.
See [PATTERN-batten-bookend](../pattern/PATTERN-batten-bookend.md).

**Landing preconditions.** This task's relocation of run state requires no `lifecycle` or `loom` run in flight at landing: no migration reads or moves the old `.lyx/lifecycle/<slug>/`/`_lyx/loom/status.json` layouts, so a run left in flight under either old layout resumes nowhere afterward.
It also requires deleting any already-deployed copy of `contracts/specs/loom-status-spec.md` by hand: `internal/stencilstore` never overwrites a hash-mismatched file, with no force-sync carve-out for specs, so a deployed copy predating this task's path rename never refreshes on its own — delete it, and the next run re-seeds it fresh.

The whole stack runs **headless** (auto mode): strands exist (the interactive-session requirement), agents run, output files are read, nobody need watch.

The stack now has two entry modes, not one: every layer from `reed` up is **told** its geometry rather than deriving it.
`internal/hubgeom` and `internal/standalonegeom` are the two constructors that tell it — hub mode and told mode respectively — with `preflight.ResolveMode` selecting between them at a standalone-capable CLI's pre-run.
The consequence a reader needs: a producer verb therefore runs in a directory that is not a git repository, with no hub, no fabric, and no orchestrator status seed.
See [PATTERN-told-geometry](../pattern/PATTERN-told-geometry.md) for the rule.

- **reed is three things, and it is built** — an **overlay** over tmux, **strand bookkeeping** (a strand = one tracked process: a metadata record with a `guid`, `name`, worktree slug, parent, and a *generic* display spec),
  and a **render** sub-package (`internal/reedengine/render`, `layout = Rules(strands, box)`).
  Callers hand reed `{cmd, name, display}` where `display` is generic (anchor / focus;
  height is derived by one rule, not caller-set: every stack strand but the bottom-most gets `collapsed_rows` rows, default 3, in insertion order, and the bottom-most gets the rest) — never a domain `type`, so reed never learns what a "phase" or "cluster" is.
  Earlier drafts split the model and view into separate `shed`/`glance` modules;
  with one terminal per worktree they fold cleanly into `internal/reedengine` + `internal/reedengine/render`.
  `lyx reed remove --name <name> --detach` removes a strand by name from inside itself through a detached remover, and reed replaces a strand atomically in its slot.
  See the `internal/reedengine` package documentation.
- **skills and the parent directive** — a spawn's skills are named on the launch spec and typed by the provider engine before the prompt, never asked for by a stencil;
  every spawned role's opening stencil renders the parent directive, so a role escalates to its parent rather than the operator.
  See [PATTERN-role-skills-typed](../PATTERN.md) and [PATTERN-parent-directive](../PATTERN.md).
- **provider-invariant** — `shuttle` runs Claude today through an **engine**;
  the verdict/output contract is provider-invariant, so a different model can be swapped in without touching the review machinery.
  Non-Claude is not a current priority.
- **the bootstrap** — `lyx loom start` (alias `lyx start`) brings up the worktree's tmux session, on a go-driven run keeps or adds the `lyx loom status` strand (replaced in its slot when the live one was launched from a different `lyx` build) and on an llm-driven run removes any status strand the session still holds, spawns the per-hub watchdog daemon (best-effort), and spawns the driver the run's own seed selects: the Go driver **detached** (via `proc`, no TTY), or a Claude strand running the loom driver in this same reed session for the `llm` driver.
  When the `llm` driver is live and parked at a hand-back, it spawns nothing and instead types one resume line into that driver's pane.
  It then returns the success envelope and never attaches or switches a tmux client; `lyx reed attach` shows the session.
  Selvage is the operator's terminal, not a strand.
  A Go-driven loom run runs in the background;
  the reed view takes the foreground.
  A `.lyx/lyxrun.cmd` launcher makes it one click.
- `reed`, `shuttle`, and `loom` each get a user-facing `lyx <module>` CLI (`lyx shuttle run|interrupt|send|state` lets an operator or another process drive one agent standalone, before loom exists); `burler` is composed by loom's own review segments (`lyx burler run` is a debug-only wrapper, not a product verb), and `proc` alone stays an internal library with no CLI of its own.

### Following one spawn down the stack

loom wants a plan-reviewer for worktree `feature-x`:

1. `loom` → its Plan-Review segment's `Bouncer` — "review this plan against the discussion until clean."
2. the segment's `Burler`-round producer → `burler.Run(profile, priorFiles)` — "run one review+fix round."
3. `burler` → starts two shuttle runs, a reviewer and a fixer — "run one reviewer agent and one fixer agent."
4. `shuttle` → `reed.AddStrand{ cmd:"claude …", worktree:"feature-x", display:{anchor:below-parent, focus:true} }`.
5. `reed` records the strand in `.lyx/reed.json`, runs the command via `proc` in a pane, re-renders the layout (`layout = rules(strands)`), and applies it.
6. The `Stop` hook fires → reed notes the edge → shuttle reads the output file → returns to burler → the reviewer's review is accepted and burler writes the ready marker the waiting fixer is released by → the fixer writes its fixer-report → burler returns the verdict → the segment's `Bouncer` reads it, decides another round or exit → on a `CONVERGED` verdict returns `Done` → loom advances.

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

`internal/gitkit` is the below-fabric leaf and the home of test git plumbing — spawn, query and commit helpers, the hermetic git environment, and the primitive repo fixture `CopyRepo` — and asserts nothing itself; it never imports fabric.
`internal/hubforge` is the repo-wide real-hub fixture factory: it builds every hub fixture in the repo through `fabriccli.CloneAndWire`, never a hand-assembled stand-in, and asserts nothing about fabric either.
`internal/testkit` holds the shared test kits, one package per kit, for test support (fakes, builders, fixtures and the scan harness) used by two or more packages; [PATTERN-testkit](../pattern/PATTERN-testkit.md) states what a kit may import and assert.
`internal/testkit/scankit` is the harness every invariant scan runs on: module-root lookup, file walk, allowlists that report stale entries, a vacuity floor and an import-allowlist assertion.
`internal/testkit/plankit` is the one test-side writer of valid plans, rendering through `planparser`'s format constant, with the file-tree fixture glyph resolution needs.
`internal/testkit/tmuxkit` gives each test package one isolated tmux socket directory through `Main`, and each test a self-cleaning `-L` key through `Socket`.
`internal/testkit/stencilkit` seeds stencil fixtures from the registry through `stencilstore`, so seeded files carry production's hash stamp.

## Sandbox Hub

The **sandbox Hub** is a dedicated bench for manual testing of lyx's core workflows — dogfooding lyx against itself.
It lives on disk at `C:\Code\lyx-test-LYXHUB` and exercises the resolved `lyx` binary under test: the dev binary via `deploy-dev` into `.dev-bin` when present, else the production binary on PATH via `deploy.cmd`.
Build it via `sandbox/win/build.cmd` (`sandbox/posix/build.sh` on Linux/macOS), run the core suite via `sandbox/win/core-suite.cmd` (`sandbox/posix/core-suite.sh`, or the `reed-suite`/`reed-suite.sh` pair for the reed-specific suite, which needs live tmux), and collect the report via `sandbox/win/fetch.cmd` (`sandbox/posix/fetch.sh`) for either.
See [sandbox-howto.md](sandbox-howto.md) for the step-by-step runbook and [sandbox-hub.md](sandbox-hub.md) for topology and design details.

## Other docs

- `internal/loomengine`, `internal/loomcli`, `internal/loomshed` and `internal/loomrecipe` — the phased orchestrator (`lyx loom`);
  design.
- [code-comment-conventions.md](code-comment-conventions.md) — the doc-comment rule's standing rationale (Go only, for now);
  a durable convention doc, kept rather than deleted, moved here by the 2026-08-29 designs audit.
- [webster-spec.md](../contracts/specs/webster-spec.md) — webster's cross-module contract: the `_lyx/webster/` boundary, `outcome.yaml`, and `summary.md`'s writer-side additions (as-built;
  kept as a durable contract doc, not deleted on landing).
- [final-summary-spec.md](../contracts/specs/final-summary-spec.md) — the producer-agnostic final-summary artifact contract: its format, its validation, and its two consumers, `internal/landingshed`'s `Publish` and `Finalize` (as-built;
  kept as a durable contract doc, not deleted on landing).
- `internal/reedengine` package documentation — the window to the world: tmux overlay + strand bookkeeping + render (as-built;
  module doc deleted per the documentation lifecycle).
- `internal/shuttleengine` package documentation — run one LLM agent via a swappable engine over the file contract (as-built;
  module doc deleted per the documentation lifecycle).
- `internal/burlerengine` package documentation — one review+fix round: reviewer then fixer, no self-grading (as-built;
  module doc deleted per the documentation lifecycle).
- `internal/treadleengine` package documentation — the generalized round-loop engine (judge, gate, round-spawn, milestone cap ladder, judge-maintained handoff, pause, run-dir lock), with a pluggable `RoundRunner` seam a future consumer (Tenter) can drive (as-built;
  module doc deleted per the documentation lifecycle).
- The board's `hardener` note — **DRAFT/concept**: behavior-based hardening of a live-substrate module (post-loom, off-spine).
- [benchmarks/](benchmarks/board-performance.md) — board performance, tracked across revisions.
- [shared-libs/](shared-libs/README.md) — the shared infrastructure plumbing.
- [research/](research/) — design exploration (reed research logs).
- [reference/tmux_scripting.md](reference/tmux_scripting.md) — tmux command reference (vendored).
- The board — tasks and notes, the single home for unscheduled ideas too (no separate long-term-ideas file).
- [sandbox-howto.md](sandbox-howto.md) — operator runbook: deploy `lyx`, build the Hub, run the suite agent (procedure).
- [sandbox-hub.md](sandbox-hub.md) — the sandbox Hub: a dedicated bench for manual (dogfooding) testing.
- [crucible/README.md](../crucible/README.md) — **`crucible`**, the **serial review+fix loop**: a reusable method for hardening a live-substrate module before merge (orchestrator-driven, model-rotating, clean-room self-fixing rounds + independent verification).
  The hand-executed prototype of the review-gate + `burler` (see the `internal/burlerengine` package documentation) round loop (and the origin of the board's `hardener` concept, named separately to avoid colliding with it);
  ships two paste-ready prompts — an [orchestrator prompt](../crucible/orchestrator-prompt.md) (drives the loop + verifies) and a [round-agent prompt template](../crucible/review-prompt-template.md) (the reviewer-fixer), to instantiate per module.
  Lives at the repo root, not under `docs/`, since it's a working method/prompt set, not documentation of shipped code.
