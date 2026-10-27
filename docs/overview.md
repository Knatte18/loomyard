# Overview: Loomyard

Loomyard is a Go toolkit of one-shot CLI modules.
Each invocation starts a process, runs one command, writes JSON to stdout, and exits — there is no daemon and no shared memory.
State lives on disk per module and is coordinated with file locks, so concurrent `lym` processes on a machine cooperate through the filesystem.
The first module, **board** (a task tracker), is implemented;
**fabric** (the git-coordination module over the code and its records, with `lym fabric add --local` for pairs that never reach origin) is implemented;
and **reed**, the clean tmux overlay built on what its now-deleted proof-of-concept (`muxpoc`) proved, is implemented.

In the long term, Loomyard is intended to **replace mill/millhouse (Python)** entirely.
We get there by building these modules as self-contained toolkits first;
orchestration comes last.
See [Principles](#principles).

[loomyard-overview.svg](loomyard-overview.svg) is a conceptual figure of how a board task becomes a landed commit, with a glossary of the weaving terms.

Module path: `github.com/Knatte18/loomyard`

## Naming: `lym` (binary) · `loom` (orchestrator module) · `ly` (skills)

Three distinct names for three layers, deliberately non-overlapping to avoid the millhouse `mill`/`millpy` collision (where one name meant two different things):

- **`lym`** — the binary/CLI, **L**oom**Y**ard e**X**ecutable — one binary with a namespaced subcommand tree (`lym board`, `lym fabric`, `lym loom`, …).
  The analog of millhouse's `millpy` backend.
- **`loom`** — the orchestrator *module* (`lym loom start`, `lym loom status`): the domain that drives the phased run, a module like `board` or `fabric`.
  See the `internal/loomengine` and `internal/loomcli` package documentation.
- **`ly`** — the skill / orchestration plugin (the analog of `mill`);
  skills are `/ly-*`.

**Never name skills `lym-*` or `loom-*`** — skills are `ly-*`, distinct from both the binary (`lym`) and every module (`loom`, `burler`, …), so no name is shared between a skill and a script/module (the ambiguity that forced the millhouse `mill` → `millpy` rename).
Internal Go feature packages follow the `<module>cli` / `<module>engine` split (e.g. `internal/boardcli` + `internal/boardengine`, `internal/fabriccli` + `internal/fabricengine`) — see the package-naming rule in [PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md).

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
   A `lym` command should make the *correct* path the path of least resistance and make drift *detectable* (`status` / a future `doctor`), rather than relying on an agent or operator remembering a rule.
   No on-disk operation is truly un-bypassable when a shell is available, so the achievable bar is "right path is easiest + mistakes are detectable," **not** "wrong path impossible."
   Hard blocks (hooks, permission rules) are brittle and out of scope.
   Example: `lym fabric` owns the overlay's git so raw `git -C` is never *needed* (it would be strictly more work), and `lym fabric status` flags drift — but it is a friction asymmetry, not a wall.
7. **Go where it can be;
   LLM only for judgment.**
   Everything deterministic — verbs, control-flow, parsing, distillation, geometry, git — is Go.
   An LLM handles only the irreducible judgment a program can't: review verdicts, triage, batch implementation, an orchestrator's recovery decisions.
   The seam is consistent everywhere: **fat Go verbs** (`lym <module> <verb>`) are the callable surface;
   an LLM session *drives* them and consumes **Go-distilled digests, never raw prose**;
   any **skill is a thin human wrapper** over those verbs, never where logic lives.

## Cwd Resolution Invariant

**All cwd resolution goes through `internal/lymcwd`, and nothing else.** `lymcwd` owns cwd resolution alone — never a records path, a junction path, or any per-module subdirectory;
those are each owned by the module that constructs them.

`internal/lymcwd` exposes this contract:

- `Getwd()` — the only permitted call to `os.Getwd` outside `cmd/lym/main.go`.
- `Resolve(cwd)` → `*Location` — resolves the current cwd into a legal worktree's coordinates, applying the strict cwd gate.
- `ResolveWithAnchor(cwd, anchor)` / `ResolveWorktree(root)` — the two ungated variants, for callers that hold something other than an acting cwd (see `docs/shared-libs/lymcwd.md`).
- `NamedWorktree(hubPath, name)` → `*Location` — the coordinates of another worktree of the hub, at that worktree's own anchor.

`Location` carries exactly four fields — `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel` — plus two derived accessors, `WorktreePath()` and `AnchorPath()`.
Every other geometry token (records paths, junctions, `_lym/<module>`, launchers, the hub-reserved name set) is a per-module constructor, joined onto `Location`'s coordinates by the module that owns that token — see [PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md) for the full per-token ownership map.

`lymcwd.Resolve` proves that cwd is the root of a git worktree and nothing more.
It succeeds in any ordinary git repository run from its root, and the `HubPath` and `RepoName` it returns are fiction in that case.
Proving a worktree is lym-initialized and Fabric-wired is a different layer's job.
See [PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md) for the tier map.

**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside `internal/lymcwd` and `cmd/lym/main.go`.
The ban is enforced at `go test` / CI time by `internal/lymcwd/enforcement_test.go`, which walks the entire source tree and fails the build if either literal token is found in any non-test `.go` file outside the allowlist.
A second scan in the same file, `TestEnforcement_GeometryLiterals`, enforces the per-token ownership map itself: no policed geometry token may be constructed as a string literal outside its registered owner directory.
A third scan, `TestEnforcement_FabricVocabulary`, enforces the separate Fabric Vocabulary Invariant: outside the owner set it names, the two side names may not appear in identifiers, string literals, or comments in production `.go` files, nor in the embedded agent prompt templates.
The fabric-sense phrase form of `host` (e.g. `host repo`, `hostBranch` — never the bare word) is banned everywhere this test reaches, including inside the owner set — `host` is retired, not merely scoped.
It shares this file's placement as a walk-helper convenience, not because the vocabulary rule is `lymcwd`'s to own — see [PATTERN-fabric-vocabulary](../pattern/PATTERN-fabric-vocabulary.md).

See [PATTERN.md](../PATTERN.md) for details.

## Documentation lifecycle

Two doc classes, opposite lifecycles:

- **Module designs** for **planned, not-yet-built** modules live in their board entry's body;
  the task building the module writes the design into the package's `doc.go` and leaves its own entry's body and scope unchanged;
  the landing marks the entry done and the board prune removes it, and the implementation and tests become the source of truth.
  A module's purpose and key design rationale then live in its Go package header comment, next to the code it documents.
- **Durable Go-to-Go contract docs** (`contracts/specs/`) pin cross-module schemas a real consumer honors — they are **kept**, not deleted on landing: `loom-status-spec.md`, `implementer-spec.md`, `webster-spec.md`, `llm-model-spec.md`, `loom-final-summary-spec.md`, `loom-plan-spec.md`. LLM-facing producer format contracts (what `Discussion-Write`/`Plan-Write` must write) live in the producer's own stencil under `contracts/stencils/`, not as a separate doc — see the Documentation Lifecycle's stencil-vs-doc split.

The other durable documentation is this `overview.md` (principles, naming, the module and shared-lib map, the records contract,
and this lifecycle convention).
Planned-but-not-built work lives on the board, one entry per item, with an unbuilt design in the entry's body.

## Records overlay model

lym organizes overlay artifacts (configuration, task state, raddle docs, and the board) into a **records repo** — a companion git repository that stays separate from the project's own repo, keeping it pristine.
The two-sided design is documented in `internal/fabricengine`'s package doc.

### Topology

```
<hub>/                              (top-level Hub, NOT a git repo)
  ├── <prime>/                      (code worktree, main branch; git repo root)
  ├── <prime>-<records>/            (records Prime worktree; git repo root)
  ├── <slug>/                       (additional code worktree; git repo root)
  ├── <slug>-<records>/             (records worktree for <slug>; git repo root)
  ├── _board/                       (records main-branch worktree; holds board.json)
  │     └── .lym/                   (hub-wide machine-local scratch; a real dir, never a junction)
  ├── <RepoName>.code-workspace     (the hub's VS Code workspace; lym-owned)
  └── _launchers/<anchor>/          (anchor-mirrored)
        └── <slug>                  (per-worktree launcher scripts)
```

`_board`, `_launchers` and the retired `_portals` are hub geometry, so none of them can be claimed as a worktree slug
(`fabricengine.IsReservedHubName`); `.lym` stays slug-reserved too, but via `structuralNeverCommittedDirs`
rather than as hub geometry, since it no longer has a hub-level presence of its own.

### Git ownership

The **code repo** is the project's source of truth, maintained by developers.
All lym-specific artifacts live in the **records repo**, a separate git repository that lym controls.
This separation keeps code commits focused on project code and delegates lym infrastructure to the records.

### Artifacts location

| Artifact | Location | Repo | Purpose |
|----------|----------|------|---------|
| `_lym/config/` | Records worktree | Records | Live YAML configuration files for all modules (board, fabric); reconciled via `lym config reconcile`; a module resolves key by key through a layer stack, see [internal/configengine](../internal/configengine/doc.go) |
| `.env` | Records worktree | Records | Git-ignored per-machine environment variable overrides (KEY=value format) |
| `_lym/raddle/` | Records worktree | Records | Raddle documentation (the raddle nav-doc overlay), reached through the `_lym` junction like every other `_lym` subtree |
| `_board/` | Hub | Board | A second records worktree, checked out on the code repo's own unsuffixed default branch (the records main branch in the common case) — never a separate clone, never a per-task records branch |
| Source | Code worktree | Code | Project source code |

### Durable vs ephemeral state (`_lym/` vs `.lym/`)

Two state roots with opposite lifecycles:

- **`_lym/`** — **durable, synced, portable.**
  Lives in the records repo (git-synced), so it survives a machine and transfers to another.
  Config, raddle, the board, and loom's orchestration **status** (current producer, run state, per-producer-call history) go here — loom resume works across machines *because* its status is fabric-synced.
- **`.lym/`** — **ephemeral, local, machine-bound.**
  Untracked in both the code and the records repo (listed in each repo's own `.git/info/exclude`, never a committed `.gitignore` in either), changing constantly while a run is live.
  The live tmux runtime state — `reed`'s (see the `internal/reedengine` package documentation) `.lym/reed.json` (the socket/session names + the strand table: each managed process, its session, parent, ephemeral pane id, and display spec) — goes here, because a pane ID or the tmux socket is meaningless on another machine.
  It is rebuilt by reconciling against live tmux on startup, never synced.
  A pane id is meaningless even on the SAME machine once the tmux server has restarted — ids are server-global and restart at `%0` — so `reed.json` also records the *pane generation*, the identity of the session incarnation its pane ids were bound against, and discards every binding minted against a different one.

The test: **would this state mean anything on a different machine?**
Orchestration progress yes → `_lym/`.
A pane handle no → `.lym/`.

### Junction model

Each code worktree has a sibling records worktree.
Code worktrees use **junctions** (Windows) or symlinks to route writes into the sibling records worktree.
Worktrees are wired eagerly at `lym fabric clone`/`lym fabric add` time — there is no separate setup step: clone and worktree-add each materialize junctions, `_lym`, and config in one call.

The wired junction set is not hardcoded,
and it is not purely the repo-wide `pathspec` list either: it is `structuralCommittedDirs` ∪ `structuralNeverCommittedDirs` ∪ the hub-reserved-filtered config names, deduplicated.
The two structural sets — `_lym` and `.lym` — are injected in code, never read from `fabric.yaml`;
only the third piece comes from the **repo-wide** `pathspec` list recorded once at `<BoardDir>/_lym/config/fabric.yaml` (read from the records main branch, via `fabricengine.BoardDir`), filtered against `fabricengine.HubReservedNames()` (the hub-structural tokens — `_board`, `_portals`, `_launchers` — that can never be a per-worktree junction).
`board.yaml` is hub-wide like it: read and written at `<BoardDir>/_lym/config/board.yaml` and never in a worktree's `_lym/config/`.
A hub-wide module skips the worktree file in its layer stack, which [internal/configengine](../internal/configengine/doc.go) describes.
Because the pathspec is repo-wide, `lym fabric reconcile` declaratively converges **every** worktree to the same recorded set — adding a junction missing on disk, removing one absent from the wired set,
and no-op'ing one already correct — rather than each worktree carrying its own drift-prone copy. `lymcwd` itself stays config-blind;
it only resolves the cwd coordinates that `fabricengine` builds the junction records onto.
This produces the two concrete junctions this repo ships with today, both placed at the repo's lym-anchor (`<code>/<anchor>/…`, which is `<code>/` itself at the default `.` anchor):
- `<anchor>/_lym` → `<hub>/<slug>-<records>/<anchor>/_lym` (config junction, structural)
- `<anchor>/.lym` → `<hub>/<slug>-<records>/<anchor>/.lym` (machine-local scratch junction, structural)

[PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md) holds the rule that forbids re-adding a junction into `<hub>/_board`: no hub-level container is ever junctioned into a worktree.

The optional `pathspec` default is empty today.
A future records-backed module is wired by appending its directory name to `pathspec`'s template default — no `fabric`/`lymcwd` code change needed — but that mechanism now applies to *optional* directories only;
a structural directory is never sourced from `pathspec`.

Raddle content is anchor-level by design — it lives at `_lym/raddle/`, reached through the existing `_lym` junction, with no `_raddle` junction of its own now or ever;
see the board's `raddle` note.

Every junction is listed in the code worktree's own `.git/info/exclude` and is never committed to a `.gitignore` in the user's repo — a tracked entry would advertise that LYM is in use.
The entry is the junction's own anchored path (`/backend/_lym`, or `/_lym` at a root anchor), never a bare name: a slash-free gitignore pattern matches at any depth, which on a subpath-anchored monorepo would silently untrack same-named directories lym never wired.
`.lym` additionally seeds `.lym/` into the **records** repo's own `.git/info/exclude` at wiring time, so records-side scratch never shows as untracked dirt either.
From the CLI's perspective, reads and writes happen transparently — code that writes to `_lym/config/loom.yaml` writes through the junction into the records repo without awareness of the indirection.

A pre-existing real `.lym` directory — every worktree that predates this junction, since several of lym's own subsystems write `.lym` unconditionally — is adopted rather than refused: its content is moved into the records-side target and replaced with the junction, one time, on the first `lym fabric reconcile` after upgrade.
`_lym` keeps the hard refusal (fabric never moves or deletes what might be the user's hand-authored content); `.lym` is the one exception because its content is always lym's own machine-local scratch.

### Branch model

Records branches mirror code-repo branching: when a new records worktree is spawned, its branch forks from the records branch whose name equals the code worktree's current branch at spawn time, preserving a shared merge-base for future squash-merge-back operations.
This guarantees subtasks (spawned from non-main branches) inherit the correct fork point: branch isolation is **not** orphan-based but **merge-base-preserving** (each on its parent's timeline). `_lym` is isolated by pathspec (junctions route it into the records;
the code repo's `.git/info/exclude` hides it) rather than by orphan topology, so no merge-back state is lost.

### Records suffix convention

The records worktree for any code worktree is deterministic:
- Code: `<hub>/<slug>/` → Records: `<hub>/<slug>-<records>/`
- Code: `<prime>/` → Records: `<prime>-<records>/` (prime is the name of the main worktree)

The suffix is fixed and non-configurable, and `internal/fabricengine`'s package doc names it.
Records paths are computed on demand from geometry and do not require a registry.

### Status

- **Go implementation** (paths geometry, paired spawn, `lym fabric` command): ✅ Implemented. `fabric` (paths geometry, paired `lym fabric add` spawn, and `lym fabric status|commit|push|pull|sync|diff|merge-in|merge|merge-stage`) is the sole git-coordination module now. `status` is the unified both-sides uncommitted-change view, also reporting `merge_in_progress`, whether THIS pair has a fabric merge parked. Paired `lym fabric add` hard-requires a records repo, which `lym fabric clone` builds — there is no separate hub-creator tool.
- **`lym config` command**: ✅ task 008 complete.
  Bare `lym config` lists modules and verbs, `lym config menu` is the interactive picker, `lym config <module>` edits, and `lym config reconcile` shipped. (A raddle config schema is **raddle** nav-doc work, not part of this task — it was only historically mis-bundled here; there is no `_raddle` junction to activate.)

```
github.com/Knatte18/loomyard/
├── cmd/lym/
│   └── main.go                   entrypoint: routes the <module> argument to a module
├── internal/adoptcli/            the adopt CLI command (`lym adopt`, the takeover of beam projects or root, and the runs under them, from another machine)
├── internal/agentname/           the sole former, parser and validator of agent names (`<shortname>:<slug>:<role>`), a stdlib-only leaf
├── internal/beamcli/             the beam CLI command (`lym beam`, the making and starting of beams from board project entries, and the wiring of their dobby)
├── internal/dobbyengine/         dobby, the daemon of a beam worktree: it starts released tasks, drives their batten runs in passes and spawns the board-tidy agent
├── internal/boardcli/            the board CLI command
├── internal/boardengine/         the board domain kernel
├── internal/hubissues/           the GitHub issues of the hub's own repository that `lym board intake` lists, reads, comments on and closes
├── internal/cleanupcli/          the cleanup CLI command (`lym cleanup`, the routine cleanups of the hub prime's leftover state, a dry run by default)
├── internal/fabriccli/           the fabric CLI command (git coordination of code and records)
├── internal/fabricengine/        the fabric domain kernel
├── internal/gatecli/             the gate CLI command (`lym gate test`, the tier runner: the hub's test tiers over a scope, slotted, bounded, counted and logged; `lym gate status`, the holders of the hub's gate slots; `lym gate report`, the machine's run records totalled per slug and gate key)
├── internal/helpcli/             the help CLI command (`lym help [<command>...]`, `lym help index`)
├── internal/shortcutcli/         the top-level operator shortcuts (`lym a`, `lym s`, `lym p`, `lym r`) from one embedded file
├── internal/idecli/              the ide CLI command
├── internal/ideengine/           the ide domain kernel
├── internal/pairteardown/        the one sequence that ends a pair: quiet wait or the skip's run-lock wait, refusal probe, owned-process end, loop and reed session end, then fabric removal
├── internal/reedcli/             the reed CLI command
├── internal/reedengine/          the reed domain kernel (overlay + strand bookkeeping)
├── internal/reedengine/render/   pure display-vocabulary leaf (Anchor, Display, ValidateDisplay)
├── internal/reportcli/           the report CLI command (`lym report convergence`, the read-only convergence report, and `lym report timing`, the read-only timing report, prime only)
├── internal/reportengine/        the report domain kernel (finished runs, wakes, stops, repairs, review rounds and landings for convergence, and per-run time, waits, attempts and stop settlers for timing, read from git objects)
├── internal/orchcli/             the orch CLI command
├── internal/orchengine/          the orch domain kernel (config, stencil renders, persisted state, cycle state machine)
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
├── internal/loomrecipe/          assembles the `*shedengine.Shed` of the loom and darn recipes from their embedded files
├── internal/shedrecipe/          the engine registry — the name to `ShedProducer`-constructor mapping a recipe loader resolves each row's `Engine` against
├── internal/shedbuild/           the recipe file format's loader and builder — decodes a recipe document and assembles the producer-definition list the shed engine already consumes
├── internal/shedverbs/           the generic run/step/status/pause/goto cobra verb bodies shared by every module that arms a `*shedengine.Shed` onto a CLI subtree
├── internal/sheddriver/          the shed layer's Go driver: its settle loop, rule table and hand-back, the tackler agent call, and the `shed` config module, told its paths and seams by the arming module
├── internal/shedcli/             the `lym shed` subtree: a named-recipe arming table for the loom, darn and batten recipes plus the three CLI seams that register it under the lym root
├── internal/statuscommit/        the shared per-transition status commit core (skip while mid-merge, commit hard-errors, push spawn warns) that `loomcli` and `battencli` wrap
├── internal/statuspane/          the pane mechanics the runs overview and the gate checklist share: a redraw-in-place pane loop and a lock-free status-file reader
├── internal/landingshed/         landing's general ShedProducers, Parent-Align, the PR-Review stub and Finalize, shared by reference across producer lists
├── internal/pawlengine/          the pawl recipe's own producers, Beam-Up, Beam-Await and Beam-Down, and the land request Beam-Await consumes, told their geometry and seams by their caller
├── internal/failuretrace/        traces a failed Parent-Align or Finalize verify log to the plan cards, batch commit ranges and untraced locations the landing failure record carries
├── internal/darnengine/          the darn recipe's hub-wide `darn.yaml` config and the verify command every darn verify reads
├── internal/driftengine/         regression around a task: the hub-wide `drift.yaml` config, the drift paths and records, the capture and compare runs and the Drift rows' producers
├── internal/driftcli/            the `lym drift` commands (`scope`, `accept`, `withdraw`, `compare`, `capture`, `defer`, `waive`, `status` and the hidden `capture-child`) and their worktree wiring over `internal/driftengine`
├── internal/languageconfig/      the hub-wide `language.yaml` config: the skills every code role loads after its own, the verify tiers the plan, review and implementer prompts render, and the hub's structure-aware rename tools the Webster implementer prompt renders
├── internal/lenses/              the `lenses.yaml` config: the lens and fan library burler, loom and darn read, holding only the names a repo tuned while the template, with the language layer a hub picks laid over it, supplies the rest, and the fan key grammar loom and darn share
├── internal/mergeresolve/        the merge-in + LLM conflict-resolution engine internal/landingshed's two producers each call
├── internal/frictionengine/      the aggregation-and-reflection step loom's terminal Friction-Reflect row runs, covering every note a halt left in the friction directory; no halt or stop runs a reflection of its own; it is re-entrant across a killed driving process
├── internal/hubgeom/             the hub-mode told-geometry teller that converts a resolved `lymcwd.Location` into each engine's geometry struct
├── internal/standalonegeom/      the told-mode geometry teller that builds each engine's geometry struct from told absolute path strings
├── internal/cliwire/             the shared standalone/hub wiring resolver for the standalone-capable CLIs, the layer that runs after `preflight.ResolveMode` has chosen a mode
├── internal/hubreconcile/        the hub config walk a start verb runs once after a binary change: build stamp, hub lock, reconcile and commit of every worktree's config
├── internal/preflightshed/       the general `Preflight` `ShedProducer` over `internal/preflight`'s tier-1/tier-2 checks, shared by reference across producer lists
├── internal/preflight/           orchestrator-agnostic tier-1/tier-2 precondition checks (geometry, worktree-pair cleanliness, Fabric readiness/sync) + the shared Report result type
├── internal/lymcwd/              cwd resolution entry gate (the sole owner of cwd resolution, nothing else)
├── internal/dotgit/              the one `.git` parser (git dir, common dir, repository test, walk-up to a worktree root), a stdlib-only leaf
├── internal/durfmt/              the duration formats the status panes and the timing report share, minute-resolution for growing times and second-resolution for closed spans, a stdlib-only leaf
├── internal/machinename/         this machine's name, its hostname cut at the first dot and lowercased, the sole reader of the hostname, a stdlib-only leaf
├── internal/sleepjournal/        the machine's suspends and hibernates read from systemd-sleep's journal entries, spawning no process, a stdlib-only leaf
├── internal/lymdirs/             the two directory-name tokens (`_lym` durable, `.lym` ephemeral), a zero-import leaf
├── internal/buildinfo/           the ldflags-stamped build channel, a zero-import leaf
├── internal/buildvcs/            the running binary's VCS identity, a stdlib-only leaf
├── internal/standalonestate/     target-path-to-hash8-and-state-directory derivation, a stdlib-only leaf
├── internal/segmentcolor/        the loom segments, lym's color palette and its tmux colors, a stdlib-only leaf
├── internal/selfexec/            the self-re-exec helper a long-lived pane process calls on a changed binary, a leaf over the standard library and `internal/logger`
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
├── internal/hibernatecli/        the `lym hibernate` commands (`request`, `cancel`, `status` and the hidden `watch`), their hub wiring and the daemon and `systemctl` spawns
├── internal/hibernateengine/     the hibernate request file and the submit, cancel, status and daemon sequences over seams
├── internal/creelcli/            the `lym creel` commands (`post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold`, `release` and the hidden `daemon`), their hub wiring and the delivery adapter over reed and shuttle
├── internal/creelengine/         the per-hub message daemon: the journal-folded pool of mail and queued injects, sender identity, reminders, injects, holds, the removed-holder sweep and the socket protocol, over a delivery seam
├── internal/output/              shared JSON output
├── internal/modelspec/           model-spec parser + models.yaml registry leaf
├── internal/runprofile/          the hub's named run profiles in profiles.yaml: template, layered-load spec, loader and lookup
├── internal/pattern/             PATTERN directive leaf (root PATTERN.md inlined per role) + format checker
├── internal/friction/            the Tier 2 friction-note directive leaf, consumed by webster, burler, and loom
├── internal/shell/               provider-invariant pane-shell mechanics leaf (pwsh + posix)
├── internal/shellguard/          the one neutral table of the guards lym installs against shell commands, rendered by each engine
├── internal/commentlint/        the comment line-break lint: fixed-column wraps in the `//` comment blocks a diff creates, over a told worktree and base
├── internal/patternlint/        the PATTERN headroom lint: growth of the overview past its headroom line, over a told worktree and base
├── internal/impactset/          the impacted-set command a gate key's `diff` scope runs: the packages a diff reaches over the import graph, plus the `//lym:guard` tests, or a fallback reason to run the full verify
├── internal/dotnetimpact/       the `dotnet` scoper's derivation: the test projects and test classes a C# diff or a card's targets reach, by type-name closure over the source, or a fallback reason to run the tier whole
├── internal/verifytree/         the one plan-verify function: clean-tree check, verified-tree record and running marker, used by landing, both webster gates and `lym webster verify`
└── internal/verifyrun/          in-process shell-command runner behind `internal/verifytree` and webster's card-verify rerun
```

`cmd/lym` is `package main`;
everything else is in `internal/`. `main` is the only thing that imports a module.

## Module dispatch

`cmd/lym/main.go` assembles all modules into a single cobra root via `newRoot()`.
Each module contributes a `Command() *cobra.Command` that is passed to `root.AddCommand(...)`, so every module and subcommand is discoverable via `lym --help` without any central dispatch table.
The `help` module is the one exception: `newRoot()` mounts it with `root.SetHelpCommand(helpcli.Command())` followed by `root.InitDefaultHelpCmd()`, replacing cobra's built-in help command.
Adding a module is three steps: import the package, add `<module>.Command()` to `root.AddCommand(...)` in `newRoot()`, and append the module name to `root.Long`.
Every invocable command also carries an `audience` annotation (`operator`, `role` or `internal`), and `lym help index` renders the operator commands as a one-line-per-command index of the running binary, which the orch's role file and the `ly:cli` skill carry.
Which commands count as invocable, and so carry one, is the audience bullet of [PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md).
A tree walk in `cmd/lym/clitree_test.go` pins the form of `Use`, `Short` and `Long` beside the audience.

`run(args, out)` is the testable seam: it builds a fresh root, merges stdout and stderr into `out`, and calls `root.ExecuteContext`, returning the process exit code without spawning a binary or trapping `os.Exit`.
Each module also exposes `RunCLI(out io.Writer, args []string) int` — exactly `return RunCLIIn("", out, args)` — as an in-process test seam that drives a module in isolation without involving the cobra root.
Every module but one also exposes `RunCLIIn(cwd string, out io.Writer, args []string) int`: `cwd == ""` delegates to `clihelp.Execute(Command(), out, args)` exactly as `RunCLI` always has, and any other value delegates to `clihelp.ExecuteIn(Command(), cwd, out, args)`, seeding `cwd` into the execution context so the module's handlers read it back via `lymcwd.CwdFrom(cmd.Context())` instead of the process working directory.
`internal/selfreportcli` is the one seam module without `RunCLIIn`, since it references `lymcwd` nowhere.
`clihelp.ExecuteIn`, `clihelp.RunRootCtx`, and `clihelp.WrapRunCtx` are `Execute`/`RunRoot`/`WrapRun`'s context-carrying siblings: they seed an explicit cwd or propagate an existing context into a command's execution instead of relying on the process working directory, letting a handler read it back via `lymcwd.CwdFrom(cmd.Context())`.
`cmd/lym/main.go` uses `RunRoot` unchanged.

All commands print JSON: `{"ok":true, ...}` on success, `{"ok":false,"error":"..."}` on failure (exit code 1).

## Modules

User-facing modules each get one `lym <module>` namespace:

- **board** — the task-tracker board, which is also the roadmap (`internal/boardcli` + `internal/boardengine`).
  One `board.json` store holds every entry, and each entry is a task or a note with labels; only a task runs.
  `types` and `labels` in `board.yaml` are maps from label to description, and `lym board labels` prints both in file order.
  `lym board intake` lists the open issues of the hub's own GitHub repository not yet on the board, imports one as a note or folds it into an entry, and closes noise with a stated reason.
  The hub's repository is the one its recorded binding names, else its origin remote, so only the loomyard hub's intake reads LoomYard's issues; intake never goes through selfreport.
  The README renders Tasks split into Running, Ready (waiting on nothing open) and dependency layers whose entries can run in parallel, then Notes grouped by type label, each subsection one numbered table that links each slug to its design doc.
  An entry may carry a priority, `high` or `low`, absent meaning normal, which orders it first or last within its subsection.
  A task may name a run profile in its `profile` field, and `lym board profiles` prints the hub's profiles with their settings and each recipe's default profile.
  Agents use the board through the `ly:board` skill.
  ✅ Implemented.
- **config** — bare `lym config` lists modules and verbs, `lym config menu` picks a module to edit interactively, and `lym config <module>` edits that module's config;
  `lym config reconcile` reconciles all module config files against their live templates (dry-run by default, `--apply` writes atomically) except seed-only modules (today: `models`), which are materialized once when absent and never rewritten again since the file is operator-owned;
  `lym config reconcile --layer default` seeds absent board files sparse, prunes but never fills a present one, and lists the one-time base strip as pending, which only a start verb's hub reconcile performs; with `--apply`, run from the prime, it commits them in `_board`;
  `lym config <module> --set key=value` (repeatable) writes one or more config values directly with no editor invocation, for scripts/agents that need a non-interactive path.
  A key under a module's declared open map adds or rewrites one entry, e.g. `lym config board --set labels.quarry="glyphs and the quarry index"`.
  A key that takes one value or a per-round list, such as loom's `reviewer_model` and `plan_reviewer_model`, is written as a list when its value starts with `[`, e.g. `lym config loom --set 'plan_reviewer_model=["opus[medium]", "sonnet[high]"]'`, and a bracketed value that is not a list of scalars is refused.
  Every `--set` write is then loaded through the module's own loader, and a value it refuses, such as a list given to loom's single-model `plan`, is refused naming the key with the file left as it was.
  A module resolves key by key through the embedded template, the repo default at `<BoardDir>/_lym/config/<module>.yaml`, the worktree's sparse file and the untracked local override at `<anchor>/.lym/config/<module>.local.yaml`; see [internal/configengine](../internal/configengine/doc.go).
  A hub-wide module's machine keys, such as gate's `slots` and `tier_parallel`, also take a never-tracked machine override at `<BoardDir>/.lym/config/<module>.machine.yaml`, written from the prime beside the local override.
  `lym config --layer default|worktree|machine|local` picks the layer a verb addresses, and `lym config <module> --resolved` prints the merged result with each unit annotated by its layer.
  A board task's `run_settings` reach the pair's local layer at batten's Seed-Child, bounded by `configreg`'s `LocalKeys` and `LocalDenied`.
  A task's `profile` names a run profile in the hub-wide, seed-only `profiles.yaml`, whose block for the task's recipe Seed-Child writes under the task's `run_settings`, and a task naming none takes its recipe's default profile;
  the starting profiles are the ladder `speed`, `light`, `solid` and `wide`, plus the darn-only `docs`, and `lym config profiles --set <name>='<json object>'` adds or replaces one whole profile (see [internal/runprofile](../internal/runprofile/doc.go)). ✅ Implemented.
- **fabric** — the sole git-coordination module over the code and its records, unified over two `internal/gitrepo.Repo` instances: clone (the hub creator), dual-worktree add/remove, coordinated checkout (switches code and records together + re-points junctions), reconcile, status, prune, cleanup, records content-sync (commit/push/pull/sync/diff; in the prime, push, commit and sync push the records side only and `push` reports an unpushed code branch as `code_push_skipped`), and a merge/conflict lifecycle (`merge-in`/`merge`/`merge-stage`/`merge --continue`/`merge --abort`, mirroring git's own exit codes and surfacing conflicts as unified, worktree-relative paths; `merge-stage` marks resolved paths so `--continue`'s index gate can pass, and is the only route for a conflict under a wired junction name, which git refuses to stage through), all in one command tree (`internal/fabriccli` + `internal/fabricengine`);
  CLI surface is `lym fabric clone|add|list|remove|checkout|pairs|reconcile|prune|cleanup|unwire|shortname|status|commit|push|pull|sync|diff|merge-in|merge|merge-stage`.
  `clone` takes the records URL first with the code URL optional, derived from the code binding recorded on the records main branch when omitted;
  `reconcile` backfills that binding for hubs whose records predate it.
  `clone` also takes `--shortname` to record the repo's shortname as `.lym-shortname` on the records main branch,
  and `shortname [<shortname>]` prints the recorded shortname or records one on a hub that has none.
  `status` is the unified both-sides uncommitted-change view (`Fabric.Status`);
  `diff` reports the side-labelled changes since a given code SHA (`Fabric.Diff`).
  `pull` is now unified across code and records, not records-only: it fast-forwards the records first, then fetches and inspects the code, detecting a rebased/force-pushed code remote via ancestry and safely re-anchoring the records' correspondence to it when it is safe to do so.
  `remove` also deletes the pair's local task branch once both worktrees are gone, when its work is pushed or landed on the recorded parent, and keeps the branch with a reason in the result otherwise, reporting both as `code_branch_deleted` and `code_branch_kept_reason` on the envelope;
  `--force` never overrides that check.
  ✅ Implemented; see the `internal/fabricengine` package documentation for rationale.
- **ide** — one-shot VS Code launcher with interactive menu.
  `lym ide spawn <prime>` opens the hub workspace `<hub>/<RepoName>.code-workspace` (the prime, `Bolt` and every task pair) whose `settings` carry the prime's `.vscode/settings.json`.
  The file is lym-owned and regenerated on each prime spawn, overwriting any edit made in it;
  a task slug still opens its bare folder.
  The generated `tasks.json` holds one on-demand `reed attach` task, with the `lym` path stamped absolute at generation time, reached through "Tasks: Run Task" or the seeded `ctrl+alt+a` key binding;
  no task runs on folder open, so opening a worktree in VS Code starts no session and no `claude`.
  `lym ide spawn` and batten's Seed-IDE row write this config through one shared path.
  `lym ide spawn` regenerates `tasks.json` on every spawn, overwriting an untracked one, and keeps `settings.json` when present.
  It keeps `.vscode/` out of git through the repository's shared `info/exclude` at the anchor subpath rather than `.gitignore`, and leaves a tracked `tasks.json` alone with a warning.
  `lym ide spawn` also seeds a `// lym:begin` … `// lym:end` block into the user's VS Code `keybindings.json`, forwarding the four Alt keys of reed's tmux bindings to the integrated terminal and binding `ctrl+alt+a` to run the `reed attach` task, and reports the outcome in its JSON result.
  It writes only between its own markers, and a file it cannot merge safely is skipped with a reason and never fails the spawn.
  Batten does not open it: VS Code opens only when the operator runs `lym ide`. ✅ Implemented.
- **selfreport** — file bugs and enhancements against `Knatte18/loomyard` via go-github through `internal/githubclient`, filed by `lym selfreport create <title>`, run by the operator or by an agent; the friction reflection is its only automatic caller, and no Go code files an issue itself.
  It files LoomYard bugs only, from whatever hub lym runs in; an issue about another repository is filed to that repository's own issues.
  Credentials resolve from `GH_TOKEN`/`GITHUB_TOKEN` first, with the `gh` CLI (`gh auth token`) as a bounded, non-blocking fallback token source — not a hard prerequisite.
  Target repo is hardcoded;
  supports `--body` (or `-` for stdin) and `--label`;
  defaults to `bug`.
  Callable from any sandbox agent context with no config. ✅ Implemented.
- **reed** — **the window to the world**: tmux overlay + **strand** bookkeeping + render (`internal/reedcli` + `internal/reedengine` + `internal/reedengine/render`). Hosts every managed process as a strand in a window of its own or its group's, persists to `.lym/reed.json` (`lym reed up|add|remove|status|list|attach|switch|resume|watchdog|down`); `lym reed add` places a strand `--anchor own-window` by default. Every strand carries a hierarchical full name (see the `internal/agentname` package documentation), mirrored into its pane title and Claude session name and exported to its process as `LYM_STRAND_NAME` and `LYM_PARENT`, with the watchdog repairing drift; `list` is the hub-wide name directory. Built on what its proof-of-concept, `muxpoc`, proved first (env hygiene, native `--resume`); `muxpoc` has since been deleted, its job done. `reed attach` and `reed watchdog` are this module's two registered interactive-handoff exceptions ([PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md)): `attach` hands the operator's stdio to a `tmux attach-session` child in place, pre-flighting through checking tmux's floor version, booting the session if cold and checking server/session liveness, and `watchdog` is the detached per-hub name-repair and reap daemon that blocks for its process lifetime — in both cases every fallible step runs pre-flight, on the envelope, and only the terminal-handover/daemon tail itself is exempt from emitting JSON. `add` and `attach` boot this worktree's session when none is up, with `up`'s own semantics — a bare substrate, with no persisted strand relaunched — instead of refusing; every other reed verb still refuses on a cold worktree. ✅ Implemented. See the `internal/reedengine` package documentation.
- **shuttle** — run **one** LLM agent as an interactive tmux strand over the file contract (`internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttleengines` + `internal/shuttlecli`; `lym shuttle run|open|interrupt|send|state`). `lym shuttle open` starts a named session with no turn in its own window, which closes when the session exits; `--model` takes a model spec naming the engine, model and effort, `--list-models` prints the model registry's aliases with their engine, model and default effort, and the output names the engine, model and effort the session got. Every session shuttle launches, run or opened, starts in its engine's bypass mode unless the caller passes `prompt`, which an interactive session accepts. `lym shuttle await <path>... [--cap]` is the read-only capped wait on one or more files, the keep-cache-warm wait any agent can run, needing no hub or git repository. `lym shuttle state` is read-only: it lists the session state of each running run (busy, idle-done, idle-stalled, idle-fresh, asking, dead or unknown, with its cause, since and history), read from the run's files, in shadow mode where no stop or notice depends on it. `Stop`-hook completion is read off an events file and classified into three outcomes — `done`/`died`/`timeout`. A turn end without every output file never ends a run: `Wait` holds it and keeps polling the same agent until the outputs exist, the pane dies or the deadline passes, whatever the agent's reason (a question, a status report, a live `AskUserQuestion` call). An autonomous run notifies its parent once per held turn end as creel mail to the orch, and once per permission or input dialog its pane shows past 60 s, and an interactive run is answered in its pane; `lym shuttle send` ends every message with a fixed tail. Background work is read off the Stop payload's `background_tasks` first, with the transcript as the fallback when the payload carries no list; a turn end with outstanding work is waiting, not held, unless its only outstanding work is non-awaited shells, which a gated run judges as a plain turn end and an ungated run holds once they pass the shell wait bound. `Attach` waits on a held run, and on a legacy `asking` record whose strand is still live, instead of respawning beside it. `died` is narrow: it means a strand reed STILL TRACKS has a pane that is not alive, the provider never came up inside the startup window, or the provider's process is proven gone behind a live pane shell (by the process registry, or a process-ending `SessionEnd` line past the wait's first liveness tick), with `done` instead when every output file exists. Reed's strand table losing the strand entirely — a `reed down`/`remove`, a deleted or rebuilt `reed.json`, a worktree renamed under an in-flight run — is reported as a mechanism failure carrying the run's identity, never as `died`, because reed's bookkeeping going away says nothing about the agent, whose process is often still working in its pane. `PreToolUse` guardrails deny the in-process `Agent` tool in both interactive and autonomous runs (on by default, switchable off via `claude.yaml`'s `deny_agent_tool`, and narrowed in a `ForkSubagents` run to permit fork subagents while still refusing every other subagent type), and `AskUserQuestion` too when the run is autonomous (`Interactive: false`, the default); a `PreToolUse(Bash)` guardrail denies `python`, `python3` and `python3.<minor>` in command position in every run mode (on by default, switchable off via `shuttle.yaml`'s `deny_python`), and a second `PreToolUse(Bash)` guardrail, installed in every run mode with no switch, denies a raw module-wide `go test`, `go build` or `go vet`, a raw `go test` over the `tmux` or `llm` tier, and any command naming the gate-slot or strand-name environment variable, steering the session to `lym gate test`, and a third `PreToolUse(Bash)` guardrail, installed in every run mode with no switch, denies an `rm` on a computed path (a command substitution or a variable expansion in the command) so no permission dialog stalls the run; the python deny's steer names `sed -n` among the readers. The engine announces each deny it installs to the session's system prompt (`--append-system-prompt`, on launch and resume), so a session knows the tool is unavailable before it tries one. The engine sets the prompt-cache TTL per role on launch and resume from `claude.yaml`'s `prompt_cache_ttl` and `prompt_cache_ttl_roles` (template: 5 minutes for `driver`, 1 hour for `webster` and `orch`, 5 minutes for every other role). The provider is swappable behind an **engine** seam; the engine is picked from the registry by `shuttle.yaml`'s `engine:` key, Claude being the one registered engine. Per-run `Model`/`Effort` knobs (`lym shuttle run --model`/`--effort`; effort values `low|medium|high|xhigh|max`, empty = provider default) are engine-validated, not policed by `Spec.validate`. `Spec.Version` is a programmatic engine-validated version pin (claudeengine composes the pinned model id; no CLI flag — consumers drive it via the model-spec notation's `v=` param). Starting a run returns only once its provider is past its startup gates, and a provider that never comes up is torn down (run dir and last pane capture kept) and reported as an error from `Start`, or as `died`/`timeout` from `Run`. ✅ Implemented. See the `internal/shuttleengine` package documentation.
- **orch** — the hub orchestrator hosted as an interactive Claude session in a reed strand of the prime worktree or of a beam, cycled before its context fills (`internal/orchcli` + `internal/orchengine`; `lym orch stop|status|refresh|distill|withdraw|rules`, in the prime and in any beam). `lym beam start` first ensures the worktree's `runs` strand, which runs `lym batten overview` as the beam's window, named `beam: prime` or `beam: <slug>`, and `stop` leaves it; the orchestrator has a window of its own, and the session's windows run the beam window, agents, shell. A detached watcher has the session write a note, then clears it and resumes it from that note (`refresh`) or compacts it in place (`distill`), never clearing or compacting without one. ✅ Implemented. See the `internal/orchengine` package documentation.
- **webster** — the implementer module: one long-lived Master session that reads the plan and the repo's rules once, never the code, then forks one implementer per execution batch in-session (Claude Code's Agent tool), each fork reading its cards' code itself, instead of spawning a fresh reed/tmux strand per batch;
  bracket verbs (`begin-batch`/`record-batch`) drive each in-session fork (Master ends its turn while a backgrounded fork works and records the batch on the fork's completion notification, shuttle reading that turn end as waiting rather than done), and a genuine model escalation (recovery after a stuck/report-less fork) spawns a cold strand.
  Consumes the flat card-list plan (pinned in `contracts/stencils/loom/loom-template-plan.md`, the Plan producer's own stencil) via its own sole parser, `internal/planparser`, groups cards into execution batches via `internal/batcher`, a separate config-selected registry module webster consumes (profiles in `batcher.yaml`, the `cautious` cost profile by default), then runs those batches in card order, refusing a backward dependency derived from the cards' own `Targets`/`Uses` refs, records the partition in `state.json` at first init, and carries the plan's `## verify:` command as a must-pass gate on Merriam's strand: a failure is rerun once (a pass on the rerun records the identities as flaky;
  a run that exceeds the verify timeout is not rerun, since a hang is not flakiness, and fails the gate at once with the timeout, the log path and the log tail as findings),
  and a failure that survives goes back to Merriam, who fixes it through one fixer fork, or on the gate's last attempt and after a fixer that committed nothing through the cold fixer `lym webster fix-verify` spawns at the recovery role, up to `verify_gate_attempts` attempts (`internal/websterengine` + `internal/webstercli`). ✅ Implemented.
  `lym webster verify` runs the same plan-level verify on demand through `internal/verifytree`;
  like the other read-only verbs it boots no reed session.
  `run` refuses a `--plan-dir` override that moves the plan off the mode's own default, in BOTH modes: Master's in-pane verb invocations are typed flagless from its stencil, so they resolve the default and would refuse against a plan they cannot see. Every other verb keeps honouring the override.
  In standalone mode (`lym webster run --target-dir <repo>`, a plain git checkout with no hub), `run` boots its own private reed session in-process (`lym reed up` is hub-only and cannot reach standalone's derived geometry), requires the plan at the derived state directory's own `_lym/plan`, and renders every prompt's plan-directory references as the absolute state-directory paths the panes can actually read.
  The target is normalized to its repository root before anything derives from it, so running from a subdirectory drives the same state directory, reed session and plan as running from the root — the derivation hashes one spelling of the repository, never the directory the operator happened to stand in.
  A `begin-batch` refusal with `plan_conflict` goes first to the cold plan fixer `lym webster fix-plan` spawns at the master role, which matches the cards the conflict names to the tree and rebaselines them, and only a conflict whose fix would change what a card does ends the run stuck.
  `recover-batch`, `fix-verify` and `fix-plan` boot the same private reed session `run` does, because each spawns a cold strand of its own; the read-only verbs still boot nothing.
  Loom's `Webster` row runs webster when `loom.yaml`'s `implementer` key, settable per task through `run_settings`, names `webster`, its only value; the row's `Implementer` engine reads the key, and webster partitions the plan itself at the row's start.
  See [webster-spec.md](../contracts/specs/webster-spec.md) and [implementer-spec.md](../contracts/specs/implementer-spec.md), the contract every implementer of that row keeps.
- **planparser** — the sole parser of the on-disk flat card-list plan format (`_lym/plan/`, see [loom-plan-spec.md](../contracts/specs/loom-plan-spec.md));
  no other package reads that tree directly, and it also declares where that tree *is* — the worktree-relative form (`PlanDirName`/`PlanDirRel`) and the absolute told-anchor form (`PlanDir`/`PlanOverview`), with the caller supplying the anchor path (`internal/planparser`). ✅ Implemented.
- **planglyph** — the sole owner of every `quarry.Repo` call (`Open`, `Resolve`, `EncloseAt`, `DeltaGit`) and of the package-level `quarry.Name`, plus the resolve-backed validation pass layered on top of `planparser`'s pure checks: `planglyph.ValidateFormat`/`Validate` call `planparser.ValidateFormat`/`Validate` and append only resolve findings, composing rather than reimplementing (`internal/planglyph`). ✅ Implemented.
- **planindex** — the cgo-free seam over `planglyph`: the plan gates' finding types, `ErrQuarryUnavailable` and the `Index` and `Delta` interfaces a package receives instead of importing the package that links tree-sitter (`internal/planindex`). ✅ Implemented.
- **help** — cobra's help command, replaced: `lym help [<command>...]` prints a command's help and refuses a word that names no subcommand, and `lym help index [--audience <audience>]` prints a generated one-line-per-command index of the running binary, filtered by the `audience` annotation every invocable command carries (`internal/helpcli`; the renderer is `clihelp.RenderIndex`). ✅ Implemented. See the `internal/helpcli` package documentation.
- **shortcuts** — `lym a` (`lym reed attach`), `lym s` (`lym beam start`, then `lym reed attach`), `lym p` (`lym tender pause`) and `lym r` (`lym tender resume`) are top-level operator shortcuts declared in `internal/shortcutcli/shortcuts.yaml`, embedded in the binary and added only by a commit to that file; global verbosity flags reach every expanded command and other words the last; they are not modules. See the `internal/shortcutcli` package documentation.
- **cleanup** — the routine cleanups of the hub prime's leftover state, as one verb (`internal/cleanupcli`; `lym cleanup [--apply]`, from the prime and from any beam).
  Without `--apply` it is a dry run that lists, per segment, what `--apply` would remove.
  The segments run in order and each calls its owner's own cleanup in-process: `lym batten cleanup`, `lym board prune`, the orch's handoff cleanup, the prime's `.scratch/` children older than 14 days that no open board entry cites, logger's trace retention sweep, and the temp root's sweep of dead owners' run directories.
  A failing segment never stops the later ones.
  ✅ Implemented. See the `internal/cleanupcli` package documentation.
- **adopt** — the takeover of beam projects, or of root, and of the runs under them from another machine of the operator, as one verb (`internal/adoptcli`; `lym adopt <beam>...`, `lym adopt root` and `lym adopt --all`, with `--keep-local` and `--drop-local`, from the prime and from any beam).
  It reconciles the hub against origin, stops a beam or a run under it whose pair holds local commits origin lacks or is live, adds the missing pairs, records this machine as the owner of the beams and of the prime in one pushed owner write, lifts the owner pauses and resumes each moved run as a detached first pass, driven on by its parent's dobby.
  A call that recorded an owner then starts the hub's `finalize` gate once, detached, in the prime, so a break this machine brings to the landed code shows at adopt time.
  It calls each owning module's function in-process and has no engine package of its own.
  ✅ Implemented. See the `internal/adoptcli` package documentation.
- **beam** — the making, landing, starting and stopping of a beam, a long-lived pair that groups board work, and dobby, the daemon every beam and the prime run (`internal/beamcli` + `internal/dobbyengine`; `lym beam create <slug>`, `lym beam land <slug>`, which writes the land request the beam's pawl run consumes at Beam-Await, `lym beam start [slug] [--all]`, `lym beam stop [slug]` and `lym beam status`, from the prime and from any beam, and the hidden `lym beam dobby`).
  Create runs from the beam's parent worktree, claims the root when none is recorded, records this machine as a board project's owner with the beam flag and the pawl recipe, and starts the beam's pawl run as a detached first pass, whose Worktree-Create makes the pair, forked from the parent beam of a nested beam, and whose Beam-Up marks it.
  Land starts a short batten run over the `beam` recipe, after a refusal that stops it while members or forked pairs remain under the beam.
  Start brings up a beam's dobby and orch, the one the acting worktree is or the one a slug names, or with `--all` the prime and every beam this machine owns, each part only when it is not live.
  Stop takes a beam's orch and dobby down.
  Status prints, once, the dobby section a beam's window draws, for the prime and for each local beam: alive or not, the runs it drives with their state, stage, time in stage and last pass, and each released task it has not started with what the task waits on, and the beam's orch: strand live, watcher live and the recorded session.
  Dobby starts the beam's released tasks from an empty status, drives their batten runs in short passes, notices the nearest live orch and spawns the board-tidy agent every few landings.
  It writes one line to its `dobby.log` for each task it starts and each pass it gives a run.
  The verbs call each owning module's function in-process, and `internal/dobbyengine` holds the daemon.
  ✅ Implemented. See the `internal/beamcli` package documentation.
- **report** — the read-only convergence and timing reports (`internal/reportcli` + `internal/reportengine`; `lym report convergence [--since <date>] [--fresh-days <n>] [--format json|markdown]` and `lym report timing [--since <date>] [--by day|week] [--format json|markdown]`, prime only).
  The convergence report says, per finished run and per ISO week, what woke the orch, what stopped and repaired runs, how many review rounds each segment took and how landings fared.
  The timing report says where run time went, per run, per phase and per day or week, with machine sleep, waits and the time a run stood paused apart, and who settled each stop.
  Both cover loom and darn runs, each tagged with its recipe, and read records lym already writes.
  ✅ Implemented. See the `internal/reportengine` package documentation.
- **tender** — the one ordered pause that makes the hub safe to suspend within seconds (`internal/tendercli` + `internal/tenderengine`; `lym tender pause [<slug>] [--grace <seconds>]`, `lym tender resume [<slug>]`, `lym tender revive [<slug>]`, `lym tender status` and `lym tender reap`, from any worktree of the hub; `lym p` and `lym r` are shortcuts for the first two verbs).
  `lym tender pause` records an entry, waits a short grace for busy agent sessions and network operations to finish, interrupts every covered session still in a running turn with Esc and holds every path that would start agent or network work; the orchestrator is interrupted by its own watcher at the grace end.
  `lym tender resume` lifts the entry and gives each interrupted session one line to carry on.
  `lym tender revive [<slug>]` gives one fixed carry-on line to each session stalled on an API error, and the orch a checked creel inject, reported with its id; each send runs under the pause record's write lock, one at a time, so it types only while the session still reads stalled, sends no key and kills nothing.
  `lym tender status` lists the live entries, the network operations in flight and the parent locks held, every live session in the hub with its state, cause and whether an entry covers it, the lym processes running a replaced binary, the processes registered to runs with their orphans, and each pair's owner machine with the run-held entries owned by another machine; it changes nothing, and any caller may run it.
  `lym tender reap` ends the orphans of one fresh reading of that processes part, which are the processes of a slug with no pair and no running batten and the unregistered lym processes in a deleted hub directory, never a hub-owner process or one working inside a registered worktree; the orch and an external caller may run it, and a survivor makes it exit non-zero naming the pid and role.
  An agent may pause, resume or revive only its own pair; the orchestrator and an external caller may do each for the hub or any pair.
  Every lym process in a hub runs its network git and GitHub calls under a gate that waits while a pause covers it, and every reader that reacts to an unexpected stop or to idle time consults the record (`internal/pauserecord`).
  ✅ Implemented. See the `internal/tenderengine` and `internal/tendercli` package documentation.
- **hibernate** — a queued hibernate of the machine that lym's own Go code carries out once every session in the hub is safely paused, so no LLM session runs it (`internal/hibernatecli` + `internal/hibernateengine`; `lym hibernate request [--after <slug>] [--after-all]`, `lym hibernate cancel` and `lym hibernate status`, from any worktree of the hub; Linux only).
  `request` records the hub's one request and spawns a detached daemon, the hidden `lym hibernate watch`, then returns; the trigger is at once, the end of one batten run's Land row, or no batten run in flight.
  The daemon waits for its trigger, pauses the hub through tender's engine, confirms every session paused, hibernates the machine, detects the wake and lifts its own pause entry by identity, then posts creel mail to the orch when it is done or failed.
  `cancel` ends the request, stops the daemon and lifts the entry, and is refused while the machine is going into hibernation; `status` changes nothing and any caller may run it.
  Only the orch or an external caller may request or cancel; an agent asks its parent session.
  Bounds: the daemon waits a fixed window for the pause to confirm and for the wake, and fails the request, leaving its reason in the file, when either runs out.
  ✅ Implemented. See the `internal/hibernateengine` and `internal/hibernatecli` package documentation.
- **creel** — the per-hub message daemon, the one way to reach an agent: mail from an agent strand, announced by a reminder typed at the receiver's idle and pulled with `lym creel read`, while mail from any other sender is carried in full in the line creel types when it fits the line cap, and lines injected into its pane (`internal/creelcli` + `internal/creelengine`; `lym creel post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold` and `release`, from any worktree of the hub, and the hidden `lym creel daemon`).
  The first verb of a hub starts its daemon, which folds its pool from an append-only journal, names a sender from the connecting process's ancestry, and delivers through shuttle only into a pane that reads idle.
  Notices and the orch's typed lines go through creel: batten, loom, burler, webster and hibernate post their notices as mail to the orch strand, signed as their own program.
  ✅ Implemented. See the `internal/creelengine` and `internal/creelcli` package documentation.
- **gate** — the tier runner: `lym gate test` runs the numbered tiers the hub declares in `gate.yaml` over explicit targets or `--all`, each tier taking the hub-wide gate slots it asks for, bounded by its `timeout_min`, counted by its `go-json` or `vstest` counter, and logged with a summary line and a run record under the board; a `--card` scope under the `go` scoper also vets the card's packages under each other tier tag that adds a file to one of them, so a tag-set compile break fails at the card; a failing tier stops no later tier, vet or guard run, a closing line names every failed run, and the call exits with the first failing run's code. When the hub stays busy past the keep-cache-warm cap `lym shuttle await` defaults to, it answers `slot_free: false` naming the holders and exits 75, which is no failure when no earlier run failed, and the caller runs it again; `--wait` lifts the cap (`internal/gatecli`; `lym gate test [-C <dir>] [--gate <key>] [--tier N]... [--all] [--guards] [--wait] [<targets>...] [-- <args>]`; `--gate <key>` runs the tiers and scope `gate.yaml`'s gate key names, and with `--guards` the verb also runs every `//lym:guard` test of the module under the command of the tier its file belongs to). `lym gate card-verify <card id>` runs a plan card's `**Verify:**` line through the shared `internal/cardverify` runner and exits with the line's code. A run inside a slot an enclosing gate run holds takes no second one, and a git worktree outside every hub runs unslotted under its own `gate.yaml`. A Go test binary that keeps a core busy past `busy_quit_sec` gets one SIGQUIT, so its goroutine stacks reach the log long before the tier's own timeout.
  The `dotnet` scoper narrows a C# hub's computed, `--base` and `--card` scopes to the test projects and classes that name a changed type, rendered into the tier's `{filter}` placeholder (`internal/dotnetimpact`), and a hub's `build` step runs once per call inside the slots of the first tier that sets `shared_build`, with its own summary line and run record.
  `inert_paths` makes a diff of files no test reads run no tier, and a tier with `carry: false` runs whole under every scope form but explicit targets and never carries.
  [dotnet-hub.md](dotnet-hub.md) sets a .NET hub up. ✅ Implemented. See the `internal/gatecli` package documentation.
- **quarry** — the planner's only source of glyph spellings: four read-only repository queries over the current worktree's own glyph alphabet, each delegating to a `planglyph` wrapper and emitting quarry's own rendering verbatim, with no repository-path flag on any of them (`internal/quarrycli`; `lym quarry toc|glyphs|resolve|expand`).
  `toc` answers a table-of-contents query for a path;
  `glyphs` answers the same query under quarry's frozen depth-all/symbols-on preset, projected into a flat, depth-first index — the spelling the planner copies verbatim, and with `--text` quarry's own one-line-per-symbol view, which a line filter can cut whole symbols from;
  `resolve` checks that one or more copied spellings, given positionally in one call, name something real, answering each glyph separately: it prints every answer and exits non-zero when any glyph is negative;
  `expand` reports a type glyph's own head plus every member whose owner chain begins with it.
  `delta` and `name` are deliberately absent — both are pipeline-internal, and `name` in an agent's hands is a glyph-spelling machine, the one thing the copied-verbatim rule exists to prevent.
  `quarrycli` is a named CLI/Cobra Invariant package-naming deviation: it imports `internal/planglyph`, not a `quarryengine`. ✅ Implemented.
- **discussionparser** — the sole reader of `_lym/discussion/`'s on-disk format (the decision record's required sections, the support log's existence and the relevant list's format);
  it takes told absolute paths and declares no location of its own — deliberately unlike `planparser`, because `loomengine`'s accessors take a `*lymcwd.Location`, which this stdlib-only leaf may not import.
  Consumed by `loomshed.ValidateLoomDiscussion` and through it `loomshed.NewDiscussionGate` (the Discussion-Write and Discussion-Burler-Fix rows' own gates), `loomshed.NewDarnDiscussionGate` (the Darn-Write row's gate) and by the `lym loom validate-discussion` verb (`internal/discussionparser`). ✅ Implemented.
- **summaryparser** — the sole declarer of the final-summary artifact's filename and the sole parser of its format (see [loom-final-summary-spec.md](../contracts/specs/loom-final-summary-spec.md));
  it takes told paths and declares no directory of its own, and is stdlib-only so neither consumer depends on a producer.
  Consumed by `internal/landingshed`'s `Finalize`, by `internal/websterengine`, by the `description` gate on loom's `Describe` row, and by the `lym loom validate-description` verb (`internal/summaryparser`). ✅ Implemented.
- **batcher** — the batchifier registry that groups a plan's flat card list into webster's execution batches, through named profiles in `batcher.yaml` picked by its `active:` key (template default: the `cautious` profile, the cost batchifier; an empty value resolves to identity, one card per batch); its own standalone configreg module, separate from webster's (`internal/batcher`). ✅ Implemented.
- **stencil** — the operator surface over the hub's producer-prompt stencils (`internal/stencilcli` + `internal/stencilstore`; `lym stencil list|validate|diff|sync|promote`): `list` reports every registered stencil's board-copy path and edit state, `validate` reports marker mismatches between a board copy and its shipped default, `diff` shows upstream changes not yet taken or (`--all`/`--exit-code`) board edits not yet ported back, `sync` force-refreshes every stencil against the shipped registry even from a `-dev` or unstamped build, committing under its own `lym: sync <subtree> (...)` subject, and `promote` copies a board-copy edit back into the worktree's `contracts/stencils/` source tree. The port-back drift warning classifies a differing board copy five ways (hand-edited, source-ahead, both, neither, behind) and names the matching remedy: `promote` for a hand edit, a production deploy for a source-ahead copy, a manual reconcile for both, `lym stencil sync` for neither, and syncing the worktree with main for behind, a worktree whose HEAD lacks the binary's build commit. `list` and `sync` also cover the deployed `contracts/specs` registry; `validate`, `diff`, and `promote` do not — a spec declares no markers for `validate` to compare, and `diff`/`promote` both need the worktree source directory a deployed spec deliberately does not have. The root pre-run seeds both subtrees only when a write is due: a board copy moves forward only, by the build ancestry or commit time recorded in its banner (the writer keys), and only from a clean, stamped production binary, while an unstamped or dev build seeds an absent file and warns instead of refreshing. A due write pulls Bolt first and writes nothing when Bolt cannot be brought up to date; each seed commit is labelled `lym: seed <subtree> (<revision> <channel> <executable>)` with the binary that made it, and a seed commit that conflicts at push is dropped and regenerated. ✅ Implemented.
- **loom** — phased orchestrator: drives its flat, ordered producer list (`contracts/recipes/loom-recipe.yaml`, assembled by `internal/loomrecipe`), each gated by a `Bouncer` review segment (`internal/loomcli` + `internal/loomengine` + `internal/loomshed` + `internal/loomrecipe` + `internal/shedverbs`; `lym loom start|resume|run|step|status|pause|commit-records|validate-discussion|validate-plan|insert-card|remove-card|restore-renumber|validate-description|lint-comments|lint-pattern|fmt|circling accept|continue|decision add|peers|peers show`).
  `internal/shedverbs` owns the generic `run`/`step`/`status`/`pause` verb bodies loomcli arms; `internal/loomcli` hosts only `start` and the arming glue.
  `insert-card`, `remove-card` and `restore-renumber` are the plan-editing verbs: they add or drop one card and renumber the cards after it through `internal/planparser` under a journal that makes an interrupted call restorable, refuse a position at or before the last card webster has begun, and hold webster's state-mutation lease while they run.
  A second embedded recipe, `contracts/recipes/darn-recipe.yaml`, drives a darn run: one gated `Darn-Write` writer that writes and validates the decision record first and, once its `discussion` gate passes, commits it, then reads the board entry, edits, tests and commits, after a `Drift-Baseline` row that records the regression scope and captures the baseline and before a `Drift-Verify` row that compares the writer's code with it, then the `Code-Review` perch (`Code-Bouncer`, `Code-Burler-Review` and `Code-Burler-Fix`) reviews the committed diff and carries its open findings into the record, then `Describe` rewrites the change description from the final diff, then PR-Review and Finalize, with no discussion, plan, webster or Parent-Align rows.
  The perch is loom's own, with the same rows, rubric and `loom.yaml` `code_*` keys for models, fans, budget, hard cap and auto-accept; only its answer key differs.
  In darn the answer key is the task's board entry, which the discussion gate's commit snapshots into `_lym/discussion/board-entry.md`, as amended by the record's `Added after Discussion` entries; the record's other sections are the writer's own claim, checked against the snapshot.
  A `code_max_bounces` of `0` makes `Code-Bouncer` settle approved at once and `Describe`'s entry gate skip it, so the writer's own description stands.
  darn.yaml's former perch keys move to their `loom.yaml` keys at reconcile and in a task's `run_settings`.
  `loomcli` reads the run's seed once at arming and builds, routes and wires the recipe it names, `loom` when the run has no seed; `lym loom start` never rewrites a recorded recipe.
  A darn run takes its verify command and attempt budget from the hub-wide `darn.yaml` (`internal/darnengine`), for the writer's and `Code-Burler-Fix`'s verify gates alike, and the writer's gate halts the run for the parent once the budget is spent;
  the darn gates run the diff's impacted set or their row key over the touched trees' targets, and `tree_scope` in `darn.yaml` scopes a non-production diff at every verify site, Finalize included.
  `lym loom resume` re-spawns `Darn-Write`, `lym loom goto --to Darn-Write` re-enters it and keeps the spent review budget and any settled review verdict.
  A darn run whose status file still names the writer row's former name, `Darn`, is renamed onto `Darn-Write` when a `lym loom` verb arms it.
  `start` is the session bootstrap, performing four steps in order: resolve the recorded parent branch and seed+commit the status file with the records when it is absent; ensure the worktree's tmux session is up, keep or add the status strand, which runs the hidden `lym loom checklist` verb as the session's overview window, on either driver, then spawn the per-hub watchdog daemon, best-effort; read this run's seed and, unless a driver is already alive, spawn the driver its recorded choice selects — the detached Go driver, `internal/sheddriver` mounted by `lym loom drive`, whose driver lock the handshake waits for, or a Claude strand running the loom driver in this worktree's own reed session, in a window of its own after the overview and before the shell; and print the success envelope, never attaching a tmux client (`lym reed attach` shows the session) — the operator's terminal is Selvage, not a strand. `step` brings the tmux session up and never adds or removes the status strand.
  `start` and `resume` are the sites that read a recorded seed's driver value, per the `internal/loomcli` package documentation.
  `start` resumes a parked loom driver by typing one line into its pane (returning once the line's delivery is verified), and refuses after a bounded wait when the pane is not ready, since the driver may have resumed on its own.
  A live driver over a run halted at a hand-back with no park marker yet is still writing its stop report, and a Go driver holding its driver lock is still settling the stop, so `start` refuses with the retryable kind `driver_not_parked`;
  batten's Inner-Run retries it, at most once per `notice_probe_s`, without recording the approval as acted on.
  When `start` would spawn or resume a driver over a pair with an unfinished merge it refuses with the non-retryable kind `merge_in_progress`, listing the conflicted paths under `conflicts` and naming the remedy.
  A run whose current producer is Parent-Align or Finalize is let through over a parked fabric `merge-in` of its own parent branch, since that row aborts and redoes it; `lym loom resume` makes the same exception.
  `lym loom resume` wakes a halted run's live, parked driver through the same branch, and a run awaiting at a review segment's Bouncer row with a pending circling decision takes that branch too, since its driver's step re-calls the Bouncer, which acts on the decision;
  it never spawns an LLM driver, adds a strand or brings reed up, but spawns a Go driver for a halted go-seeded run that has none, while `start` still resumes as before;
  it reads the driver strand from reed's sessionless directory, and refuses every other state with its way forward, as [refusal-spec.md](../contracts/specs/refusal-spec.md) defines one.
  `run` is the no-tmux escape hatch that runs the phase machine in the foreground, for debugging and CI.
  `step` bootstraps idempotently, exactly as `start` does, and drives exactly one producer through `shedengine.Shed`'s own `Step`, emitting a JSON envelope; it spawns no detached driver, making it the single-producer primitive an external supervisor drives.
  `step --until-stop` runs one child step after another in a loop and returns the one envelope of the first stop, with a `loop` object beside the stop step's envelope.
  `status` reports the current phase as a single JSON envelope and, with `--watch`, tails it, printing a line only when the composed activity changes rather than once per poll.
  On a terminal `status` renders a human view instead, and `--json` forces the envelope there; `--watch --json` is refused.
  While a run waits on a verify or a shuttle wait, `status` also carries a waiting note naming the producer, what it waits on and for how long.
  The envelope carries `run_id`, `progress` (the main-line position and bounce count over the recipe's producer graph) and `last_step` (the record `lym shed step` keeps under `.lym`).
  It also carries the build identity (`vcs_revision`, `vcs_modified`) of the `lym` that ran that step, and `binary_changed`, true when the running `lym` is a different known build.
  A routed `stuck` reads as `bounced to <row>` in `activity.last`.
  The run directory is named by the worktree slug (`_lym/shed/<slug>/`), with `self` kept as an alias that resolves to it,
  and a legacy `_lym/shed/self/` still resolves.
  `pause` records a `--before` or `--after` condition and refuses bare, naming `lym tender pause` for stopping the agents.
  `validate-discussion` runs the same checks Discussion-Write's and Discussion-Burler-Fix's own gates run, standalone, exiting 0 on a clean gate and 1 otherwise, with findings in the failure envelope so a writer agent can self-check before handing off.
  A loom run also checks the relevant list when it exists; a darn run never reads it.
  On a darn run the verb also checks the record's history for dropped `Added after Discussion` entries and commits the decision record and support log on a clean run.
  `decision add [<slug>] --by parent|operator --title <t> --decision <d> --rationale <r> [--amends-plan]` appends one design call made after the Discussion to the decision record under an `Added after Discussion` heading, re-runs the discussion check (restoring the record on a finding) and commits the record.
  `--from <file>` in place of the per-entry flags records a JSON array of such calls in one write and one commit, and each live agent gets one notice naming the new headings, with every entry's text inline only for webster's Merriam, which reads no record.
  The verb also works on a darn run once Darn-Write has committed its record, even while Darn-Write runs, under three checks: the record equals its committed copy, it holds every `Added after Discussion` entry its history held, and the committed record holds the appended entry afterwards.
  `--amends-plan` marks a decision that amends the approved plan: the call also appends the title of each entry to the plan amendment record, committed with the entries, and is refused on a darn run and for a plan that is not approved.
  It is refused while Discussion-Write is running, refuses a named session other than the run's parent orch, the nearest live orch up its beam chain (the operator's own unnamed shell always passes), and resumes nothing.
  After the commit it sends each live shuttle run of the worktree, but the tackler, the arbiter and the caller, one creel line naming the record and the heading of each entry, and reports the strands told under `notified` and any failed notice under `notice_errors` without failing.
  `validate-plan` runs the same checks a non-twill Plan-Write run's and Plan-Burler-Fix's own gates run, standalone, with the same exit-code and findings-envelope contract, over the current worktree's plan instead of its discussion.
  `validate-plan --rework` runs the format-only checks over a whole new plan, plus a check that its `first_card` equals the card number the caller was told.
  `validate-plan --lens-review` runs the check a twill-spawned Plan-Write run's own gate runs: the flag-less checks plus a check that the plan directory's `lens-review.md` holds a recorded lens review.
  `validate-plan --overlay` runs the flag-less checks over the plan with proposed contents of its existing plan files, read from stdin as one JSON object, laid over it in memory, so a fix is checked before any file is edited and no plan file is written.
  `validate-description` runs the same checks the `Describe` row's `description` gate runs over `_lym/landing/summary.md`, standalone, with the same exit-code and findings-envelope contract.
  `lint-comments [--base <commit>]` runs `internal/commentlint` over the worktree: the fixed-column-wrapped breaks the working tree, or the range from `--base` to HEAD, creates in `//` comment blocks.
  Every card gate ends with the two lints, and both only read git and files.
  `fmt <file>...` rewrites each named `.go` file inside the worktree to its `go/format` form in-process, and refuses the whole call, writing nothing, when any named path is missing, outside the worktree, not a `.go` file or unparsable.
  `lint-pattern [--base <commit>]` runs `internal/patternlint` over the worktree: it compares `PATTERN.md` at HEAD with the working tree's, or at `--base` with HEAD's, and fails when the overview grew past its headroom line, naming the bytes to cut.
  An overview that stays above the line or shrinks passes.
  A review segment's escalation to the run's parent halts the run `awaiting`, the planned hand-off state: it behaves like `blocked` for resume but spends no bounce budget, triggers no friction reflection and raises no anomaly; a `blocked` or `failed` halt writes a Go halt note and starts no reflection, which the run's Friction-Reflect row runs over every note.
  A review segment settles a spent bounce budget by the soft-cap rule, from the judge's per-finding `mechanical` or `design-wrong` fix tags.
  It escalates to the run's parent, also `awaiting`, for a circling design-wrong finding, at the hard cap, or when the rule cannot read its inputs.
  The Bouncer writes a brief and a one-line parent notice into its run directory and returns the notice as `parent_notice`; the loom driver relays it to the parent, whose one-shot fork reads the brief and decides.
  The fork settles with `lym loom circling accept|continue [<slug>]`, which records the decision and resumes nothing, and resumes the run with `lym loom resume`.
  A `continue` past the soft cap runs exactly one more round without spending budget, and no round at or past the hard cap is continued.
  A batten-driven run is not resumed by batten on such a decision, so it is resumed the same way with `lym loom resume` in the task worktree.
  `Plan-Bouncer` skips its judge only for an exempt generation, one whose live cards are all `Prosa` on non-source files.
  The `Webster` row commits the plan directory alongside its run record.
  `commit-records` commits and pushes the run's records through fabric, pushing the task branch along with them — the status file, the review round record, friction notes and drive reports — and is what the loom driver's end-of-session command runs after the driver writes its stop report, and what a loom-launched driver runs at a hand-back before it parks until `lym loom start` resumes it; a tree with nothing to commit succeeds without a commit.
  Friction notes live under `_lym/loom/friction/` and drive reports under `_lym/shed/<slug>/drive-reports/`, both committed with the run.
  Besides the agents' notes, the friction directory holds the Go-written halt notes, which name their anomaly kind, the crash-resume notes written at a `run` or `step` entry and `lym webster` refusal notes, and the driver's repair records.
  `lym loom start` writes a handoff voucher right before it spawns a driver, so a deliberate resume never reads as a crash.
  A loom run's tail is `Webster → Parent-Align → Code-Review → Drift-Verify → Describe → PR-Review → Finalize`, where `Code-Review` is the Code-Perch of `Code-Bouncer`, `Code-Burler-Review` and `Code-Burler-Fix`;
  a darn run's tail is `Darn-Write → Drift-Verify → Code-Review → Describe → PR-Review → Finalize`, with no `Parent-Align`.
  The three regression rows, `Drift-Baseline` before `Plan-Write` in loom and before `Darn-Write` in darn, `Drift-Await` before `Webster` and `Drift-Verify` as above, each carry the entry gate `drift.enabled` of the hub-wide `drift.yaml`, so a hub that leaves it off records each of them skipped and spawns or runs nothing for them.
  `Parent-Align` merges the parent's upstream tip into the task with no parent lock, verifies the merged tree through the `parent-align` gate key and runs the merge fixer in parent-align mode, which settles a conflict or a failing tree and aligns the task to what the parent brought in; it writes the merge record with the aligned tip, and the Code-Perch then reviews the task's code against that.
  `PR-Review` is a stub for the human pull-request review that is not built: it always returns Done, and logs one line when its flag is on.
  `Finalize` takes the parent lock first, merges the parent into the task again and reroutes to `Parent-Align` on a loom conflict or a merge too large to land; it runs the `finalize` gate key and the merge fixer in finalize mode, lands on the parent with the task trailer and releases the lock on every exit.
  A halt at `Parent-Align` or `Finalize` (a spent or over-cap fixer, an escalation, a reroute-cap stop) is handed back to the orch through every driver, and the status marks it `hand_back`.
  `Webster` reroutes to `Plan-Burler-Review` when begin-batch left a new plan conflict that the plan fixer `lym webster fix-plan` could not clear.
  `Describe` writes the change description at `_lym/landing/summary.md`, the single source for the landing commit, whose message ends in the trailer block Go appends: `landing.yaml`'s `trailer_text`, then the `engine_trailers` text of each engine that ran a session in the task, each copied verbatim and each switchable off by an empty value.
  `Finalize` tolerates an already-landed parent and marks the board task `done` after the parent merge.
  Every run transition before that writes the board task's status as `<state> · <producer>` (for example `awaiting · Code-Bouncer`), so the board README shows where each run stands; a failed board write only warns.
  `Parent-Align` and `Finalize` each run the plan's verify command after a parent merge-in that changed the task tree, and halt Stuck on a failure, with the command's output in the loom verify-output log;
  a pending-verify marker keeps the gate armed across a resume until a verify passes.
  `landing.yaml` also holds the merge fixer's `fixer` model-spec, `fixer_timeout_min`, `parent_align_fix_attempts`, `finalize_fix_attempts` and `finalize_fix_max_lines`, and `deploy_branches`, the base branches whose landing runs `deploy`, which `lym config reconcile` renames from the retired `require_pr_to_base`; reconcile also renames the retired `merge_fix_attempts` key to `parent_align_fix_attempts` and `gate.yaml`'s `merge-in` gate key to `parent-align`.
  `landing.yaml` gains `describe` (the row's model), `describe_timeout_min`, `trailer_text` and `engine_trailers`; an existing hub takes their template defaults until `lym config reconcile --apply` writes them, moving a retired `co_authored_by` value into `engine_trailers.claude`, and an in-flight run parked past `Webster-Bouncer` is restarted rather than migrated.
  `lym loom status --watch` is this module's registered interactive-handoff exception ([PATTERN-cli-cobra](../pattern/PATTERN-cli-cobra.md)): it self-displays the polled status line then blocks forever as its own keepalive tail, with every fallible step running pre-flight, on the envelope, and only that tail exempt from emitting JSON.
  ✅ Implemented. loom's config module (`loom.yaml`, holding the `discussion`/`plan`/`judge` role model-specs and the shared `reviewer_model`/`fixer_model`, where `seed` names the Bouncer seed pass model, `discussion_judge`/`plan_judge`/`code_judge` and `discussion_seed`/`plan_seed`/`code_seed` override the judge and seed per review segment, an empty one taking its fallback, and `reviewer_model` and `fixer_model` each take one model-spec or a per-round list, and `<segment>_reviewer_model`/`<segment>_fixer_model` for Discussion, Plan and Code override them per review segment, `discussion_reviewer_fan`/`plan_reviewer_fan`/`code_reviewer_fan`, which name a `lenses.yaml` fan, or list one per round, to cluster-review Discussion-Review, Plan-Review or Code-Review, `discussion_fixer_fan`/`plan_fixer_fan`/`code_fixer_fan`, which do the same for the fixer, an empty entry running the single fixer, all six closed to the local layer, `discussion_timeout_min`/`plan_timeout_min`/`review_timeout_min`, `discussion_interactive`, `discussion_advisors`, the Discussion-Write advisors, one `{name, model}` entry each, where an empty list runs the lead alone, deciding each question itself, `discussion_lenses` and `plan_lenses`, which each name the `lenses.yaml` fan whose lenses the Discussion-Write or Plan-Write lead forks itself for, empty turning the forks off, all three closed to the local layer, `review_circling_checkpoint`, `review_max_bounces`, `review_amend_max_bounces`, the amendment allowance a review segment re-entered over a changed subject runs under, the per-perch `discussion_max_bounces`/`plan_max_bounces`/`code_max_bounces`, where null takes `review_max_bounces` and 0 runs no review round, `review_hard_max_bounces` and the per-perch `discussion_hard_max_bounces`/`plan_hard_max_bounces`/`code_hard_max_bounces`, where 0 takes `review_hard_max_bounces` and a hard cap below its perch's budget is refused at load, and `discussion_auto_accept`/`plan_auto_accept`/`code_auto_accept`, which make a perch accept a harvested round at a spent budget instead of running the soft-cap rule) exists and reconciles via `lym config reconcile --apply` (the bare verb is a dry run that only reports added and removed keys and writes nothing).
  Reconcile renames the retired `review`, `fix`, `<segment>_review`, `<segment>_fix`, `<segment>_fan` and `<segment>_fix_fan` keys of `loom.yaml` to their new names, and the retired `webster_*` perch keys to `code_*`; `darn.yaml`'s perch keys, `review_fan` and `review_fix_fan` among them, move to `loom.yaml`'s `code_*` keys; the retired `merge_*` perch keys, `rework`, `alignment_pr_approve`, `alignment`, `alignment_timeout_min`, `skip_alignment` and `parent_review_wait_min` are dropped.
  The four driver and tackler keys of `loom.yaml`, and of `darn.yaml`, are overrides of `shed.yaml`'s defaults of the same names: `driver_kind`, `tackler`, `tackler_timeout_min` and `tackler_max_spawns`, where an empty string, a 0 or a null inherits, and a set `tackler_max_spawns` of 0 turns the tackler off.
  The reviewer and fixer model keys and `review_timeout_min` are the review segments' own models and timeout, and live here rather than in the recipe because the recipe is embedded in the binary and a recipe-literal model would be untunable without a rebuild.
  The Discussion producer: a seat table run on `MultiLLMProducer`, its prompt shipped as an embedded default in the top-level `contracts/stencils` package and read at call time from the hub's stencils directory (`contracts/stencils/loom/loom-template-discussion.md`), built by `internal/loomengine`'s `discussiontable.go`.
  The producer runs in one of two modes, selected by `discussion_interactive`: autonomous by default, or interactive when the key is set, so an operator can interview the agent from its pane instead of it self-judging every answer.
  Both prompt renderings ship in the same stencil, selected by the `{{.mode_rules}}` marker.
  Two independent axes shape the table, both set in `loom.yaml`.
  The first is the advisors: with `discussion_advisors` empty, the default, the lead interviews and writes the same two files alone, deciding each question itself;
  with one `{name, model}` entry per advisor, the lead is a chair while the advisors study the task, the board, the code and PATTERN in parallel.
  Each advisor is a seat named by its entry, answers the chair over the session message channel, and writes a notes file `advisor-<name>.md` beside the discussion files.
  The second is the lens forks: with `discussion_lenses` naming a `lenses.yaml` fan, the lead settles its design, writes and self-checks the record, forks itself once per lens of that fan, takes the forks' findings in and finalises.
  The forks review the record their inherited context holds, without reading it again, and only the lead forks, never an advisor.
  Both axes together are valid: advisors with the lead's lens forks.
  A `loom.yaml` that still holds the retired `discussion_producer` or `plan_producer` key is rewritten when the hub's config is reconciled: `twill` becomes the twill fan in the new lens key, every other value an empty one, and `discussion_advisors` is set to match, the retired `seats` entries becoming `advisor-<n>` for the operator to rename.
  The producer runs as seats of `MultiLLMProducer`, over a table built in Go by `internal/loomengine`'s `DiscussionTable`, whose lead's session may spawn fork subagents when `discussion_lenses` is set.
  The lead opens with `contracts/stencils/loom/loom-template-discussion-chair.md`, which includes the discussion stencil whole and adds the advisors section, worded to cover a lead with no advisors too, so a table without advisors runs the lead alone, deciding each question itself; each advisor opens with `contracts/stencils/loom/loom-template-discussion-advisor.md`.
  The row keeps its name, gates and routing whatever the two keys hold.
  The Planner producer, the same way (`contracts/stencils/loom/loom-template-plan.md`), composed by `internal/loomengine`'s `prompt.go` + `plan.go`.
  With `plan_lenses` naming a `lenses.yaml` fan, the plan writer explores, has one fork (fork0) write and self-check the plan, reads the plan in full and self-checks it, forks itself once per lens of that fan, judges each finding as a critical fixer, and records each disposition in `lens-review.md`.
  Every lens fan's writer, this one, the Discussion-Write lead's and Burler's reviewer and fixer alike, saves each fork's final message verbatim in a lens report beside the file its lens review is recorded in, `<record stem>.lens-<lens>.md`, in the format `contracts/specs/twill-lens-report-spec.md` pins, so an audit reads every report from the run records.
  It runs `validate-plan --lens-review` as its last self-check, and its gate refuses Done until `lens-review.md` holds the lens review.
  The row keeps its name, `gates` key and routing while a fork-authorized run's plan gate also requires the lens review.
  Plan has no advisor key.
  See the `internal/loomengine` and `internal/loomcli` package documentation.
- **shed** — the generic outer phase-FSM `loom` and the eventual `Hardener` are each built on: a Go engine that walks one flat, ordered producer list, honoring resume, crash-recovery, and pause uniformly at producer granularity, with no predefined slots (`internal/shedengine`).
  The shipped engine adapters — the `Webster` adapter, the Burler review row and fix row producers, `MultiLLMProducer` over `seatengine`, and the Bouncer (the generic review-gate producer rather than a wrapper over an engine) — live in one package, `internal/shedadapters`, alongside their shared context and archive helpers.
  No `lym shed` verb of its own by design — a product's own CLI constructs a `Shed` with its own producer list and calls `Run`, and a bare verb would be a command with no list to walk.
  The skeleton (the loop, the status file, the `ShedProducer` interface) is ✅ **implemented**; the engine adapters (the `Webster` adapter, the Burler review row and fix row producers, `MultiLLMProducer`, and the Bouncer) are ✅ **implemented** too, shipped as `internal/shedadapters`.
  `internal/shedcheck` is the shipped structural checker over an assembled producer list, enforced by a `go test` invariant over loom's own list rather than called from any production constructor — see its own package documentation for the finding kinds it reports.
  The Shed recipe group's engine registry (piece 1 of that group) is ✅ **implemented** too, as `internal/shedrecipe`; its `registry` map literal declares every engine name a recipe row may use.
  The recipe file format and the loader/builder shipped too, as `internal/shedbuild`, and loom's own conversion to a recipe file has now shipped as well: `contracts/recipes/loom-recipe.yaml` plus `internal/loomrecipe`, which assembles it into the `*shedengine.Shed` `internal/loomcli` runs.
  The generic verb set is ✅ **implemented** too, as `internal/shedverbs`: the `run`/`step`/`status`/`pause` cobra bodies every arming module builds its own subtree from, owning no path of its own and deriving nothing.
  The named-recipe `lym shed` subtree is ✅ **implemented** as `internal/shedcli`, arming `loom`, `darn` or `batten` by run-id rather than by an explicit recipe name: each verb (`lym shed run|step|status|pause [<run-id>]`, defaulting to `self`) reads the addressed run's own `seed.json` and looks its `recipe` field up in the table, and `lym shed seed <run-id> --recipe <name> [--driver <name>] [--param k=v]` writes that first seed, with the driver defaulting to `llm` for a recipe that has a bootstrap verb (loom and darn, whose unseeded `start` records `llm` too) and to `go` for one that does not (batten), refusing fabric's own checkouts (the Board checkout, a pair's other side) where no run is ever driven, and refusing a recipe's seed wherever that recipe's own verbs would refuse to drive it (a batten seed anywhere but the hub's prime worktree).
  The generic `step` envelope printed on stdout names the durable `trace_file` the invocation wrote and the `envelope_path` of the full envelope, which holds the run's `friction_dir` and `scratch_dir`, plus the reflection's `friction` status when non-empty, `status` names `trace_dir`, and fabric writes every recorded mutation to that trace.
  The driver stencil (`contracts/stencils/shed/shed-template-driver.md`) launches a session that drives any seeded run recipe-blind through one background job, `lym shed step <run-id> --until-stop`, whose envelope at a stop is the only thing it reads.
  At an `error` or `interrupted` stop the driver spawns exactly one fork of itself, which reads the traces and the repair guide (`contracts/stencils/shed/shed-template-driver-guide.md`, deployed beside the stencils), writes the stop report and repair records, and returns one action for the driver to run; an orchestrator session forks one driver loop per run.
  A message asking for a stop at a point becomes `lym shed pause <run-id> --before|--after <producer>`.
  At a stop it cannot settle, the driver runs `lym loom arbiter`, which spawns a cold arbiter agent and returns a verdict the driver applies or hands on.
  The driver re-launches the loop once after a deployed `lym`, and a loom-launched driver parks at a hand-back until `lym loom start` resumes it.
  `internal/shedrun` is the sole declarer of the `shed` run-directory path segment (`_lym/shed/<run-id>/` durable, `.lym/shed/<run-id>/` ephemeral), the run-id vocabulary (including the default literal `self`), and the `seed.json` contract — the closed `recipe`/`driver` vocabularies and the sole `ReadSeed`/`WriteSeed`/`List` reader-writer, consumed by `loomcli`, `battencli`, and `shedcli` alike. See [PATTERN-shed-driver](../pattern/PATTERN-shed-driver.md).
  See the `internal/shedengine`, `internal/shedadapters`, `internal/shedcheck`, `internal/shedrecipe`, `internal/shedbuild`, `internal/shedverbs`, `internal/shedcli`, `internal/shedrun`, and `internal/loomrecipe` package documentation, and [the recipe format](../contracts/specs/shed-recipe-spec.md).
- **burler** — one review+fix round: two fresh sessions, a reviewer and then a fixer, each the unnamed lead of its own one-seat table on the seats engine (strands `burler-review` and `burler-fix`), no self-grading, over the shuttle file contract (`internal/burlerengine` + `internal/burlercli`).
  Each review perch's fan key turns the lens fan on for it and its fix fan key the twill fixer, each per round and off by default: `loom.yaml`'s `discussion_reviewer_fan`, `plan_reviewer_fan` and `code_reviewer_fan` with their `*_fixer_fan` partners, the last pair serving a darn run's Code-Review too.
  The forks of a code perch reach the diff through read-only git, and `lenses.yaml`'s `review-code` and `fix-code` fans carry lenses for a committed diff.
  The reviewer writes the review, Go accepts it and records its digest, and only then does a fixer start; it validates and fixes the findings, disputing one only with evidence that its premise is false or that every fix makes the target worse, and fixing a real finding with its own change when the proposed one is wrong.
  Profile-driven: `{overlay, source}` fix-scope, tool-use, `language-skills` (a code round's seats load the hub's `language.yaml` skills after their own); each review segment's `<segment>_reviewer_model` and `<segment>_fixer_model` keys, falling back to `loom.yaml`'s `reviewer_model` and `fixer_model`, pick each seat's model per round.
  Cluster review fans the reviewer out into N fork-subagent reviewers by naming a fan (`cluster-fan`) from the `lenses.yaml` lens/fan library — never on by default.
  Strict frontmatter verdict parse;
  debug CLI `lym burler run`, and the read-only review-gate self-check `lym burler validate-review <review-file>`. ✅ Implemented.
  See the `internal/burlerengine` package documentation.
- **hardener** — **DRAFT / concept.**
  Behavior-based reviewer that *runs* a live-substrate module (needs a sandbox repo) to harden it before merge;
  on-demand, post-loom, **off the spine**, shares only the `burler` round discipline.
  See the board's `hardener` note.
- **batten** — drives one task worktree's whole lifecycle — create, seed the child's own inner run from the Board task's own `recipe` (`loom`, or `darn` for a darn child, empty meaning `loom`), run it to a terminal state, land it into the prime, and tear down — as a single Shed run from the hub's prime worktree (`internal/battenshed` + `internal/battenrecipe` + `internal/battencli`; `lym batten run|step|status|pause|goto|cancel <run-id>`, `lym batten lock list|release` for the hub's parent locks, and `lym batten cleanup [--apply]`).
  `lym batten cancel <slug> [--remove]` ends a run for good from the prime, whatever its state, leaving its claim `draft` and removing its records, and with `--remove` its pair and task branches.
  The hidden `lym batten overview` is the pane process of the `runs` strand of the prime and of each beam, which `lym beam start` ensures, in the beam's window named `beam: prime` or `beam: <slug>`.
  It draws the dobby of its own worktree first: alive or not, each run it drives once, with its state, stage, time in stage and last pass, and each released task it has not started with what the task waits on, the lines `lym beam status` prints.
  The stage cell carries the stage's entry number (`7. Land`, `6.5 Plan-Write` for a child stage under Run-Shed), and a run a live pause covers shows `paused <minutes>` in its state cell.
  Below the dobby come the orch lines of the worktree's own orch and of every beam below it, every orch in the prime's window.
  It reads files, taking the dobby's and each driver's lock for an instant to learn whether it is held.
  `lym batten cleanup` is a dry run unless given `--apply`: it removes each done run's records run directory with a records commit and a spawned push, and empties that run's scratch beside its lock files and any orphan scratch older than a day.
  A run is one pass after another: `lym batten run <slug>` runs one pass and exits, started by the run's parent worktree's dobby or by hand, and a hand pass makes sure that dobby is live once it is over.
  `lym adopt` starts a run's first pass detached with the prime as its working directory, and `lym beam land <slug>` starts none: it writes a land request into the beam worktree's `.lym/beam/` directory, which the beam's pawl run consumes at Beam-Await, from anywhere in the hub.
  `Worktree-Create` forks a task's pair from its beam, the nearest open beam up its board project chain, whichever worktree started the run, and from the prime for a task outside any beam.
  A beam member's start checks no tracked dirt of the prime, since its pair forks from the beam's last commit, and a beam whose pair is absent here or whose own landing holds it stops the create with a precondition naming the way forward.
  A beam project runs the same way, its pair forked unmarked at the anchor its seed records; a member's beam is read again once its pair is made, and a beam whose run holds its entry removes the member's pair and refuses naming the member's cancel and the board move off the beam.
  `lym batten run` drives its own run through the same driver body in-process, over the loop `step --until-stop` runs: an unexpected stop meets the driver's rule table, and a stop no rule settles meets a cold tackler agent in the prime's reed session, before it is handed to the orch.
  The tackler is set by `batten.yaml`'s `tackler`, `tackler_timeout_min` and `tackler_max_spawns`, overrides of `shed.yaml`'s keys of the same names.
  A stop the tackler escalates meets the arbiter next, wired in batten's driver after the tackler and set by `batten.yaml`'s `arbiter`, `arbiter_timeout_min` and `arbiter_max_spawns`, overrides of `shed.yaml`'s keys of the same names; its goto is held to the halted row.
  The process holds the run's driver lock for its life, so a second `lym batten run` for the slug is refused.
  Between `Seed-Child` and `Run-Shed`, three `Command` rows each run one shell command from `internal/shedadapters`' `Command` producer, each run only when its entry gate's `batten.yaml` setting, which a task's `run_settings` may set, is on: `Open-Window` (`open_window`, on by default, command `open_window_command`) opens one desktop terminal window on the task worktree, titled with the slug, through `lym reed window`;
  `Seed-IDE` (`seed_ide`, on, command `seed_ide_command`) runs `lym ide seed`, which writes the task worktree's VS Code colour and `.vscode/` config as `lym ide spawn` does, without the user's key bindings;
  `Open-IDE` (`open_ide`, off, command `open_ide_command`) runs `lym ide open`, which opens VS Code on it.
  A command key is read from the template and the board default layer only; `lym config batten --layer default --set <key>=<value>` from the prime changes one, and a different value in the task worktree's own layers is ignored with a warning.
  A failed command only warns, and the row passes on.
  The Board task's `recipe` must be `loom`, `darn` or empty: `Run-Shed` starts `lym loom start` inside the task worktree, which serves both recipes, so `Seed-Child` refuses any other registered recipe before writing a seed rather than committing one the child's own bootstrap would refuse.
  `Seed-Child` halts `blocked` with a named `stuck_reason`, not a hard failure, for all three ways `WriteSeed` can refuse: an unknown recipe, a recipe the task worktree cannot bootstrap, and a pre-existing child seed that disagrees with the one being written (`shedrun.ErrDisagreeingSeed`/`battenshed.ErrDisagreeingChildSeed`) — the same business-judgment treatment for all three, since none of them is a path-resolution or write failure.
  `Worktree-Create` is idempotent against its own post-condition, which is the whole pair fabric's `Add` builds, not merely the task worktree directory: a process killed anywhere inside `Add`'s own multi-step sequence never runs `Add`'s in-process rollback, so a bare directory check would be satisfied by that sequence's very first step alone. The row instead checks the pair is fully materialised (the task worktree's sibling exists, its junctions are wired, and its origin record names the parent branch `Seed-Child` passes to the child's bootstrap) before reporting done; a worktree present but not yet complete halts `blocked` naming a manual cleanup from prime (`lym fabric remove --force <slug>`, which removes whatever part of the pair `Add` reached, and deletes the local task branch itself when its work is pushed or landed) rather than silently skipping the create.
  A create refused on a leftover branch (a torn-down pair keeps its task branch on the remote, and locally too unless `remove` proved its work pushed or landed) halts `blocked` with batten's own remedy — delete the branch locally and on the remote, and any orphaned sibling branch, then resume — never fabric's `lym fabric checkout`, which from prime switches prime's own pair onto the task's branches.
  A child that is `awaiting` (a review segment's escalation) is watched, not failed: the escalation is settled with `lym loom circling accept` or `continue` and then `lym loom resume` in the task worktree, and a done child's driver is waited for (up to `driver_exit_grace_s`) before teardown.
  `Land` runs between `Run-Shed` and `Worktree-Teardown`, once the child run is done, under a hub-level land lock: it pulls the prime when the child's parent branch is the prime's branch (waiting up to 10 min for the prime's tracked files to be clean), runs the hub-wide `deploy` command of the landing config in the prime when that parent branch is in `deploy_branches`, prunes the done entries off the board, and notices the orch only when the pull's records side stopped, the one notice of a successful landing.
  A failed `Land` step halts batten `blocked` at `Land` with the pair intact and one failure notice per episode, and a `lym batten pause` seen while `Land` waits does the same with no notice, so the way back is `lym batten goto <slug> --to Land` and then `lym batten run <slug>`.
  Batten no longer notices the orch of a done child: the `Worktree-Teardown` row notifies only on a failure, and the `Land` row also notifies every landing.
  The `Open-Window`, `Seed-IDE` and `Open-IDE` rows own the terminal window and VS Code; `Run-Shed` opens neither.
  VS Code opens only when `open_ide` is on or the operator runs `lym ide`.
  A run resumed past `Open-Window` runs that command again when no client is attached to the pair's reed session, when `batten.open_window` is on; a resume with a client attached opens none.
  That window closes when the run's session ends or its worktree is removed: it closes at once when the attach exits 0 (an operator detach or an ended pair session included), and after any other exit as soon as the worktree is gone within thirty seconds, else it stays open as a shell.
  A child that halts (`blocked`, `failed`, `paused`) makes `Run-Shed` wait, budget-exempt and without a time limit,
  and batten never resumes a halted child: the operator resumes the child from inside the task worktree (`lym loom resume`) and the watch carries on, with one warning per halt episode;
  the reason, naming `lym loom start` too when no driver can be woken, is what batten logs and writes to the row's stuck-reason file,
  and `lym batten status` does not show it while the child is halted;
  `lym batten pause <slug>` stops the wait within one check.
  When a halted or awaiting child's reed state holds a dead driver strand, batten revives the pair's strands through reed resume once per halt episode per batten process, which brings the parked driver back with its own session and leaves the run halted and the park marker in place,
  so a later `lym loom resume` wakes that driver;
  a retiring strand is left to `lym loom start`.
  `Run-Shed` spawns the child's bootstrap while the task worktree has no status file, and again while that status is `running` and its spawn was not confirmed by this batten process: the confirmation marker holds the pid of the process that wrote it,
  so a restarted batten spawns the bootstrap again and a bootstrap that failed or was killed after seeding is retried on resume rather than watched as running for the whole budget.
  A running child with a live or retiring driver strand in its reed state, or a held run lock, is adopted instead: batten records the confirmation for itself and spawns nothing,
  so a second driver is never stacked,
  and the bootstrap itself is idempotent against a driver already alive.
  `Worktree-Teardown` is idempotent against its own post-condition just as `Worktree-Create` is: re-entered after its own removal already succeeded (a process killed before the transition persisted), it finds the pair gone and reports done.
  That post-condition is the pair's, not the task worktree's alone: fabric removes the task worktree before its sibling, so a removal interrupted between the two halves leaves the sibling, its launcher entries, and both branches behind — re-entered there, the row halts `blocked` naming the leftover and `lym fabric prune --apply` as the remedy rather than reporting done over the debris.
  Teardown first pushes an `archive/<slug>/<tip>` tag of the pair's other-side branch's tip to its remote, so the run's committed records stay reachable after the branch is gone, then deletes that branch locally and on the remote (the task branch stays on the remote, and locally unless `remove` proved its work pushed or landed), and a re-entered row whose worktrees are already gone finishes that deletion rather than reporting done over a surviving copy, which would make a later create of the slug refuse its push; a failed remote deletion halts the row `blocked`, and resuming once the remote is reachable retries it.
  A failed archive push halts the row the same way before anything is removed, and its reason names the resume.
  Teardown is one row sequencing session shutdown before worktree removal, and never forces: anything an agent left uncommitted or untracked in the child (a build artefact, a note) blocks the row after the session is already down, with fabric's own refusal as the `stuck_reason` — the operator cleans the child and re-steps, never `--force`;
  an uncommitted change in the pair's sibling, where the run records live, instead names batten's own recovery (`lym loom commit-records` in the task worktree, then resume the batten run), and, should the resumed teardown refuse again, committing or removing the leftover by hand, since it is then not a run record;
  uncommitted content in the task worktree itself keeps fabric's refusal, and neither is ever `--force`.
  A fresh pair's records branch starts without its parent's `_lym/shed/` run records, which fabric's `add` drops in the pair's first records commit, so a child never sees prime's batten records or any other run's.
  `Worktree-Create` and `Worktree-Teardown` wait for a prime lock another run holds, polling every 2 s for up to 10 min, and halt `blocked` only once that wait runs out.
  `step` drives exactly one producer forward from the run's persisted current producer, seeding a fresh run first when none is persisted yet.
  `lym loom step` is the single-producer primitive of the same shape over a run that is already seeded.
  `run` and `step` carry `--driver` (batten's own, `go`-only: batten has no bootstrap verb, so `llm` is refused by name) and `--child-driver` (the driver the task worktree's own inner run uses, `llm` or `go`; unset, the child recipe's `driver_kind`, then `shed.yaml`'s, `go` by default); `--driver` is recorded into the run's write-once seed at first seeding and `--child-driver` only when typed, and an explicitly typed flag that disagrees with an already-seeded run is refused rather than silently dropped.
  `Run-Shed` checks its child once per call, and a call that ends nothing yields instead of waiting: its memory between calls rides in a per-run record, and the scheduling dobby runs the next pass.
  A pass returns a verdict only for a state change, an arm event, a pause or a failed status stat, each a budget-exempt `Stuck` that is its own history entry.
  Anything costing a process or a multiplexer round trip runs at most once per `notice_probe_s` (30 s).
  A running child is checked without a time limit; `lym batten pause`, honoured at the next pass, cancellation and the notices bound it.
  The history writes of those returns rewrite the run's durable `status.json` without committing it — the status commit skips a transition it has already committed — so prime's own pair carries an uncommitted change at `_lym/shed/<slug>/status.json` while a watch goes between them.
  That is deliberate (the alternative is one identical commit per bounce), and it has one operator-visible consequence worth knowing: `lym fabric checkout` refuses hub-wide while a batten run is watching, because it requires a clean pair.
  `status` bounds the history it reports to the most recent entries, alongside the true `history_length` and a `history_truncated` flag — the history grows only on a state change, a pause or a failed status stat — and surfaces the blocked run's own producer-supplied `stuck_reason`, kept for the `Run-Shed` block through the budget arm, whose persisted `error` stays the fixed budget literal (the other rows carry no `on_stuck`,
  so their own reason is the persisted `error`), plus the teardown row's `abandonedSession` — recorded by the producer rather than only returned,
  so a `step`-driven lifecycle reports it as well as a `run`-driven one.
  ✅ Implemented. See the `internal/battenshed` and `internal/battenrecipe` package documentation.

The cross-OS spawn primitive **proc**, and the generic outer phase-FSM **shed**, are the two remaining internal (non-CLI) layers — proc the base of the stack, shed the generic engine `loom` configures rather than a stack layer of its own;
see the [Execution stack](#execution-stack-orchestration-layers) section below for how proc / reed / shuttle fit together. (Earlier drafts split reed into separate `shed`/`glance` modules;
both folded back into reed — see the `internal/reedengine` package documentation. This `shed` is an abandoned earlier `reed` model/view draft, unrelated to `Shed` (`internal/shedengine`) the outer phase-FSM.)

The user-facing modules sit on a thin layer of shared infrastructure (`internal/configengine`, `internal/gitexec`, `internal/gitrepo`, `internal/lock`, `internal/logger`, `internal/output`, `internal/lymcwd`, `internal/dotgit`, `internal/lymdirs`, `internal/state`, `internal/shell`, `internal/verifytree`, `internal/verifyrun`, `internal/modelspec`, `internal/pattern`, `internal/friction`, `internal/buildinfo`, `internal/standalonestate`, `internal/segmentcolor`) — defined in [shared-libs/README.md](shared-libs/README.md). `internal/pattern` is the leaf that inlines the root `PATTERN.md` overview into the prompts of the agents that design, plan, edit or judge, for the roles in its own doc, and ships a format checker for `PATTERN.md`.
`internal/friction` is the leaf that returns the role-appropriate friction-note directive injected into all seven agent prompts when Tier 2 is enabled, with the note path composed by its own non-clobbering `NotePath`.
Above the engines sits a separate precondition-and-geometry layer, not the shared-infrastructure layer above: `internal/preflight` is the tier-1/tier-2 precondition layer (worktree geometry, worktree-pair cleanliness, Fabric readiness/sync), and `internal/hubgeom` and `internal/standalonegeom` are its hub-mode and told-mode constructors of the `Geometry` struct each engine is handed — see [PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md).
`internal/cliwire` sits between the two: it is the CLI-boundary resolver a standalone-capable CLI calls once `preflight.ResolveMode` has chosen hub or standalone mode, and it hands each CLI the told strings — the resolved target, the derived standalone state directory, the resolved stencils and plan directories — that `internal/hubgeom` and `internal/standalonegeom` then build into a `Geometry` struct.
`internal/preflightshed` sits alongside these as the producer-shaped wrapper around that same layer, letting a `Shed` producer list name it as a single row rather than each caller composing `internal/preflight.Check` for itself.

## Execution stack (orchestration layers)

The orchestrator is not one module but a **layered stack**, each layer knowing only the one below it.
It exists in this shape for one reason: agents must run as **interactive tmux sessions, never headless `claude -p`** (an economic constraint — see the `internal/shuttleengine` package documentation), so spawning an agent is not a plain `exec` but "place a pane, launch a provider in it, drive it, detect completion."

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

Regression sits beside this stack: the Drift rows are shed producers over seats, and the baseline capture is a detached registered process, not a gate tier (see [internal/driftengine](../internal/driftengine/doc.go)).

The batten Shed nests loom's: it is its own recipe whose `Run-Shed` row seeds and drives a task's run, loom or darn, as a child process and polls that run's own persisted status for the verdict, so there are two status files by design — the task's, committed on the task branch, and batten's own, durable under prime's own `_lym/shed/<slug>/` — each resuming independently.
See [PATTERN-pair-teardown](../pattern/PATTERN-pair-teardown.md).

**Landing preconditions.** This task's relocation of run state requires no `lifecycle` or `loom` run in flight at landing: no migration reads or moves the old `.lym/lifecycle/<slug>/`/`_lym/loom/status.json` layouts, so a run left in flight under either old layout resumes nowhere afterward.
It also requires deleting any already-deployed copy of `contracts/specs/loom-status-spec.md` by hand: `internal/stencilstore` never overwrites a hash-mismatched file, with no force-sync carve-out for specs, so a deployed copy predating this task's path rename never refreshes on its own — delete it, and the next run re-seeds it fresh.

The whole stack runs **headless** (auto mode): strands exist (the interactive-session requirement), agents run, output files are read, nobody need watch.

The stack now has two entry modes, not one: every layer from `reed` up is **told** its geometry rather than deriving it.
`internal/hubgeom` and `internal/standalonegeom` are the two constructors that tell it — hub mode and told mode respectively — with `preflight.ResolveMode` selecting between them at a standalone-capable CLI's pre-run.
The consequence a reader needs: a producer verb therefore runs in a directory that is not a git repository, with no hub, no fabric, and no orchestrator status seed.
See [PATTERN-cwd-resolution](../pattern/PATTERN-cwd-resolution.md) for the rule.

- **reed is three things, and it is built** — an **overlay** over tmux, **strand bookkeeping** (a strand = one tracked process: a metadata record with a `guid`, `name`, worktree slug, parent, and a *generic* display spec),
  and a **render** sub-package (`internal/reedengine/render`, the closed display vocabulary).
  Callers hand reed `{cmd, name, display}` where `display` is generic (an anchor, focus and an optional window group) — never a domain `type`, so reed never learns what a "phase" or "cluster" is.
  Earlier drafts split the model and view into separate `shed`/`glance` modules;
  with one terminal per worktree they fold cleanly into `internal/reedengine` + `internal/reedengine/render`.
  `lym reed remove --name <name> --detach` removes a strand by name from inside itself through a detached remover, and reed replaces a strand atomically in its slot.
  See the `internal/reedengine` package documentation.
- **beams sit beside the stack, not in it** — a beam is a pair whose origin record carries the beam mark, and `lym beam` calls fabric, the board and batten in-process, so no layer above learns of it.
  Orch and batten run in the prime and in any beam, and a beam's tasks fork from the beam's own branch.
  See the `internal/beamcli` package documentation.
- **skills and the parent directive** — a spawn's skills are named on the launch spec and delivered by the provider engine before the prompt, on the launch line or typed, never asked for by a stencil;
  every spawned role's opening stencil renders the parent directive, so a role escalates to its parent rather than the operator.
  See [PATTERN-parent-directive](../PATTERN.md).
- **provider-invariant** — `shuttle` runs Claude today through an **engine**;
  the verdict/output contract is provider-invariant, so a different model can be swapped in without touching the review machinery.
  Non-Claude is not a current priority.
- **the bootstrap** — `lym loom start` brings up the worktree's tmux session, on either driver keeps or adds the `lym loom checklist` status strand, the session's overview window, one frame of the recipe's rows in order with their visit lines under them and then the state and wait lines (replaced in its slot when the live one was launched from a different `lym` build), spawns the per-hub watchdog daemon (best-effort), and spawns the driver the run's own seed selects: the Go driver **detached** (via `proc`, no TTY), or a Claude strand running the loom driver in this same reed session for the `llm` driver.
  When the `llm` driver is live and parked at a hand-back, it spawns nothing and instead types one resume line into that driver's pane.
  It then returns the success envelope and never attaches or switches a tmux client; `lym reed attach` shows the session.
  Selvage is the operator's terminal, not a strand.
  A Go-driven loom run runs in the background;
  the reed view takes the foreground.
  A `.lym/lymrun.cmd` launcher makes it one click.
- `reed`, `shuttle`, and `loom` each get a user-facing `lym <module>` CLI (`lym shuttle run|open|interrupt|send|state|await` lets an operator or another process drive one agent standalone, before loom exists); `burler` is composed by loom's own review segments (`lym burler run` is a debug-only wrapper, not a product verb), and `proc` alone stays an internal library with no CLI of its own.

### Following one spawn down the stack

loom wants a plan-reviewer for worktree `feature-x`:

1. `loom` → its Plan-Review segment's `Bouncer` — "review this plan against the discussion until clean."
2. the segment's Burler review row → `burler.Review(profile, opts)`, then its Burler fix row → `burler.Fix(profile, opts, digest)` — "run one review, then one fix."
3. `burler` → runs a one-seat table on the seats engine per half, each a shuttle run — "run one reviewer agent, then one fixer agent."
4. `shuttle` → `reed.AddStrand{ cmd:"claude …", worktree:"feature-x", display:{anchor:own-window, focus:true} }`.
5. `reed` records the strand in `.lym/reed.json` and runs the command via `proc` in a pane of a window of its own.
6. The `Stop` hook fires → reed notes the edge → shuttle reads the output file → returns to burler → the reviewer's review is accepted and the review row records its digest → the fix row spawns the fixer, which writes its fixer-report → the segment's `Bouncer` reads it, decides another round or exit → on a `CONVERGED` verdict returns `Done` → loom advances.

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

Fabric's own cross-cutting suite follows the same black-box convention but keeps its own name: a set of `package fabricengine_test` files inside `internal/fabricengine/`, `tier1`-tagged.
It is the **live-state integration harness**: it drives real cloned hubs, built by really cloning rather than hand-assembling a fixture, into dirty and hostile on-disk states and asserts what a destructive verb is and is not permitted to touch.
See `internal/fabricengine`'s own package doc for the state matrix, the verb table, and the sabotage-proof table recording that each of the crucible campaign's eight data-loss defects still fails on demand when its guarding check is neutered.

`internal/gitkit` is the below-fabric leaf and the home of test git plumbing — spawn, query and commit helpers, the hermetic git environment, and the primitive repo fixture `CopyRepo` — and asserts nothing itself; it never imports fabric.
`internal/hubforge` is the repo-wide real-hub fixture factory: it builds every hub fixture in the repo through `fabriccli.CloneAndWire`, never a hand-assembled stand-in, and asserts nothing about fabric either.
`internal/testkit` holds the shared test kits, one package per kit, for test support (fakes, builders, fixtures and the scan harness) used by two or more packages; [PATTERN-test-isolation](../pattern/PATTERN-test-isolation.md) states what a kit may import and assert.
`internal/testkit/scankit` is the harness every invariant scan runs on: module-root lookup, file walk, allowlists that report stale entries, a vacuity floor and an import-allowlist assertion.
`internal/testkit/plankit` is the one test-side writer of valid plans, rendering through `planparser`'s format constant, with the file-tree fixture glyph resolution needs.
`internal/testkit/tmuxkit` gives each test package one isolated tmux socket directory through `Main`, and each test a self-cleaning `-L` key through `Socket`.
`internal/testkit/stencilkit` seeds stencil fixtures from the registry through `stencilstore`, so seeded files carry production's hash stamp.

## Sandbox Hub

The **sandbox Hub** is a dedicated bench for manual testing of lym's core workflows — dogfooding lym against itself.
It lives on disk at `C:\Code\lym-test-LYMHUB` and exercises the resolved `lym` binary under test: the dev binary via `deploy-dev` into `.dev-bin` when present, else the production binary on PATH via `deploy.cmd`.
Build it via `sandbox/win/build.cmd` (`sandbox/posix/build.sh` on Linux/macOS), run the core suite via `sandbox/win/core-suite.cmd` (`sandbox/posix/core-suite.sh`, or the `reed-suite`/`reed-suite.sh` pair for the reed-specific suite, which needs live tmux), and collect the report via `sandbox/win/fetch.cmd` (`sandbox/posix/fetch.sh`) for either.
See [sandbox-howto.md](sandbox-howto.md) for the step-by-step runbook and [sandbox-hub.md](sandbox-hub.md) for topology and design details.

## Other docs

- `internal/loomengine`, `internal/loomcli`, `internal/loomshed` and `internal/loomrecipe` — the phased orchestrator (`lym loom`);
  design.
- [code-comment-conventions.md](code-comment-conventions.md) — the doc-comment rule's standing rationale (Go only, for now);
  a durable convention doc, kept rather than deleted, moved here by the 2026-08-29 designs audit.
- [webster-spec.md](../contracts/specs/webster-spec.md) — webster's cross-module contract: the `_lym/webster/` boundary, `outcome.yaml`, and `summary.md`'s writer-side additions (as-built;
  kept as a durable contract doc, not deleted on landing).
- [loom-final-summary-spec.md](../contracts/specs/loom-final-summary-spec.md) — the producer-agnostic final-summary artifact contract: its format, its validation, and its two consumers, `internal/landingshed`'s `Finalize` and `Describe` (as-built;
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
- [sandbox-howto.md](sandbox-howto.md) — operator runbook: deploy `lym`, build the Hub, run the suite agent (procedure).
- [sandbox-hub.md](sandbox-hub.md) — the sandbox Hub: a dedicated bench for manual (dogfooding) testing.
- [crucible/README.md](../crucible/README.md) — **`crucible`**, the **serial review+fix loop**: a reusable method for hardening a live-substrate module before merge (orchestrator-driven, model-rotating, clean-room self-fixing rounds + independent verification).
  The hand-executed prototype of the review-gate + `burler` (see the `internal/burlerengine` package documentation) round loop (and the origin of the board's `hardener` concept, named separately to avoid colliding with it);
  ships two paste-ready prompts — an [orchestrator prompt](../crucible/orchestrator-prompt.md) (drives the loop + verifies) and a [round-agent prompt template](../crucible/review-prompt-template.md) (the reviewer-fixer), to instantiate per module.
  Lives at the repo root, not under `docs/`, since it's a working method/prompt set, not documentation of shipped code.
