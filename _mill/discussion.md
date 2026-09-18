# Discussion: Worktree spawn/teardown as Shed producers

```yaml
task: Worktree spawn/teardown as Shed producers
slug: worktree-lifecycle-shed-producers
status: discussing
parent: main
```

## Problem

A task's worktree lifecycle is three CLI invocations an operator bridges by hand: `lyx fabric add <slug>` to create the warp+weft pair, `lyx loom run` inside that new worktree to drive the task to completion, and — once the task has landed — `lyx reed down` followed by `lyx fabric remove <slug>` to tear it down.
Nothing sequences those steps, nothing remembers where in the sequence a machine died, and nothing guarantees the teardown half ever runs at all.

The concrete damage today is the missing `reed down`: no call site anywhere in the tree calls it, and `Topology.Remove`/`removeWarpWorktreeDir` neither knows nor checks whether a live tmux session is bound to the directory it is deleting.
On Linux that silently orphans the session — POSIX allows removing a directory a live process holds as its cwd — leaving a tmux session addressable by no `reed` verb ever again, because no worktree of that name exists to derive its geometry from.
On Windows the same removal fails outright.

**Why now:** `shedengine` and its recipe layer are shipped and generic, loom's own producer list is stable, and `landingshed`'s `Publish`/`Finalize` rows already establish the "producer shared by reference, owned by no one product" pattern this task needs.
The machinery to make the lifecycle one driven run exists; only the rows are missing.

## Scope

**In:**

- A new `internal/lifecycleshed` package owning three `shedengine.ShedProducer` implementations: `WorktreeCreate`, `LoomRun`, `WorktreeTeardown`.
- A new embedded recipe, `contracts/recipes/lifecycle-recipe.yaml`, wiring those three rows as `Worktree-Create → Loom-Run → Worktree-Teardown`.
- A new `internal/lifecyclerecipe` package — `loomrecipe`'s twin — parsing that recipe, building it against a caller-supplied `shedrecipe.Env`, and returning the assembled `*shedengine.Shed`.
- Three new `internal/shedrecipe` registry entries (`WorktreeCreate`, `LoomRun`, `WorktreeTeardown`), taking the registry from fourteen keys to seventeen, with the coverage-guard update in the same commit.
- The `shedrecipe.Env` fields those three entries read: `Slug`, `TaskWorktreeRoot`, and three injected closures (`CreateWorktree`, `RunLoom`, `TeardownWorktree`).
- A new `internal/lifecyclecli` cobra module exposing `lyx lifecycle run <slug>` and `lyx lifecycle status <slug>`, registered under the root in `cmd/lyx/main.go`.
- A new `--no-attach` flag on `lyx loom run`: everything the verb does today except the final terminal handover.
- Doc updates landing in the same commit: `manifest/designs/worktree-lifecycle-shed-producers.md` rewritten from "Planned, not designed in depth" to the as-built design, `docs/overview.md`'s module table and execution stack, `manifest/roadmap.md`'s Planned entry moved to done, and a new `CONSTRAINTS.md` invariant (see Constraints below).

**Out:**

- The `AddStrand`/`attach` self-heal item. It stays its own roadmap item — see the `self-heal-dependency-is-softer-than-recorded` decision.
- Any `Reed-Up` producer row. The bootstrap remains ambient.
- VS Code embedding. A worktree may still spawn VS Code, and that stays an operator action outside the Shed run entirely.
- Any change to loom's own recipe, producer list, status file location, resume semantics, or seed-ownership checks. `contracts/recipes/loom-recipe.yaml` is untouched by this task.
- Remote/GitHub branch deletion at teardown. That is its own Planned roadmap item (`fabric: no remote/GitHub branch deletion`); this task's teardown does exactly what `lyx fabric remove` does today.
- Orphan reaping. The per-hub daemon's reaping extension is the separately-tracked safety net for when this task's deliberate sequencing does not run.
- Generalizing loom's `run`/`drive`/`step` verbs into a Shed-generic watchdog. This task incidentally produces the second `shedrecipe` consumer that item is waiting on, but performs none of that generalization.
- `pause`, `step`, and `drive` verbs on the lifecycle CLI. Pause belongs to the nested loom run, which keeps its own.

## Decisions

### nested-shed-not-one-flat-list

- **Decision:** The lifecycle is its own small Shed — `Worktree-Create → Loom-Run → Worktree-Teardown` — whose middle row drives loom's existing Shed as a child process. It is not loom's own producer list with two extra rows bolted on.
- **Rationale:** A single flat list would have to persist one `shedengine.Status` across a span in which the task worktree does not yet exist (before Create) and no longer exists (after Teardown), so that status file cannot live in the task worktree.
  Relocating it out of the task worktree breaks four things at once: resume for any in-flight task, `loomengine.VerifySeedOwnership`'s forked-worktree guard, the anchoring `loomengine.LoomStatusFile` derives from `l.AnchorPath()`, and the Cwd Resolution Invariant's rule that each module owns its own relative subpath under the anchor.
  The nested shape keeps loom's status where loom's task history wants it — committed on the task branch — and gives the lifecycle its own, separate, per-machine status.
  Two status files is the honest model: the task's run state belongs to the task, the lifecycle's state belongs to the hub, and each resumes independently.
- **Rejected:** One flat list with a hub-relocated status (breaks the above); a teardown row only, leaving creation manual (does not meet the task).

### lifecycle-driver-runs-from-prime

- **Decision:** The lifecycle Shed runs from the hub's **prime** worktree — the warp repository itself — never from the task worktree it manages.
- **Rationale:** This is the enabling fact the whole design rests on, and it falls out of the existing APIs rather than being imposed: `Topology.Add(l, slug, opts)` and `Topology.Remove(l, slug, force)` both take the `*lyxcwd.Location` of the worktree the operator is *running from*, and both refuse to operate on the worktree they are invoked in.
  Running from prime also makes the fork point correct for free — `lyx fabric add` forks the new weft branch from the HEAD of the worktree it runs from, and prime is on `main`, which is exactly this task family's recorded `parent`.
  Neither bookend touches prime, so neither bookend can destroy its own driver's cwd or its own status file.
- **Rejected:** Running from the task worktree (Create cannot run there — it does not exist yet — and Teardown would be deleting its own process cwd, which fails outright on Windows); running from a bare hub directory that is not a git worktree (`lyxcwd.Resolve` requires cwd to be a git worktree root, and every hub-level container is off-limits per the Hub Containment Invariant).

### lifecycle-status-is-ephemeral

- **Decision:** The lifecycle Shed's status and both locks live under prime's ephemeral tree — `<prime>/.lyx/lifecycle/<slug>/status.json`, `run.lock`, `status.json.lock` — and the constructed `shedengine.Shed` leaves `CommitStatus` nil.
- **Rationale:** `shedengine.Shed.CommitStatus` documents nil as the absent value meaning "commit nothing", so this needs no new mechanism.
  Crash recovery still works: Shed reads its status back off disk, and `.lyx` content is never-tracked but entirely durable on disk.
  Committing it instead would mean writing to `weft:main` on every producer transition — the board carve-out's territory — for state that is per-machine and per-attempt, buying contention and history noise for nothing.
  Under the Durable-vs-Ephemeral State Invariant a never-tracked file must live under `.lyx` at the mirrored subpath, which is what the layout above is.
- **Rejected:** Durable `_lyx/lifecycle/<slug>/status.json` committed onto weft:main via the board carve-out (contention and noise, no gain); a hub-level location outside any worktree (no anchor to mirror against).

### loom-run-drives-via-no-attach

- **Decision:** `LoomRun` spawns `lyx loom run --no-attach` as a child process with its working directory set to the task worktree, waits for that command to return, and then polls loom's own `_lyx/loom/status.json` until it reaches a terminal state, mapping `done` → `shedengine.Done` and `blocked`/`paused` → `shedengine.Stuck`.
  `--no-attach` is a new flag on the existing verb: identical behaviour through the handshake, returning instead of handing the terminal to `tmux attach`.
- **Rationale:** Only `lyx loom run` may seed the status file, because only it owns the commit-before-precondition ordering the bootstrap needs — `drive` says so in its own refusal message and pre-flights on the absence of a status file.
  So `LoomRun` cannot use `drive`, and duplicating the seed ordering anywhere else would fork a rule that exists in exactly one place today.
  The terminal handover is the only part of `run` a non-interactive parent cannot use, and it is already the narrow CLI/Cobra interactive-handoff exception — a flag that skips it is the smallest possible change.
  Reading the verdict from loom's persisted status rather than from the child's exit code is deliberate: `run` returns as soon as the detached driver takes the run lock, long before the task is finished, so the status file is the only truth about the run's outcome.
- **Rejected:** `lyx loom drive` plus a new `lyx loom seed` verb (splits the seed ordering across two verbs — the exact thing `drive`'s own doc comment warns against); importing `loomrecipe`/`loomcli` and running loom's Shed in-process (an engine importing a `*cli` package inverts the CLI/Cobra Invariant's dependency direction, and loom's whole wiring resolves through `lyxcwd.Resolve(cwd)` against a cwd this process cannot have).

### teardown-escalates-never-forces

- **Decision:** `Worktree-Teardown` is reachable only from `Loom-Run`'s `Done`. `Loom-Run` carries no `on_stuck`, so a blocked or paused loom run escalates to a human with the worktree fully intact.
  `Worktree-Teardown` calls `Topology.Remove(primeLocation, slug, false)` — never `force: true` — and returns `Stuck` when fabric refuses on dirtiness.
- **Rationale:** Shed's routing model gives this for free: an empty `OnStuck` escalates, and a `Done`-only edge into Teardown means the destructive row is unreachable from any failure path.
  A dirty worktree at teardown time means unlanded work, and deleting it is unrecoverable — precisely the class of decision the Fabric Destruction Chokepoint Invariant reserves gates for, where `--force` answers dirtiness only and is the operator's call, never a producer's.
- **Rejected:** Unconditional teardown with `force: true` (silently destroys unlanded work); a recipe `force:` config key (a recipe file is the wrong place to pre-authorize a destructive override for every future run).

### teardown-is-one-row-sequencing-internally

- **Decision:** `Worktree-Teardown` is a single producer that internally sequences `reedengine.Engine.Down()` and then `Topology.Remove`, in that order. It is never two Shed rows, and it never probes for a live session first.
- **Rationale:** Settled in the design doc and unchanged here: `reed down` is idempotent and cheap, loading no state and reaching no foreign-session refusal, so calling it against a worktree that never had a session costs one no-op.
  Two rows would put a resumable boundary in the middle of a teardown, where a crash between them leaves exactly the orphaned-session state this task exists to prevent.
  The reed `Geometry` is built from the task worktree's still-live `*lyxcwd.Location` via `hubgeom.ReedGeometry`, which is valid right up until `Remove` runs.
- **Rejected:** Two rows (`Reed-Down` then `Fabric-Remove`); shelling out to `lyx reed down` and `lyx fabric remove` as child processes (both engines are directly callable in-process and already under test as such).

### create-is-fabric-add-and-nothing-else

- **Decision:** `WorktreeCreate` calls `Topology.Add(primeLocation, slug, fabricengine.AddOptions{})` and nothing else. No reed boot, no VS Code spawn, no strand.
- **Rationale:** The design doc settles that no explicit bootstrap row is needed, and the `self-heal-dependency-is-softer-than-recorded` decision below shows the bootstrap is already covered on this task's critical path.
  VS Code embedding is a passive tmux client attaching to a session the driven path brings up anyway, so it is an operator action, not a producer.
- **Rejected:** Also running `reed up` (redundant — `lyx loom run` already ensures the substrate); also spawning VS Code (an interactive convenience with no place in a driven run).

### self-heal-dependency-is-softer-than-recorded

- **Decision:** The `AddStrand`/`attach` self-heal item is **out of scope** and is not a blocker for this task, despite the design doc and roadmap both recording it as a hard dependency.
  It remains its own Planned roadmap item, unchanged.
- **Rationale:** Verified in-tree: the self-heal has not landed — `reedengine.Engine.AddStrand` (`strand.go:386`) and `AttachArgv` (`attach.go:84`) both still call `requireSessionLocked` and fail with the no-session error.
  But this task's critical path never depends on it: `lyx loom run` step 2 already ensures the worktree's tmux session is up and its status strand exists, before any producer that spawns a strand runs.
  The self-heal only affects the *optional* cold-attach conveniences the design doc lists — an operator running `lyx reed attach`, or VS Code's `folderOpen` task doing so — on a worktree nobody has visited.
  Blocking this task on it would be blocking on a dependency that the driven path does not actually have.
  This correction belongs in the rewritten design doc.
- **Rejected:** Pulling the self-heal in as this task's first batch (expands scope into a separately-tracked roadmap item for no benefit on this task's path); adding an explicit `Reed-Up` row instead (re-introduces the row the design doc deliberately argues away, and duplicates what `lyx loom run` already does).

### three-registry-entries-closures-not-engine-imports

- **Decision:** The three producers reach `fabricengine` and `reedengine` through **injected closures** on `shedrecipe.Env` (`CreateWorktree func(context.Context) error`, `RunLoom func(context.Context) (string, error)`, `TeardownWorktree func(context.Context) error`), filled by `internal/lifecyclecli`'s own `wire()`.
  The registry entries validate `Env.Slug`, `Env.TaskWorktreeRoot`, and the relevant closure, then hand them to the `lifecycleshed` constructors.
- **Rationale:** This matches the convention `Env.CommitDiscussion`, `Env.CommitPlan`, and `Env.ApprovePlan` already set for exactly this situation — work that needs a resolved `*lyxcwd.Location`, which the Shed Recipe Registry Invariant bars this package from importing.
  `internal/lifecyclecli` is the layer that legitimately resolves geometry, so it is the layer that closes over it.
  `Env.Slug` and `Env.TaskWorktreeRoot` are run-wide values, not per-row ones, which is exactly the class `Env` carries; anything per-row would have to be a `Config` key instead.
- **Rejected:** Having the registry entries import `fabricengine`/`reedengine` directly (possible — `shedrecipe` already pulls `fabricengine` transitively through `landingshed.Deps` — but it would make the entries resolve geometry, which the invariant forbids); a whole-struct `Deps` passthrough like `Env.Landing` (three closures do not justify a struct, and the closure convention is the closer precedent); hand-assembling the `ProducerDef`s in Go outside the registry (breaks the Shed Recipe Registry Invariant's rule that every recipe row's engine resolves through `Lookup`).

### lifecycle-cli-two-verbs

- **Decision:** A new `internal/lifecyclecli` cobra module with exactly two verbs: `lyx lifecycle run <slug>` (drives the lifecycle Shed in the foreground) and `lyx lifecycle status <slug>` (prints the lifecycle status file).
  Package naming follows the shipped `landingshed`/`preflightshed`/`loomshed` convention: `lifecycleshed` for the producers, `lifecyclerecipe` for the builder, `lifecyclecli` for the verbs.
- **Rationale:** The `run`/`drive` split exists in loom only because `run` hands the terminal to tmux and `drive` does not; at hub level there is no reed session and no handover, so one foreground verb covers both roles.
  `step` and `pause` belong to the nested loom run, which keeps its own.
  A separate module rather than verbs under `lyx board` or `lyx fabric` keeps the CLI/Cobra Invariant's one-module-one-subtree shape and avoids giving `fabric` — whose destruction chokepoint is deliberately narrow — an orchestrating verb.
- **Rejected:** Verbs under `lyx board` (board owns the task board, not task execution); verbs under `lyx fabric` (fabric is git/worktree mechanics and must not orchestrate); a full `run`/`drive`/`step`/`pause` quartet mirroring loom (three of the four have no meaning here).

### recipe-is-embedded-not-on-disk

- **Decision:** `contracts/recipes/lifecycle-recipe.yaml` is embedded into the binary alongside `loom-recipe.yaml`, and `lifecyclerecipe.New` uses `shedbuild.Parse` on the embedded bytes, never `shedbuild.Load`. It never calls `shedbuild.Check`.
- **Rationale:** Verbatim the reasoning `loomrecipe.New` already records: there is no on-disk runtime location for this recipe, and `Check` is authoring-time only because a resumed run legitimately starts mid-graph, making reachability-from-entry the wrong production question.
- **Rejected:** An on-disk recipe read at runtime (no location to read it from, and a mutable recipe for a destructive pipeline is a hazard).

## Technical context

**The Shed seam.** `shedengine.ShedProducer` is a one-method interface — `Call(ctx) (Outcome, OutputPointer, error)` — with two obligations the engine cannot enforce: return exactly `Done` or `Stuck`, and surface context cancellation as a non-nil error, never as `Stuck`.
Every existing producer package discharges the second obligation with a local `entryErr`/`cancelErr` pair; `internal/preflightshed/ctx.go` and `internal/loomshed/ctx.go` are the two shipped copies, and both document the duplication as deliberate.
`lifecycleshed` follows the same pattern with its own copy.

**Routing.** `shedengine.ProducerDef` carries `Name`, `Producer`, `OnStuck`, `OnDone`, `Segment`, `MaxBounces`. There is no positional fallback: an empty `OnDone` ends the whole run quietly, an empty `OnStuck` escalates to a human (`state: "blocked"`). A non-empty `OnStuck` must name a producer sharing the same `Segment`, which the validator enforces — the lifecycle recipe uses no segments at all, since it has no review pair.

**The registry.** `internal/shedrecipe/registry.go` holds one `map[string]Constructor` literal, reached only through `Lookup`/`Names`, with no `init()` self-registration and no runtime `Register`. Its own comment pins the table at fourteen keys and states that any fifteenth entry must arrive with a coverage-guard update in the same commit — this task adds three, so that guard (`internal/loomrecipe/coverage_guard_test.go`) moves with them.
Entry constructors live in `entries_*.go` files grouped by shape; the three new ones are closest to `entries_simple.go`'s shape (validate a couple of `Env` fields, call a constructor) and should get their own `entries_lifecycle.go`.

**Env validation helpers.** `internal/shedrecipe/env.go` provides `requireAbsRoot(entry, field, value)` and `requireSeam(entry, field, seam)`; the latter detects typed-nil interface and func values via reflection. An entry validates exactly the fields it reads and never a field it does not, so a caller filling only what its own recipe needs is legal — which is what lets the lifecycle recipe leave loom's dozens of `Env` fields empty.
`Env.Slug` is a plain string, not a path, so it needs a new non-`requireAbsRoot` check (non-empty) rather than reusing the absolute-path helper.

**The recipe builder.** `internal/loomrecipe/loomrecipe.go` is the template to copy: a `ShedPaths` struct carrying the five values `shedengine.Shed` reads and no registry entry reads (`StatusPath`, `LockPath`, `StatusLockPath`, `MaxBounces`, `CommitStatus`), plus a `New(env, paths)` that parses, builds, and assembles.
It also performs one coherence check across its two arguments — `env.StatusPath != paths.StatusPath` and the same for `StatusLockPath` — because loom's `Env` and `ShedPaths` deliberately carry duplicate copies read by different consumers.
The lifecycle recipe has **no** such duplication: no lifecycle registry entry reads `StatusPath` or `StatusLockPath`, so `lifecyclerecipe.New` needs no such check and must not grow one for symmetry's sake.

**Path derivation.** `loomengine/config.go` derives loom's status paths as `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, ...)` — a module's own constant joined onto `AnchorPath()`, never a `lyxcwd` call, per the Cwd Resolution Invariant.
`lifecyclecli` derives its own the same way against prime's `Location`, with a per-slug segment: `<prime anchor>/.lyx/lifecycle/<slug>/`.

**Fabric APIs.**
`Topology.Add(l *lyxcwd.Location, slug string, opts AddOptions) (AddResult, error)` — `AddOptions` is an alias of `SyncOptions` (`SkipGit`/`SkipPush`); the zero value is the ordinary path. `Add` validates the slug, creates both worktrees, wires junctions, records the pair's parent-branch provenance, and pushes, rolling everything back on any failure.
`Topology.Remove(l *lyxcwd.Location, slug string, force bool) (RemoveResult, error)` — removes the pair plus every warp junction, portal junction, and launcher; refuses a dirty worktree on either side unless forced; refuses hub geometry, prime, reserved entries, and weft-suffixed names.
Both embed `MutationRecord` and accumulate a `*Mutations` record; the Mutation Record Invariant requires every mutating result type to expose it under the fixed envelope key set, which matters if these results surface through the CLI envelope.

**Reed APIs.**
`reedengine.New(cfg Config, geom Geometry) *Engine`, with `hubgeom.ReedGeometry(l)` as the hub-mode teller. `Engine.Down() (DownResult, error)` takes the op lock, captures the server pid and pane process subtrees before `kill-session`, deletes the state file, and is the one lyx-only escape from the foreign-session refusal because it loads no state. It is idempotent.
Note its documented behaviour after a worktree rename: it reports the abandoned session in the result and at `Warn`, and deliberately does not kill it. The teardown producer should surface that field rather than swallow it.

**Loom bootstrap.** `internal/loomcli/run.go`'s `runCmd` performs four steps: resolve parent and seed+commit the status file, ensure the reed substrate and the `loom-status` strand, spawn the detached `lyx loom drive` child unless the run lock is already held, then hand the terminal over via `tmux attach`.
The handshake between steps 3 and 4 (`awaitRunLock`, `bootstrap.go`) polls at most 300 times at 100 ms for the driver to take the run lock, with a four-way outcome: ready, child-died, halted, deadline.
`--no-attach` returns after that handshake instead of executing step 4 — the terminal handover is already the CLI/Cobra Invariant's narrow interactive-handoff exception, so skipping it removes an exception rather than adding one.

**Loom status shape.** `shedengine.Status` carries `current_producer`/`state`/`error`/`pause_requested`/`activity`/`history`; loom's own three fields (`slug`, `parent`, `start_sha`) live inside the opaque `Product` passthrough. `state` takes the three clean-exit values whose strings are identical to `RunOutcome`'s — `done`, `blocked`, `paused` — so `LoomRun`'s mapping is a direct read, not a lookup table.
Read it through `state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)`, taking loom's own status lock, per the external-writer lock contract in `shedengine/doc.go`.

**Process spawning.** `internal/proc.Detach` is the existing detach helper; `internal/loomcli/run.go` and `internal/boardengine/spawn.go` are the two shipped spawn sites to model on.
The Live-Substrate Spawn Observability invariant requires every spawn reachable from a `lyx` command to log via `internal/logger` — `Info` for a lifecycle spawn, and teardown logged wherever the code waits for one. `LoomRun` waits, so it logs both.

**Vocabulary.** `lifecycleshed`, `lifecyclerecipe`, and `lifecyclecli` are **not** in the Fabric Vocabulary Invariant's owner set, so none of their identifiers, string literals, or comments may name warp or weft. The ban is machine-enforced by `internal/lyxcwd`'s `TestEnforcement_FabricVocabulary`, which walks every identifier.
`landingshed/doc.go` records the same constraint for itself and is the model for how to phrase around it ("the task worktree", "the pair", never the two sides).

**Build prerequisite.** `lyx` is a cgo binary (`CGO_ENABLED=1` plus a C compiler on `PATH`); nothing in this task changes that, but any batch running `go build`/`go test` needs it.

## Constraints

From `CONSTRAINTS.md`, the invariants this task is bound by:

- **Cwd Resolution Invariant** — `internal/lyxcwd` alone resolves cwd. Each new module joins its own constant onto `AnchorPath()`; none calls `os.Getwd` or `git rev-parse --show-toplevel`.
- **Told-Geometry Invariant** — `lifecycleshed` takes told absolute paths and injected closures, with no direct production import of `internal/lyxcwd`. `lifecyclecli` is the tier that resolves. Add `internal/lifecycleshed` and `internal/lifecyclerecipe` to the invariant's bound-packages list in the same commit.
- **Durable-vs-Ephemeral State Invariant** — the lifecycle status file and both locks are never-tracked and therefore live under `.lyx` at the mirrored subpath; `lifecycleshed` derives no `.lyx` path of its own.
- **Hub Containment Invariant** — no hub-level container is junctioned into a worktree; the lifecycle state lives under prime's own anchor, not under `_board`/`_portals`/`_launchers`.
- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `state`, `lock`; `StatusPath`/`LockPath`/`StatusLockPath` stay caller-supplied. Unchanged by this task.
- **Shed Recipe Registry Invariant** — every registry value constructs a `shedengine.ShedProducer` through the one `map[string]Constructor` reached only via `Lookup`/`Names`; no `init()` self-registration, no runtime `Register`, no direct `lyxcwd` import, every path told.
- **Recipe-Format Sole-Parser Invariant** — `internal/shedbuild` is the sole parser of the recipe file format and declares no on-disk location for recipe files. `lifecyclerecipe` parses embedded bytes through it.
- **CLI / Cobra Invariant** — `lifecyclecli` exposes `Command() *cobra.Command` and `RunCLI(out, args) int`, carries non-empty `Short` on every command, routes errors as JSON through `internal/output`, and checks `clihelp.ShouldAbort` first in every `RunE`. Package naming: `lifecyclecli` imports `lifecycleshed`/`lifecyclerecipe`; neither imports cobra. The help-tree tests move with it.
- **Fabric Destruction Chokepoint Invariant** — teardown routes through `Topology.Remove`, whose gates run in order (containment, ownership, dirtiness, force) and whose refusals are never silently discarded. `--force` answers dirtiness only, and this task never passes it.
- **Fabric Git Invariant** — every git op goes through `internal/fabricengine` in-process; the new producers add no raw git and no agent-driven git.
- **Fabric Vocabulary Invariant** — the three new packages are outside the owner set; no identifier, literal, or comment in them may name either side of the pair.
- **Mutation Record Invariant** — `AddResult`/`RemoveResult` carry mutation records; anything surfacing them through the CLI envelope exposes `mutations` (always an array) and `partial` (always a bool), and a pre-flight failure emits a bare `output.Err` with neither key.
- **Live-Substrate Spawn Observability** — `LoomRun` spawns a real OS process and waits for it, so it logs the spawn at `Info` and the teardown at the wait site. Any retry loop caps attempt count, not only elapsed time.
- **Test Tier Purity Invariant** — no `gitexec.Run`/`RunGit`, `exec.Command`, `gitkit.Copy*`, or `hubforge.NewHub` outside `integration`/`smoke`-tagged files; no `time.Sleep` ≥ 1s in an untagged file.
- **Hermetic Git Test Environment Invariant** — every new test package whose tests spawn git calls `gitkit.HermeticGitEnv()` in `TestMain` before `m.Run()`.
- **hubforge Fabric-Fixture Invariant** — the integration test's hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`, never hand-assembled.
- **Sandbox Suite Coverage** — `lifecyclecli` is a registered lyx module, so it is either exercised by the sandbox suite or explicitly excluded with a written reason.
- **Markdown Link Integrity** — every inline link in the rewritten `manifest/designs/worktree-lifecycle-shed-producers.md` and in the `docs/overview.md` edits must resolve, file part and anchor.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle`.

**New invariant to record in `CONSTRAINTS.md`, same commit:**

> **Lifecycle Bookend Invariant** — a producer that creates or destroys a task worktree never runs from inside that worktree.
> The lifecycle Shed is driven from the hub's prime worktree; its status file and locks live under prime's own ephemeral tree, never under the worktree being managed.
> A teardown row sequences session shutdown before worktree removal, in one producer, never two rows.

## Testing

`internal/lifecycleshed` — Tier 1, untagged, table-driven over injected seams. Each of the three producers gets: the happy path returning `Done`; the seam returning an error mapped to `Stuck` (or a hard error, per the producer's own contract); `entryErr`/`cancelErr` coverage proving a cancelled context surfaces as a non-nil error and never as `Stuck` — this is the obligation Shed cannot enforce and is a TDD candidate.
`WorktreeTeardown` additionally needs ordering coverage: a fake recording the call sequence must show session-shutdown strictly before removal, and a removal refusal must still report the shutdown that already happened rather than swallowing it.
`LoomRun` needs verdict-mapping coverage over a fake status reader for all three terminal states plus a status file that never reaches one, and a fake process runner for the spawn-failure path.

`internal/shedrecipe` — extend the existing entry tests: each new entry validates exactly the `Env` fields it reads and rejects nil closures (including typed-nil, which `requireSeam` handles), rejects an empty `Slug`, and rejects unknown `Config` keys via `configRejectUnknown`. The registry `Names()` test and `internal/loomrecipe/coverage_guard_test.go` both move from fourteen to seventeen keys.

`internal/lifecyclerecipe` — parse and build the embedded recipe against a minimal fake `Env`, asserting the three row names, the `Worktree-Create → Loom-Run → Worktree-Teardown` `on_done` chain, `Loom-Run`'s empty `on_stuck`, `Worktree-Teardown`'s empty `on_done`, and that no row declares a `Segment`. A row-names-match-the-Go-constants test mirrors loom's own durable-identity guard, since these names are on-disk identities that resume depends on.

`internal/lifecyclecli` — Tier 1 CLI tests in the shape the existing `*cli` packages use: `Short` non-empty on every command, arg arity, `clihelp.ShouldAbort` honoured, JSON error envelopes, and the help-tree test. Path-derivation tests assert the `<prime anchor>/.lyx/lifecycle/<slug>/` layout and that the three paths are distinct (`LockPath != StatusLockPath` is enforced by `Shed.validate()` and must not first fail at runtime).

`internal/loomcli` — `--no-attach` needs a test proving the verb performs every step through the handshake and then returns without attaching, and that the flag's absence leaves today's behaviour byte-identical. The existing `parity_test.go`/`cli_test.go` are the right homes.

Integration, `integration`-tagged — one end-to-end over a real `hubforge` hub: drive the lifecycle Shed with a stubbed `RunLoom` that returns `Done` without spawning anything, and assert the pair exists after `Worktree-Create` and is gone after `Worktree-Teardown`. A second case stubs `RunLoom` to return `Stuck` and asserts the run halts `blocked` with the worktree still present — the safety property the whole design turns on. `TestMain` calls `gitkit.HermeticGitEnv()`.

Cross-cutting: a resume test proving the lifecycle Shed restarts mid-list from its persisted status (crash after `Worktree-Create`, resume into `Loom-Run`) without re-running the create row.

## Q&A log

- **Q:** Should the lifecycle be one flat list (loom's rows plus bookends) or its own Shed nesting loom's? **A:** [auto-pick] Separate hub-anchored lifecycle recipe whose middle row drives loom's existing Shed as a child process. **Why:** a single list would have to persist status across a span where the task worktree does not exist, forcing a relocation that breaks resume, `VerifySeedOwnership`, and the Cwd Resolution Invariant's anchoring.
- **Q:** Where does the lifecycle driver run and where does its state live? **A:** [auto-pick] From the hub's prime worktree, with status and locks under prime's `.lyx/lifecycle/<slug>/` and `CommitStatus` nil. **Why:** prime is the one worktree neither bookend touches and is already the correct `Location` for `Topology.Add`/`Remove`; committing per-transition status to weft:main buys contention for per-machine state.
- **Q:** How does `Loom-Run` drive loom, given `drive` refuses without a seeded status file and `run` always attaches? **A:** [auto-pick] Spawn `lyx loom run --no-attach` (new flag) with `Dir` set to the task worktree, then poll loom's `status.json` to a terminal state. **Why:** only `run` owns the commit-before-precondition seed ordering, and the terminal handover is the only part a non-interactive parent cannot use.
- **Q:** Should teardown ever force, and what runs it? **A:** [auto-pick] Teardown is reachable only from `Loom-Run`'s `Done`; `Loom-Run` has no `on_stuck` so failures escalate with the worktree intact; teardown never forces and returns `Stuck` on a dirtiness refusal. **Why:** a dirty worktree at teardown means unlanded work, and forcing destroys it unrecoverably.
- **Q:** One teardown row or two? **A:** [auto-pick] One row sequencing session shutdown then removal internally, with no liveness probe first. **Why:** settled in the design doc; two rows put a resumable boundary mid-teardown where a crash leaves exactly the orphaned session this task exists to prevent.
- **Q:** Does `Worktree-Create` do anything beyond `Topology.Add`? **A:** [auto-pick] No — no reed boot, no VS Code, no strand. **Why:** the bootstrap is ambient and `lyx loom run` already ensures the substrate; VS Code embedding is an operator action outside the driven run.
- **Q:** The design doc records the `AddStrand`/`attach` self-heal as a hard dependency — has it landed, and must this task wait? **A:** [auto-pick] It has not landed, and this task does not need it; it stays its own roadmap item. **Why:** verified that `AddStrand` (`strand.go:386`) and `AttachArgv` (`attach.go:84`) both still call `requireSessionLocked`, but `lyx loom run` step 2 already ensures the session, so the self-heal affects only the optional cold-attach conveniences. The rewritten design doc records this correction.
- **Q:** What CLI surface? **A:** [auto-pick] A new `internal/lifecyclecli` module with `lyx lifecycle run <slug>` and `lyx lifecycle status <slug>`; packages named `lifecycleshed`/`lifecyclerecipe`/`lifecyclecli`. **Why:** the `run`/`drive` split exists only because of loom's terminal handover, which has no hub-level equivalent; `step`/`pause` belong to the nested loom run.
- **Q:** Registry entries, or hand-assembled `ProducerDef`s? **A:** [auto-pick] Three new registry keys — `WorktreeCreate`, `LoomRun`, `WorktreeTeardown` — taking the table from fourteen to seventeen with the coverage guard updated in the same commit. **Why:** the Shed Recipe Registry Invariant requires every recipe row's engine to resolve through `Lookup`; this also produces the second `shedrecipe` consumer the `shed-generic-watchdog` item is waiting on.
- **Q:** How do the entries reach `fabricengine`/`reedengine` without resolving geometry? **A:** [auto-pick] Injected closures on `shedrecipe.Env`, filled by `lifecyclecli`'s `wire()`. **Why:** matches the `CommitDiscussion`/`CommitPlan`/`ApprovePlan` convention already shipped for exactly this situation.
- **Q:** Testing split? **A:** [auto-pick] Tier-1 unit tests per producer over injected seams, plus recipe parse/build and registry coverage, plus one `integration`-tagged end-to-end over a real `hubforge` hub with `RunLoom` stubbed — including the `Stuck`-preserves-the-worktree case and a mid-list resume case. **Why:** Test Tier Purity bars real git and process spawns from untagged files, so the seam split is forced regardless.
