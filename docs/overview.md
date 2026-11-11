# Overview: Loomyard

Loomyard is a Go toolkit of one-shot CLI modules.
Each invocation starts a process, runs one command, writes JSON to stdout, and exits — there is no daemon and no shared memory.
State lives on disk per module and is coordinated with file locks, so concurrent `lym` processes on a machine cooperate through the filesystem.
Its modules include **board**, a task tracker;
**fabric**, the git-coordination module over the code and its records;
and **reed**, the clean tmux overlay.

In the long term, Loomyard is intended to **replace mill/millhouse (Python)** entirely.
We get there by building these modules as self-contained toolkits first;
orchestration comes last.
See [Principles](#principles).

[loomyard-overview.svg](loomyard-overview.svg) is a conceptual figure of how a board task becomes a landed commit.
It runs from the dobby that starts the run to the landing, and it carries a glossary of the weaving terms.

Module path: `github.com/Knatte18/loomyard`

## Naming: `lym` (binary) · `loom` (orchestrator module) · `ly` (skills)

Three distinct names for three layers, non-overlapping so that no name means two things:

- **`lym`** — the binary/CLI, **L**oom**Y**ard e**X**ecutable.
  It is one binary with a namespaced subcommand tree (`lym board`, `lym fabric`, `lym loom`, …).
  It is the analog of millhouse's `millpy` backend.
- **`loom`** — the orchestrator *module* (`lym loom start`, `lym loom status`):
  the domain that drives the phased run, a module like `board` or `fabric`.
  See the `internal/loomengine` and `internal/loomcli` package documentation.
- **`ly`** — the skill / orchestration plugin (the analog of `mill`);
  skills are `/ly-*`.

**Never name skills `lym-*` or `loom-*`.**
Skills are `ly-*`, distinct from both the binary (`lym`) and every module (`loom`, `burler`, …), so no name is shared between a skill and a script or module.
Internal Go feature packages follow the `<module>cli` / `<module>engine` split, e.g. `internal/boardcli` + `internal/boardengine`.
The package-naming rule is in [PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md).

## Principles

1. **Toolkit-first.**
   Build small, composable primitives (board, fabric, reed) before any orchestrator that ties them together.
   mill's Agent Dispatch orchestrates for now.
2. **Self-contained modules, deep internal tests.**
   All of a module's domain logic and its test suite live in its own package.
   What modules share is a thin layer of infrastructure plumbing — see [shared-libs/README.md](shared-libs/README.md).
3. **One-shot, daemonless, file-coordinated.**
   A command does its work, writes JSON, exits.
   Processes cooperate through files + locks, not a server.
   The future reed daemon is the one exception, for crash recovery tmux can't self-detect.
4. **cwd-authoritative; cwd ≠ git-repo-path.**
   Config and state resolve from the current working directory, which need *not* equal the git-repo root.
5. **Full control, incremental milestones.**
   Land one milestone at a time;
   refactors are behaviour-preserving with the existing test suite as guardrail.
6. **Correctness by tool-design, not by recall.**
   A `lym` command makes the *correct* path the path of least resistance.
   It makes drift *detectable* (`status` / a future `doctor`) instead of relying on an agent or operator remembering a rule.
   No on-disk operation is un-bypassable when a shell is available.
   The achievable bar is "right path is easiest + mistakes are detectable," **not** "wrong path impossible."
   Hard blocks (hooks, permission rules) are brittle and out of scope.
   Example: `lym fabric` owns the overlay's git so raw `git -C` is never *needed* (it would be more work),
   and `lym fabric status` flags drift — but it is a friction asymmetry, not a wall.
7. **Go where it can be; LLM only for judgment.**
   Everything deterministic — verbs, control-flow, parsing, distillation, geometry, git — is Go.
   An LLM handles only the judgment a program can't:
   review verdicts, triage, batch implementation, an orchestrator's recovery decisions.
   The seam is consistent everywhere: **fat Go verbs** (`lym <module> <verb>`) are the callable surface;
   an LLM session *drives* them and consumes **Go-distilled digests, never raw prose**;
   any **skill is a thin human wrapper** over those verbs, never where logic lives.

## Cwd Resolution Invariant

**All cwd resolution goes through `internal/lymcwd`, and nothing else.**
`lymcwd` owns cwd resolution alone, never a records path, a junction path, or any per-module subdirectory;
the module that constructs each of those owns it.

`internal/lymcwd` exposes this contract:

- `Getwd()` — the only permitted call to `os.Getwd` outside `cmd/lym/main.go`.
- `Resolve(cwd)` → `*Location` — resolves the current cwd into a legal worktree's coordinates, applying the strict cwd gate.
- `ResolveWorktree(root)` — the ungated variant.
  It serves callers that hold a worktree root rather than an acting cwd; see `docs/shared-libs/lymcwd.md`.
- `ResolveWithin(cwd)` — accepts a cwd anywhere under the anchored directory or in the records sibling.
  It serves a read-only verb a session runs from wherever its harness moved the cwd.
- `NamedWorktree(hubPath, name)` → `*Location` — the coordinates of another worktree of the hub, at that worktree's own anchor.

`Location` carries `RepoName`, `HubPath`, `WorktreeName` and `AnchorRel`, plus the derived accessors `WorktreePath()` and `AnchorPath()`.
Every other geometry token is a per-module constructor, joined onto `Location`'s coordinates by the module that owns it.
The tokens are records paths, junctions, `_lym/<module>` and the hub-reserved name set.
See [PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md) for the full per-token ownership map.

`lymcwd.Resolve` proves that cwd is the root of a git worktree and nothing more.
It succeeds in any ordinary git repository run from its root;
in that case the `HubPath` and `RepoName` it returns are fiction.
Proving a worktree is lym-initialized and Fabric-wired is a different layer's job.
See [PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md) for the tier map.

**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside `internal/lymcwd` and `cmd/lym/main.go`.
`internal/lymcwd/enforcement_test.go` enforces the ban at `go test` / CI time.
It walks the entire source tree.
It fails the build if either literal token is found in any non-test `.go` file outside the allowlist.
`TestEnforcement_GeometryLiterals` in the same file enforces the per-token ownership map itself:
no policed geometry token may be constructed as a string literal outside its registered owner directory.
`TestEnforcement_FabricVocabulary` enforces the separate Fabric Vocabulary Invariant, which bars the two side names outside the owner set it names.
They may not appear in identifiers, string literals, or comments in production `.go` files.
They may not appear in the embedded agent prompt templates either.
The fabric-sense phrase form of `host` is banned everywhere this test reaches, including inside the owner set.
Examples are `host repo` and `hostBranch`; the bare word is not banned.
The test sits in this file as a walk-helper convenience, not because the vocabulary rule is `lymcwd`'s to own; see [PATTERN-fabric-boundary](../pattern/PATTERN-fabric-boundary.md).

See [PATTERN.md](../PATTERN.md) for details.

## Documentation lifecycle

Two doc classes, opposite lifecycles:

- **Module designs** for **planned, not-yet-built** modules live in their board entry's body.
  The task building the module writes the design into the package's `doc.go`,
  and it leaves its own entry's body and scope unchanged.
  The landing marks the entry done and the board prune removes it; the implementation and tests then become the source of truth.
  A module's purpose and key design rationale live in its Go package header comment, next to the code it documents.
- **Durable Go-to-Go contract docs** (`contracts/specs/`) pin cross-module schemas a real consumer honors.
  They are **kept**, not deleted on landing.
  `loom-plan-spec.md` has by-need companions, such as `loom-plan-checks-spec.md` (the validation-check catalogue).
  LLM-facing producer format contracts live in the producer's own stencil under `contracts/stencils/`.
  Examples are what `Discussion-Write` and `Plan-Write` must write.

The other durable documentation is this `overview.md`:
principles, naming, the module and shared-lib map, the records contract, and this lifecycle convention.

## Records overlay model

lym organizes overlay artifacts (configuration, task state, raddle docs, and the board) into a **records repo**.
The records repo is a companion git repository, separate from the project's own repo, which stays pristine.
The two-sided design is documented in `internal/fabricengine`'s package doc.

### Topology

```
<hub>/                              (top-level Hub, NOT a git repo)
  ├── <prime>/                      (code worktree, main branch; git repo root)
  ├── <prime>-<records>/            (records Prime worktree; git repo root)
  ├── <slug>/                       (additional code worktree; git repo root)
  ├── <slug>-<records>/             (records worktree for <slug>; git repo root)
  ├── _bolt/                       (the Bolt checkout, records main-branch worktree; holds board.json)
  │     └── .lym/                   (hub-wide machine-local scratch; a real dir, never a junction)
  └── <RepoName>.code-workspace     (the hub's VS Code workspace; lym-owned)
```

`_bolt` is hub geometry, and `_portals` and `_launchers` are retired hub containers, so none of them can be claimed as a worktree slug.
`lym cleanup` removes what a hub still holds under `_launchers`.
The check is `fabricengine.IsReservedHubName`.
`.lym` is slug-reserved too, but via `structuralNeverCommittedDirs` rather than as hub geometry.
It has no hub-level presence of its own.

### Git ownership

The **code repo** is the project's source of truth, maintained by developers.
All lym-specific artifacts live in the **records repo**, a separate git repository that lym controls.
This separation keeps code commits focused on project code and delegates lym infrastructure to the records.

### Artifacts location

| Artifact | Location | Repo | Purpose |
|----------|----------|------|---------|
| `_lym/config/` | Records worktree | Records | Live YAML configuration files for all modules (board, fabric); reconciled via `lym config reconcile`; a module resolves key by key through a layer stack, see [internal/configengine](../internal/configengine/doc.go) |
| `.env` | Records worktree | Records | Git-ignored per-machine environment variable overrides (KEY=value format) |
| `_lym/raddle/` | Records worktree | Records | A raddle run's directory: its proposal, cold proposal, approval record, state, staged store files and part records, reached through the `_lym` junction like every other `_lym` subtree |
| `_lym/usage/` | Records worktree | Records | A run's usage records: one file per agent session, the run's `run.json` and its `landed.json`, reached through the `_lym` junction like every other `_lym` subtree |
| `_bolt/` | Hub | Board | The Bolt checkout, a second records worktree, checked out on the code repo's own unsuffixed default branch (the records main branch in the common case) — never a separate clone, never a per-task records branch |
| Source | Code worktree | Code | Project source code |

### Durable vs ephemeral state (`_lym/` vs `.lym/`)

Two state roots with opposite lifecycles:

- **`_lym/`** — **durable, synced, portable.**
  Lives in the records repo (git-synced), so it survives a machine and transfers to another.
  Config, raddle, the board, and loom's orchestration **status** go here.
  Status is the current producer, run state, and per-producer-call history.
  Loom resume works across machines *because* its status is fabric-synced.
- **`.lym/`** — **ephemeral, local, machine-bound.**
  Untracked in both the code and the records repo, and changing constantly while a run is live.
  Each repo lists it in its own `.git/info/exclude`, never in a committed `.gitignore`.
  The live tmux runtime state goes here, because a pane ID or the tmux socket is meaningless on another machine.
  That is `reed`'s `.lym/reed.json`, described in the `internal/reedengine` package documentation.
  It holds the socket and session names and the strand table.
  The strand table lists each managed process with its session, parent, ephemeral pane id, and display spec.
  It is rebuilt by reconciling against live tmux on startup, never synced.
  A pane id is meaningless even on the same machine once the tmux server has restarted.
  Ids are server-global and restart at `%0`.
  So `reed.json` records the *pane generation*, the identity of the session incarnation its pane ids were bound against.
  It discards every binding minted against a different generation.

The test: **would this state mean anything on a different machine?**
Orchestration progress yes → `_lym/`.
A pane handle no → `.lym/`.

### Junction model

Each code worktree has a sibling records worktree.
Code worktrees use **junctions** (Windows) or symlinks to route writes into the sibling records worktree.
Worktrees are wired at `lym fabric clone` and `lym fabric add` time, with no separate setup step.
Each of them materializes junctions, `_lym`, and config in one call.

The wired junction set is not hardcoded, and it is not purely the repo-wide `pathspec` list.
It is `structuralCommittedDirs` ∪ `structuralNeverCommittedDirs` ∪ the hub-reserved-filtered config names, deduplicated.
The two structural sets, `_lym` and `.lym`, are injected in code and never read from `fabric.yaml`.
The third piece comes from the **repo-wide** `pathspec` list, recorded once at `<BoltDir>/_lym/config/fabric.yaml`.
It is read from the records main branch in the Bolt checkout, via `fabricengine.BoltDir`.
It is filtered against `fabricengine.HubReservedNames()`.
Those are the hub-structural tokens `_bolt` (the Bolt checkout), `_portals` and `_launchers`, which can never be a per-worktree junction.
`board.yaml` is hub-wide too: it is read and written at `<BoltDir>/_lym/config/board.yaml`, never in a worktree's `_lym/config/`.
A hub-wide module skips the worktree file in its layer stack, which [internal/configengine](../internal/configengine/doc.go) describes.
Because the pathspec is repo-wide, `lym fabric reconcile` converges **every** worktree to the same recorded set.
It adds a junction missing on disk, removes one absent from the wired set, and leaves one already correct alone.
No worktree carries its own drift-prone copy.
`lymcwd` itself stays config-blind;
it only resolves the cwd coordinates that `fabricengine` builds the junction records onto.
The junctions this repo ships with are placed at the repo's lym-anchor, `<code>/<anchor>/…`.
That is `<code>/` itself at the default `.` anchor:
- `<anchor>/_lym` → `<hub>/<slug>-<records>/<anchor>/_lym` (config junction, structural)
- `<anchor>/.lym` → `<hub>/<slug>-<records>/<anchor>/.lym` (machine-local scratch junction, structural)

Wiring also links agents' scratch directory, `<anchor>/.scratch` → `<hub>/<slug>-<records>/<anchor>/.lym/scratch`.
The link is excluded as `/<anchor>/.scratch` and is no junction of the wired set.
At a subpath anchor nothing is wired or excluded at the repository root.
`lym fabric reconcile` removes a root `.scratch` link and its `/.scratch` exclude line.

[PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md) holds the rule that no hub-level container is ever junctioned into a worktree.
The Bolt checkout (`<hub>/_bolt`) is one such container.

The optional `pathspec` default is empty.
A records-backed module is wired by appending its directory name to `pathspec`'s template default, with no `fabric`/`lymcwd` code change;
that mechanism applies to *optional* directories only.
A structural directory is never sourced from `pathspec`.

A raddle run's directory is anchor-level by design.
It lives at `_lym/raddle/`, reached through the `_lym` junction, with no `_raddle` junction of its own.

Every junction is listed in the code worktree's own `.git/info/exclude`, never in a committed `.gitignore`.
A tracked entry in the user's repo would advertise that LYM is in use.
The entry is the junction's own anchored path (`/backend/_lym`, or `/_lym` at a root anchor), never a bare name.
A slash-free gitignore pattern matches at any depth.
On a subpath-anchored monorepo it would silently untrack same-named directories lym never wired.
`.lym` additionally seeds `.lym/` into the **records** repo's own `.git/info/exclude` at wiring time.
Records-side scratch then never shows as untracked dirt.
Reads and writes through a junction are transparent to the CLI.
Code that writes to `_lym/config/loom.yaml` writes into the records repo without awareness of the indirection.

A real `.lym` directory already in a worktree is adopted rather than refused.
The first `lym fabric reconcile` moves its content into the records-side target and replaces it with the junction.
Several lym subsystems write `.lym` unconditionally, so such a directory is common.
`_lym` keeps the hard refusal (fabric never moves or deletes what might be the user's hand-authored content);
`.lym` is the one exception because its content is always lym's own machine-local scratch.

### Branch model

Records branches mirror code-repo branching.
A new records worktree's branch forks from the records branch named after the code worktree's branch at spawn time.
This guarantees subtasks (spawned from non-main branches) inherit the correct fork point:
branch isolation is **not** orphan-based but **merge-base-preserving** (each on its parent's timeline).
`_lym` is isolated by pathspec rather than by orphan topology:
junctions route it into the records, and the code repo's `.git/info/exclude` hides it.

### Records suffix convention

The records worktree for any code worktree is deterministic:
- Code: `<hub>/<slug>/` → Records: `<hub>/<slug>-<records>/`
- Code: `<prime>/` → Records: `<prime>-<records>/` (prime is the name of the main worktree)

The suffix is fixed and non-configurable, and `internal/fabricengine`'s package doc names it.
Records paths are computed on demand from geometry and do not require a registry.

### Status

- **Go implementation** (paths geometry, paired spawn, `lym fabric` command): implemented.
  `fabric` (paths geometry, paired `lym fabric add` spawn, and `lym fabric status|commit|push|pull|sync|diff|merge-in|merge|merge-stage`)
  is the sole git-coordination module.
  `status` is the unified both-sides uncommitted-change view.
  It also reports `merge_in_progress`, whether this pair has a fabric merge parked.
  Paired `lym fabric add` requires a records repo, which `lym fabric clone` builds; there is no separate hub-creator tool.
- **`lym config` command**: implemented.
  Bare `lym config` lists modules and verbs.
  `lym config menu` is the interactive picker, `lym config <module>` edits, and `lym config reconcile` reconciles.

```
github.com/Knatte18/loomyard/
├── cmd/lym/
│   └── main.go                   entrypoint: routes the <module> argument to a module
├── internal/adoptcli/            the adopt CLI command (`lym adopt`, the takeover of beam projects or root, and the runs under them, from another machine)
├── internal/agentconfig/         the hub-wide `agents.yaml` config: the skills each role lym starts loads, and the effort of its Claude sessions and their subagents by role and model, a leaf
├── internal/agentname/           the sole former, parser and validator of agent names (`<shortname>:<slug>:<role>`), a stdlib-only leaf
├── internal/beamcli/             the beam CLI command (`lym beam`, the making and starting of beams from board project entries, and the wiring of their dobby, the prime's carrying the hibernate request and bringing back the Slack relay)
├── internal/dobbyengine/         dobby, the daemon of a beam worktree: it starts released tasks, drives their batten runs in passes and spawns the board-tidy agent; the prime's dobby also starts `lym gate testmap` every few landings
├── internal/boardcli/            the board CLI command
├── internal/boardengine/         the board domain kernel
├── internal/hubissues/           the GitHub issues of the hub's own repository that `lym board intake` lists, reads, comments on and closes
├── internal/pullrequest/         the run's pull-request record and its GitHub calls, for PR-Review, Finalize and pair teardown
├── internal/cleanupcli/          the cleanup CLI command (`lym cleanup`, the routine cleanups of the hub prime's leftover state, a dry run by default)
├── internal/fabriccli/           the fabric CLI command (git coordination of code and records)
├── internal/fabricengine/        the fabric domain kernel
├── internal/gatecli/             the gate CLI command (`lym gate test`, the tier runner: the hub's test tiers over a scope, slotted, bounded, counted and logged; `lym gate status`, the holders of the hub's gate slots; `lym gate report`, the machine's run records totalled per slug and gate key; `lym gate testmap`, the per-package coverage maps rebuilt from the hub's prime worktree)
├── internal/helpcli/             the help CLI command (`lym help [<command>...]`, `lym help index`)
├── internal/shortcutcli/         the top-level operator shortcuts (`lym a`, `lym s`, `lym p`, `lym r`) from one embedded file
├── internal/idecli/              the ide CLI command
├── internal/ideengine/           the ide domain kernel
├── internal/ideconfig/           the hub-wide `ide.yaml` config: the files a VS Code window lym opens shows on start, per place, and whether a markdown file opens rendered
├── internal/pairteardown/        the one sequence that ends a pair: quiet wait or the skip's run-lock wait, refusal probe, owned-process end, loop and reed session end, then fabric removal
├── internal/reedcli/             the reed CLI command
├── internal/reedengine/          the reed domain kernel (overlay + strand bookkeeping)
├── internal/reedengine/render/   pure display-vocabulary leaf (Anchor, Display, ValidateDisplay)
├── internal/reportcli/           the report CLI command (`lym report convergence`, the read-only convergence report, `lym report timing`, the read-only timing report, `lym report cost`, the read-only cost report, `lym report raddle`, the read-only predicted-against-actual cost of each filed Sketch, and `lym report replies`, the read-only count of creel's expected replies, prime only)
├── internal/reportengine/        the report domain kernel (finished runs, wakes, stops, repairs, review rounds and landings for convergence, per-run time, waits, attempts and stop settlers for timing, per-cell dollars, hours and cost fits from the runs' usage records for cost, each filed Sketch's predicted cost against its parts' actual cost from the raddle store for raddle, read from git objects, and the per-role count of creel's expected replies, read from the creel log)
├── internal/orchcli/             the orch CLI command
├── internal/orchengine/          the orch domain kernel (config, stencil renders, persisted state, cycle state machine)
├── internal/orchroute/           the orch lookups below `orchcli`: the orch paths, the liveness of a worktree's orch, its strand name and the walk to the nearest live orch (`Asker`)
├── internal/ghissuescli/         the ghissues CLI command
├── internal/ghissuesengine/      the ghissues domain kernel
├── internal/selfreportcli/       the selfreport CLI command
├── internal/selfreportengine/    the selfreport domain kernel, filing LoomYard bugs to LoomYard's own repository only
├── internal/shuttleengines/      the engine registry: the named engines a shuttle config builds into a set, and the only production importer of a concrete engine
├── internal/treadleengine/       generalized round-loop engine (judge/gate/round-spawn/cap/pause/lock)
├── internal/shedengine/          generic outer phase-FSM: walks one flat producer list, honoring resume, crash-recovery, and pause at producer granularity
├── internal/shedtransient/       the one translation from lower-level failure classifications into the shed engine's transient mark
├── internal/seatengine/          the seat runner: starts, joins, probes, resumes and stops a chair and its advisors over shuttle, each seat in its own strand
├── internal/shedadapters/        the Shed engine adapters (Webster, the Burler review and fix row producers, MultiLLMProducer, and the Bouncer) over shuttle/websterengine/burlerengine/seatengine
├── internal/shedcheck/           authoring-time structural checker over an assembled OnDone/OnStuck producer graph
├── internal/loomcli/             loom's cobra module: the session bootstrap, arming `internal/shedverbs`' generic status and pause verb bodies, and mounting `internal/sheddriver` for `lym loom drive`
├── internal/loomshed/            loom's own row-name constants and producer constructors over `shedengine`
├── internal/loomrecipe/          assembles the `*shedengine.Shed` of the tapestry, darn, pawl, sampler, motif, raddle and arras recipes from their embedded files, and expands tapestry's and arras's `Motif` block from motif, the recipe that makes a sketch, the decision record
├── internal/shedrecipe/          the engine registry — the name to `ShedProducer`-constructor mapping a recipe loader resolves each row's `Engine` against
├── internal/shedbuild/           the recipe file format's loader and builder — decodes a recipe document and assembles the producer-definition list the shed engine already consumes
├── internal/shedverbs/           the generic run/step/status/pause/goto cobra verb bodies shared by every module that arms a `*shedengine.Shed` onto a CLI subtree
├── internal/sheddriver/          the shed layer's Go driver: its settle loop, rule table and hand-back, the tackler agent call, and the `shed` config module, told its paths and seams by the arming module
├── internal/shedcli/             the `lym shed` subtree: a named-recipe arming table for the tapestry, motif, raddle, arras, darn, pawl, sampler and batten recipes plus the three CLI seams that register it under the lym root
├── internal/statuscommit/        the shared per-transition status commit core (skip while mid-merge, commit hard-errors, push spawn warns) that `loomcli` and `battencli` wrap
├── internal/statuspane/          the pane mechanics the runs overview and the gate checklist share: a redraw-in-place pane loop and a lock-free status-file reader
├── internal/landingshed/         landing's general ShedProducers, Parent-Align, the PR-Review row and Finalize, shared by reference across producer lists
├── internal/pawlengine/          the pawl recipe's own producers, Beam-Up, Beam-Await and Beam-Down, the land request Beam-Await consumes and the beam's ready record, told their geometry and seams by their caller
├── internal/failuretrace/        traces a failed Parent-Align or Finalize verify log to the plan cards, batch commit ranges and untraced locations the landing failure record carries
├── internal/darnengine/          the darn recipe's hub-wide `darn.yaml` config and the verify command every darn verify reads
├── internal/desktophook/         the hub-wide `desktop.yaml` config and the machine's window command, run after `lym reed window` and `lym ide` open a window, and the machine's added status-bar buttons
├── internal/samplerengine/       the sampler recipe's hub-wide `sampler.yaml` config
├── internal/heddleengine/        heddle's instruction layers, the one lym ships in its binary and the repository's `heddle.json`, their rendering into a worktree's CLAUDE and AGENTS files, its `.claude/settings.local.json` and a session's system prompt, the Bolt checkout's `HEDDLE.md` overview of every entry, and the hub-wide `heddle.yaml` that turns the hub's overrides of shipped entries on
├── internal/driftengine/         regression around a task: the hub-wide `drift.yaml` config, the drift paths and records, the capture and compare runs and the Drift rows' producers
├── internal/driftcli/            the `lym drift` commands (`check`, `amend`, `capture`, `defer`, `status` and the hidden `capture-child`) and their worktree wiring over `internal/driftengine`
├── internal/envconfig/           the hub-wide `env.yaml` config: the environment variables lym exports to gate tiers and the shared build, Drift capture and compare, and agent sessions, with a machine layer that wins name by name
├── internal/languageconfig/      the hub-wide `language.yaml` config: the skills every code role loads after its own, the verify tiers the plan, review and implementer prompts render, and the hub's structure-aware rename tools the Webster implementer prompt renders
├── internal/lenses/              the `lenses.yaml` config: the lens and fan library burler, tapestry and darn read, holding only the names a repo tuned while the template, with the language layer a hub picks laid over it, supplies the rest, and the fan key grammar tapestry and darn share
├── internal/mergeresolve/        the merge-in and LLM conflict-resolution engine that the producers of `internal/landingshed` call
├── internal/frictionengine/      the aggregation-and-reflection step loom's terminal Friction-Reflect row runs, covering every note a halt left in the friction directory; no halt or stop runs a reflection of its own, and the step is re-entrant across a killed driving process
├── internal/hubgeom/             the hub-mode told-geometry teller that converts a resolved `lymcwd.Location` into each engine's geometry struct
├── internal/standalonegeom/      the told-mode geometry teller that builds each engine's geometry struct from told absolute path strings
├── internal/cliwire/             the shared standalone/hub wiring resolver for the standalone-capable CLIs, the layer that runs after `preflight.ResolveMode` has chosen a mode
├── internal/hubreconcile/        the hub config walk a start verb runs once after a binary change: build stamp, hub lock, reconcile and commit of every worktree's config
├── internal/preflightshed/       the general `Preflight` `ShedProducer` over `internal/preflight`'s tier-1/tier-2 checks, shared by reference across producer lists
├── internal/preflight/           orchestrator-agnostic tier-1/tier-2 precondition checks (geometry, worktree-pair cleanliness, Fabric readiness/sync) + the shared Report result type
├── internal/lymcwd/              cwd resolution entry gate (the sole owner of cwd resolution, nothing else)
├── internal/dotgit/              the one `.git` parser (git dir, common dir, repository test, walk-up to a worktree root), a stdlib-only leaf
├── internal/durfmt/              the duration formats the status panes and the timing report share, minute-resolution for growing times and second-resolution for closed spans, a stdlib-only leaf
├── internal/consoletitle/        the one join of a console window's title, `<T#> [<hub shortname>:<P#>] <name>` or `ORCH [...]` with a missing part left out, shared by VS Code and terminal windows, a stdlib-only leaf
├── internal/machinename/         this machine's name, read from the machine file in the per-user config directory before the hostname rule (the hostname cut at the first dot and lowercased), the sole reader of the hostname and of that file, a leaf over the standard library and `internal/standalonestate`
├── internal/machinecli/          the `lym machine name` command, its seams and the owner sweep
├── internal/sleepjournal/        the machine's suspends and hibernates read from systemd-sleep's journal entries, spawning no process, a stdlib-only leaf
├── internal/lymdirs/             the two directory-name tokens (`_lym` durable, `.lym` ephemeral), a zero-import leaf
├── internal/buildinfo/           the ldflags-stamped build channel, a zero-import leaf
├── internal/buildvcs/            the running binary's VCS identity, a stdlib-only leaf
├── internal/standalonestate/     target-path-to-hash8-and-state-directory derivation, also resolving the per-user config directory, a stdlib-only leaf
├── internal/segmentcolor/        the loom segments, lym's color palette and its tmux colors, a stdlib-only leaf
├── internal/selfexec/            the self-re-exec helper a long-lived pane process calls on a changed binary, and `LymBinary`, the one lookup of the running lym binary for a pinned or spawned command, which refuses a test binary; a leaf over the standard library and `internal/logger`
├── internal/runproc/             the owner record of a run's detached and long-lived processes, one file per process under the run's slug or the hub scope, and the one sequence that ends them, a leaf over the standard library, `internal/proc`, `internal/state` and `internal/logger`
├── internal/fswatch/             the one file-change event source over fsnotify, a leaf
├── internal/configengine/        shared config resolution
├── internal/gitexec/             shared git operations
├── internal/toolfloor/           the git and tmux version floor and its version-line parser, a stdlib-only leaf
├── internal/gitrepo/             typed Repo over one local git checkout: go-git for local reads, gitexec for remote-auth/mutation
├── internal/githubclient/        GitHub token resolution, caching, and authenticated *github.Client construction — auth only, no per-operation wrappers
├── internal/lock/                shared file locking
├── internal/lymtemp/             lym's per-user temp root in the system temp directory, one locked run directory per process and the sweep of dead owners' run directories, a leaf over the standard library and `internal/lock`
├── internal/gateslot/            the hub-wide pool of gate slots: file-lock slots, holder records, the inherited-slot environment and wait records
├── internal/cardverify/          the runner of a plan card's `**Verify:**` lines, shared by webster's record-batch re-run and `lym gate card-verify`: a gate call runs unslotted, any other line takes one slot under a timeout
├── internal/parentlock/          the per-branch parent lock: one record valid while its owner process lives, a ticket queue and compare-and-release
├── internal/tendercli/           the `lym tender` commands (`pause`, `resume`, `revive`, `status` and `reap`) and the network gate every lym process in a hub runs under
├── internal/tenderengine/        the pause, resume, revive and status sequences over seams: the entry, the grace, the Esc, the resume line and the revive line
├── internal/hibernatecli/        the `lym hibernate` commands (`request`, `cancel`, `status`), their hub wiring, the prime dobby's carrier and the `systemctl` spawn
├── internal/hibernateengine/     the hibernate request file, the submit, cancel and status sequences, and the carrier the prime's dobby runs the request with, over seams
├── internal/hibernateconfig/     the hub-wide `hibernate.yaml` config: whether the wake from a `lym hibernate request` lifts the hub pause that request added
├── internal/reportconfig/        the hub-wide `report.yaml` config: the run count a cell of `lym report cost` needs before it shows figures
├── internal/hubregistry/         the machine record of hubs in the per-user state directory, which maps a hub's shortname to its path, over `internal/state` and the Bolt shortname record
├── internal/creelconfig/         the hub-wide `creel.yaml` config: the shortnames of the hubs whose orch this hub obeys
├── internal/creelcli/            the `lym creel` commands (`post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold`, `release`, `hubs` and the hidden `daemon`, `hook-read` and `hook-wait`), their hub wiring and the delivery adapter over reed and shuttle
├── internal/creelengine/         the per-hub message daemon: the journal-folded pool of mail and queued injects, sender identity, reminders for a strand without hook delivery, the per-strand mail flags, the tool-boundary and Stop claim, the idle waiter and bell, injects, holds, the removed-holder sweep and the socket protocol, over a delivery seam
├── internal/slackcli/            the `lym slack` commands (`login`, `setup`, `start`, `stop`, `away`, `back`, `status` and the hidden `relay`, `hook`, `bar` and `toggle`), their pane check and hub wiring
├── internal/slackengine/         the per-hub Slack relay: the Web API client, the Socket Mode connection, the machine's credential and presence files and the hub's setup, forwarding, channel-map and relay-state files, the push-woken inbound read of each orch's channel with its presence commands, the outbound read of mail for SLACK, and the relay's run loop and double-fork start
├── internal/output/              shared JSON output
├── internal/modelspec/           model-spec parser + models.yaml registry leaf
├── internal/runprofile/          the hub's named run profiles in profiles.yaml: template, layered-load spec, loader and lookup
├── internal/pattern/             PATTERN directive leaf (root PATTERN.md inlined per role) + format checker
├── internal/friction/            the Tier 2 friction-note directive leaf, consumed by webster, burler, and loom
├── internal/shell/               provider-invariant pane-shell mechanics leaf (pwsh + posix)
├── internal/shellguard/          the one neutral table of the guards lym installs against shell commands, rendered by each engine
├── internal/commentlint/        the comment line lint: fixed-column wraps and over-long lines in the `//` comment blocks a diff creates, over a told worktree and base
├── internal/patternlint/        the PATTERN headroom lint: growth of the overview past its headroom line, over a told worktree and base
├── internal/impactset/          the impacted-set command a gate key's `diff` scope runs: the packages a diff reaches over the import graph, plus the `//lym:guard` tests, or a fallback reason to run the full verify
├── internal/testcover/          the per-test coverage engine: runs each test of a package alone under a coverage binary, keys blocks by content, judges contained and removable tests, reads and writes the per-package coverage map, and selects an importing package's tests from it or names the miss that runs the package whole
├── internal/dotnetimpact/       the `dotnet` scoper's derivation: the test projects and test classes a C# diff or a card's targets reach, by type-name closure over the source, or a fallback reason to run the tier whole
├── internal/verifytree/         the one plan-verify function: clean-tree check, verified-tree record and running marker, used by landing, both webster gates and `lym webster verify`
└── internal/verifyrun/          in-process shell-command runner behind `internal/verifytree` and webster's card-verify rerun
```

`cmd/lym` is `package main`;
everything else is in `internal/`.
`main` is the only thing that imports a module.

## Module dispatch

`cmd/lym/main.go` assembles all modules into a single cobra root via `newRoot()`.
Each module contributes a `Command() *cobra.Command` that is passed to `root.AddCommand(...)`,
so every module and subcommand is discoverable via `lym --help` without any central dispatch table.
The `help` module is the one exception:
`newRoot()` mounts it with `root.SetHelpCommand(helpcli.Command())` followed by `root.InitDefaultHelpCmd()`,
replacing cobra's built-in help command.
Adding a module takes three steps:
import the package, add `<module>.Command()` to `root.AddCommand(...)` in `newRoot()`, and append the module name to `root.Long`.
Every invocable command also carries an `audience` annotation (`operator`, `role` or `internal`).
`lym help index` renders the operator commands as a one-line-per-command index of the running binary.
The `ly:cli` skill carries that index, and the orch's role file points at it.
Which commands count as invocable, and so carry one, is the audience bullet of [PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md).
A tree walk in `cmd/lym/clitree_test.go` pins the form of `Use`, `Short` and `Long` beside the audience.

`run(args, out)` is the testable seam.
It builds a fresh root, merges stdout and stderr into `out`, and calls `root.ExecuteContext`.
It returns the process exit code without spawning a binary or trapping `os.Exit`.
Each module also exposes `RunCLI(out io.Writer, args []string) int`, which is `return RunCLIIn("", out, args)`.
It is an in-process test seam that drives a module in isolation without involving the cobra root.
Every module but `internal/selfreportcli`, which references `lymcwd` nowhere, also exposes `RunCLIIn(cwd string, out io.Writer, args []string) int`:
`cwd == ""` delegates to `clihelp.Execute(Command(), out, args)`,
and any other value delegates to `clihelp.ExecuteIn(Command(), cwd, out, args)`.
`ExecuteIn` seeds `cwd` into the execution context.
The module's handlers read it back via `lymcwd.CwdFrom(cmd.Context())` instead of the process working directory.
`clihelp.ExecuteIn`, `clihelp.RunRootCtx` and `clihelp.WrapRunCtx` carry a context into a command's execution.
They seed an explicit cwd or propagate an existing context instead of relying on the process working directory.
A handler reads the cwd back via `lymcwd.CwdFrom(cmd.Context())`.
`cmd/lym/main.go` uses `RunRoot`.

All commands print JSON: `{"ok":true, ...}` on success, `{"ok":false,"error":"..."}` on failure (exit code 1).
On a terminal, an `operator` command prints each envelope line as a short summary, rendered by `output.Summary`.
The summary gives the outcome, the fields, and each error, warning and way forward on one line each.
Long lists collapse to a count.
The global `--envelope` flag keeps the JSON on a terminal,
and output that is no terminal, such as an agent's or a script's, always gets it.
`clihelp.UseReadableOutput` makes the choice in the root's pre-run.

## Modules

User-facing modules each get one `lym <module>` namespace:

- **board** — the task-tracker board, which is also the roadmap (`internal/boardcli` + `internal/boardengine`).
  One `board.json` store holds every entry, and each entry is a task or a note with labels; only a task runs.
  `types` and `labels` in `board.yaml` are maps from label to description;
  `lym board labels` prints both in file order.
  `default_recipe` names the recipe a task with no recipe field runs, `darn` in the template.
  A Sketch, or any discussed note whose decision record is stored, becomes a task only as a raddle run's entry.
  A plain `lym board promote` of it makes that entry.
  `lym board raddle <slug>` prints the approved proposal from the raddle store on the Bolt.
  The store keeps each filed Sketch's proposal after prune.
  `lym board intake` lists the open issues of the hub's own GitHub repository not yet on the board.
  It imports one as a note or folds it into an entry, and it closes noise with a stated reason.
  The hub's repository is the one its recorded binding names, else its origin remote,
  so only the loomyard hub's intake reads LoomYard's issues.
  Intake never goes through selfreport.
  The README renders Tasks split into Running, Ready (waiting on nothing open) and dependency layers.
  The entries of one dependency layer can run in parallel.
  It then renders Notes grouped by type label, Sketches first.
  Each subsection is one numbered table that links each slug to its design doc.
  A discussed entry's row also carries a `Sketch` link to the decision record `decision-<slug>.md`.
  Every project, the root included as `P0` with its file `project-root.md`, renders through one project section.
  A section nests the projects under it, each with its own slug, age, owner, Tasks, Notes and Done.
  The README therefore shows every open project's entries split the same way, with the root's section last.
  Each of its top-level sections, the root's included, is a `<details open>` block the reader can collapse.
  A project file shows its project and the projects under it.
  An entry may carry a priority, `high` or `low`, absent meaning normal.
  The priority orders the entry first or last within its subsection.
  A task may name a run profile in its `profile` field,
  and `lym board profiles` prints the hub's profiles with their settings and each recipe's default profile.
  Agents use the board through the `ly:board` skill.
  A session keeping it for the operator follows the `ly:board-policy` skill:
  the orch, and the board agent the prime's `board` status-bar button opens for a short talk about one or a few entries.
  ✅ Implemented.
- **config** — bare `lym config` lists modules and verbs.
  `lym config menu` picks a module to edit interactively, and `lym config <module>` edits that module's config.
  `lym config reconcile` reconciles all module config files against their live templates.
  It is a dry run by default, and `--apply` writes atomically.
  Seed-only modules, such as `models`, are the exception:
  their file is materialized once when absent and never rewritten, since the operator owns it.
  `lym config reconcile --layer default` seeds absent board files sparse.
  It prunes but never fills a present board file that is not seed-only.
  For a seed-only board file such as `profiles.yaml`, it carries a template change in and keeps the operator's values.
  The change is made against the template base `<module>.template-base` beside the file.
  It lists the one-time base strip as pending; only a start verb's hub reconcile performs that strip.
  With `--apply`, run from the prime, it commits the changes in the Bolt checkout.
  `lym config <module> --set key=value` (repeatable) writes one or more config values directly with no editor invocation.
  It gives scripts and agents a non-interactive path.
  A key under a module's declared open map adds or rewrites one entry.
  For example, `lym config board --set labels.quarry="glyphs and the quarry index"`.
  A key that takes one value or a per-round list is written as a list when its value starts with `[`.
  Loom's `reviewer_model` and tapestry's `plan_reviewer_model` are such keys.
  For example, `lym config tapestry --set 'plan_reviewer_model=["opus[medium]", "sonnet[high]"]'`.
  A bracketed value that is not a list of scalars is refused.
  Every `--set` write is loaded through the module's own loader.
  A value the loader refuses, such as a list given to loom's single-model `plan`, is refused with the key named.
  The file is left as it was.
  A module resolves key by key through its layers.
  The embedded template comes first, then the repo default at `<BoltDir>/_lym/config/<module>.yaml`.
  The worktree's sparse file follows, and the untracked local override at `<anchor>/.lym/config/<module>.local.yaml` is last.
  See [internal/configengine](../internal/configengine/doc.go).
  A module's machine keys, such as gate's `slots` and `tier_parallel` and codex's `sandbox_mode`, also take a machine override.
  The override lives at `<BoltDir>/.lym/config/<module>.machine.yaml` and is never tracked.
  It is written from the prime beside the local override.
  `lym config --layer default|worktree|machine|local` picks the layer a verb addresses,
  and `lym config <module> --resolved` prints the merged result with each unit annotated by its layer.
  A board task's `run_settings` reach the pair's local layer at batten's Seed-Child.
  `configreg`'s `LocalKeys` and `LocalDenied` bound them.
  A task's `profile` names a run profile in the hub-wide, seed-only `profiles.yaml`.
  Seed-Child writes the profile's block for the task's recipe under the task's `run_settings`.
  A task naming no profile takes its recipe's default profile.
  The starting profiles are the ladder `speed`, `light`, `solid` and `wide`, plus the darn-only `docs`.
  `lym config profiles --set <name>='<json object>'` adds or replaces one whole profile,
  and a dotted key such as `docs.darn.loom.code_max_bounces=0` sets one value inside it.
  See [internal/runprofile](../internal/runprofile/doc.go). ✅ Implemented.
- **fabric** — the sole git-coordination module over the code and its records (`internal/fabriccli` + `internal/fabricengine`).
  It unifies two `internal/gitrepo.Repo` instances in one command tree.
  It covers clone (the hub creator), dual-worktree add/remove, reconcile, status, prune and cleanup.
  Coordinated checkout switches code and records together and re-points junctions.
  Records content-sync is commit, push, pull, sync and diff;
  in the prime, push, commit and sync push the records side only.
  `push` reports an unpushed code branch as `code_push_skipped`.
  The merge and conflict lifecycle is `merge-in`, `merge`, `merge-stage`, `merge --continue` and `merge --abort`.
  It mirrors git's own exit codes and surfaces conflicts as unified, worktree-relative paths.
  `merge-stage` marks resolved paths so `--continue`'s index gate can pass.
  It is the only route for a conflict under a wired junction name, which git refuses to stage through.
  CLI surface is `lym fabric clone|add|list|remove|checkout|pairs|reconcile|prune|cleanup|unwire|shortname|status|commit|push|pull|sync|diff|merge-in|merge|merge-stage`.
  `clone` takes the records URL first with the code URL optional.
  When omitted, the code URL is derived from the code binding recorded on the records main branch.
  `reconcile` backfills that binding for hubs whose records predate it.
  `clone` also takes `--shortname` to record the repo's shortname as `.lym-shortname` on the records main branch,
  and `shortname [<shortname>]` prints the recorded shortname or records one on a hub that has none.
  `status` is the unified both-sides uncommitted-change view (`Fabric.Status`);
  `diff` reports the side-labelled changes since a given code SHA (`Fabric.Diff`).
  `pull` is unified across code and records:
  it fast-forwards the records first, then fetches and inspects the code.
  It detects a rebased or force-pushed code remote via ancestry and re-anchors the records' correspondence to it when that is safe.
  `remove` also deletes the pair's local task branch once both worktrees are gone.
  It deletes the branch only when its work is pushed or landed on the recorded parent;
  otherwise it keeps the branch and gives a reason in the result.
  The envelope reports the outcome as `code_branch_deleted` and `code_branch_kept_reason`.
  `--force` never overrides that check.
  ✅ Implemented; see the `internal/fabricengine` package documentation for rationale.
- **ide** — one-shot VS Code launcher with interactive menu.
  `lym ide spawn <prime>` opens the hub workspace `<hub>/<RepoName>.code-workspace` (the prime, `Bolt` and every task pair).
  Its `settings` carry the prime's `.vscode/settings.json`.
  `lym ide spawn <beam>` opens the beam's workspace `<hub>/<RepoName>-<beam>.code-workspace` the same way.
  It holds the beam, its parent, `Bolt` and the beam's pairs.
  The files are lym-owned and regenerated on each prime or beam spawn, overwriting any edit made in them;
  a task slug opens its bare folder.
  `lym ide spawn` and `lym ide open` also open files beside the folder or workspace.
  The hub-wide `ide.yaml` lists them under `open_files` for the worktree's place: the prime, a beam or a task.
  Paths are relative to the worktree's anchored directory, and `{beam}` stands for the beam's slug.
  An entry naming no file is skipped.
  The template opens `README.md` for a beam and nothing for the prime or a task.
  `ide.yaml`'s `render_markdown` is on by default.
  With it on, a seed points the `*.md` entry of `workbench.editorAssociations` at the markdown preview,
  so a markdown file opens rendered.
  The entry lives in the worktree's `settings.json`, and a setting the operator holds in its array form is left as it is.
  The generated `tasks.json` holds one on-demand `reed attach` task.
  The `lym` path in it is stamped absolute at generation time.
  The task is reached through "Tasks: Run Task" or the seeded `ctrl+alt+a` key binding;
  no task runs on folder open, so opening a worktree in VS Code starts no session and no `claude`.
  `lym ide spawn` and batten's Seed-IDE row write this config through one shared path.
  `lym ide spawn` regenerates `tasks.json` on every spawn, overwriting an untracked one.
  In an existing `settings.json` it sets only the window title and colour keys lym owns.
  The title bar takes the project's colour:
  green for the prime and a task under it, and a beam's recorded `beam_color` for the beam and a task under it.
  The activity bar takes the worktree's own colour, which a task picks avoiding its siblings';
  `window.title` is the console title, read from the board when the config is written:
  `<T#> [<hub shortname>:<P#>] <slug>` for a task, and the orch's `ORCH [<hub shortname>:<P#>] <slug>` for a beam.
  The prime's is the orch's `ORCH [<hub shortname>:P0] root`, naming the root project rather than the worktree.
  The shortname is lower case, and `<P#>` is the handle of the beam above the task, else of its project, else the root project's `P0`;
  a part that does not exist is left out.
  A terminal window `lym reed window` opens takes the same title.
  A beam's own `<RepoName>-<beam>.code-workspace` carries the beam's settings, as the hub's carries the prime's.
  Each has the workspace look:
  `window.title` names the hub shortname, then the repository or beam followed by `workspace`,
  and the activity bar takes the workspace's blue-grey, which no worktree takes.
  `lym ide seed` writes the same config and, for the prime, the hub workspace file, and launches nothing;
  with no slug it seeds the worktree it runs in, the prime included.
  It keeps `.vscode/` out of git through the repository's shared `info/exclude` at the anchor subpath, not `.gitignore`.
  It leaves a tracked `tasks.json` alone with a warning.
  `lym ide spawn` and `lym ide seed` also seed a `// lym:begin` … `// lym:end` block in the user's `keybindings.json`.
  The block forwards the Alt keys of reed's tmux bindings through VS Code's integrated terminal.
  It also binds `ctrl+alt+a` to run the `reed attach` task.
  Both verbs report the outcome in their JSON result.
  A user-level binding applies in every VS Code window, so the keys reach reed in any hub.
  It writes only between its own markers,
  and a file it cannot merge safely is skipped with a reason and never fails the verb.
  Batten does not open it: VS Code opens only when the operator runs `lym ide`. ✅ Implemented.
- **selfreport** — files bugs and enhancements against `Knatte18/loomyard` via go-github through `internal/githubclient`.
  `lym selfreport create <title>` files one, run by the operator or by an agent;
  the friction reflection is its only automatic caller, and no Go code files an issue itself.
  It files LoomYard bugs only, from whatever hub lym runs in;
  an issue about another repository is filed to that repository's own issues.
  Credentials resolve from `GH_TOKEN`/`GITHUB_TOKEN` first.
  The `gh` CLI (`gh auth token`) is a bounded, non-blocking fallback token source, not a hard prerequisite.
  Target repo is hardcoded;
  supports `--body` (or `-` for stdin) and `--label`;
  defaults to `bug`.
  Callable from any sandbox agent context with no config. ✅ Implemented.
- **reed** — **the window to the world**:
  tmux overlay + **strand** bookkeeping + render.
  Its packages are `internal/reedcli`, `internal/reedengine` and `internal/reedengine/render`.
  It hosts every managed process as a strand in a window of its own or its group's.
  It persists to `.lym/reed.json` (`lym reed up|add|remove|status|list|attach|switch|resume|watchdog|down`);
  `lym reed add` places a strand `--anchor own-window` by default. Every strand carries a hierarchical full name (see the `internal/agentname` package documentation).
  The name is mirrored into its pane title and Claude session name, and exported to its process as `LYM_STRAND_NAME` and `LYM_PARENT`;
  the watchdog repairs drift.
  `list` is the hub-wide name directory. `reed attach` and `reed watchdog` are this module's registered interactive-handoff exceptions ([PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md)):
  `attach` hands the operator's stdio to a `tmux attach-session` child in place, pre-flighting through checking tmux's floor version,
  booting the session if cold and checking server/session liveness,
  and `watchdog` is the detached per-hub name-repair and reap daemon that blocks for its process lifetime.
  In both cases every fallible step runs pre-flight, on the envelope, and only the terminal-handover/daemon tail itself is exempt from emitting JSON. `add` and `attach` boot this worktree's session when none is up, instead of refusing.
  The boot has `up`'s own semantics: a bare substrate, with no persisted strand relaunched.
  Every other reed verb still refuses on a cold worktree.
  ✅ Implemented.
  See the `internal/reedengine` package documentation.
- **shuttle** — run **one** LLM agent as an interactive tmux strand over the file contract (`internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttleengine/codexengine` + `internal/shuttleengines` + `internal/shuttlecli`; `lym shuttle run|open|state|await|await-forks`).
  `lym shuttle open` starts a named session with no turn in its own window, which closes when the session exits,
  and each repeatable `--skill` is loaded in its one first turn;
  in a hub an opened Claude session carries creel's hook delivery and the creel trust sentence as a run session does,
  so mail reaches it through its hooks,
  and an opened Codex session runs under the lym Codex home with no shell guard and no creel hooks;
  `--model` takes a model spec naming the engine, model and effort;
  `--list-models` prints the model registry's aliases with their engine, model and default effort,
  and the output names the engine, model and effort the session got.
  Every session shuttle launches, run or opened, starts in its engine's bypass mode unless the caller passes `prompt`, which an interactive session accepts.
  `lym shuttle await <path>... [--cap]` is the read-only capped wait on one or more files, the keep-cache-warm wait any agent can run, needing no hub or git repository;
  its cap defaults to the caller role's keep-warm cap from `claude.yaml`.
  `lym shuttle await-forks [--cap]` is the same capped wait over the forks the calling agent session spawned, read from its transcript through the engine:
  a reviewer, fixer or writer that waits inside it takes every fork's result in one turn instead of waking once per fork.
  `lym shuttle state` is read-only:
  it lists the session state of each running run (busy, idle-done, idle-stalled, idle-fresh, asking, dead or unknown, with its cause, since and history), read from the run's files, in shadow mode where no stop or notice depends on it.
  `Stop`-hook completion is read off an events file and classified into the outcomes `done`/`died`/`timeout` and `api-error`.
  A spec with `APIErrorRetries` re-sends a turn the provider's API ended on an error that many times, then ends the run `api-error`;
  a turn end the engine reads as a model the provider does not know, Claude's `model_not_found`, ends the run `api-error` at once;
  a start whose skill-load hold read its cap of API errors ends `api-error` too, after the hold's back-offs.
  Any other turn end without every output file never ends a run:
  `Wait` holds it and keeps polling the same agent until the outputs exist, the pane dies or the deadline passes, whatever the agent's reason (a question, a status report, a live `AskUserQuestion` call).
  An autonomous run notifies its parent once per held turn end as creel mail to the orch,
  and once per permission or input dialog its pane shows past 60 s,
  and an interactive run is answered in its pane;
  a line typed into a run's pane goes through `lym creel post`, `lym creel inject` or `lym creel interrupt`, never a shuttle verb.
  Background work is read off the Stop payload's `background_tasks` first, with the transcript as the fallback when the payload carries no list;
  a turn end with outstanding work is waiting, not held, unless its only outstanding work is non-awaited shells, which a gated run holds at once with an output file missing, a gated run with every output file present counts done once they pass the shell wait bound after telling the agent to read them,
  and an ungated run holds once they pass the shell wait bound.
  `Attach` waits on a held run, and on a legacy `asking` record whose strand is still live, instead of respawning beside it.
  `died` is narrow:
  it means a strand reed still tracks has a pane that is not alive, the provider never came up inside the startup window, or the provider's process is proven gone behind a live pane shell (by the process registry, or a process-ending `SessionEnd` line past the wait's first liveness tick), with `done` instead when every output file exists.
  Reed's strand table losing the strand entirely — a `reed down`/`remove`, a deleted or rebuilt `reed.json`, a worktree renamed under an in-flight run — is reported as a mechanism failure carrying the run's identity, never as `died`, because reed's bookkeeping going away says nothing about the agent, whose process is often still working in its pane;
  the exception is a handle whose own `Stop` removed the strand, which ends `died` and is recorded as stopped unless the files show the run done.
  No guardrail denies the in-process `Agent` tool:
  a subagent or fork shows in the session's own pane and lym reads its transcript under the session's `subagents` directory.
  A `PreToolUse` guardrail denies `AskUserQuestion` when the run is autonomous (`Interactive: false`, the default);
  a `PreToolUse(Bash)` guardrail denies `python`, `python3` and `python3.<minor>` in command position in every run mode, a version check included (on by default, switchable off via `shuttle.yaml`'s `deny_python`),
  and a second `PreToolUse(Bash)` guardrail, installed in every run mode with no switch, denies a raw module-wide `go test`, `go build` or `go vet`, a raw `go test` over the `tmux` or `llm` tier, and any command naming the gate-slot or strand-name environment variable, steering the session to `lym gate test`, and to a package-scoped compile check the hub's `refuse_raw` does not refuse,
  and a third `PreToolUse(Bash)` guardrail, installed in every run mode with no switch, denies an `rm` on a computed path (a command substitution or a variable expansion in the command) so no permission dialog stalls the run,
  and a fourth `PreToolUse(Bash)` guardrail, installed in every run mode with no switch, denies `sed -i`, `perl -i`, and a redirect, heredoc or `tee` into a source file, steering the session to read a file with Read and change it with Edit or Write,
  and a fifth `PreToolUse(Bash)` guardrail, installed in every run mode with no switch, denies a headless `claude` launch with `-p` or `--print`, whose work shows in no pane, steering the session to a subagent through the `Agent` tool;
  the python deny's steer carries the same edit rule.
  The engine announces each deny it installs to the session's system prompt,
  and every Claude session it launches with a run directory also gets the creel trust sentence,
  through one `append-system-prompt.txt` file named by `--append-system-prompt-file` on launch and resume,
  so a session knows the tool is unavailable before it tries one and follows `[creel]` mail from its hooks.
  Every settings document the Claude engine writes, a run's and an opened session's alike, holds only what differs per session or role.
  What every session of a worktree shares comes from the worktree's heddle-rendered `.claude/settings.local.json`:
  Claude Code's `attribution` with empty commit and pull-request texts, so no session lym launches credits itself and a landing's credit block stays the one credit,
  prompt suggestions off, which the idle read needs, and claude.ai connectors, Claude Code's bundled skills and its `Workflow` tool off.
  While `claude.yaml`'s `session_scope` block is on, the documents scope what each session loads, over the user's and the repository's settings:
  an installed plugin is on only when the session preloads one of its skills, so the orch and the board session get the `ly` plugin in every hub and a writer does not, the block's `skills_off` list is off, a plugin that is on lists all of its skills, since no setting hides one skill of it,
  and every session but the orch's gets no auto-memory.
  Every agent keeps the same tool set, so no document denies a tool to one role.
  The skills a session loads come from the hub's `agents.yaml`, by the config role its spec names, followed by `language.yaml`'s skills for a code role;
  a launch whose model-spec names no effort takes `--effort` from `agents.yaml`'s efforts table by role and model.
  The appended prompt tells the session the explicit `effort` each Agent tool call takes and the skills a subagent's brief names,
  and the settings set `CLAUDE_CODE_SUBAGENT_EFFORT`, so a subagent whose call names no effort still runs at the role's.
  The worktree's `.claude/settings.local.json` carries the efforts table's default as `effortLevel` and as that variable,
  so a session started by hand in the worktree runs at it.
  The engine sets the prompt-cache TTL per role on launch, resume and open from `claude.yaml`'s `prompt_cache_ttl` and `prompt_cache_ttl_roles` (template: 5 minutes for `driver`, 1 hour for `webster` and `orch`, 5 minutes for every other role).
  `claude.yaml`'s `keep_warm_cap` (template 250s) and `keep_warm_cap_roles` set the per-role keep-warm cap beside them.
  While `claude.yaml`'s `batching_nudge` block is on, a `PostToolUse` hook after each `Read`, `Grep`, `Glob`, `Bash`, `Edit` or `Write` call runs the hidden `lym shuttle hook-batching`, which adds one line naming the breach and quoting the rule to a session above the block's context threshold whose recent messages make a chain of single lookups (a Bash call with a read-only lookup command such as `grep`, `find` or a git read counting as one), a sliced read of a short or already-read file, or a chain of edit-only messages;
  a subagent's or fork's call is judged against that agent's own transcript, context and nudge count;
  it never denies a call, and the block's switches and thresholds are config.
  The provider is swappable behind an **engine** seam;
  the engine is picked from the registry by `shuttle.yaml`'s `engine:` key, or per run by a model-spec naming it.
  The registered engines are `claude` and `codex`.
  `codex` drives the Codex CLI's TUI under one shared lym Codex home (`codex.yaml`'s `home`, linked to the operator's own login and sessions, its folder trust rendered by lym), with `-a never -s` and the mode `codex.yaml`'s `sandbox_mode` settles (`auto`, the template's, probes Codex's sandbox at each launch and runs `workspace-write` when it starts, `danger-full-access` with a warning when it refuses to start, as on a machine whose bubblewrap cannot create a user namespace, and refuses the launch when the probe itself cannot run; an explicit `workspace-write` refuses the launch there, naming the cause; a machine key the hub's machine override may pin) and, under `workspace-write`, as writable roots, the records checkout it is told through its config dirs (none outside a hub), the checkout's git dir and the common git dir it links to, so a session in a linked worktree can commit, and the folders of `codex.yaml`'s `writable_roots` that exist (template: the Go build cache);
  it loads skills as SKILL.md files the model reads by absolute path under `codex.yaml`'s `skill_roots`, and installs the shell guard rows of the `shellguard` table as `PreToolUse` hooks matching `Bash` with Codex's own steer and notice texts (`apply_patch` in place of Read, Edit and Write), the python and publish rows following their switches and the refuse-raw row a non-empty `refuse_raw` list;
  a Codex run in a hub carries creel's hook delivery, the mail handed over as a hook block after a tool call and at a turn end, an idle session woken by its async waiter hook through `codex queue`, and the creel trust sentence in its prompt file, while an opened Codex session stays a pane-mail strand;
  it has no fork audit, and the model registry's `sol`, `astra` and `luna` aliases name it.
  `lym shuttle run --model` takes the model spec `lym shuttle open` takes,
  so an alias picks its engine from the model registry and the bracket the effort;
  the model and effort are engine-validated, not policed by `Spec.validate`.
  `Spec.Version` is a programmatic engine-validated version pin (claudeengine composes the pinned model id; no CLI flag — consumers drive it via the model-spec notation's `v=` param).
  A prompt deferred behind the skill-load turn is typed again at a held turn end, or when `Attach` takes over the run, while no turn start carries it,
  so an agent that sat idle without its task gets it.
  Starting a run returns only once its provider is past its startup gates,
  and a provider that never comes up is torn down (run dir and last pane capture kept) and reported as an error from `Start`, or as `died`/`timeout` from `Run`.
  ✅ Implemented.
  See the `internal/shuttleengine` package documentation.
- **orch** — the hub orchestrator hosted as an interactive Claude session in a reed strand of the prime worktree or of a beam, cycled before its context fills (`internal/orchcli` + `internal/orchengine`; `lym orch stop|status|refresh|distill|withdraw|rules`, in the prime and in any beam).
  `lym beam start` first ensures the worktree's `runs` strand, which runs `lym batten overview` as the beam's window, named `beam: prime` or `beam: <slug>` opened with the beam's board handle as in `P1: beam: <slug>`,
  and `stop` leaves it;
  ensuring it adds the runs strand beside a `loom-status` strand a pawl run's `lym loom start` put in an overview window first,
  as the session's first overview window,
  and a failure to add, replace or record it is a warning on the bring-up report naming `lym beam start`, never a stop;
  the orchestrator has a window of its own,
  and the session's windows run the beam window, agents, shell.
  A detached watcher has the session write a note, then clears it and resumes it from that note (`refresh`) or compacts it in place (`distill`), never clearing or compacting without one.
  ✅ Implemented.
  See the `internal/orchengine` package documentation.
- **webster** — the implementer module:
  one long-lived Master session that reads the plan and the repo's rules once, never the code, then forks one implementer per execution batch in-session (Claude Code's Agent tool), each fork reading its cards' code itself, instead of spawning a fresh reed/tmux strand per batch;
  bracket verbs (`begin-batch`/`record-batch`) drive each in-session fork (Master ends its turn while a backgrounded fork works and records the batch on the fork's completion notification, shuttle reading that turn end as waiting rather than done),
  and a model escalation (recovery after a stuck/report-less fork) spawns a cold strand.
  Consumes the flat card-list plan (pinned in the plan-format blocks `contracts/stencils/loom/loom-block-plan-*.md`, which `contracts/stencils/loom/loom-template-plan.md`, the Plan producer's own stencil, includes) via its own sole parser, `internal/planparser`.
  It groups cards into execution batches via `internal/batcher`, a separate config-selected registry module webster consumes (profiles in `batcher.yaml`, the `wide` cost profile by default), then runs those batches in card order, refusing a backward dependency derived from the cards' own `Targets`/`Uses` refs.
  It records the partition in `state.json` at first init.
  It also carries the plan's `## verify:` command as a must-pass gate on Merriam's strand:
  a failure is rerun once (a pass on the rerun records the identities as flaky;
  a run that exceeds the verify timeout is not rerun, since a hang is not flakiness, and fails the gate at once with the timeout, the log path and the log tail as findings),
  and a failure that survives goes back to Merriam, who fixes it through one fixer fork, or on the gate's last attempt and after a fixer that committed nothing through the cold fixer `lym webster fix-verify` spawns at the recovery role, up to `verify_gate_attempts` attempts (`internal/websterengine` + `internal/webstercli`).
  ✅ Implemented.
  `lym webster verify` runs the same plan-level verify on demand through `internal/verifytree`;
  like the other read-only verbs it boots no reed session.
  `run` refuses a `--plan-dir` override that moves the plan off the mode's own default, in both modes:
  Master's in-pane verb invocations are typed flagless from its stencil,
  so they resolve the default and would refuse against a plan they cannot see.
  Every other verb keeps honouring the override.
  In standalone mode (`lym webster run --target-dir <repo>`, a plain git checkout with no hub), `run` boots its own private reed session in-process (`lym reed up` is hub-only and cannot reach standalone's derived geometry).
  It requires the plan at the derived state directory's own `_lym/plan`.
  It renders every prompt's plan-directory references as the absolute state-directory paths the panes can read.
  The target is normalized to its repository root before anything derives from it,
  so running from a subdirectory drives the same state directory, reed session and plan as running from the root;
  the derivation hashes one spelling of the repository, never the directory the operator happened to stand in.
  A `begin-batch` refusal with `plan_conflict` goes first to the cold plan fixer `lym webster fix-plan` spawns at the master role, which matches the cards the conflict names to the tree and rebaselines them,
  and only a conflict whose fix would change what a card does ends the run stuck.
  `recover-batch`, `fix-verify` and `fix-plan` boot the same private reed session `run` does, because each spawns a cold strand of its own;
  the read-only verbs still boot nothing.
  Loom's `Webster` row runs webster when `tapestry.yaml`'s `implementer` key, settable per task through `run_settings`, names `webster`, its only value;
  the row's `Implementer` engine reads the key,
  and webster partitions the plan itself at the row's start.
  See [webster-spec.md](../contracts/specs/webster-spec.md) and [implementer-spec.md](../contracts/specs/implementer-spec.md), the contract every implementer of that row keeps.
- **planparser** — the sole parser of the on-disk flat card-list plan format (`_lym/plan/`, see [loom-plan-spec.md](../contracts/specs/loom-plan-spec.md));
  no other package reads that tree directly,
  and it also declares where that tree *is*:
  the worktree-relative form (`PlanDirName`/`PlanDirRel`) and the absolute told-anchor form (`PlanDir`/`PlanOverview`), with the caller supplying the anchor path (`internal/planparser`).
  ✅ Implemented.
- **planglyph** — the sole owner of every `quarry.Repo` call (`Open`, `Resolve`, `EncloseAt`, `DeltaGit`) and of the package-level `quarry.Name`, plus the resolve-backed validation pass layered on top of `planparser`'s pure checks:
  `planglyph.ValidateFormat`/`Validate` call `planparser.ValidateFormat`/`Validate` and append only resolve findings, composing rather than reimplementing (`internal/planglyph`).
  ✅ Implemented.
- **planindex** — the cgo-free seam over `planglyph`:
  the plan gates' finding types, `ErrQuarryUnavailable` and the `Index` and `Delta` interfaces a package receives instead of importing the package that links tree-sitter (`internal/planindex`).
  ✅ Implemented.
- **help** — cobra's help command, replaced:
  `lym help [<command>...]` prints a command's help and refuses a word that names no subcommand,
  and `lym help index [--audience <audience>]` prints a generated one-line-per-command index of the running binary, filtered by the `audience` annotation every invocable command carries (`internal/helpcli`; the renderer is `clihelp.RenderIndex`).
  ✅ Implemented.
  See the `internal/helpcli` package documentation.
- **shortcuts** — `lym a` (`lym reed attach`), `lym s` (`lym adopt --here --no-orch`, then `lym beam start`, then `lym reed attach`), `lym p` (`lym tender pause`) and `lym r` (`lym tender resume`) are top-level operator shortcuts declared in `internal/shortcutcli/shortcuts.yaml`, embedded in the binary and added only by a commit to that file;
  global verbosity flags reach every expanded command and other words the last;
  they are not modules.
  See the `internal/shortcutcli` package documentation.
- **cleanup** — the routine cleanups of the hub prime's leftover state, as one verb (`internal/cleanupcli`; `lym cleanup [--apply]`, from the prime and from any beam).
  Without `--apply` it is a dry run that lists, per segment, what `--apply` would remove.
  The segments run in order and each calls its owner's own cleanup in-process:
  `lym batten cleanup`, `lym board prune`, the orch's handoff cleanup, the `.scratch/` children at the prime's anchor older than 14 days that no open board entry cites, logger's trace retention sweep, the temp root's sweep of dead owners' run directories,
  and fabric's sweep of the hub's retired `_launchers` directory.
  A failing segment never stops the later ones.
  ✅ Implemented. See the `internal/cleanupcli` package documentation.
- **adopt** — the takeover of beam projects, or of root, and of the runs under them from another machine of the operator, as one verb (`internal/adoptcli`; `lym adopt <beam>...`, `lym adopt root` and `lym adopt --all`, with `--keep-local` and `--drop-local`, from the prime and from any beam).
  The call runs no step itself: it validates the request, starts a hidden detached worker, `lym adopt worker`, and follows the adopt record, printing each step as it ends.
  The worker takes `adopt.lock` and writes the adopt record, `adopt.json`, as each step ends, so a killed worker resumes from the first step not done, a re-run of the same subjects follows or resumes it, and `lym adopt --discard` removes a record no worker will finish.
  `lym beam status` shows an unfinished adopt.
  `lym adopt --here` adopts the subject of the worktree it runs in, `root` in the prime and the beam in a beam: it follows or resumes an unfinished adopt that takes the worktree up, and otherwise pulls Bolt and ends `owned_here` when no other machine owns the subject; `lym s` runs it with `--no-orch` before `lym beam start`.
  `lym adopt --archives <subject>...` lists the `archive-owner/` tags the old owner pushed whose commit the slug's branch on origin lacks, and the adopt's end envelope names that call.
  The steps run in order: the worker reconciles the hub against origin, stops a beam or a run under it whose pair holds local commits origin lacks or is live, and adds the missing pairs.
  It pauses each moved run, and each unstarted released task of an adopted subject bar a front task, with an adopt pause entry.
  An adopt passes every pause, so a pause the operator holds does not stop its own network calls.
  Its reconcile waits up to a bound on a records branch's held push lock, so a deferred records push that is ending does not leave the branch busy.
  A record stopped at `done` restarts at `reconcile`, so the stops it held back are judged afresh.
  It then records this machine as the owner of the beams and of the prime in one pushed owner write, before anything else runs.
  When the call records the prime's owner, the landing config deploys into the prime's branch and the prime's head is ahead of the running lym's build, the hub's landing deploy runs next,
  and a failed deploy stops the record with the owner written and the runs still paused.
  The beam steps start each dobby and its orch, and `--no-orch` leaves the orch down; a call from an orch session passes it by default.
  The worker then starts one adopt check per adopted subject, detached, in that subject's worktree, which runs the hub's `adopt` gate there, reports each tier to the subject's orch, shows its tier in flight, time, tiers done and result on the prime's reed status bar, and shows in `lym beam status`,
  so a break this machine brings to the landed code shows at adopt time.
  A green check only reports.
  The adopt pause stays until an operator lifts it with `lym tender resume`.
  It calls each owning module's function in-process and has no engine package of its own.
  ✅ Implemented. See the `internal/adoptcli` package documentation.
- **beam** — the making, landing, starting and stopping of a beam, a long-lived pair that groups board work, and dobby, the daemon every beam and the prime run (`internal/beamcli` + `internal/dobbyengine`; `lym beam create <slug>`, `lym beam ready <slug>`, which flags the beam ready to merge so its dobby writes the land request once nothing works under it, and is final: no verb takes the flag back, `lym beam land <slug>`, which writes the land request the beam's pawl run consumes at Beam-Await and refuses a beam not flagged ready, `lym beam merge-parent <slug>`, which files a darn task in the beam, its Code-Review perch on, whose run merges the beam's parent branch into the beam, `lym beam start [slug] [--all]`, `lym beam stop [slug]` and `lym beam status`, from the prime and from any beam, and the hidden `lym beam dobby`).
  Create runs from the beam's parent worktree, claims the root when none is recorded, records this machine as a board project's owner with the beam flag and the pawl recipe, and starts the beam's pawl run as a detached first pass, whose Worktree-Create makes the pair, forked from the parent beam of a nested beam, whose Record-Write writes the project record from the project entry's body under `_lym/project/<slug>/`, and whose Beam-Up marks it.
  Before it starts that run, create removes a done batten run under the same slug whose pair is gone and whose child ran motif, reporting `motif_run_cleared`;
  any other done run is still refused, naming `lym batten cleanup --apply`.
  The project record binds every member:
  a member's Discussion-Write lead and darn writer read each enclosing beam's record from that beam's worktree, the board refuses an edit of the project's body once the record exists,
  and `lym loom decision add <beam-slug>`, run by the beam's orch, is the one way its scope changes, its notice reaching every live member run under the beam.
  The pawl recipe is a loom recipe whose rows are Preflight, Record-Write, Beam-Up, Beam-Await, Finalize and Beam-Down, with Beam-Await as its build row and no review round of its own.
  At Beam-Await the beam's orch works the landing with the operator through `lym beam describe`, which records the change description the orch writes, the end result and every member pull request, never the record's added entries, and updates a recorded pull request, `lym beam pr open`, `update`, `status` and `close`, and `lym beam ready`, which is final;
  Finalize then merges the beam into its parent, through the pull request the beam's landing record names when one is recorded, and keeps that pull request's text.
  Land starts a short batten run over the `beam` recipe, after a refusal that stops it while members or forked pairs remain under the beam.
  Start brings up a beam's dobby and orch, the one the acting worktree is or the one a slug names, or with `--all` the prime and every beam this machine owns, each part only when it is not live.
  Stop takes a beam's orch and dobby down; the prime's stop also ends its test-map refresh, whether or not its dobby is live.
  Status prints, once, the dobby section a beam's window draws, for the prime and for each local beam:
  alive or not, the runs it drives with their state, stage, time in stage and last pass, and each released task it has not started with what the task waits on, and the beam's orch: strand live, watcher live and the recorded session, then one adopt check item per subject, shown only when the hub holds a record of it: the subject, its state, the tier in flight, the tiers done and the paths of its log and output.
  Dobby starts the beam's released tasks from an empty status, drives their batten runs in short passes, sends the nearest live orch its own notices and the `[dobby notice]` lines of a running child (a dead driver, an API-error stall, a usage limit, idle agents), and spawns the board-tidy agent every few landings.
  The prime's dobby also starts `lym gate testmap --max <test_map_batch>` after every `test_map_every` landings, unless a refresh it registered as `runproc.RoleTestMap` still runs.
  It logs to its `dobby.log` the changes it sees, such as a task start, a run changing state and a failing pass episode, plus a heartbeat line every ten minutes.
  A front task, filed through `internal/fronttask`, is the one open task of its worktree that dobby starts first, past `max_runs`, while no other task of that worktree starts.
  Its `on_land` action runs once it lands or its entry is removed by hand with `lym board remove`, from a closed action table and on the subject's owner machine only.
  The verbs call each owning module's function in-process, and `internal/dobbyengine` holds the daemon.
  ✅ Implemented. See the `internal/beamcli` package documentation.
- **report** — the read-only convergence, timing, cost, raddle and replies reports (`internal/reportcli` + `internal/reportengine` + `internal/reportconfig`; `lym report convergence [--since <date>] [--fresh-days <n>] [--format json|markdown]`, `lym report timing [--since <date>] [--by day|week] [--format json|markdown]`, `lym report cost [--since <date>] [--format json|markdown]`, `lym report raddle [--format json|markdown]` and `lym report replies`, prime only).
  The convergence report says, per finished run and per ISO week, what woke the orch, what stopped and repaired runs, how many review rounds each segment took and how landings fared.
  The timing report says where run time went, per run, per phase and per day or week, with machine sleep, waits and the time a run stood paused apart, and who settled each stop.
  The cost report says, per recipe, profile and stored-record flag, what landed runs cost in dollars and hours, read from the usage records each run keeps, with a fixed cost and a cost per 100 changed lines fitted over the runs that landed code;
  a cell under `report.yaml`'s `cost_min_runs` reads "too few runs".
  The raddle report says, per Sketch a raddle run filed, the cost its approved proposal predicted against the summed cost of its filed parts' runs, the parts not yet landed and whether the operator changed the proposal, read from the raddle store on the board branch.
  The replies report counts, per receiver role over the creel journal's seven days, the replies creel was asked for, the replies it got, the chases it sent and the messages left unanswered.
  Convergence and timing cover tapestry and darn runs, each tagged with its recipe, and read records lym already writes.
  ✅ Implemented. See the `internal/reportengine` package documentation.
- **tender** — the one ordered pause that makes the hub safe to suspend within seconds (`internal/tendercli` + `internal/tenderengine`; `lym tender pause [<slug>] [--grace <seconds>]`, `lym tender resume [<slug>]`, `lym tender revive [<slug>]`, `lym tender status` and `lym tender reap`, from any worktree of the hub; `lym p` and `lym r` are shortcuts for the first two verbs).
  `lym tender pause` records an entry, waits a short grace for busy agent sessions and network operations to finish, interrupts every covered session still in a running turn with Esc and holds every path that would start agent or network work;
  the orchestrator is interrupted by its own watcher at the grace end.
  `lym tender resume` lifts the entry and gives each interrupted session one line to carry on.
  `lym tender revive [<slug>]` gives one fixed carry-on line to each session stalled on an API error, and the orch a checked creel inject, reported with its id;
  each send runs under the pause record's write lock, one at a time,
  so it types only while the session still reads stalled, sends no key and kills nothing.
  `lym tender status` lists the live entries, the network operations in flight and the parent locks held, every live session in the hub with its state, cause and whether an entry covers it, the lym processes running a replaced binary, the processes registered to runs with their orphans, and each pair's owner machine with the run-held entries owned by another machine;
  it changes nothing, and any caller may run it.
  `lym tender reap` ends the orphans of one fresh reading of that processes part, which are the processes of a slug with no pair and no running batten and the unregistered lym processes in a deleted hub directory, never a hub-owner process or one working inside a registered worktree;
  the orch and an external caller may run it,
  and a survivor makes it exit non-zero naming the pid and role.
  An agent may pause, resume or revive only its own pair; the orchestrator and an external caller may do each for the hub or any pair.
  Every lym process in a hub runs its network git and GitHub calls under a gate that waits while a pause covers it,
  and every reader that reacts to an unexpected stop or to idle time consults the record (`internal/pauserecord`).
  ✅ Implemented. See the `internal/tenderengine` and `internal/tendercli` package documentation.
- **hibernate** — a queued hibernate of the machine that lym's own Go code carries out once every session in the hub is safely paused, so no LLM session runs it (`internal/hibernatecli` + `internal/hibernateengine` + `internal/hibernateconfig`; `lym hibernate request [--after <slug>] [--after-all] [--resume-on-wake]`, `lym hibernate cancel` and `lym hibernate status`, from any worktree of the hub; Linux only).
  `request` records the hub's one request and returns;
  the trigger is at once, the end of one batten run's Land row, or no batten run in flight.
  The prime's dobby carries the request, and no beam's dobby does:
  it takes the request up from disk, again after a restart, waits for its trigger, pauses the hub through tender's engine, confirms every session paused, hibernates the machine, detects the wake and reconciles the hub with origin, then posts creel mail to the orch when it is done or failed.
  The wake lifts the request's own pause entry by identity only when the request resumes on wake, chosen by `--resume-on-wake` or else the hub-wide `hibernate.yaml`'s `resume_on_wake`, off by default;
  otherwise the hub stays paused until `lym tender resume` or `lym hibernate cancel` lifts it,
  and a new request is refused meanwhile.
  `cancel` ends the request and lifts the entry, and is refused while the machine is going into hibernation;
  `status` changes nothing and any caller may run it.
  Only the orch or an external caller may request or cancel; an agent asks its parent session.
  Bounds: the sequence waits a fixed window for the pause to confirm and for the wake, and fails the request, leaving its reason in the file, when either runs out.
  ✅ Implemented. See the `internal/hibernateengine` and `internal/hibernatecli` package documentation.
- **machine** — this machine's name (`internal/machinecli` + `internal/machinename`; `lym machine name [<name>]`: show, or set and sweep owners on every live hub and the hub of the working directory, from any directory).
  The show gives the name, whether it comes from the machine file or the hostname rule, the file's path and the former names the file records.
  The set writes the machine file and rewrites the owners recorded under the former names, on each swept hub, to the new name.
  ✅ Implemented. See the `internal/machinecli` package documentation.
- **creel** — the per-hub message daemon, the one way to reach an agent:
  mail to a Claude strand shuttle launched with creel's hooks arrives in full through those hooks, while every other strand, a hand-added one or a session launched without those hooks included, gets a typed notice or reminder and pulls its mail with `lym creel read`, and lines are injected into a pane (`internal/creelcli` + `internal/creelengine` + `internal/creelconfig`; `lym creel post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold`, `release` and `hubs`, from any worktree of the hub, and the hidden `lym creel daemon`, `lym creel hook-read` and `lym creel hook-wait`).
  A hook strand gets each post in full, quoted with its sender, at its next tool call in the `PostToolUse` hook's additional context and at its turn end in the `Stop` hook's additional context, which continues the turn and records no turn end, at most 6 a turn;
  the hooks test the strand's flag file, which creel keeps while the strand holds unread mail,
  and only the main thread takes mail,
  so a subagent's hook claims nothing.
  An idle hook strand is woken by an `asyncRewake` waiter the daemon rings once per claimable set, which wakes it with the mail itself, or by a typed fixed bell line where no waiter reaches,
  and the woken turn's hooks hand it the mail.
  Every line a claim hands out fits one hook field,
  so `lym creel post` refuses a longer one with the way forward of writing the text to a file and posting its path,
  and `lym creel post --interrupt` stops the receiver's running turn and then posts.
  The daemon also carries mail from a strand to a strand of another hub on the machine:
  the sending daemon queues the post, the receiving daemon confirms it with the sending one before any agent sees it,
  and the orch of a hub the `creel` config lists as trusted is obeyed.
  Every Claude session shuttle launches gets the sentence that tells it to follow `[creel]` mail and how to reply through one `append-system-prompt.txt` file;
  it names the session's own hub and the stamps of mail from another hub.
  The first verb of a hub starts its daemon, which folds its pool from an append-only journal, names a sender from the connecting process's ancestry, and delivers through shuttle only into a pane that reads idle.
  A post that expects a reply (`post --expect-reply`, or `--wait-reply`) is chased when its holder read it and did not answer:
  two content-free `CREEL` chases a quarter of an hour of the holder's running time apart, then a journaled `unanswered` outcome that answers a waiting `--wait-reply` and, for a sender that will read it, a notice.
  Notices and the orch's typed lines go through creel: batten, loom, burler, webster and hibernate post their notices as mail to the orch strand, signed as their own program.
  ✅ Implemented. See the `internal/creelengine` and `internal/creelcli` package documentation.
- **slack** — the per-hub relay between the operator's Slack channels and the hub's orchs (`internal/slackcli` + `internal/slackengine`; `lym slack login`, `setup`, `start`, `stop`, `away`, `back` and `status`, and the hidden `lym slack relay`, `hook`, `bar` and `toggle`).
  `login` stores the app's bot token and app-level token in a machine credential file in one run and `setup` records the hub's one operator, each after Slack confirms it;
  only the operator's terminal may run them,
  and an orch's pane may run `start`, `stop`, `away` and `back`.
  The app's scopes and events are the manifest in `docs/slack-app.md`.
  Each orch with a status bar, the prime's and each beam's, has its own `slack` button there, kept in the hub's forwarding file;
  the first press gives the orch a channel of its own, `<machine>-<shortname>-<project handle>-<project slug>` such as `neve-ly-p0-root`,
  which the relay makes, adopts when the bot made it, and joins,
  and a second press switches the orch's forwarding off and leaves the channel in place.
  The relay runs only while some orch of the hub has its button on, so the machine's connection follows the buttons.
  One relay of the machine holds the Socket Mode connection, and each event it receives wakes the relay of the hub whose channel it belongs to.
  The relay forwards the operator's messages in an orch's channel to that orch as creel mail from `<shortname>:SLACK` that carries their text alone, answers `away`, `back` and `status` in the channel, and posts the mail the orchs and programs leave for `SLACK` to their own channel, at least once, and a message Slack accepted is not posted again within one relay process.
  Each post goes under its sender's name in the cross-machine address form `<machine>::<sender>`.
  A channel is archived only when its orch retires, under a dated name, and `lym slack stop` archives nothing.
  It starts through a double fork, so no pane's tree holds it, and its files sit in the hub's `.lym/slack/` directory.
  The presence is the machine's, beside its credential file, so away holds for every hub and orch on the machine.
  The operator's message in Slack sets the presence to `slack`, and a line the operator types in an orch's pane sets it back to `terminal`, through the orch's prompt hook (`lym slack hook`), which tells those lines from the ones lym types.
  While the presence is `slack`, the turn-end hook of an orch whose button is on posts each turn's final reply to its channel.
  reed's status bar shows the session orch's button and the presence as the `slack` and `away` toggles; a click on `slack` flips that orch's button, and on `away` switches the presence.
  While the presence is `slack`, each worktree's dobby leaves `SLACK` one line for a run waiting at PR-Review or Sampler-Session, every `remind_after_min` minutes of the wait.
  ✅ Implemented. See the `internal/slackengine` and `internal/slackcli` package documentation.
- **gate** — the tier runner:
  `lym gate test` runs the numbered tiers the hub declares in `gate.yaml` over explicit targets or `--all`, each tier taking the hub-wide gate slots it asks for, bounded by its `timeout_min`, counted by its `go-json` or `vstest` counter, and logged with a summary line and a run record under the board;
  a `--card` scope under the `go` scoper also vets the card's packages under each other tier tag that adds a file to one of them,
  so a tag-set compile break fails at the card;
  a failing tier stops no later tier, vet or guard run;
  the gate key's lint steps run after the tiers;
  a gate key's advisory `redundancy` report step runs last, judging the diff's new and changed Go tests against the coverage maps without changing the result,
  and Code-Review's rubric reads its report while it matches the reviewed HEAD's tree;
  a failure report lists every failure with its package, test and first lines, the files the change touches and each failed run's log;
  a closing line names every failed run,
  and the call exits with the first failing run's code.
  A call from an agent strand without `--wait` runs in a detached runner it starts or joins, answers `done: false` and exits 75, which is no failure, when the caller's keep-warm cap passes before the run ended,
  and the same command again prints the run's result once it ended;
  `--wait`, and a call with no strand, run in-process and block until the slots free and the tiers end (`internal/gatecli`; `lym gate test [-C <dir>] [--gate <key>] [--tier N]... [--all] [--guards] [--wait] [<targets>...] [-- <args>]`;
  `--gate <key>` runs the tiers and scope `gate.yaml`'s gate key names,
  and with `--guards` the verb also runs every `//lym:guard` test of the module under the command of the tier its file belongs to).
  `lym gate card-verify <card id>` runs a plan card's `**Verify:**` line through the shared `internal/cardverify` runner and exits with the line's code.
  `lym gate testmap [--max <n>] [<package>...]` rebuilds the stale per-package coverage maps of `internal/testcover` from the hub's prime worktree:
  each tier0 and tier1 test runs alone inside one gate slot, bounded by `redundancy_build_timeout_min` and `redundancy_test_timeout_sec`,
  under a coverage binary over every in-module package its test binary links.
  A package with a modified or untracked Go file, or whose map another call is building, is skipped,
  and so is one whose linked packages hold a modified or untracked file just before its build or after its tests.
  A run inside a slot an enclosing gate run holds takes no second one,
  and a git worktree outside every hub runs unslotted under its own `gate.yaml`.
  A Go test binary that keeps a core busy past `busy_quit_sec` gets one SIGQUIT,
  so its goroutine stacks reach the log long before the tier's own timeout.
  The `dotnet` scoper narrows a C# hub's computed, `--base` and `--card` scopes to the test projects and classes that name a changed type, rendered into the tier's `{filter}` placeholder (`internal/dotnetimpact`),
  and a hub's `build` step runs once per call inside the slots of the first tier that sets `shared_build`, with its own summary line and run record.
  `inert_paths` makes a diff of files no test reads run no tier,
  and a tier with `carry: false` runs whole under every scope form but explicit targets and never carries.
  [dotnet-hub.md](dotnet-hub.md) sets a .NET hub up.
  ✅ Implemented.
  See the `internal/gatecli` package documentation.
- **quarry** — the planner's only source of glyph spellings:
  read-only repository queries over the current worktree's own glyph alphabet, each delegating to a `planglyph` wrapper and emitting quarry's own rendering verbatim, with no repository-path flag on any of them (`internal/quarrycli`; `lym quarry toc|glyphs|resolve|expand`).
  `toc` answers a table-of-contents query for a path;
  `glyphs` answers the same query under quarry's frozen depth-all/symbols-on preset, projected into a flat, depth-first index — the spelling the planner copies verbatim, and with `--text` quarry's own one-line-per-symbol view, which a line filter can cut whole symbols from;
  `resolve` checks that one or more copied spellings, given positionally in one call, name something real, answering each glyph separately:
  it prints every answer and exits non-zero when any glyph is negative;
  `expand` reports a type glyph's own head plus every member whose owner chain begins with it.
  `delta` and `name` are absent:
  both are pipeline-internal,
  and `name` in an agent's hands is a glyph-spelling machine, the one thing the copied-verbatim rule exists to prevent.
  `quarrycli` is a named CLI/Cobra Invariant package-naming deviation:
  it imports `internal/planglyph`, not a `quarryengine`.
  ✅ Implemented.
- **discussionparser** (`internal/discussionparser`) — the sole reader of `_lym/discussion/`'s on-disk format;
  it reads the decision record's required sections, the support log's existence and the relevant list's format.
  It takes told absolute paths and declares no location of its own, unlike `planparser`:
  `loomengine`'s accessors take a `*lymcwd.Location`, which this stdlib-only leaf may not import.
  `loomshed.ValidateDiscussion` consumes it, and through it `loomshed.NewDiscussionGate`:
  the own gates of the Discussion-Write and Discussion-Burler-Fix rows.
  The Darn-Write row's post-session guard and `lym loom validate-discussion` read it too.
  ✅ Implemented.
- **summaryparser** (`internal/summaryparser`) — parses the final-summary artifact and declares its filename.
  It is the sole parser of that format and the sole declarer of that filename.
  The format is defined in [loom-final-summary-spec.md](../contracts/specs/loom-final-summary-spec.md).
  It takes told paths and declares no directory of its own.
  It is stdlib-only, so no consumer depends on a producer.
  `internal/landingshed`'s `Finalize` and `internal/websterengine` consume it;
  so do the `description` gate on loom's `Describe` row and the `lym loom validate-description` verb.
  ✅ Implemented.
- **batcher** — the batchifier registry that groups a plan's flat card list into webster's execution batches.
  Named profiles in `batcher.yaml` define the groupings, and the `active:` key picks one.
  The template default is the `wide` profile, the cost batchifier;
  an empty value resolves to identity, one card per batch.
  It is its own standalone configreg module, separate from webster's (`internal/batcher`).
  ✅ Implemented.
- **stencil** — the operator surface over the hub's producer-prompt stencils:
  `lym stencil list|validate|diff|sync|promote`, in `internal/stencilcli` and `internal/stencilstore`.
  `list` reports every registered stencil's board-copy path and edit state.
  `validate` reports marker mismatches between a board copy and its shipped default.
  `diff` shows upstream changes not yet taken or, with `--all` or `--exit-code`, board edits not yet ported back.
  `sync` force-refreshes every stencil against the shipped registry, even from a `-dev` or unstamped build.
  It commits under its own `lym: sync <subtree> (...)` subject.
  `promote` copies a board-copy edit back into the worktree's `contracts/stencils/` source tree.
  The port-back drift warning classifies a differing board copy as hand-edited, source-ahead, both, neither or behind.
  A hand-edited copy takes `promote`, and a source-ahead copy takes a production deploy.
  A copy that is both takes a manual reconcile, and a copy that is neither takes `lym stencil sync`.
  A behind copy, from a worktree whose HEAD lacks the binary's build commit, takes a sync of the worktree with main.
  `list` and `sync` also cover the deployed `contracts/specs` registry; `validate`, `diff` and `promote` do not:
  a spec declares no markers for `validate` to compare,
  and `diff` and `promote` both need the worktree source directory a deployed spec does not have.
  The root pre-run seeds both subtrees only when a write is due.
  A board copy moves forward only, by the build ancestry or commit time recorded in its banner (the writer keys).
  It moves only from a clean, stamped production binary;
  an unstamped or dev build seeds an absent file and warns instead of refreshing.
  A due write pulls Bolt first and writes nothing when Bolt cannot be brought up to date.
  Each seed commit is labelled `lym: seed <subtree> (<revision> <channel> <executable>)`.
  The label names the binary that made it.
  A seed commit that conflicts at push is dropped and regenerated.
  ✅ Implemented.
- **loom** — phased orchestrator: drives its flat, ordered producer list, each row gated by a `Bouncer` review segment.
  The list is `contracts/recipes/tapestry-recipe.yaml`, assembled by `internal/loomrecipe`.
  The packages are `internal/loomcli` + `internal/loomengine` + `internal/loomshed`.
  Two more packages serve it: `internal/loomrecipe` and `internal/shedverbs`.
  The verbs are `lym loom start|resume|run|step|status|pause|commit-records|validate-discussion|validate-plan|validate-parts|join-parts|insert-card|remove-card|restore-renumber|validate-description|lint-comments|lint-pattern|fmt|circling accept|continue|decision add|peers|peers show|raddle-propose|raddle-approve`.
  `lym loom raddle-propose` checks the raddle agent's proposal against the format checks.
  `lym loom raddle-approve` records the operator's approval of a proposal that passed.
  `internal/shedverbs` owns the generic `run`/`step`/`status`/`pause` verb bodies that `loomcli` arms;
  `internal/loomcli` hosts only `start` and the arming glue.
  `insert-card`, `remove-card` and `restore-renumber` are the plan-editing verbs.
  They add or drop one card and renumber the cards after it through `internal/planparser`.
  A journal makes an interrupted call restorable.
  They refuse a position at or before the last card webster has begun.
  They hold webster's state-mutation lease while they run.
  A second embedded recipe, `contracts/recipes/darn-recipe.yaml`, drives a darn run.
  `Record-Write` writes the decision record and support log from the board entry, with no agent.
  `Drift-Baseline` records the regression scope from config with no agent and starts the baseline capture.
  `Drift-Await` holds the run until the capture passed or the parent deferred it.
  `Drift-Expect` seals the changes the task is expected to make.
  One gated `Darn-Write` writer reads the record as its scope, edits, tests and commits;
  its `drift` gate entry checks the code's changes against that expectation.
  `Drift-Verify` checks the writer's code by containment and has a judge rule on the expected changes.
  The `Code-Review` perch (`Code-Bouncer`, `Code-Burler-Review` and `Code-Burler-Fix`) reviews the committed diff.
  It carries its open findings into the record.
  `Drift-Recheck` then checks again, by containment and the judge, the code the perch's fixes committed.
  `Describe` rewrites the change description from the final diff.
  PR-Review and Finalize follow, with no discussion, plan, webster or Parent-Align rows.
  `Darn-Write` halts Stuck before any session when the record, or the support log beside it, is missing.
  It names the way forward: `lym loom goto --to Record-Write` then `lym loom resume`.
  When the working copy lost the record, the way forward is a restore of the committed record.
  A writer that finds the entry overtaken writes a stop report as its description and creates the stop marker.
  The marker halts the run after the session, with the report as the reason.
  The parent records its answer with `lym loom decision add`, and the run resumes.
  The perch is loom's own, with the same rows and rubric.
  It uses the `loom.yaml` `code_*` keys for models, fans, budget, hard cap and auto-accept.
  Only its answer key differs.
  In darn the answer key is the decision record alone:
  its `## Goal` holds the task's board entry, as amended by the record's `Added after Discussion` entries,
  and the record's other sections are the writer's own claim, checked against the amended entry.
  A `code_max_bounces` of `0` makes `Code-Bouncer` settle approved at once and `Describe`'s entry gate skip the row.
  The writer's own description then stands.
  A perch key found in `darn.yaml` moves to its `loom.yaml` key at reconcile and in a task's `run_settings`.
  A further embedded recipe, `contracts/recipes/sampler-recipe.yaml`, drives a sampler run.
  Its `Record-Write` row is as in darn.
  Darn's Drift rows sit around the build row as they sit around darn's writer, off by default:
  each carries the entry gate `drift.enabled` and `drift.scope == full`,
  so a task switches them on with `{"drift": {"scope": "full"}}` in its `run_settings`, and otherwise each is recorded skipped.
  With them on, the `lym drift` verbs serve the run's worktree, and a Drift Stuck routes the code fix to `Sampler-Session`.
  Then one `Sampler-Session` row keeps a single interactive agent session in the run's worktree.
  The agent works with the operator and commits as it goes.
  The operator ends the build with `lym loom sampler-done`.
  The row's session is kept like PR-Review's, and the row wakes on the file event of its record.
  `lym loom sampler-done` writes the done flag that ends the row, at the row's current entry stamp.
  The operator or the agent runs it on a committed worktree.
  The verb refuses, by the recipe's rows, on a recipe with no `SamplerSession` row.
  The `Code-Review` perch follows, left out unless `sampler.code_review` holds:
  its rows carry that entry gate, so with it off each is recorded skipped and the run goes on to `Describe`.
  `Describe` always runs, because the agent writes no description.
  Then PR-Review and Finalize, with no discussion, plan, webster or Parent-Align rows.
  The build row runs no verify gate, and Finalize's gate checks any code the run lands.
  `loomcli` reads the run's seed once at arming and builds, routes and wires the recipe the seed names.
  A run with no seed gets `tapestry`.
  `lym loom start` never rewrites a recorded recipe.
  The `lym loom` verbs serve loom runs only:
  a recipe declaring `contract: loom` is served, among them tapestry, motif, raddle, arras, darn, pawl and sampler.
  A run of any other recipe is refused, naming the `lym shed` verbs;
  a seed naming no recipe this lym knows is refused, naming the re-seed.
  A verb that applies to some loom recipes refuses on the others by the recipe's rows, never its name:
  `validate-plan`, `insert-card`, `remove-card` and `restore-renumber` refuse on a recipe with no Plan-Write row;
  such a recipe has no plan.
  `validate-parts` refuses on a recipe with no Parts-Write row, which writes no parts file.
  `join-parts` refuses on a recipe with no Parts-Plan-Write row, which has no parts to join.
  `lym shed`'s arming of the same runs never carries these refusals.
  A darn run takes its verify command and attempt budget from the hub-wide `darn.yaml` (`internal/darnengine`).
  The verify gates of the writer and of `Code-Burler-Fix` both use them.
  The writer's gate halts the run for the parent once the budget is spent.
  The darn gates run the diff's impacted set or their row key over the touched trees' targets.
  `tree_scope` in `darn.yaml` scopes a non-production diff at every verify site, Finalize included.
  `lym loom resume` re-spawns `Darn-Write`.
  `lym loom goto --to Darn-Write` re-enters it and keeps the spent review budget and any settled review verdict.
  A `lym loom` verb that arms a darn run whose status file names the writer row `Darn` renames it onto `Darn-Write`.
  `start` is the session bootstrap, performing these steps in order:
  it resolves the recorded parent branch and seeds and commits the status file with the records when it is absent.
  It ensures the worktree's tmux session is up, and it keeps or adds the status strand.
  The status strand runs the hidden `lym loom checklist` verb in an overview window named for the run's recipe.
  The window is opened with the worktree's console title, the recipe standing where the slug does, as in `T4 [ly:P3] tapestry`, on either driver.
  In a beam's session the window sits right after the `runs` strand's.
  It then spawns the per-hub watchdog daemon, best-effort.
  It reads this run's seed and, unless a driver is already alive, spawns the driver the recorded choice selects:
  the detached Go driver, `internal/sheddriver` mounted by `lym loom drive`, whose driver lock the handshake waits for;
  or a Claude strand running the loom driver in this worktree's own reed session.
  That strand has a window of its own after the overview and before the shell.
  It prints the success envelope and never attaches a tmux client (`lym reed attach` shows the session);
  the operator's terminal is Selvage, not a strand.
  `step` brings the tmux session up and never adds or removes the status strand.
  `start` and `resume` are the sites that read a recorded seed's driver value.
  The `internal/loomcli` package documentation covers this.
  `start` resumes a parked loom driver by typing one line into its pane.
  It returns once the line's delivery is verified.
  It refuses after a bounded wait when the pane is not ready, since the driver may have resumed on its own.
  A live driver over a run halted at a hand-back with no park marker yet is still writing its stop report,
  and a Go driver holding its driver lock is still settling the stop,
  so `start` refuses with the retryable kind `driver_not_parked`;
  batten's Inner-Run retries it, at most once per `notice_probe_s`, without recording the approval as acted on.
  `start` refuses with the non-retryable kind `merge_in_progress` over a pair with an unfinished merge.
  The refusal applies when `start` would spawn or resume a driver.
  It lists the conflicted paths under `conflicts` and names the remedy.
  A parked fabric `merge-in` of the run's own parent branch does not stop a run at Parent-Align or Finalize.
  That row aborts and redoes the merge.
  `lym loom resume` makes the same exception.
  `lym loom resume` wakes a halted run's live, parked driver through the same branch.
  A run awaiting at a review segment's Bouncer row with a pending circling decision takes that branch too:
  its driver's step re-calls the Bouncer, which acts on the decision.
  `lym loom resume` never spawns an LLM driver, adds a strand or brings reed up.
  It spawns a Go driver for a halted go-seeded run that has none.
  It reads the driver strand from reed's sessionless directory.
  It refuses every other state with its way forward.
  [refusal-spec.md](../contracts/specs/refusal-spec.md) defines a way forward.
  `run` is the no-tmux escape hatch that runs the phase machine in the foreground, for debugging and CI.
  `step` bootstraps idempotently, as `start` does.
  It drives one producer through `shedengine.Shed`'s own `Step` and emits a JSON envelope.
  It spawns no detached driver, which makes it the single-producer primitive an external supervisor drives.
  `step --until-stop` runs one child step after another in a loop.
  It returns the one envelope of the first stop, with a `loop` object beside the stop step's envelope.
  A step at a row that waits on the operator reads as active to the loop's watchdog and as a live wait to dobby.
  Those rows are Beam-Await, PR-Review and Sampler-Session.
  `status` reports the current phase as a single JSON envelope and, with `--watch`, tails it.
  It prints a line only when the composed activity changes, rather than once per poll;
  a wait's growing elapsed time alone is no change.
  On a terminal `status` renders a human view instead, and `--json` forces the envelope there;
  `--watch --json` is refused.
  `status` also carries a waiting note while a run waits on a verify or a shuttle wait.
  The note names the producer, what it waits on and for how long.
  The envelope carries `run_id`, `progress` and `last_step`.
  `progress` is the main-line position and bounce count over the recipe's producer graph.
  `last_step` is the record `lym shed step` keeps under `.lym`.
  It also carries the build identity (`vcs_revision`, `vcs_modified`) of the `lym` that ran that step.
  `binary_changed` is true when the running `lym` is a different known build.
  `idle_gaps`, an `idle` line each in the human view, names every stretch of the history of five minutes or more in which no step ran.
  Each history entry a producer call writes records the call's start in `started_at` beside its `at`.
  A routed `stuck` reads as `bounced to <row>` in `activity.last`.
  The run directory is named by the worktree slug (`_lym/shed/<slug>/`).
  `self` is an alias that resolves to it, and a `_lym/shed/self/` directory resolves too.
  `pause` records a `--before` or `--after` condition and refuses bare.
  The refusal names `lym tender pause` for stopping the agents.
  `validate-discussion` runs the same checks Discussion-Write's and Discussion-Burler-Fix's own gates run, standalone.
  It exits 0 on a clean gate and 1 otherwise, with findings in the failure envelope,
  so a writer agent can self-check before handing off.
  It also checks every loom record's history.
  The checks find dropped `Added after Discussion` entries and appended PATTERN bullets.
  It commits nothing.
  A run whose record row is Discussion-Write also gets the citation check.
  It gets the relevant list check too when the list exists;
  a Record-Write run never reads the relevant list.
  `decision add` takes `[<slug>] --by parent|operator --title <t> --decision <d> --rationale <r> [--amends-plan]`.
  It appends one design call made after the Discussion to the decision record.
  The call goes under an `Added after Discussion` heading.
  It re-runs the discussion check, restoring the record on a finding, and commits the record.
  `--pattern-entry <id> --pattern-action adds|changes|removes|withdraws` writes the entry's bullet too.
  The bullet goes under `## PATTERN changes`; for `withdraws`, the call strikes it in the same commit.
  `--from <file>` in place of the per-entry flags records a JSON array of such calls in one write and one commit.
  Each live agent gets one notice naming the new headings.
  Webster's Merriam, which reads no record, gets every entry's text inline.
  The verb also works on a Record-Write run once Record-Write has committed the record, under these checks:
  the record equals its committed copy;
  it holds every `Added after Discussion` entry its history held;
  and the committed record holds the appended entry afterwards.
  Each refusal names its way forward by the run's current row.
  `--amends-plan` marks a decision that amends the approved plan.
  The call also appends the title of each entry to the plan amendment record, committed with the entries.
  It is refused on a recipe with no Plan-Write row and for a plan that is not approved.
  It is refused while Discussion-Write is running.
  It refuses a named session other than the run's parent orch, the nearest live orch up its beam chain.
  The operator's own unnamed shell always passes.
  It resumes nothing.
  After the commit it sends one creel line to each live shuttle run of the worktree.
  The line names the record and the heading of each entry.
  The tackler, the arbiter and the caller get no line.
  It reports the strands told under `notified` and any failed notice under `notice_errors`, without failing.
  A Webster Master session of a run that is not running gets no notice and is named under `notice_held_back`.
  `validate-plan` runs the same checks a non-twill Plan-Write run's and Plan-Burler-Fix's own gates run, standalone.
  The exit-code and findings-envelope contract is the same as `validate-discussion`'s.
  It works over the current worktree's plan instead of its discussion.
  `validate-plan --rework` runs the format-only checks over a whole new plan.
  It adds a check that the plan's `first_card` equals the card number the caller was told.
  `validate-plan --lens-review` runs the check a twill-spawned Plan-Write run's own gate runs:
  the flag-less checks plus a check that the plan directory's `lens-review.md` holds a recorded lens review.
  `validate-plan --overlay` runs the flag-less checks over the plan with proposed file contents laid over it in memory.
  The contents are for its existing plan files and come from stdin as one JSON object.
  A fix is thus checked before any file is edited, and no plan file is written.
  `validate-plan --part <name>` runs the check a part writer's own gate runs over the named part's plan.
  The part's external exports are in scope.
  It refuses on a recipe with no Parts-Plan-Write row.
  `validate-parts` runs the check the parts gate runs over the parts file, standalone.
  It has the same exit-code and findings-envelope contract.
  `join-parts` is the chair's one call that joins the parts' plans into the plan directory;
  it runs only from the chair of a running Parts-Plan-Write row,
  so a halted or later run, a part writer, a Plan-Burler-Fix fixer and Webster cannot reach it,
  and it overwrites only the plan directory.
  `validate-description` runs the same checks the `Describe` row's `description` gate runs, standalone.
  It checks `_lym/landing/summary.md`, with the same exit-code and findings-envelope contract.
  `lint-comments [--base <commit>]` runs `internal/commentlint` over the worktree:
  it reports the fixed-column-wrapped breaks and the over-long lines in `//` comment blocks that the working tree creates.
  With `--base` it reports those that the range from `--base` to HEAD creates.
  `lint-pattern [--base <commit>]` runs `internal/patternlint` over the worktree:
  it compares `PATTERN.md` at HEAD with the working tree's, or at `--base` with HEAD's.
  It fails when the overview grew past its headroom line, naming the bytes to cut.
  An overview that stays above the line or shrinks passes.
  Both lints only read git and files.
  The same functions run as the lint steps that a `gate.yaml` gate key lists, such as the template's `card` key.
  They run inside that key's `lym gate test` call.
  `fmt <file>...` rewrites each named `.go` file inside the worktree to its `go/format` form in-process.
  It refuses the whole call and writes nothing.
  A named path that is missing, outside the worktree, not a `.go` file or unparsable triggers the refusal.
  A review segment's escalation to the run's parent halts the run `awaiting`, the planned hand-off state:
  it behaves like `blocked` for resume.
  It spends no bounce budget, triggers no friction reflection and raises no anomaly.
  A `blocked` or `failed` halt writes a Go halt note and starts no reflection;
  the run's Friction-Reflect row runs the reflection over every note.
  A review segment settles a spent bounce budget by the soft-cap rule.
  The rule reads the judge's per-finding `mechanical` or `design-wrong` fix tags.
  It escalates to the run's parent, also `awaiting`, for a circling design-wrong finding or at the hard cap.
  It escalates likewise when the rule cannot read its inputs.
  The Bouncer writes a brief and a one-line parent notice into its run directory.
  It returns the notice as `parent_notice`;
  the loom driver relays it to the parent, whose one-shot fork reads the brief and decides.
  The fork settles with `lym loom circling accept|continue [<slug>]`, which records the decision and resumes nothing.
  The fork then resumes the run with `lym loom resume`.
  `accept` also settles a run halted at Finalize after its fix reviews ran out of rounds.
  A `continue` past the soft cap runs one more round without spending budget;
  no round at or past the hard cap is continued.
  Batten does not resume a batten-driven run on such a decision.
  The run is resumed the same way, with `lym loom resume` in the task worktree.
  `Plan-Bouncer` skips its judge only for an exempt generation:
  one whose live cards are all `Prosa` on non-source files.
  The `Webster` row commits the plan directory alongside its run record.
  `commit-records` commits and pushes the run's records through fabric, and pushes the task branch along with them.
  The records are the status file, the review round record, friction notes and drive reports.
  The loom driver's end-of-session command runs it after the driver writes its stop report.
  A loom-launched driver runs it at a hand-back, before it parks until `lym loom start` resumes it.
  A tree with nothing to commit succeeds without a commit.
  Friction notes live under `_lym/loom/friction/`.
  Drive reports live under `_lym/shed/<slug>/drive-reports/`.
  Both are committed with the run.
  Every `lym loom start` or `step` commits whatever the friction directory holds along with the seed.
  A task's first one clears the directory first.
  Notes inherited from the fabric it branched from thus never reach its reflection or dirty its Preflight.
  Besides the agents' notes, the friction directory holds the Go-written halt notes, which name their anomaly kind.
  It holds the crash-resume notes written at a `run` or `step` entry too.
  `lym webster` refusal notes and the driver's repair records are there as well.
  `lym loom start` writes a handoff voucher before it spawns a driver,
  so a deliberate resume never reads as a crash.
  A tapestry run's tail is:
  `Webster → Parent-Align → Drift-Verify → Code-Review → Drift-Recheck → Describe → PR-Review → Finalize`.
  `Code-Review` is the Code-Perch of `Code-Bouncer`, `Code-Burler-Review` and `Code-Burler-Fix`.
  A darn run's tail is:
  `Darn-Write → Drift-Verify → Code-Review → Drift-Recheck → Describe → PR-Review → Finalize`, with no `Parent-Align`.
  `Drift-Recheck` runs the `DriftVerify` engine again on code committed after `Drift-Verify`'s last check,
  and its Stuck goes through the stop chain to the parent, with no route back to `Drift-Verify`.
  The regression rows are `Drift-Baseline`, `Drift-Await`, `Drift-Expect`, `Drift-Verify` and `Drift-Recheck`.
  Each carries the entry gate `drift.enabled` of the hub-wide `drift.yaml`;
  a sampler run's rows also need `drift.scope` set to `full`.
  A hub that fails either gate records each of them skipped and spawns or runs nothing for them.
  `Drift-Baseline` comes before `Parts-Write` in loom and before `Drift-Await` in darn.
  `Drift-Await` comes before `Drift-Expect`.
  `Drift-Expect` comes before `Webster` in loom, before `Darn-Write` in darn and before `Sampler-Session` in sampler.
  `Drift-Baseline` records the scope that `drift.scope` sets, with no agent.
  It reads the setting from config, including a value in a task's `run_settings`.
  Each Drift producer also reads `drift.enabled` at every pass.
  Switching it off, in the hub or in one task's local layer, lets a resume pass a Drift row the run already entered.
  `Parent-Align` merges the parent's upstream tip into the task with no parent lock.
  It verifies the merged tree through the `parent-align` gate key.
  It runs the merge fixer in parent-align mode.
  The fixer settles a conflict or a failing tree and aligns the task to what the parent brought in.
  It writes the merge record with the aligned tip, and the Code-Perch then reviews the task's code against that.
  `PR-Review` lands the task through a GitHub pull request when the resolver says so.
  The resolver decides from the parent's `beam.yaml`, the entry and GitHub's rules for the parent branch.
  The row pushes the task branch and opens or finds the pull request.
  It sets the pull request's title and body from the change description, records it and waits on it.
  When the resolver does not say so, the row passes.
  The wait takes no parent lock.
  It gates and pushes the agent's commits through the `pr-review` gate key, never onto a merged pull request.
  It mails a failure to the agent.
  It reads the pull request at most once a minute and before each gate.
  It ends Done when the pull request is merged or approved at the gated head.
  It also ends Done when the operator's `lym loom pr-continue` ends the review.
  With `loom.yaml`'s `pr_review_ask_parent` on, the row asks the parent orch to review the open pull request.
  It asks once per pull request, and the orch answers the row's session, whose agent fixes the findings.
  The row asks and does not gate: its endings do not wait on the orch's answer.
  That ending leaves the pull request for `Finalize` to merge on GitHub.
  Any other ending is Stuck.
  A pause that covers the run holds it.
  The driver strand's mark reads `PR #<n>`.
  `Finalize` takes the parent lock first and merges the parent into the task in place.
  It resolves a conflict with the conflict session under the lock.
  It runs the `finalize` gate key and the merge fixer in finalize mode.
  It lands on the parent with the credit block and releases the lock on every exit.
  It frees the lock while its fixer or fix review works.
  It then retakes the lock at the front of the queue.
  It merges the parent anew and verifies under the lock before it lands.
  Finalize never reroutes.
  A fixer change over the caps goes to its fix review, which reads only the fixer's commits.
  A rejection hands the findings back to the fixer to fix in place, for up to `finalize_fix_review_rounds` reviews.
  An approval lands the change.
  The last review that does not approve halts for the operator.
  The operator's `lym loom circling accept` lands the change with the open findings in the change description.
  A halt at `Parent-Align` or `Finalize` is handed back to the orch through every driver.
  Its causes are a spent or over-cap fixer and fix reviews out of rounds.
  The status marks the halt `hand_back`.
  On a loom run, tapestry, darn, pawl or sampler, a fixer's escalate halts with `EscalateStopPrefix` instead.
  It is not marked `hand_back`, and it goes to the arbiter first, reaching the orch only as the arbiter's hand-on.
  `Webster` reroutes to `Plan-Burler-Review` after begin-batch leaves a new plan conflict.
  The plan fixer `lym webster fix-plan` could not clear that conflict.
  `Describe` writes the change description at `_lym/landing/summary.md`, the single source for the landing commit.
  The landing commit's message and the pull request's body both end in a plain-line credit block.
  Go appends the block while `landing.yaml`'s `credit` is on.
  The block is `credit_heading`, then a ` - ` line for `credit_loomyard`.
  Another ` - ` line follows for the `credit_engines` text of each engine that ran a session in the task.
  An empty text adds no line.
  `Finalize` tolerates an already-landed parent and marks the board task `done` after the parent merge.
  Every run transition before that writes the board task's status as `<state> · <producer>`.
  An example is `awaiting · Code-Bouncer`.
  The run's start write is one of them and names its first row.
  The board README shows where each run stands from these writes; a failed board write only warns.
  Batten writes the same form for its own rows before `Run-Shed`, from its `Worktree-Create` claim on.
  A run with no `Finalize` row ends by closing its entry.
  A motif run makes the entry a Sketch, a note of the built-in type `sketch` that never runs and waits for triage.
  A raddle run ends the entry done in its own Raddle-File, never as a Sketch.
  An arras family filing is the exception: it turns the entry into the family's project, and no status write of the run reaches it afterwards.
  `Parent-Align` and `Finalize` each run the plan's verify command after a parent merge-in that changed the task tree.
  A failure halts Stuck, with the command's output in the loom verify-output log.
  A pending-verify marker keeps the gate armed across a resume until a verify passes.
  `landing.yaml` also holds the `fixer` model-spec and `fixer_timeout_min`, which govern the merge fixer.
  They also govern the fix review session `Finalize` runs over an over-cap fixer change.
  It holds `parent_align_fix_attempts`, `finalize_fix_attempts` and `finalize_fix_max_lines` too.
  `finalize_fix_review_rounds` is a landing key as well.
  `deploy_branches` lists the base branches whose landing runs `deploy`.
  `lym config reconcile` renames it from the retired `require_pr_to_base`.
  Reconcile also renames the retired `merge_fix_attempts` key to `parent_align_fix_attempts`.
  It renames `gate.yaml`'s `merge-in` gate key to `parent-align`.
  `landing.yaml` also holds `describe` (the `Describe` row's model), `describe_timeout_min` and the `credit` keys.
  An existing hub takes their template defaults until `lym config reconcile --apply` writes them.
  That run also prunes the retired `co_authored_by`, `trailer_text` and `engine_trailers` keys.
  An in-flight run parked past `Webster-Bouncer` is restarted rather than migrated.
  `lym loom status --watch` is this module's registered interactive-handoff exception.
  [PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md) defines the exception.
  It self-displays the polled status line, then blocks forever as its own keepalive tail.
  Every fallible step runs pre-flight, on the envelope, and only that tail is exempt from emitting JSON.
  ✅ Implemented.
  loom's config module is `loom.yaml`, which holds what every loom recipe shares.
  It holds the `discussion` and `judge` role model-specs and the shared `reviewer_model` and `fixer_model`.
  `reviewer_model` and `fixer_model` each take one model-spec or a per-round list.
  `seed` names the Bouncer seed pass model.
  `discussion_judge`/`code_judge` and `discussion_seed`/`code_seed` override the judge and seed per review segment.
  An empty one takes its fallback.
  `<segment>_reviewer_model`/`<segment>_fixer_model` for Discussion and Code override them per review segment.
  `split_reviewer_model`/`split_fixer_model` do the same for the arras recipe's split perch, Raddle-Review.
  An empty `split_fixer_model` takes the `raddle` key's model while that key is set.
  `split_judge` overrides the judge for the split perch, which has no seed key of its own.
  `discussion_reviewer_fan`/`code_reviewer_fan` cluster-review Discussion-Review or Code-Review.
  Each names a `lenses.yaml` fan, or lists one per round.
  `discussion_fixer_fan`/`code_fixer_fan` do the same for the fixer, an empty entry running the single fixer.
  `code_reviewer_fan` is open to the local layer, and the other three are closed to it.
  `split_reviewer_fan`/`split_fixer_fan` do the same for the split perch, and both are closed to the local layer.
  The assistant reviewer sits beside the Discussion-Review or Code-Review chair.
  `discussion_assist_reviewer_model`/`code_assist_reviewer_model` name its model, as one model-spec or a per-round list.
  An empty entry runs no assistant that round.
  `discussion_assist_reviewer_fan`/`code_assist_reviewer_fan` name the `lenses.yaml` fan the assistant runs.
  `assist_reviewer_timeout_min` is the assistant's run bound, capped to end before the chair's own timeout.
  `discussion_timeout_min`/`review_timeout_min` are timeouts, and `discussion_interactive` selects the Discussion mode.
  `discussion_advisors` lists the Discussion-Write advisors, one `{name, model}` entry each.
  An empty list runs the lead alone, deciding each question itself.
  `raddle` is the raddle agent's model-spec.
  `raddle_auto_approve` files a raddle proposal once its gate passes, with no operator approval; a run whose recipe rows hold a split perch never reads it.
  `pr_review_ask_parent` has PR-Review ask the parent orch to review the open pull request.
  It is open to the local layer, so a task's `run_settings` sets it for one run.
  `discussion_lenses` names the `lenses.yaml` fan whose lenses the Discussion-Write lead forks itself for.
  Empty turns the forks off, and the key is closed to the local layer.
  `subagent_model` names the model of the fresh subagents that Plan, Discussion-Write and darn writers hand lookups to.
  `review_circling_checkpoint` and `review_max_bounces` are review budget keys too.
  `review_amend_max_bounces` is the amendment allowance a review segment re-entered over a changed subject runs under.
  The per-perch `discussion_max_bounces`/`code_max_bounces` take `review_max_bounces` when null.
  A value of 0 runs no review round.
  `review_hard_max_bounces` and the per-perch `discussion_hard_max_bounces`/`code_hard_max_bounces` are the hard caps.
  A per-perch 0 takes `review_hard_max_bounces`, and a hard cap below its perch's budget is refused at load.
  `discussion_auto_accept`/`code_auto_accept` make a perch accept a harvested round at a spent budget.
  The perch then skips the soft-cap rule.
  `split_max_bounces`, `split_hard_max_bounces` and `split_auto_accept` are the split perch's budget, hard cap and auto-accept.
  `split_auto_accept` defaults to false, and a null `split_max_bounces` and a 0 `split_hard_max_bounces` take the shared values.
  `tapestry.yaml` is the tapestry recipe's module, per-worktree, and holds the plan perch's own keys.
  Those are the `plan` model-spec, `plan_timeout_min`, `implementer` and `plan_lenses`.
  They also include `plan_reviewer_model`/`plan_fixer_model` and `plan_reviewer_fan`/`plan_fixer_fan`.
  `plan_assist_reviewer_model`/`plan_assist_reviewer_fan` and `plan_judge`/`plan_seed` are plan keys too.
  So are `plan_max_bounces`, `plan_hard_max_bounces` and `plan_auto_accept`.
  An unset model list, judge or seed, a null budget and a hard cap of 0 take `loom.yaml`'s value.
  Both modules exist and reconcile via `lym config reconcile --apply`.
  The bare verb is a dry run that only reports added and removed keys and writes nothing.
  Reconcile renames the retired `webster_*` perch keys of `loom.yaml` to `code_*`.
  It renames the retired `review` and `fix` keys of `loom.yaml` to their new names.
  It renames the retired `<segment>_review`, `<segment>_fix`, `<segment>_fan` and `<segment>_fix_fan` keys the same way.
  It moves a value `loom.yaml` tuned for a plan perch key, or for `implementer`, into `tapestry.yaml`.
  `darn.yaml`'s perch keys, `review_fan` and `review_fix_fan` among them, move to `loom.yaml`'s `code_*` keys.
  It drops the retired `merge_*` perch keys and `rework`.
  It also drops `alignment_pr_approve`, `alignment`, `alignment_timeout_min` and `skip_alignment`.
  `parent_review_wait_min` is dropped too.
  The driver, tackler and arbiter keys live in `shed.yaml` alone.
  A loom recipe that needs other values sets them in its profile block's `shed` object.
  That object lands in the pair's local `shed.yaml` layer like any run setting.
  The reviewer and fixer model keys and `review_timeout_min` are the review segments' own models and timeout.
  They live here rather than in the recipe because the recipe is embedded in the binary,
  and a recipe-literal model would be untunable without a rebuild.
  The Discussion producer is a seat table run on `MultiLLMProducer`.
  `internal/loomengine`'s `DiscussionTable` builds it in Go.
  `DiscussionTable` lives in `discussiontable.go`.
  Its prompt is `contracts/stencils/loom/loom-template-discussion.md`.
  The prompt ships as an embedded default in the top-level `contracts/stencils` package.
  It is read at call time from the hub's stencils directory.
  The producer runs in one of two modes, selected by `discussion_interactive`.
  Interactive mode, set by the key, lets an operator interview the agent from its pane.
  Autonomous mode, the default, has the agent self-judge every answer.
  Both prompt renderings ship in the same stencil, selected by the `{{.mode_rules}}` marker.
  Two independent axes shape the table, both set in `loom.yaml`.
  The first is the advisors.
  With `discussion_advisors` empty, the default, the lead works alone:
  it interviews and writes the same two files, deciding each question itself.
  With one `{name, model}` entry per advisor, the lead is a chair.
  The advisors study the task, the board, the code and PATTERN in parallel.
  Each advisor is a seat named by its entry, and it answers the chair over the session message channel.
  It writes a notes file `advisor-<name>.md` beside the discussion files.
  The second is the lens forks.
  With `discussion_lenses` naming a `lenses.yaml` fan, the lead settles its design and writes and self-checks the record.
  It then forks itself once per lens of that fan, takes the forks' findings in and finalises.
  The forks review the record their inherited context holds, without reading it again.
  Only the lead forks, never an advisor.
  Both axes together are valid: advisors with the lead's lens forks.
  Reconcile rewrites a `loom.yaml` that holds the retired `discussion_producer` or `plan_producer` key.
  `twill` becomes the twill fan in the new lens key, and every other value becomes an empty one.
  `discussion_advisors` is set to match, the retired `seats` entries becoming `advisor-<n>` for the operator to rename.
  The lead's session may spawn fork subagents when `discussion_lenses` is set.
  The lead opens with `contracts/stencils/loom/loom-template-discussion-chair.md`.
  That stencil includes the discussion stencil whole and adds the advisors section.
  The section also covers a lead with no advisors, so a table without advisors runs the lead alone.
  Each advisor opens with `contracts/stencils/loom/loom-template-discussion-advisor.md`.
  The row keeps its name, gates and routing whatever the two keys hold.
  The Planner producer is built the same way.
  Its prompt is `contracts/stencils/loom/loom-template-plan.md`.
  `internal/loomengine`'s `prompt.go` and `plan.go` compose it.
  With `plan_lenses` naming a `lenses.yaml` fan, the plan writer explores.
  It has one fork (fork0) write and self-check the plan.
  The writer then reads the plan in full and self-checks it.
  It forks itself once per lens of that fan and judges each finding as a critical fixer.
  It records each disposition in `lens-review.md`.
  Every lens fan's writer saves each fork's final message verbatim in a lens report, `<record stem>.lens-<lens>.md`.
  The report sits beside the file its lens review is recorded in.
  `contracts/specs/twill-lens-report-spec.md` pins its format, so an audit reads every report from the run records.
  The plan writer runs `validate-plan --lens-review` as its last self-check.
  Its gate refuses Done until `lens-review.md` holds the lens review.
  The row keeps its name, `gates` key and routing;
  a fork-authorized run's plan gate also requires the lens review.
  Plan has no advisor key.
  With `tapestry.plan_split` true, the plan is written in two steps by the `Parts-Write` and `Parts-Plan-Write` rows.
  They sit between `Drift-Baseline` and `Plan-Write`.
  Each carries the entry gate `tapestry.plan_split`, and `Plan-Write` carries `tapestry.plan_split != true`.
  With the flag off, both new rows are recorded skipped and `Plan-Write` runs as before;
  with it on, `Parts-Plan-Write`'s Done enters `Plan-Write`, whose gate skips it to `Plan-Bouncer`.
  `Parts-Write` splits the task into parts with contracts in `_lym/parts/parts.md`, guarded by the `parts` gate.
  `Parts-Plan-Write` runs one writer seat per part, each guarded by the `plan-part` gate of its `part_gates` key.
  It also runs a chair that joins their cards into the plan through `lym loom join-parts`, guarded by the `plan` gate.
  Neither row carries `on_stuck`.
  Each reads the flag when the run enters it, so the flag is meant to be set before a run starts.
  See the `internal/loomengine` and `internal/loomcli` package documentation.
- **shed** — the generic outer phase-FSM under `loom` and the eventual `Hardener` (`internal/shedengine`).
  It is a Go engine that walks one flat, ordered producer list with no predefined slots.
  It honors resume, crash-recovery and pause uniformly at producer granularity.
  The shipped engine adapters live in one package, `internal/shedadapters`.
  The package also holds their shared context and archive helpers.
  They are the `Webster` adapter, the Burler review row and fix row producers and `MultiLLMProducer` over `seatengine`.
  The Bouncer, the generic review-gate producer, is not a wrapper over an engine.
  The engine itself has no `lym shed` verb.
  A product's own CLI constructs a `Shed` with its own producer list and calls `Run`.
  A bare verb would be a command with no list to walk; the `lym shed` subtree below arms named recipes.
  The skeleton (the loop, the status file, the `ShedProducer` interface) is ✅ **implemented**.
  So are the engine adapters.
  `internal/shedcheck` is the structural checker over an assembled producer list.
  A `go test` invariant enforces it over loom's own list; no production constructor calls it.
  Its package documentation lists the finding kinds it reports.
  The Shed recipe group's engine registry is ✅ **implemented** as `internal/shedrecipe`.
  Its `registry` map literal declares every engine name a recipe row may use.
  The recipe file format and the loader/builder are `internal/shedbuild`.
  Loom's own recipe is `contracts/recipes/tapestry-recipe.yaml`.
  `internal/loomrecipe` assembles it into the `*shedengine.Shed` that `internal/loomcli` runs.
  The generic verb set is ✅ **implemented** as `internal/shedverbs`.
  It holds the `run`/`step`/`status`/`pause` cobra bodies every arming module builds its own subtree from.
  It owns no path of its own and derives nothing.
  The named-recipe `lym shed` subtree is ✅ **implemented** as `internal/shedcli`.
  It arms `tapestry`, `darn` or `batten` by run-id rather than by an explicit recipe name.
  Each verb, `lym shed run|step|status|pause [<run-id>]`, defaults to run-id `self`.
  It reads the addressed run's own `seed.json` and looks its `recipe` field up in the table.
  `lym shed seed <run-id> --recipe <name> [--driver <name>] [--param k=v]` writes that first seed.
  The driver defaults to `llm` for a recipe that has a bootstrap verb, and to `go` for one that does not.
  Tapestry and darn have a bootstrap verb, and their unseeded `start` records `llm` too; batten has none.
  The seed verb refuses fabric's own checkouts, the Board checkout and a pair's other side, where no run is ever driven.
  It also refuses a recipe's seed wherever that recipe's own verbs would refuse to drive it:
  a batten seed anywhere but the hub's prime worktree.
  The generic `step` envelope printed on stdout names the durable `trace_file` the invocation wrote.
  It also names the `envelope_path` of the full envelope.
  The full envelope holds the run's `friction_dir` and `scratch_dir`.
  It also holds the reflection's `friction` status when non-empty.
  `status` names `trace_dir`, and fabric writes every recorded mutation to that trace.
  The driver stencil is `contracts/stencils/shed/shed-template-driver.md`.
  It launches a session that drives any seeded run recipe-blind.
  The session drives through one background job, `lym shed step <run-id> --until-stop`.
  The job's envelope at a stop is the only thing the session reads.
  At an `error` or `interrupted` stop the driver spawns exactly one fork of itself.
  The fork reads the traces and the repair guide.
  The guide is `contracts/stencils/shed/shed-template-driver-guide.md`, deployed beside the stencils.
  It writes the stop report and repair records, and it returns one action for the driver to run.
  An orchestrator session forks one driver loop per run.
  A message asking for a stop at a point becomes `lym shed pause <run-id> --before|--after <producer>`.
  At a stop it cannot settle, the driver runs `lym loom arbiter`.
  That verb spawns a cold arbiter agent and returns a verdict the driver applies or hands on.
  The driver re-launches the loop once after a deployed `lym`.
  A loom-launched driver parks at a hand-back until `lym loom start` resumes it.
  `internal/shedrun` is the sole declarer of the `shed` run-directory path segment.
  The segment is `_lym/shed/<run-id>/` durable and `.lym/shed/<run-id>/` ephemeral.
  It also declares the run-id vocabulary, including the default literal `self`, and the `seed.json` contract:
  the closed `recipe`/`driver` vocabularies and the sole `ReadSeed`/`WriteSeed`/`List` reader-writer.
  See [PATTERN-shed-driver](../pattern/PATTERN-shed-driver.md).
  See the package documentation of each `internal/shed*` package and of `internal/loomrecipe`,
  and [the recipe format](../contracts/specs/shed-recipe-spec.md).
- **burler** — one review+fix round (`internal/burlerengine` and `internal/burlercli`).
  It runs two fresh sessions, a reviewer and then a fixer, with no self-grading, over the shuttle file contract.
  Each is the unnamed lead of its own one-seat table on the seats engine, in strands `burler-review` and `burler-fix`.
  Each review perch's fan key turns the lens fan on for it, and its fix fan key turns on the twill fixer.
  Both are per round and off by default.
  The keys are `discussion_reviewer_fan` and `code_reviewer_fan` in `loom.yaml`.
  The third is `plan_reviewer_fan` in `tapestry.yaml`.
  Each has a `*_fixer_fan` partner, and the last pair serves a darn run's Code-Review too.
  The arras recipe's split perch has its own pair, `split_reviewer_fan` and `split_fixer_fan` in `loom.yaml`.
  Each perch's `*_assist_reviewer_fan` key names the fan its assistant reviewer runs.
  The forks of a code perch reach the diff through read-only git.
  `lenses.yaml`'s `review-code` and `fix-code` fans carry lenses for a committed diff.
  The reviewer writes the review, Go accepts it and records its digest, and only then does a fixer start.
  The fixer validates and fixes the findings.
  It disputes a finding only with evidence that its premise is false or that every fix makes the target worse.
  It fixes a real finding with its own change when the proposed one is wrong.
  A round is profile-driven: `{overlay, source}` fix-scope, tool-use and `language-skills`.
  A code round's seats load the hub's `language.yaml` skills after their own.
  Each review segment's `<segment>_reviewer_model` and `<segment>_fixer_model` keys pick each seat's model per round.
  They fall back to `loom.yaml`'s `reviewer_model` and `fixer_model`.
  Cluster review fans the reviewer out into N fork-subagent reviewers by naming a fan (`cluster-fan`) from `lenses.yaml`.
  It is never on by default.
  The verdict parse of the frontmatter is strict.
  `lym burler run` is the debug CLI.
  `lym burler validate-review <review-file>` is the read-only review-gate self-check. ✅ Implemented.
  See the `internal/burlerengine` package documentation.
- **hardener** — **DRAFT / concept.**
  Behavior-based reviewer that *runs* a live-substrate module (needs a sandbox repo) to harden it before merge;
  on-demand, post-loom, **off the spine**, shares only the `burler` round discipline.
  See the board's `hardener` note.
- **batten** — drives one task worktree's whole lifecycle as a single Shed run from the hub's prime worktree.
  The lifecycle is to create the worktree, seed the child's own inner run from the Board task's own `recipe`,
  run it to a terminal state, land it into the prime, and tear down.
  The `recipe` is one of `tapestry`, `motif`, `raddle`, `arras`, `darn` and `sampler`.
  An empty `recipe` means `board.yaml`'s `default_recipe`.
  `motif` is for a Discussion-only child that lands no code.
  `raddle` is for a child that files a Sketch's split on the board.
  `arras` is for a child that discusses a task and splits its decision record into board entries.
  `darn` is for a darn child, and `sampler` for a sampler child.
  The code is `internal/battenshed`, `internal/battenrecipe` and `internal/battencli`.
  Its verbs are `lym batten run|step|status|pause|goto|cancel <run-id>` and `lym batten cleanup [--apply]`.
  `lym batten lock list|release` lists and releases the hub's parent locks.
  `lym batten cancel <slug> [--remove]` ends a run for good from the prime, whatever its state.
  It leaves the task's claim `draft` and removes the run's records; `--remove` also removes its pair and task branches.
  The hidden `lym batten overview` is the pane process of the `runs` strand of the prime and of each beam.
  `lym beam start` ensures it, in the beam's window named `beam: prime` or `<handle>: beam: <slug>`.
  A pawl run's `lym loom start` first puts a `loom-status` strand in an overview window, the session's first.
  Ensuring the `runs` strand adds it beside that strand.
  A failure to add, replace or record it is a warning on the bring-up report, never a stop.
  It draws the dobby of its own worktree first, alive or not, as the lines `lym beam status` prints.
  It draws each run it drives once, with its state, stage, time in stage and last pass.
  It draws each released task it has not started with what the task waits on.
  The stage cell carries the stage's entry number, such as `7. Land`.
  A child stage under Run-Shed carries one like `6.5 Plan-Write`.
  A run a live pause covers shows `paused <minutes>` in its state cell.
  Below the dobby come the orch lines of the worktree's own orch and of every beam below it.
  The prime's window shows every orch.
  It reads files, taking the dobby's and each driver's lock for an instant to learn whether it is held.
  `lym batten cleanup` is a dry run unless given `--apply`.
  It removes each done run's records run directory with a records commit and a spawned push.
  It also empties that run's scratch beside its lock files, and any orphan scratch older than a day.
  A done motif run is also cleared by `lym beam create` and by the start of a raddle run under its slug.
  A run is one pass after another.
  `lym batten run <slug>` runs one pass and exits, started by the run's parent worktree's dobby or by hand.
  A hand pass makes sure that dobby is live once it is over.
  `lym beam land <slug>` starts no pass:
  it writes a land request into the beam worktree's `.lym/beam/` directory from anywhere in the hub.
  The beam's pawl run consumes the request at Beam-Await.
  `Worktree-Create` forks a task's pair from its beam, whichever worktree started the run.
  The beam is the nearest open beam up the task's board project chain.
  A task outside any beam forks from the prime.
  A beam member's start checks no tracked dirt of the prime, since its pair forks from the beam's last commit.
  A beam whose pair is absent here or whose own landing holds it stops the create.
  The stop is a precondition naming the way forward.
  A beam project runs the same way, its pair forked unmarked at the anchor its seed records.
  A member's beam is read again once its pair is made.
  If that beam's run holds its entry, the row removes the member's pair and refuses.
  The refusal names the member's cancel and the board move off the beam.
  `lym batten run` drives its own run through the same driver body in-process, over the loop `step --until-stop` runs.
  An unexpected stop meets the driver's rule table.
  A stop no rule settles meets a cold tackler agent in the prime's reed session, before it is handed to the orch.
  `batten.yaml`'s `tackler`, `tackler_timeout_min` and `tackler_max_spawns` set the tackler.
  They override `shed.yaml`'s keys of the same names.
  A stop the tackler escalates meets the arbiter next, wired in batten's driver after the tackler.
  `batten.yaml`'s `arbiter`, `arbiter_timeout_min` and `arbiter_max_spawns` set the arbiter.
  They override `shed.yaml`'s keys of the same names.
  The arbiter's goto is held to the halted row.
  The process holds the run's driver lock for its life, so a second `lym batten run` for the slug is refused.
  The `Command` rows `Open-Window`, `Seed-IDE` and `Open-IDE` sit between `Seed-Child` and `Run-Shed`.
  Each runs one shell command through `internal/shedadapters`' `Command` producer.
  Each runs only when its entry gate's `batten.yaml` setting is on, and a task's `run_settings` may set that setting.
  `Open-Window` (`open_window`, on by default, command `open_window_command`) opens one desktop terminal window.
  The window opens on the task worktree through `lym reed window`.
  The window is titled with the task's console title, `<T#> [<hub shortname>:<P#>] <slug>`, through the command's `{title}` placeholder.
  A part the hub or board does not give is left out.
  `Seed-IDE` (`seed_ide`, on, command `seed_ide_command`) runs `lym ide seed`.
  `lym ide seed` writes the task worktree's VS Code title, colours and `.vscode/` config and the user's key bindings.
  `lym ide spawn` writes the same.
  `Open-IDE` (`open_ide`, off, command `open_ide_command`) runs `lym ide open`, which opens VS Code on it.
  After the window opens, `lym reed window`, `lym ide spawn` and `lym ide open` each wait `desktop.window_wait_ms`.
  They then run the machine's `desktop.window_command`.
  An example is a script that snaps the new window into the desktop's tiling.
  Both are machine keys, set per machine with `lym config desktop --layer machine --set <key>=<value>` from the prime.
  With the command empty, the default, nothing runs.
  A command key is read from the template and the board default layer only.
  `lym config batten --layer default --set <key>=<value>` from the prime changes one.
  A different value in the task worktree's own layers is ignored with a warning.
  A failed command only warns, and the row passes on.
  The Board task's `recipe` must be `tapestry`, `darn` or empty.
  `Run-Shed` starts `lym loom start` inside the task worktree, which serves both recipes,
  so `Seed-Child` refuses any other registered recipe before writing a seed.
  That avoids committing a seed the child's own bootstrap would refuse.
  `Seed-Child` halts `blocked` with a named `stuck_reason`, not a hard failure, for each refusal `WriteSeed` can make.
  A refusal is an unknown recipe, a recipe the task worktree cannot bootstrap, or a disagreeing pre-existing child seed.
  A disagreeing seed surfaces as `shedrun.ErrDisagreeingSeed` or `battenshed.ErrDisagreeingChildSeed`.
  All get the same business-judgment treatment, since none is a path-resolution or write failure.
  `Worktree-Create` is idempotent against its own post-condition.
  The post-condition is the whole pair fabric's `Add` builds, not merely the task worktree directory.
  A process killed inside `Add`'s multi-step sequence never runs `Add`'s in-process rollback,
  so a bare directory check would pass after the sequence's first step alone.
  The row instead checks that the pair is fully materialised before reporting done:
  the task worktree's sibling exists, its junctions are wired,
  and its origin record names the parent branch `Seed-Child` passes to the child's bootstrap.
  A worktree present but not yet complete halts `blocked`, naming a manual cleanup from prime.
  It does not silently skip the create.
  The cleanup is `lym fabric remove --force <slug>`, which removes whatever part of the pair `Add` reached.
  It deletes the local task branch itself when its work is pushed or landed.
  A create refused on a leftover branch halts `blocked` with batten's own remedy.
  A torn-down pair keeps its task branch on the remote, and locally too unless `remove` proved its work pushed or landed.
  The remedy is to delete the branch locally and on the remote, and any orphaned sibling branch, then resume.
  It is never fabric's `lym fabric checkout`, which from prime switches prime's own pair onto the task's branches.
  A child that is `awaiting`, a review segment's escalation, is watched, not failed.
  Settle the escalation with `lym loom circling accept` or `continue`.
  Then run `lym loom resume` in the task worktree.
  `accept` also settles a run halted at Finalize after its fix reviews ran out of rounds.
  A done child's driver is waited for, up to `driver_exit_grace_s`, before teardown.
  `Land` runs between `Run-Shed` and `Worktree-Teardown`, once the child run is done, under a hub-level land lock.
  A child that landed no code only has the done entries pruned off the board, with no pull, deploy or notice.
  Such a child is one whose status history holds no finished `Finalize`, such as a motif run.
  An entry still carrying `notify_on_land` stays until its dobby has sent the line and cleared the field.
  Otherwise `Land` pulls the prime when the child's parent branch is the prime's branch.
  It waits up to 10 min for the prime's tracked files to be clean.
  It runs the landing config's hub-wide `deploy` command in the prime when that parent branch is in `deploy_branches`.
  It prunes the done entries off the board, again keeping one that still carries `notify_on_land`.
  It notices the orch only when the pull's records side stopped, the one notice of a successful landing.
  A failed `Land` step halts batten `blocked` at `Land` with the pair intact and one failure notice per episode.
  A `lym batten pause` seen while `Land` waits does the same with no notice.
  The way back is `lym batten goto <slug> --to Land`, and then `lym batten run <slug>`.
  Batten does not notice the orch of a done child.
  The `Worktree-Teardown` row notifies only on a failure, and the `Land` row also notifies every landing.
  The `Open-Window`, `Seed-IDE` and `Open-IDE` rows own the terminal window and VS Code; `Run-Shed` opens neither.
  VS Code opens only when `open_ide` is on or the operator runs `lym ide`.
  A run resumed past `Open-Window` runs that command again when no client is attached to the pair's reed session.
  The re-run needs `batten.open_window` on, and a resume with a client attached opens none.
  That window closes when the run's session ends or its worktree is removed.
  It closes at once when the attach exits 0, which includes an operator detach and an ended pair session.
  After any other exit it closes as soon as the worktree is gone within thirty seconds;
  otherwise it stays open as a shell.
  A child that halts (`blocked`, `failed`, `paused`) makes `Run-Shed` wait, budget-exempt and without a time limit.
  Batten never resumes a halted child.
  The operator resumes the child from inside the task worktree with `lym loom resume`, and the watch carries on.
  The watch warns once per halt episode.
  Batten logs the reason and writes it to the row's stuck-reason file.
  The reason names `lym loom start` too when no driver can be woken.
  `lym batten status` does not show the reason while the child is halted.
  `lym batten pause <slug>` stops the wait within one check.
  If a halted or awaiting child's reed state holds a dead driver strand, batten revives the pair's strands.
  It revives them through reed resume, once per halt episode per batten process.
  That brings the parked driver back with its own session and leaves the run halted and the park marker in place,
  so a later `lym loom resume` wakes that driver.
  A retiring strand is left to `lym loom start`.
  `Run-Shed` spawns the child's bootstrap while the task worktree has no status file.
  It spawns it again while that status is `running` and its spawn was not confirmed by this batten process.
  The confirmation marker holds the pid of the process that wrote it, so a restarted batten spawns the bootstrap again.
  A bootstrap that failed or was killed after seeding is thus retried on resume.
  It is not watched as running for the whole budget.
  A running child with a live or retiring driver strand in its reed state, or a held run lock, is adopted instead.
  Batten records the confirmation for itself and spawns nothing, so a second driver is never stacked.
  The bootstrap itself is idempotent against a driver already alive.
  `Worktree-Teardown` is idempotent against its own post-condition.
  Re-entered after its removal already succeeded, it finds the pair gone and reports done.
  That happens when a process is killed before the transition persists.
  The post-condition is the pair's, not the task worktree's alone:
  fabric removes the task worktree before its sibling,
  so a removal interrupted between the two halves leaves the sibling and both branches behind.
  Re-entered there, the row halts `blocked`, naming the leftover and `lym fabric prune --apply` as the remedy.
  It never reports done over the debris.
  Teardown first tags the tip of the pair's other-side branch `archive/<slug>/<tip>` and deletes that branch locally.
  A detached child then pushes the tag to its remote.
  The run's committed records thus stay reachable after the branch is gone.
  That child also deletes the branch on the remote.
  The task branch stays on the remote, and locally unless `remove` proved its work pushed or landed.
  A re-entered row whose worktrees are already gone finishes that deletion.
  It does not report done over a surviving copy.
  A surviving copy would make a later create of the slug refuse its push.
  A failed remote deletion halts the row `blocked`.
  `lym batten run <slug>` retries it on any machine that owns the run once the remote is reachable.
  That works because a resume at this row whose task worktree is gone runs no origin reconcile.
  It judges the owner on the local board alone.
  A failed archive push halts the row the same way before anything is removed, and its reason names the resume.
  Teardown is one row that sequences session shutdown before worktree removal, and it never forces.
  Anything an agent left uncommitted or untracked in the child blocks the row after the session is already down.
  A build artefact or a note is an example.
  Fabric's own refusal is the `stuck_reason`.
  The operator cleans the child and re-steps, never with `--force`.
  An uncommitted change in the pair's sibling, where the run records live, instead names batten's own recovery.
  The recovery is `lym loom commit-records` in the task worktree, then resuming the batten run.
  Should the resumed teardown refuse again, the leftover is not a run record.
  The reason then names committing or removing it by hand.
  Uncommitted content in the task worktree itself keeps fabric's refusal, and neither case is ever forced with `--force`.
  A fresh pair's records branch starts without its parent's `_lym/shed/` run records.
  Fabric's `add` drops them in the pair's first records commit,
  so a child never sees prime's batten records or any other run's.
  `Worktree-Create` and `Worktree-Teardown` wait for a prime lock another run holds, polling every 2 s for up to 10 min.
  They halt `blocked` only once that wait runs out.
  `Worktree-Create` holds it only while fabric's `add` reads the fork point and writes the hub's worktree registry.
  It never holds it over the board claim, the config reconcile or the pair's push,
  and it holds it over origin's probes only for the time they still need once the pre-flight reads are done,
  so runs started together create their pairs side by side.
  `step` drives exactly one producer forward from the run's persisted current producer, seeding a fresh run first when none is persisted yet.
  `lym loom step` is the single-producer primitive of the same shape over a run that is already seeded.
  `run` and `step` carry `--driver` and `--child-driver`.
  `--driver` is batten's own and `go`-only: batten has no bootstrap verb, so `llm` is refused by name.
  `--child-driver` is the driver the task worktree's own inner run uses, `llm` or `go`; unset, it is `shed.yaml`'s `driver_kind`, `go` by default.
  `--driver` is recorded into the run's write-once seed at first seeding, and `--child-driver` only when typed.
  An explicitly typed flag that disagrees with an already-seeded run is refused rather than silently dropped.
  `Run-Shed` checks its child once per call, and a call that ends nothing yields instead of waiting.
  Its memory between calls rides in a per-run record, and the scheduling dobby runs the next pass.
  A pass returns a verdict only for a state change, an arm event, a pause or a failed status stat, each a budget-exempt `Stuck` that is its own history entry.
  Anything costing a process or a multiplexer round trip runs at most once per `notice_probe_s` (30 s).
  A running child is checked without a time limit; `lym batten pause`, honoured at the next pass, cancellation and the scheduling dobby's run notices bound it.
  The history writes of those returns rewrite the run's durable `status.json` without committing it.
  The status commit skips a transition it has already committed, so the write stays uncommitted.
  So prime's own pair carries an uncommitted change at `_lym/shed/<slug>/status.json` while a watch goes between them.
  The scheduling dobby keeps the episode keys of the run notices it sent in `_lym/shed/<slug>/notice-episode`.
  The next status commit carries that file, which avoids one identical commit per bounce.
  Its operator-visible consequence is that `lym fabric checkout` refuses hub-wide while a batten run is watching.
  Checkout requires a clean pair.
  `status` bounds the history it reports to the most recent entries.
  It reports the true `history_length` and a `history_truncated` flag alongside.
  The history grows only on a state change, a pause or a failed status stat.
  `status` also surfaces the blocked run's own producer-supplied `stuck_reason`.
  It is kept for the `Run-Shed` block through the budget arm, whose persisted `error` stays the fixed budget literal;
  the other rows carry no `on_stuck`, so their own reason is the persisted `error`.
  It also surfaces the teardown row's `abandonedSession`, recorded by the producer rather than only returned,
  so a `step`-driven lifecycle reports it as well as a `run`-driven one.
  ✅ Implemented. See the `internal/battenshed` and `internal/battenrecipe` package documentation.

The cross-OS spawn primitive **proc** and the generic outer phase-FSM **shed** are internal (non-CLI) layers.
Proc is the base of the stack; shed is the generic engine `loom` configures rather than a stack layer of its own.
See the [Execution stack](#execution-stack-orchestration-layers) section below for how proc / reed / shuttle fit together.

The user-facing modules sit on a thin layer of shared infrastructure, defined in [shared-libs/README.md](shared-libs/README.md).
`internal/pattern` is the leaf that inlines the root `PATTERN.md` overview into the prompts of the agents that design, plan, edit or judge.
The roles are listed in its own doc, and it also ships a format checker for `PATTERN.md`.
`internal/friction` is the leaf that returns the role-appropriate friction-note directive.
The directive is injected into the agent prompts when Tier 2 is enabled.
`internal/friction` composes the note path with its own non-clobbering `NotePath`.
Above the engines sits a separate precondition-and-geometry layer, distinct from the shared infrastructure.
`internal/preflight` is the tier-1/tier-2 precondition layer (worktree geometry, worktree-pair cleanliness, Fabric readiness/sync).
`internal/hubgeom` and `internal/standalonegeom` are its hub-mode and told-mode constructors of the `Geometry` struct each engine is handed.
See [PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md).
`internal/cliwire` sits between the two.
It is the CLI-boundary resolver of a standalone-capable CLI.
The CLI calls it once `preflight.ResolveMode` has chosen hub or standalone mode.
It hands each CLI the told strings:
the resolved target, the derived standalone state directory, and the resolved stencils and plan directories.
`internal/hubgeom` and `internal/standalonegeom` then build those strings into a `Geometry` struct.
`internal/preflightshed` sits alongside these as the producer-shaped wrapper around that same layer.
It lets a `Shed` producer list name the layer as a single row.
No caller composes `internal/preflight.Check` itself.

## Execution stack (orchestration layers)

The orchestrator is not one module but a **layered stack**, each layer knowing only the one below it.
It exists in this shape for one reason: agents must run as **interactive tmux sessions, never headless `claude -p`**.
The constraint is economic; see the `internal/shuttleengine` package documentation.
So spawning an agent is not a plain `exec` but "place a pane, launch a provider in it, drive it, detect completion."

```
internal/proc     spawn any OS process (windowless / detached), cross-OS      [OS primitive]
internal/reed     the window to the world — overlay + strand bookkeeping +     [builds on proc]  ✅
                  render; hosts every managed process as a strand in its own
                  window, persists to .lym/reed.json
internal/shuttle  run ONE LLM agent in a strand via a swappable engine over    [builds on reed]    ✅
                  the file contract; Stop-hook completion
seats             one step as an unnamed lead and its named advisors, each     [builds on shuttle] ✅
                  in its own strand, talking over the session message
burler            one review+fix round: reviewer (+cluster) → fixer           [builds on seats] ✅
shed              generic outer phase-FSM: walk one flat producer list,        [stdlib +           ✅
                  honoring resume/crash-recovery/pause at producer granularity  internal/state,lock
                                                                                 only -- skeleton]
loom              phase machine: drive each phase through a Bouncer gate       [builds on shed,
                                                                                 burler]
```

Regression sits beside this stack.
The Drift rows are shed producers over seats,
and the baseline capture is a detached registered process, not a gate tier.
See [internal/driftengine](../internal/driftengine/doc.go).

The batten Shed nests loom's, as its own recipe.
Its `Run-Shed` row seeds a task's run, tapestry, motif, raddle, arras, darn or sampler and drives it as a child process,
and it polls that run's own persisted status for the verdict.
There are two status files by design:
the task's, committed on the task branch, and batten's own, durable under prime's own `_lym/shed/<slug>/`.
Each resumes independently.
See [PATTERN-pair-remote](../pattern/PATTERN-pair-remote.md).

**Landing preconditions.** The relocation of run state requires no `lifecycle` or `loom` run in flight at landing.
No migration reads or moves the old `.lym/lifecycle/<slug>/`/`_lym/loom/status.json` layouts.
A run left in flight under either old layout therefore resumes nowhere afterward.
It also requires deleting any already-deployed copy of `contracts/specs/loom-status-spec.md` by hand.
`internal/stencilstore` never overwrites a hash-mismatched file and has no force-sync carve-out for specs,
so a deployed copy predating the path rename never refreshes on its own.
Delete it, and the next run re-seeds it fresh.

The whole stack runs **headless** (auto mode):
strands exist (the interactive-session requirement), agents run, output files are read, nobody need watch.

The stack has two entry modes: every layer from `reed` up is **told** its geometry rather than deriving it.
`internal/hubgeom` and `internal/standalonegeom` are the two constructors that tell it, for hub mode and told mode respectively.
`preflight.ResolveMode` selects between them at a standalone-capable CLI's pre-run.
A producer verb therefore runs in a directory that is not a git repository.
It needs no hub, no fabric, and no orchestrator status seed.
See [PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md) for the rule.

- **reed is three things, and it is built.**
  It is an **overlay** over tmux, **strand bookkeeping**, and a **render** sub-package.
  The render sub-package is `internal/reedengine/render`, the closed display vocabulary.
  A strand is one tracked process: a metadata record with a `guid`, `name`, worktree slug, parent, and a generic display spec.
  Callers hand reed `{cmd, name, display}`, where `display` is generic (an anchor, focus and an optional window group).
  Reed never learns what a "phase" or "cluster" is, because `display` carries no domain `type`.
  `lym reed remove --name <name> --detach` removes a strand by name from inside itself through a detached remover.
  Reed replaces a strand atomically in its slot.
  See the `internal/reedengine` package documentation.
- **beams sit beside the stack, not in it** — a beam is a pair whose origin record carries the beam mark.
  `lym beam` calls fabric, the board and batten in-process, so no layer above learns of it.
  Orch and batten run in the prime and in any beam, and a beam's tasks fork from the beam's own branch.
  See the `internal/beamcli` package documentation.
- **skills and the parent directive** — a spawn's skills are named on the launch spec,
  and the provider engine delivers them before the prompt, on the launch line or typed, never asked for by a stencil;
  every spawned role's opening stencil renders the parent directive,
  so a role escalates to its parent rather than the operator.
  See [PATTERN-stencil-ownership](../PATTERN.md).
- **provider-invariant** — `shuttle` runs Claude today through an **engine**;
  the verdict/output contract is provider-invariant,
  so a different model can be swapped in without touching the review machinery.
  Non-Claude is not a current priority.
- **the bootstrap** — `lym loom start` brings up the worktree's tmux session.
  It also spawns the per-hub watchdog daemon (best-effort).
  On either driver it keeps or adds the `lym loom checklist` status strand.
  The strand sits in an overview window named for the run's recipe.
  The window opens with the worktree's console title, the recipe standing where the slug does, right after the `runs` strand's in a beam's session.
  The strand shows one frame of the recipe's rows in order, with their visit lines under them.
  Then come the state and wait lines.
  Bootstrap replaces the strand in its slot when the live one was launched from a different `lym` build.
  It spawns the driver the run's own seed selects.
  That is the Go driver **detached** (via `proc`, no TTY).
  For the `llm` driver it is a Claude strand running the loom driver in this same reed session.
  When the `llm` driver is live and parked at a hand-back, it spawns nothing.
  It types one resume line into that driver's pane instead.
  It then returns the success envelope and never attaches or switches a tmux client;
  `lym reed attach` shows the session.
  Selvage is the operator's terminal, not a strand.
  A Go-driven tapestry run runs in the background;
  the reed view takes the foreground.
  A `.lym/lymrun.cmd` launcher makes it one click.
- `reed`, `shuttle`, and `loom` each get a user-facing `lym <module>` CLI.
  `lym shuttle run|open|state|await` lets an operator or another process drive one agent standalone.
  `burler` is composed by loom's own review segments (`lym burler run` is a debug-only wrapper, not a product verb),
  and `proc` alone stays an internal library with no CLI of its own.

### Following one spawn down the stack

loom wants a plan-reviewer for worktree `feature-x`:

1. `loom` → its Plan-Review segment's `Bouncer` — "review this plan against the discussion until clean."
2. the segment's Burler review row → `burler.Review(profile, opts)`;
   its Burler fix row → `burler.Fix(profile, opts, digest)` — "run one review, then one fix."
3. `burler` → runs a one-seat table on the seats engine per half, each a shuttle run:
   "run one reviewer agent, then one fixer agent."
4. `shuttle` → `reed.AddStrand{ cmd:"claude …", worktree:"feature-x", display:{anchor:own-window, focus:true} }`.
5. `reed` records the strand in `.lym/reed.json` and runs the command via `proc` in a pane of a window of its own.
6. The `Stop` hook fires, and reed notes the edge.
   Shuttle reads the output file and returns to burler.
   The reviewer's review is accepted, and the review row records its digest.
   The fix row spawns the fixer, which writes its fixer-report.
   The segment's `Bouncer` reads it and decides another round or exit;
   on a `CONVERGED` verdict it returns `Done`, and loom advances.

### The disambiguating test

- About **the OS**? → `proc`.
- About **a tmux mechanic, a strand, or where it is placed**? → `reed`.
- About **running an LLM and getting its answer**? → `shuttle`.
- About **one review+fix round**? → `burler`.
- About **whether an artifact passes (loop rounds until clean/stuck)**? → a `Bouncer` review segment.
- About **hardening a live-substrate module by running it** (post-loom, off-spine)? → `hardener` (DRAFT).
- About **what to run next**? → `loom`.

## Tests

Per-file unit tests sit next to the source they test (`store.go` ↔ `store_test.go`).
The cross-cutting suites — benchmarks, concurrency stress, and git-backed integration — live in the black-box `internal/boardengine/boardtest` package.

Fabric's own cross-cutting suite follows the same black-box convention but keeps its own name.
It is a set of `package fabricengine_test` files inside `internal/fabricengine/`, tagged `tier1`.
It is the **live-state integration harness**.
It drives real cloned hubs into dirty and hostile on-disk states, and it asserts what a destructive verb is and is not permitted to touch.
The hubs are built by really cloning rather than hand-assembling a fixture.
See `internal/fabricengine`'s own package doc for the state matrix, the verb table, and the sabotage-proof table.
The sabotage-proof table records that each data-loss defect still fails on demand when its guarding check is neutered.

`internal/gitkit` is the below-fabric leaf and the home of test git plumbing.
It holds spawn, query and commit helpers, the hermetic git environment, and the primitive repo fixture `CopyRepo`.
It asserts nothing itself, and it never imports fabric.
`internal/hubforge` is the repo-wide real-hub fixture factory.
It builds every hub fixture in the repo through `fabriccli.CloneAndWire`, never a hand-assembled stand-in.
It asserts nothing about fabric either.
`internal/testkit` holds the shared test kits, one package per kit, for test support used by two or more packages.
That support covers fakes, builders, fixtures and the scan harness.
[PATTERN-test-isolation](../pattern/PATTERN-test-isolation.md) states what a kit may import and assert.
`internal/testkit/scankit` is the harness every invariant scan runs on.
It provides module-root lookup, file walk and allowlists that report stale entries.
It also provides a vacuity floor and an import-allowlist assertion.
`internal/testkit/plankit` is the one test-side writer of valid plans, rendering through `planparser`'s format constant.
It also holds the file-tree fixture glyph resolution needs.
`internal/testkit/tmuxkit` gives each test package one isolated tmux socket directory through `Main`.
It gives each test a self-cleaning `-L` key through `Socket`.
`internal/testkit/stencilkit` seeds stencil fixtures from the registry through `stencilstore`.
Seeded files carry production's hash stamp.

## Sandbox Hub

The **sandbox Hub** is a dedicated bench for manual testing of lym's core workflows — dogfooding lym against itself.
It lives on disk at `C:\Code\lym-test-LYMHUB` and exercises the resolved `lym` binary under test.
That is the dev binary via `deploy-dev` into `.dev-bin` when present.
Otherwise it is the production binary on PATH via `deploy.cmd`.
Build it via `sandbox/win/build.cmd` (`sandbox/posix/build.sh` on Linux/macOS).
Run the core suite via `sandbox/win/core-suite.cmd` (`sandbox/posix/core-suite.sh`).
Run the reed-specific suite, which needs live tmux, via the `reed-suite`/`reed-suite.sh` pair.
Collect the report for either suite via `sandbox/win/fetch.cmd` (`sandbox/posix/fetch.sh`).
See [sandbox-howto.md](sandbox-howto.md) for the step-by-step runbook and [sandbox-hub.md](sandbox-hub.md) for topology and design details.

## Other docs

- `internal/loomengine`, `internal/loomcli`, `internal/loomshed` and `internal/loomrecipe` — the phased orchestrator (`lym loom`).
- [code-comment-conventions.md](code-comment-conventions.md) — the doc-comment rule's standing rationale (Go only, for now).
- [webster-spec.md](../contracts/specs/webster-spec.md) — webster's cross-module contract.
  It covers the `_lym/webster/` boundary, `outcome.yaml`, and `summary.md`'s writer-side additions.
- [loom-final-summary-spec.md](../contracts/specs/loom-final-summary-spec.md) — the final-summary artifact contract.
  It covers the format, the validation, and the consumers in `internal/landingshed`.
- `internal/reedengine` package documentation — the window to the world: tmux overlay + strand bookkeeping + render.
- `internal/shuttleengine` package documentation — run one LLM agent via a swappable engine over the file contract.
- `internal/burlerengine` package documentation — one review+fix round: reviewer then fixer, no self-grading.
- `internal/treadleengine` package documentation — the generalized round-loop engine,
  with a pluggable `RoundRunner` seam a future consumer (Tenter) can drive.
- The board's `hardener` note — **DRAFT/concept**: behavior-based hardening of a live-substrate module (post-loom, off-spine).
- [benchmarks/](benchmarks/board-performance.md) — board performance, tracked across revisions.
- [shared-libs/](shared-libs/README.md) — the shared infrastructure plumbing.
- [research/](research/README.md) — measurement studies of LoomYard's own runs and tools, grouped by topic, with an index.
- [reed-proposal.md](reed-proposal.md), [reed-exploration.md](reed-exploration.md), [logging-inventory.md](logging-inventory.md) and [quarry-holistic-fix-log.md](quarry-holistic-fix-log.md) — design notes and work logs.
- [reference/weaving-glossary.md](reference/weaving-glossary.md) — weaving and textile terms that can serve as names in lym.
- [reference/tmux_scripting.md](reference/tmux_scripting.md) — tmux command reference (vendored).
- The board — tasks and notes, the single home for unscheduled ideas too (no separate long-term-ideas file).
- [sandbox-howto.md](sandbox-howto.md) — operator runbook: deploy `lym`, build the Hub, run the suite agent (procedure).
- [sandbox-hub.md](sandbox-hub.md) — the sandbox Hub: a dedicated bench for manual (dogfooding) testing.
- [crucible/README.md](../crucible/README.md) — **`crucible`**, the **serial review+fix loop**: a reusable method for hardening a live-substrate module before merge (orchestrator-driven, model-rotating, clean-room self-fixing rounds + independent verification).
  The hand-executed prototype of the review-gate + `burler` (see the `internal/burlerengine` package documentation) round loop (and the origin of the board's `hardener` concept, named separately to avoid colliding with it);
  ships two paste-ready prompts — an [orchestrator prompt](../crucible/orchestrator-prompt.md) (drives the loop + verifies) and a [round-agent prompt template](../crucible/review-prompt-template.md) (the reviewer-fixer), to instantiate per module.
  Lives at the repo root, not under `docs/`, since it's a working method/prompt set, not documentation of shipped code.
