# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success.
Do NOT commit.
Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish.
When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent.
In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides).
Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/worktree-lifecycle-shed-producers rm <file>`.

### From discussion.md

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
- Three new `internal/shedrecipe` registry entries (`WorktreeCreate`, `LoomRun`, `WorktreeTeardown`), taking the registry from fourteen keys to seventeen, with the coverage-guard restructuring in the same commit (see the `registry-coverage-guard-spans-consumers` decision).
- A `RecipeEngines() []string` accessor on each recipe package, plus a new cross-consumer registry-coverage guard; `internal/loomrecipe`'s own guard loses the half that can no longer be true from inside one consumer.
- `internal/shedbuild`'s `newTestEnv` fixture extended to fill all five new read seams — `Env.Slug`, `Env.CreateWorktree`, `Env.LoomRun`, `Env.Teardown`, and `Env.PrimeLock` — the second registry-wide test that fails on the three new keys, alongside the `loomrecipe` coverage guard.
- A `**Covers:** lifecycle` sandbox scenario in `tools/sandbox/SANDBOX-FABRIC-SUITE.md` and its `sandbox/posix`/`sandbox/win` runner counterparts.
- The `shedrecipe.Env` fields those three entries read: `Slug`, one injected closure (`CreateWorktree`), two whole-struct passthroughs (`LoomRun lifecycleshed.LoomRunDeps`, `Teardown lifecycleshed.TeardownDeps`), and `PrimeLock lifecycleshed.PrimeLock` (read by the two bookend entries only).
- A new `internal/lifecyclecli` cobra module exposing `lyx lifecycle run <slug>` and `lyx lifecycle status <slug>` — with `Command()`, `RunCLI`, and `RunCLIIn` — registered under the root in `cmd/lyx/main.go`, and refusing on both verbs when not invoked from the hub's prime worktree.
- Two advisory locks, not one: the per-slug `run.lock` Shed already needs, plus a prime-wide `<prime anchor>/.lyx/lifecycle/run.lock` acquired and released by the two bookend producers themselves, reaching them as an `Env.PrimeLock` seam.
- A new `--no-attach` flag on `lyx loom run`: everything the verb does today except the final terminal handover.
- The CLI/Cobra Invariant's `RunCLIIn` module-count line amended from "eleven of twelve" to "twelve of thirteen".
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
  This is **enforced at runtime**: `lyx lifecycle run` and `lyx lifecycle status` both refuse on the envelope when their resolved `*lyxcwd.Location` is not the hub's prime worktree, comparing `location.WorktreeName` against `fabricengine.PrimeName(location)`.
  `PrimeName` returns `(string, error)` and has a third outcome — it fails when the worktree list cannot be read or carries no main entry — and that outcome is **also a refusal envelope on both verbs, never a hard error and never a pass**.
  Failing closed is the only safe reading: an unresolvable prime name means the hub geometry is already broken, and treating it as "probably fine, proceed" would run the bookends from an unverified vantage point, which is precisely what the refusal exists to prevent. The envelope names `PrimeName`'s own error, which already distinguishes the two causes.
  Note `fabricengine.Remove`'s own `refusePrimeSlug` deliberately treats the same failure as non-fatal (`remove.go:205-211`) and that asymmetry is intentional, not an inconsistency to reconcile: there, the failure only weakens one guard among several that still refuse; here, it is the whole check.
- **Rationale:** The refusal is what makes the arrangement true, and it has to be added rather than inherited — an earlier draft of this decision claimed `Add` and `Remove` "both refuse to operate on the worktree they are invoked in", and that is **wrong on both counts**.
  `Topology.Remove`'s only self-protection is `refusePrimeSlug` (`internal/fabricengine/remove.go:208-219`), which compares the *named slug* against `PrimeName(l)`; removing the worktree you are standing in is not refused at all, so a lifecycle run driven from the task worktree would delete its own cwd — silently on Linux, and fatally on Windows.
  `Add` has no self-refusal either; a colliding slug surfaces as its branch-exists error, which is a different check for a different reason.
  With the fact corrected, the reasons to run from prime stand on their own: both verbs take the `*lyxcwd.Location` of the worktree they are *invoked from* and operate on a named slug relative to it, so prime is the natural vantage point; `lyx fabric add` forks the new weft branch from the HEAD of the worktree it runs from, and prime is on `main`, which is exactly this task family's recorded `parent`; and neither bookend touches prime, so neither can destroy its own driver's cwd or its own status file.
  Refusing rather than silently re-resolving to prime is deliberate: an operator who runs the verb from a task worktree has a mistaken mental model, and quietly doing the right thing elsewhere would leave that model uncorrected.
- **Rejected:** Running from the task worktree (Create cannot run there — it does not exist yet — and Teardown would be deleting its own process cwd, which fails outright on Windows); running from a bare hub directory that is not a git worktree (`lyxcwd.Resolve` requires cwd to be a git worktree root, and every hub-level container is off-limits per the Hub Containment Invariant); relying on fabric's existing refusals to prevent either (they do not cover this, as above); silently resolving to prime from whatever worktree the operator happens to be in (hides the operator's error); leaving the constraint to review discipline alone (the Lifecycle Bookend Invariant has no enforcing test precisely because cwd is a runtime property — this refusal is the runtime check that closes that gap).

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

### loom-run-poll-bound

- **Decision:** `LoomRun` polls loom's `status.json` on a fixed interval with an **attempt-count** cap, both settable as `Loom-Run` recipe `Config` keys with defaults: `poll_interval_s` (default `5`) and `poll_attempts` (default `8640`, which is twelve hours at the default interval).
  The verdict table is exhaustive:
  The table is exhaustive over all five `shedengine.State` values, not three — see the correction in "Loom status shape" below.
  - loom's `state` reaches `done` → `shedengine.Done`.
  - `state` reaches `blocked` or `paused` → `Stuck`, with the reason carrying loom's own `error` field and `current_producer` so the escalation names which loom row halted.
  - `state` reaches `failed` → `Stuck`, same reason content. `failed` is a real persisted value, not a theoretical one: `shedengine.Run` writes it on a producer hard error and on an unrecognised outcome (`internal/shedengine/run.go:226`, `:289`).
    Omitting it would be the worst kind of gap here — `failed` is not a polling state, so a `LoomRun` that only recognised `done`/`blocked`/`paused` would keep polling a finished, failed run until the attempt cap expired, stalling the lifecycle for the full twelve-hour default before escalating with a misleading timeout reason.
  - the attempt cap is exhausted with `state` still `running` → `Stuck`, reason naming the interval, the attempt count, and the elapsed wall-clock.
  - `status.json` is absent after `Spawn` returns → `Stuck`. The child's own handshake already confirmed a driver took the run lock, so a missing seed at that point is a real inconsistency for a human, not a retry case.
  - `status.json` exists but does not decode, or the status lock cannot be taken → **hard error**, not `Stuck`.
  - `Spawn` itself fails → `Stuck`.
- **Rationale:** The Live-Substrate Spawn Observability invariant requires a retry loop around a real spawn to cap attempt **count**, not only elapsed time, so the cap is expressed in attempts and the interval is what converts it to wall-clock.
  The twelve-hour default is deliberately generous: a real loom run spans hours, and a cap that fires on a healthy long task would be worse than no cap at all. It is a `Config` key rather than a constant precisely because the right value is per-deployment.
  `Stuck` rather than a hard error for the cap and the missing-seed case follows this task's general refusal policy (see `refusal-to-outcome-mapping`): both are operator-fixable states, and `Stuck` records the reason in the lifecycle status file where a human can read it, whereas a hard error aborts the run and leaves `Run` returning an unpopulated `Result`.
  A decode failure is the exception because it is mechanism failure, not a producer verdict.
- **Rejected:** An elapsed-time-only deadline (the invariant names count explicitly); no cap at all (a wedged driver hangs the lifecycle run forever with no status entry explaining why); hardcoded constants (the right wall-clock budget is not knowable from here); treating `paused` as a terminal `Done` (a paused loom run is unfinished, and tearing down its worktree would destroy the work it paused mid-way through).

### refusal-to-outcome-mapping

- **Decision:** One rule governs all three producers: **an operator-fixable refusal is `Stuck`; a hard `error` is reserved for mechanism failure and context cancellation.** Applied per producer:
  - `Worktree-Create` — every `Topology.Add` error is `Stuck`, and the stuck reason passes fabric's own error text through verbatim rather than rewording it. Two refusals are called out by name because they are the everyday ones and neither is obvious from the row's description: a **dirty prime** (`internal/fabricengine/add.go:54-60` runs `worktreeDirty(scopeTracked, l.WorktreePath())` against the *driving* worktree, so uncommitted tracked changes on prime fail every lifecycle run before anything is created), and a **pre-existing warp branch** (`add.go:66-76`, whose error already names both ways out — `lyx fabric checkout`, or deleting a leftover branch — which is exactly why it is passed through unreworded).
  - `Worktree-Teardown` — `reedengine.Engine.Down()` error is `Stuck`, and teardown then **does not proceed to `Remove`**; abandoning the ordering is the one thing this single row exists to prevent. Every `Topology.Remove` error is likewise `Stuck`, which includes more than dirtiness: `ErrMergeInProgress` for the named pair (`internal/fabricengine/remove.go:63-71`) and the other-direction refusal when some *other* pair in the hub is mid-merge on these branches.
  - `LoomRun` — per the table in `loom-run-poll-bound`.
- **Rationale:** Every `Add`/`Remove` refusal is a precondition a human can clear and then resume into, which is precisely what `Stuck` plus an empty `OnStuck` gives: the run halts `blocked`, the reason is persisted, the worktree state is untouched, and re-running the verb resumes from the same row.
  A hard error gives none of that — `shedengine.Run` returns an unpopulated `Result` alongside it, so the reason never reaches the status file.
  The dirty-prime case deserves its named callout because its blast radius is disproportionate to its obviousness: it is the one refusal that fires before any lifecycle work happens at all, on a condition the operator may not associate with the worktree they are creating.
- **Rejected:** Mapping only dirtiness and leaving the rest to "the producer's own contract" (no section stated that contract, which is what made this ambiguous); hard-erroring on fabric refusals (loses the persisted reason); rewording fabric's error text into lifecycle-flavoured prose (fabric's messages already name their remedies, and paraphrasing them would fork the remedy text).

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

- **Decision:** The three producers reach `fabricengine`, `reedengine`, `loomengine`, and `lyxcwd` through injected seams filled by `internal/lifecyclecli`'s own `wire()`. The seam shapes are **not uniform**, because the three producers are not the same shape:
  - `Env.CreateWorktree func(context.Context) error` is a single closure. That producer's whole job is one told action, so there is nothing else for it to own.
  - `Env.Teardown` is a whole-struct `lifecycleshed.TeardownDeps` passthrough with two separate fields: `Shutdown func(context.Context) (abandonedSession string, err error)` and `Remove func(context.Context) error`.
    The split is not stylistic. The teardown producer is required to call shutdown strictly before removal, to call removal **not at all** when shutdown fails, to say which of the two failed in its stuck reason, and to surface the abandoned-session value on an otherwise-`Done` row — and not one of those four is observable, or testable, behind a single opaque `error`.
    `Shutdown` returns the abandoned-session string rather than logging it internally so the producer can log it at `Warn` in its own name; the envelope half is the CLI's, since nothing reaches an envelope from inside a producer (see "Reed APIs" in Technical context).
  - `Env.LoomRun` is a whole-struct `lifecycleshed.LoomRunDeps` passthrough, following `Env.Landing`'s precedent rather than the closure one, because `LoomRun` is a producer with real logic of its own and therefore needs more than one seam: `Spawn func(context.Context) error` (runs the child command in the task worktree), `ResolveStatus func() (statusPath, statusLockPath string, err error)` (evaluated lazily at `Call` time — see `loom-status-paths-derived-lazily-by-lifecyclecli`), `ReadStatus func(statusPath, statusLockPath string) (state string, found bool, err error)`, and `Now func() time.Time` (nil selects the production clock).
  - There is **no** `Env.TaskWorktreeRoot`. An earlier draft carried one and no entry read it: the task worktree's path is never told to `shedrecipe` at all, because at `wire()` time it does not exist yet and because every consumer of it lives behind the seams above.
  - `Env.Slug` is a **new** field on `shedrecipe.Env` — there is no `Slug` field there today (`internal/shedrecipe/recipe.go`). All three entries read it, for producer identity and stuck-reason text.
- **Rationale:** `Env.CommitDiscussion`, `Env.CommitPlan`, and `Env.ApprovePlan` set the closure convention for exactly this situation — work needing a resolved `*lyxcwd.Location`, which the Shed Recipe Registry Invariant bars `shedrecipe` from importing — and `Env.Landing` sets the whole-struct convention for a producer whose dependency set is genuinely plural.
  Picking per producer rather than forcing one shape is what keeps the seam boundary and the test doubles in agreement: `LoomRun`'s unit tests substitute `Spawn` and `ReadStatus` individually, `WorktreeTeardown`'s substitute `Shutdown` and `Remove` individually, and the integration test stubs those same fields — so there is no layer at which a producer's required logic is both inside the producer and inside a closure.
  The governing test is whether the producer has behaviour of its own that a caller must be able to observe: `WorktreeCreate` has none, and the other two have several.
  `Env.Slug` is a run-wide value, which is exactly the class `Env` carries; anything per-row is a `Config` key instead (which is where `LoomRun`'s poll knobs live — see `loom-run-poll-bound`).
- **Rejected:** One fat `RunLoom func(ctx) (string, error)` closure covering spawn *and* poll (it leaves `lifecycleshed.LoomRun` with no logic to test, contradicts the per-seam fakes the test plan needs, and hides the poll bound inside the CLI layer where no recipe key can reach it); a single `TeardownWorktree func(ctx) error` closure (the same defect, in the producer whose entire reason to exist is an ordering guarantee that a lone `error` cannot express); forcing `WorktreeCreate` into a struct for symmetry (a one-field struct that exists to look like its neighbours is not a seam, it is ceremony); having the registry entries import `fabricengine`/`reedengine` directly (possible — `shedrecipe` already pulls `fabricengine` transitively through `landingshed.Deps` — but it would make the entries resolve geometry, which the invariant forbids); hand-assembling the `ProducerDef`s in Go outside the registry (breaks the Shed Recipe Registry Invariant's rule that every recipe row's engine resolves through `Lookup`).

### loom-status-paths-derived-lazily-by-lifecyclecli

- **Decision:** `internal/lifecyclecli` derives loom's status and status-lock paths, and derives them **lazily** — inside the `LoomRunDeps.ResolveStatus` closure, evaluated on `LoomRun.Call`, never at `wire()` time.
  The same rule binds **every seam that touches the task worktree**, not just this one, and each chain is pinned end to end rather than sketched:
  - `LoomRunDeps.Spawn`'s `Dir` — the same `Location`'s `AnchorPath()`, per "Process spawning".
  - `TeardownDeps.Shutdown` — `lyxcwd.ResolveWorktree` for the `Location`, then `reedengine.LoadConfig(location.AnchorPath(), "reed")` for the config **and** `hubgeom.ReedGeometry(location)` for the geometry, then `reedengine.New(cfg, geom)`. `New` takes both, and every shipped caller loads the config exactly this way (`internal/reedcli/cli.go:89`); omitting the config step leaves the chain unbuildable.
  - `TeardownDeps.Remove` — the slug resolved against prime's own `Location`, which `lifecyclecli` already holds.
  All of them resolve inside their closure body, on `Call`, for the same reason.
  `ResolveStatus` is named explicitly only because it is the one whose laziness is visible in its own signature; do not read that as singling it out.
  The derivation chain is fixed: `fabricengine.WorktreePath(primeLocation, slug)` gives the task worktree root, `lyxcwd.ResolveWorktree(thatRoot)` gives its `*lyxcwd.Location`, and `loomengine.LoomStatusFile(l)` / `loomengine.LoomStatusLock(l)` give the two paths.
  Nothing about the task worktree is told to `shedrecipe` or to `lifecycleshed`; both receive only the closure.
- **Rationale:** Laziness is required, not stylistic — the same constraint `landingshed.Deps.OpenFabric`/`OpenParentFabric` already record for their own closures. At `wire()` time the task worktree does not exist (`Worktree-Create` has not run), so `lyxcwd.ResolveWorktree` would fail on a path that is about to become valid.
  Routing through `loomengine`'s accessors rather than joining path segments is what keeps `lifecycleshed` clear of both the Lyxdirs Single-Declarer Invariant (no production file outside `internal/lyxdirs` may name `_lyx` or `.lyx` in path-construction context) and the Told-Geometry Invariant (no direct `lyxcwd` import).
  `fabricengine.WorktreePath` is reused rather than reimplemented because fabric owns that derivation and already uses it internally in `remove.go`.
  `lyxcwd.ResolveWorktree(worktreeRoot)` is the existing entry point for resolving a `Location` from a known root rather than from cwd (`internal/lyxcwd/lyxcwd.go:87`), which is exactly this situation — the lifecycle driver's own cwd is prime, not the worktree being resolved.
- **Rejected:** Telling `shedrecipe` the three paths up front (they cannot be resolved before Create runs); having `lifecycleshed` join `_lyx`/`loom`/`status.json` itself (violates Lyxdirs Single-Declarer); having `lifecycleshed` take a `*lyxcwd.Location` (violates Told-Geometry, and the package would then resolve geometry it has no business resolving).

### registry-coverage-guard-spans-consumers

- **Decision:** The registry's closed-coverage claim — "no registered engine is unreachable" — moves out of `internal/loomrecipe` and becomes a single cross-consumer guard that unions every recipe consumer's engine set.
  Concretely, four changes land together:
  1. Each recipe package gains `RecipeEngines() []string`, returning the engine names its own embedded recipe's rows reference, derived by parsing that recipe through `shedbuild.Parse` — never a hand-maintained literal.
  2. A new guard in `internal/shedrecipe`'s **external** test package (`package shedrecipe_test`, which may import both consumers without an import cycle) unions `loomrecipe.RecipeEngines()` and `lifecyclerecipe.RecipeEngines()` and asserts every name in `shedrecipe.Names()` is in that union or on the allowlist. The `SingleLLM`/`Stub` allowlist entries and their written reasons move here verbatim.
  3. `internal/loomrecipe/coverage_guard_test.go` loses exactly its fourth half — the `shedrecipe.Names()` orphan assertion — and keeps its three loom-local directions (every built row is in `loomRowEngines`, every table key names a real row, every mapped engine resolves through `Lookup`). Its `coverageGuardAllowedUnreachableEngines` map is deleted, not emptied.
  4. `internal/lifecyclerecipe` gets its own three-direction guard of the same shape over its own three rows.
  A comment on `registry` in `internal/shedrecipe/registry.go` points at the new cross-consumer guard as the place a new key's coverage is checked, replacing its current "fourteen keys, any fifteenth needs a coverage-guard update" note.
- **Rationale:** `TestCoverageGuard_EveryLoomRowHasAnEngine`'s fourth half asserts that every entry in `shedrecipe.Names()` is either reached by one of loom's own rows or listed in `coverageGuardAllowedUnreachableEngines`.
  The three lifecycle engines are reached only by `lifecyclerecipe`, so that test fails the moment they are registered.
  The assertion is not wrong — it is stated in a package that structurally cannot see the answer, which is exactly what stops being true the first time the registry has two consumers.
  Adding the three keys to the allowlist instead would invert that allowlist's stated purpose: it names engines *no row anywhere* reaches, and recording three engines that a shipped recipe does reach as tolerated orphans would make the guard lie and make the next consumer's author copy the lie.
  Deriving each consumer's engine set from its own recipe rather than writing it down keeps the union honest without adding a second hand-maintained table alongside `loomRowEngines`.
  The row→engine direction stays per-consumer and hand-written, because `shedengine.ProducerDef` carries no engine name and only the row-name side is derivable from a built list.
- **Rejected:** Extending `coverageGuardAllowedUnreachableEngines` with the three new keys (weakens loom's guard and mislabels a legitimate second consumer as an orphan); giving `lifecyclerecipe` a full copy of the fourth half too (two consumers each asserting closed coverage over a shared registry means each fails on the other's engines — the same bug, doubled); putting the union guard in `cmd/lyx` where every module is already assembled (it is a claim about the registry, so it belongs beside the registry; `cmd/lyx`'s own guards are about cobra registration); dropping the closed-coverage claim entirely (it is the assertion that catches a registered engine no recipe ever wired).

### mutation-records-are-logged-not-enveloped

- **Decision:** The lifecycle envelope **deliberately does not carry** `mutations`/`partial`. The `CreateWorktree` and `TeardownDeps.Remove` closures return `error` only, and the `AddResult`/`RemoveResult` values — with their embedded `MutationRecord` — are consumed inside those CLI-side closures, which **log the record at `Info` through `internal/logger`** before returning.
  A rolled-back or partial `Add` therefore leaves the operator fabric's error text *plus* a logged mutation record, not error text alone.
- **Rationale:** The Mutation Record Invariant binds every mutating fabric verb's own result type and the envelope *those verbs* emit; `lyx fabric add`/`remove` still carry `mutations` and `partial` exactly as today, unchanged by this task.
  `lyx lifecycle run` is not a fabric verb — its envelope reports a Shed run's outcome, and a run may perform zero, one, or two fabric mutations at arbitrary points hours apart, so there is no coherent single `mutations` array for it to expose. A `partial` bool would be worse still: partial *relative to what* has no answer at run scope.
  Logging rather than discarding is the part that matters. The record's diagnostic value is real precisely in the rollback case, and `Info` keeps it where an operator debugging a failed create already looks — the driver log — without inventing an envelope shape the invariant never asked for.
  Stating this resolves a conditional the Constraints list otherwise leaves dangling ("anything surfacing them through the CLI envelope exposes…"): nothing here surfaces them, by decision.
- **Rejected:** Putting `mutations`/`partial` on the lifecycle envelope (no coherent scope, and `partial` is meaningless at run level); widening the seam signatures to return `AddResult`/`RemoveResult` so the producers could carry them (the producers have no envelope channel either — the same structural fact that forced `abandonedSession` CLI-side — so this buys nothing and drags fabric result types into `lifecycleshed`); discarding the records silently (throws away the one diagnostic that distinguishes a clean refusal from a half-applied, rolled-back create).

### lifecycle-cli-two-verbs

- **Decision:** A new `internal/lifecyclecli` cobra module with exactly two verbs: `lyx lifecycle run <slug>` (drives the lifecycle Shed in the foreground) and `lyx lifecycle status <slug>` (prints the lifecycle status file).
  Package naming follows the shipped `landingshed`/`preflightshed`/`loomshed` convention: `lifecycleshed` for the producers, `lifecyclerecipe` for the builder, `lifecyclecli` for the verbs.
- **Rationale:** The `run`/`drive` split exists in loom only because `run` hands the terminal to tmux and `drive` does not; at hub level there is no reed session and no handover, so one foreground verb covers both roles.
  `step` and `pause` belong to the nested loom run, which keeps its own.
  A separate module rather than verbs under `lyx board` or `lyx fabric` keeps the CLI/Cobra Invariant's one-module-one-subtree shape and avoids giving `fabric` — whose destruction chokepoint is deliberately narrow — an orchestrating verb.
- **Rejected:** Verbs under `lyx board` (board owns the task board, not task execution); verbs under `lyx fabric` (fabric is git/worktree mechanics and must not orchestrate); a full `run`/`drive`/`step`/`pause` quartet mirroring loom (three of the four have no meaning here).

### lifecycle-run-resumes-and-refuses-concurrency

- **Decision:** `lyx lifecycle run <slug>`'s dispositions are stated **per `shedengine.State` value**, never via the word "terminal" — that word means different things in `loom-run-poll-bound` (where `blocked` and `paused` are terminal *for the polled loom run*) and here, and the ambiguity hid the dominant recovery path:
  - **`running`, `blocked`, `failed`, `paused`** → resume silently from the persisted `current_producer`. No re-seed, no prompt, no flag.
    `blocked` is not an edge case here, it is the everyday path: every `Stuck` in `refusal-to-outcome-mapping` lands there, and that decision's rationale promises "re-running the verb resumes from the same row". `shedengine.Run` already resumes from `StateBlocked`/`StateFailed`, so this is the CLI matching engine behaviour rather than adding any.
  - **`done`** → refuse on the envelope, naming the slug as already completed and naming `<prime anchor>/.lyx/lifecycle/<slug>/` as the directory to delete to run the slug again. It does not silently re-run: the per-slug state lives on prime and therefore survives the teardown that removed the worktree, so a completed slug's status file is the normal steady state, not a leftover.
  - **No status file at all** → a fresh start, which is the first-run path.
  - **`run.lock` is already held** → refuse on the envelope, naming the lock path. Never wait, never spawn a second driver.
- **Rationale:** `lyx lifecycle run` *is* the driver, running in the foreground — there is no detached child and therefore none of the bootstrap handshake loom needs, so loom's `mustSpawnDriver`/`awaitRunLock` machinery has no counterpart here and must not be copied.
  `shedengine.Shed` already takes `LockPath` non-blocking for the whole of one `Run`, so refusing is its native behaviour; the verb's only job is to report it legibly instead of surfacing a raw lock error.
  Refusing rather than waiting is the right default for a foreground verb an operator is watching: a silent wait is indistinguishable from a hang, and the second invocation is nearly always a mistake rather than a queue.
  The `done` refusal exists because the alternative — re-running a completed slug from a `done` status — would drive `Worktree-Create` against a slug whose branch still exists, producing fabric's pre-existing-branch refusal as a confusing second-order failure instead of a clear first-order one.
- **Rejected:** Waiting on a held `run.lock` (a foreground verb that hangs silently is worse than one that refuses); refusing on any pre-existing status file (kills resume, which the whole crash-recovery story depends on); refusing on `blocked` specifically (it is the state every escalation produces, so refusing there would make the escalation unrecoverable by the verb that created it); silently re-running a `done` slug (surfaces as fabric's branch-exists refusal two producers later); a `--force`/`--restart` flag (nothing in scope needs it, and deleting the per-slug directory is already the explicit, obvious gesture).

### cross-slug-concurrency-takes-a-prime-wide-lock

- **Decision:** A second, **prime-wide** advisory lock at `<prime anchor>/.lyx/lifecycle/run.lock` — one level above the per-slug directory — is taken and released by **the two bookend producers themselves**, each at the top of its own `Call` and released before that `Call` returns. Not by `lyx lifecycle run`.
  It reaches them as an injected seam, `Env.PrimeLock`, whose `Acquire func() (release func() error, ok bool, err error)` is filled by `lifecyclecli` (the layer that may name the path) and read by the `WorktreeCreate` and `WorktreeTeardown` entries only.
  A producer that cannot take it returns **`Stuck`**, with the reason naming **the lock path** — not the holding slug, and not a CLI envelope refusal.
  Two earlier drafts of this decision were wrong here and both corrections matter: it first said "refuses on the envelope", which stopped being true once the lock moved into the producers; it then said the reason names the holding slug, which is unbuildable — `lock.TryAcquireWriteLock` reports contention as a bare `(nil, false, nil)` (`internal/lock/lock.go:29-41`), the lock file carries no holder record, and the `Acquire` seam has no identity to return.
  Naming the path alone is what the per-slug refusal already does, so the two refusals read alike.
  `Stuck` is the right verdict regardless: it is resumable, it persists the reason, and it is exactly what `refusal-to-outcome-mapping` prescribes for every other operator-fixable refusal.
  The per-slug `run.lock` stays exactly as it is, and the two locks answer different questions: the per-slug one means "this slug is already being driven", the prime-wide one means "someone else is mutating prime right now".
- **Rationale:** The per-slug lock permits two runs for *different* slugs to drive `Topology.Add`/`Remove` against the same prime repository at once, and neither verb takes a lock of its own — `Add` (`internal/fabricengine/add.go:45-60`) reads prime's working tree for its dirty probe, creates worktrees, and mutates branches with no mutual exclusion.
  Two concurrent `Add`s therefore race on prime's index and on each other's dirty probe, and the failure is silent and intermittent rather than loud.
  Scoping the lock to the two bookend rows rather than the whole run is what keeps concurrency useful: a lifecycle run is dominated by `Loom-Run`, which spans hours and touches prime not at all, so a run-wide prime lock would serialize entire tasks to protect two short critical sections.
  **Putting the acquire/release inside the producers is what makes that scope expressible at all.** `lyx lifecycle run` drives one opaque `shedengine.Shed.Run` call and has no visibility into which row is executing, so a CLI-side lock could only be run-wide — the thing this decision rejects. Producer-side ownership also solves three problems at once for free: the bound is structural rather than a convention someone must maintain; a resume that re-enters directly at `Worktree-Teardown` re-takes the lock when that row runs, with no special-casing of the resume path; and the whole thing is unit-testable at the producer layer over a fake `Acquire`.
  Driving the lifecycle row-by-row through `shedengine.Shed.Step` instead would also make the scope expressible, but at a real cost — `Step` re-takes the run lock per call and the CLI would have to re-implement `Run`'s own routing and terminal handling — to buy nothing the producer-side seam does not already give.
  Returning `Stuck` rather than waiting matches the per-slug lock's reasoning: both bookends are seconds long, so refusing is an accurate "try again shortly" rather than a wasted queue.
- **Rejected:** Leaving it undecided (the race is real, and "tolerated" is a decision that has to be made deliberately rather than by omission); a prime-wide lock held for the whole run (serializes hours of independent LLM work to protect seconds of git); acquiring it in `lyx lifecycle run` around the `Shed.Run` call (the CLI cannot see row boundaries, so this collapses into the run-wide lock above); driving row-by-row via `Shed.Step` to regain those boundaries (re-takes the run lock per step and forces the CLI to re-implement `Run`'s routing, for no gain over the producer-side seam); a sidecar holder file recording the holding slug so the reason could name it (it buys one word of diagnostics and brings its own staleness problem — a crashed holder leaves a name nothing clears, so the reason would confidently blame a slug that is no longer running); refusing any second concurrent slug outright (the common, intended case is several tasks running at once, which is the entire point of a hub); relying on git's own index lock (it protects a single git invocation, not the multi-step create/remove sequences, and its failure surfaces as an opaque git error rather than a lifecycle refusal); adding the lock inside `fabricengine` (it would change `lyx fabric add`/`remove` for every existing caller, which is outside this task's scope and not this task's call to make).

### recipe-is-embedded-not-on-disk

- **Decision:** `contracts/recipes/lifecycle-recipe.yaml` is embedded into the binary alongside `loom-recipe.yaml`, and `lifecyclerecipe.New` uses `shedbuild.Parse` on the embedded bytes, never `shedbuild.Load`. It never calls `shedbuild.Check`.
- **Rationale:** Verbatim the reasoning `loomrecipe.New` already records: there is no on-disk runtime location for this recipe, and `Check` is authoring-time only because a resumed run legitimately starts mid-graph, making reachability-from-entry the wrong production question.
- **Rejected:** An on-disk recipe read at runtime (no location to read it from, and a mutable recipe for a destructive pipeline is a hazard).

## Technical context

**The Shed seam.** `shedengine.ShedProducer` is a one-method interface — `Call(ctx) (Outcome, OutputPointer, error)` — with two obligations the engine cannot enforce: return exactly `Done` or `Stuck`, and surface context cancellation as a non-nil error, never as `Stuck`.
Every existing producer package discharges the second obligation with a local `entryErr`/`cancelErr` pair; `internal/preflightshed/ctx.go` and `internal/loomshed/ctx.go` are the two shipped copies, and both document the duplication as deliberate.
`lifecycleshed` follows the same pattern with its own copy.

**Routing.** `shedengine.ProducerDef` carries `Name`, `Producer`, `OnStuck`, `OnDone`, `Segment`, `MaxBounces`. There is no positional fallback: an empty `OnDone` ends the whole run quietly, an empty `OnStuck` escalates to a human (`state: "blocked"`). A non-empty `OnStuck` must name a producer sharing the same `Segment`, which the validator enforces — the lifecycle recipe uses no segments at all, since it has no review pair.

**The registry.** `internal/shedrecipe/registry.go` holds one `map[string]Constructor` literal, reached only through `Lookup`/`Names`, with no `init()` self-registration and no runtime `Register`. Its own comment pins the table at fourteen keys and states that any fifteenth entry must arrive with a coverage-guard update in the same commit.
Entry constructors live in `entries_*.go` files grouped by shape; the three new ones are closest to `entries_simple.go`'s shape (validate a couple of `Env` fields, call a constructor) and should get their own `entries_lifecycle.go`.

**The second registry-wide test.** `internal/shedbuild/build_engines_test.go`'s `TestBuild_EveryRegisteredEngineBuilds` drives its assertion off `shedrecipe.Names()` rather than a local list, and `fixture_test.go`'s `newTestEnv` doc comment states the consequence outright: a new registry entry with a new required `Env` seam fails that test until the fixture covers it.
The three new entries require **five** seams — `Slug`, `CreateWorktree`, `LoomRun`, `Teardown`, and `PrimeLock` (the last read by the two bookend entries, added when the prime-wide lock moved into the producers) — none of which `newTestEnv` fills, so this test fails in the same commit as the coverage guard and for the same reason.
Fill `PrimeLock` with a fake `Acquire` returning a no-op release and `ok == true`.
Fill the four the way that fixture already fills `DiscussionSpec`/`CommitDiscussion`/`PlanSpec`/`CommitPlan` and `Landing`: closures returning nil and structs whose own closures return nil, with every path derived from the single `t.TempDir()` root — the package's guard exists to catch a told-geometry violation, so no fixture value may reference a path outside that root.
`engineMinimalConfig` also needs rows for the two new engines carrying `Config` keys (`Loom-Run`'s `poll_interval_s`/`poll_attempts`), unless their defaults make an empty `Config` legal, which is the simpler outcome and the one to prefer.

**The coverage guard, and why it has to be restructured.** `internal/loomrecipe/coverage_guard_test.go`'s `TestCoverageGuard_EveryLoomRowHasAnEngine` asserts four things, and the fourth is the one this task collides with: for every name in `shedrecipe.Names()`, that name must be reached by one of loom's own seventeen rows (via the hand-written `loomRowEngines` map) or be listed in `coverageGuardAllowedUnreachableEngines`, which today holds `SingleLLM` and `Stub` with written reasons.
The three lifecycle engines satisfy neither, so this test fails as soon as they are registered. That is a design gap rather than a mechanical follow-on — see the `registry-coverage-guard-spans-consumers` decision for the resolution and for what exactly changes in this file.
The test's own comment already records that the fourth half was an addition rather than a weakening, and that the registry is "generic Shed machinery shared by reference with a future product's producer list rather than loom's private property" — this task is that future product arriving.

**Env validation helpers.** `internal/shedrecipe/env.go` provides `requireAbsRoot(entry, field, value)` and `requireSeam(entry, field, seam)`; the latter detects typed-nil interface and func values via reflection. An entry validates exactly the fields it reads and never a field it does not, so a caller filling only what its own recipe needs is legal — which is what lets the lifecycle recipe leave loom's dozens of `Env` fields empty.
`Env.Slug` is a plain string, not a path, so it needs a new non-`requireAbsRoot` check (non-empty) rather than reusing the absolute-path helper.

**The recipe builder.** `internal/loomrecipe/loomrecipe.go` is the template to copy: a `ShedPaths` struct carrying the five values `shedengine.Shed` reads and no registry entry reads (`StatusPath`, `LockPath`, `StatusLockPath`, `MaxBounces`, `CommitStatus`), plus a `New(env, paths)` that parses, builds, and assembles.
It also performs one coherence check across its two arguments — `env.StatusPath != paths.StatusPath` and the same for `StatusLockPath` — because loom's `Env` and `ShedPaths` deliberately carry duplicate copies read by different consumers.
The lifecycle recipe has **no** such duplication: no lifecycle registry entry reads `StatusPath` or `StatusLockPath`, so `lifecyclerecipe.New` needs no such check and must not grow one for symmetry's sake.

**Path derivation.** `loomengine/config.go` splits loom's two anchors deliberately, and the split matters here because `LoomRun` reads both: `LoomStatusFile(l)` is `filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "loom", "status.json")` — that is `_lyx/loom/status.json`, **durable and tracked** — while `LoomStatusLock(l)` is under `lyxdirs.DotLyxDirName`, `.lyx/loom/status.json.lock`, because the lock is a never-tracked transient.
`LoomRunLock` is likewise `.lyx`. Do not poll `.lyx/loom/status.json`; it does not exist.
Both are a module's own constant joined onto `AnchorPath()`, never a `lyxcwd` call, per the Cwd Resolution Invariant.
The lifecycle's own status is under `.lyx` rather than `_lyx` — the opposite of loom's status — and that asymmetry is deliberate on both sides: loom's status is committed onto the task branch as task history, and the lifecycle's is per-machine and never committed (see `lifecycle-status-is-ephemeral`). `lifecyclecli` derives it against prime's `Location` with a per-slug segment: `<prime anchor>/.lyx/lifecycle/<slug>/`.

**Fabric APIs.**
`Topology.Add(l *lyxcwd.Location, slug string, opts AddOptions) (AddResult, error)` — `AddOptions` is an alias of `SyncOptions` (`SkipGit`/`SkipPush`); the zero value is the ordinary path. `Add` validates the slug, creates both worktrees, wires junctions, records the pair's parent-branch provenance, and pushes, rolling everything back on any failure.
`Topology.Remove(l *lyxcwd.Location, slug string, force bool) (RemoveResult, error)` — removes the pair plus every warp junction, portal junction, and launcher; refuses a dirty worktree on either side unless forced; refuses hub geometry, prime, reserved entries, and weft-suffixed names.
Both embed `MutationRecord` and accumulate a `*Mutations` record; the Mutation Record Invariant requires every mutating result type to expose it under the fixed envelope key set, which matters if these results surface through the CLI envelope.

**Reed APIs.**
`reedengine.New(cfg Config, geom Geometry) *Engine`, with `hubgeom.ReedGeometry(l)` as the hub-mode teller. `Engine.Down() (DownResult, error)` takes the op lock, captures the server pid and pane process subtrees before `kill-session`, deletes the state file, and is the one lyx-only escape from the foreign-session refusal because it loads no state. It is idempotent.
Note its documented behaviour after a worktree rename: `DownResult.AbandonedSession` names a session it deliberately does not kill, reported in the result and at `Warn` (`lifecycle.go:53-58`, `:902`).
Teardown must not swallow it, and the carrier is split because no single channel can do both halves.
`ShedProducer.Call` returns only `(Outcome, OutputPointer{Path}, error)` and `shedengine.Result` carries only `Outcome`/`HaltedProducer`/`Reason`/`History`, so a producer has **no** channel for a string on an otherwise-`Done` row. It cannot ride the stuck reason either, because an abandoned session does not make teardown fail: `Down` succeeded, `Remove` proceeds, and the row returns `Done`.
So: the **producer** logs it at `Warn` through `internal/logger` — that is what `Shutdown` returning the value is for — and the **CLI-side `Shutdown` closure records it** into a variable `lifecyclecli` owns, which the verb reads after `Shed.Run` returns and puts on the envelope under the key `abandonedSession`, the same spelling `internal/reedcli/up.go:96-97` uses.
This corrects an earlier sentence claiming "the producer, not the CLI closure, owns putting it on the result": the producer owns *observing and logging* it, and the CLI owns the envelope, because the envelope is the CLI's and nothing reaches it from inside a producer.

**Loom bootstrap.** `internal/loomcli/run.go`'s `runCmd` performs four steps: resolve parent and seed+commit the status file, ensure the reed substrate and the `loom-status` strand, spawn the detached `lyx loom drive` child unless the run lock is already held, then hand the terminal over via `tmux attach`.
The handshake between steps 3 and 4 (`awaitRunLock`, `bootstrap.go`) polls at most 300 times at 100 ms for the driver to take the run lock, with a four-way outcome: ready, child-died, halted, deadline.
`--no-attach` returns after that handshake instead of executing step 4 — the terminal handover is already the CLI/Cobra Invariant's narrow interactive-handoff exception, so skipping it removes an exception rather than adding one.

**Loom status shape.** `shedengine.Status` carries `current_producer`/`state`/`error`/`pause_requested`/`activity`/`history`; loom's own three fields (`slug`, `parent`, `start_sha`) live inside the opaque `Product` passthrough.
The persisted `state` field takes **five** values — `running`, `paused`, `done`, `blocked`, `failed` (`internal/shedengine/status.go:15-21`) — not three. The three-clean-exit-values claim is true of `RunOutcome`, the in-memory result type, and reading it as a claim about `State` is the mistake that produced the incomplete verdict table; `LoomRun` reads the persisted `State`, so it must handle all five.
Read it through `state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)`, taking loom's own status lock, per the external-writer lock contract in `shedengine/doc.go`.

**Process spawning.** `LoomRunDeps.Spawn` runs the child **in the foreground and waits for it** — it is emphatically not a detached spawn, so `internal/proc.Detach` is the wrong model and must not be copied from the two shipped detached sites (`internal/loomcli/run.go`, `internal/boardengine/spawn.go`). The child is `lyx loom run --no-attach`, which itself returns once its own handshake confirms the detached loom driver took the run lock; waiting on it is what makes `LoomRun`'s subsequent poll meaningful.
The `lyx` binary is resolved via `os.Executable()`, the in-tree precedent (`internal/loomcli/run.go:124`, `internal/loomcli/sharedbootstrap.go:206`).
The child's `Dir` is the task worktree's resolved **`Location.AnchorPath()`**, not its worktree root — the two are the same only in a root-anchored hub. The child runs `lyx loom run`, which resolves through `lyxcwd.Resolve`, and that gates cwd to equal `Join(worktreeRoot, AnchorRel)`; a bare worktree root therefore fails with `ErrCwdOutsideAnchor` on any subpath-anchored hub. `hubgeom.ReedGeometry` fills `PaneCwd` with `AnchorPath()` for precisely this reason.
`Dir` is derived inside the same lazy closure as the status paths, from the same resolved `Location`, so the two cannot drift apart.
`os.Executable()` re-exec carries one hard rule: never under `go test`. `internal/gitkit/reexecguard.go` exists as the backstop for exactly this class of spawn point, so the unit tests substitute `Spawn` rather than ever reaching a real re-exec, and the integration test stubs it too.
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
- **CLI / Cobra Invariant** — `lifecyclecli` exposes `Command() *cobra.Command`, `RunCLI(out, args) int`, **and `RunCLIIn(cwd, out, args) int`**, carries non-empty `Short` on every command, routes errors as JSON through `internal/output`, and checks `clihelp.ShouldAbort` first in every `RunE`. Package naming: `lifecyclecli` imports `lifecycleshed`/`lifecyclerecipe`; neither imports cobra. The help-tree tests move with it.
  `RunCLIIn` is carried rather than skipped for a concrete reason, not for conformity: the path-derivation tests that serve as the Lifecycle Bookend Invariant's mechanical proxy need an injectable cwd, and so does testing the non-prime-worktree refusal from `lifecycle-driver-runs-from-prime` — neither is reachable through `RunCLI` alone.
  The invariant's own count line says "eleven of twelve also carry `RunCLIIn`" (eleven confirmed in-tree); it becomes **twelve of thirteen**, amended in the same commit.
- **Fabric Destruction Chokepoint Invariant** — teardown routes through `Topology.Remove`, whose gates run in order (containment, ownership, dirtiness, force) and whose refusals are never silently discarded. `--force` answers dirtiness only, and this task never passes it.
- **Fabric Git Invariant** — every git op goes through `internal/fabricengine` in-process; the new producers add no raw git and no agent-driven git.
- **Fabric Vocabulary Invariant** — the three new packages are outside the owner set; no identifier, literal, or comment in them may name either side of the pair.
- **Mutation Record Invariant** — `AddResult`/`RemoveResult` carry mutation records; anything surfacing them through the CLI envelope exposes `mutations` (always an array) and `partial` (always a bool), and a pre-flight failure emits a bare `output.Err` with neither key. The lifecycle envelope surfaces neither, by the `mutation-records-are-logged-not-enveloped` decision, so this invariant's conditional does not bind it; `lyx fabric add`/`remove` are unchanged.
- **Live-Substrate Spawn Observability** — `LoomRun` spawns a real OS process and waits for it, so it logs the spawn at `Info` and the teardown at the wait site. Any retry loop caps attempt count, not only elapsed time.
- **Test Tier Purity Invariant** — no `gitexec.Run`/`RunGit`, `exec.Command`, `gitkit.Copy*`, or `hubforge.NewHub` outside `integration`/`smoke`-tagged files; no `time.Sleep` ≥ 1s in an untagged file.
- **Hermetic Git Test Environment Invariant** — every new test package whose tests spawn git calls `gitkit.HermeticGitEnv()` in `TestMain` before `m.Run()`.
- **hubforge Fabric-Fixture Invariant** — the integration test's hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`, never hand-assembled.
- **Sandbox Suite Coverage** — `lifecycle` is **covered, not excluded**: a new scenario in `tools/sandbox/SANDBOX-FABRIC-SUITE.md` carrying a `**Covers:** lifecycle` tag, with the matching runner step in `sandbox/posix/fabric-suite.sh` and `sandbox/win`'s counterpart.
  The fabric suite is the right home because the scenario's observable effects are worktree-pair and refusal behaviour, which that suite already creates and asserts on in a disposable hub.
  **The scenario deliberately never completes a `Loom-Run` row**, and an earlier draft of this bullet was wrong to claim it could: `Worktree-Create` makes a brand-new pair with no loom status, `Loom-Run` then spawns `lyx loom run --no-attach`, and `mustSpawnDriver(runLockHeld)` is `!runLockHeld` (`internal/loomcli/bootstrap.go:30-32`), so a fresh pair always spawns a real driver into loom's LLM rows. There is no `step` or `pause` verb on the lifecycle CLI to interpose a fixture mid-run, by the `lifecycle-cli-two-verbs` decision.
  What the scenario covers instead is everything observable without an LLM, which is the whole module for a module-level guard: `lyx lifecycle status <slug>` round-tripping a **hand-written lifecycle status fixture**; `lyx lifecycle run` refusing on a `done` status; `lyx lifecycle run` refusing on a held `run.lock`; `lyx lifecycle run` refusing from a non-prime worktree; and `Worktree-Create` halting `blocked` against a deliberately dirty prime.
  The hand-written-fixture approach is not a workaround invented here — `tools/sandbox/SANDBOX-CORE-SUITE.md`'s S8 scenario already hand-writes `_lyx/loom/status.json` for exactly this reason, and records it in a **Fixture note**; this scenario carries the same note naming the same cause.
  The full driven path is covered by the `integration`-tagged end-to-end instead, where `LoomRunDeps`' fields can be stubbed. Say so in the scenario, so a sandbox operator does not read the gap as an oversight.
  Exclusion was considered and rejected: neither of the two reasons on the existing allowlist applies — nothing here opens an interactive window or files a real GitHub issue, and the module is not an alias of another module's verb. `cmd/lyx/sandbox_coverage_test.go` fails on a registered-but-untagged module, so this is enforced rather than aspirational.
- **Markdown Link Integrity** — every inline link in the rewritten `manifest/designs/worktree-lifecycle-shed-producers.md` and in the `docs/overview.md` edits must resolve, file part and anchor.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle`.

**New invariant to record in `CONSTRAINTS.md`, same commit:**

> **Lifecycle Bookend Invariant** — a producer that creates or destroys a task worktree never runs from inside that worktree.
> The lifecycle Shed is driven from the hub's prime worktree; its status file and locks live under prime's own ephemeral tree, never under the worktree being managed.
> A teardown row sequences session shutdown before worktree removal, in one producer, never two rows.
> **Enforcement: review discipline, with two partial mechanical proxies.** There is no enforcing test, and that is a decision rather than an omission: the invariant constrains which directory a running process is driven from, which has no static shape an AST scan can see.
> The two proxies that do exist are named so a reviewer knows what is already covered and what is not: `internal/lifecycleshed`'s seam-enforcement scan bars a direct `internal/lyxcwd` import, so the package cannot resolve its way into the managed worktree; and `internal/lifecyclecli`'s path-derivation tests pin the status/lock paths to prime's anchor, so a relocation under the managed worktree fails there.
> Neither proves the driver's own cwd, which stays a review obligation.

## Testing

`internal/lifecycleshed` — Tier 1, untagged, table-driven over injected seams. Each of the three producers gets: the happy path returning `Done`; a seam error mapped per the `refusal-to-outcome-mapping` table; and `entryErr`/`cancelErr` coverage proving a cancelled context surfaces as a non-nil error and never as `Stuck` — the obligation Shed cannot enforce, and a TDD candidate.
`WorktreeCreate` needs both named refusals covered as `Stuck` with fabric's text passed through unreworded, but the two assertions differ because the two errors do: the **branch-exists** error names its own remedies (`internal/fabricengine/add.go:73-76`), so assert that remedy wording survives; the **dirty-prime** error is the bare string `"source worktree has uncommitted changes"` (`add.go:59`) with no remedy in it at all, so assert verbatim pass-through and nothing more. An assertion demanding remedy wording from both would be unsatisfiable against the second.
`WorktreeTeardown` needs ordering coverage over its two separate `TeardownDeps` fields, with a fake recording the call sequence: `Shutdown` strictly before `Remove` on the happy path, **and `Remove` never called at all** when `Shutdown` fails. A `Remove` refusal must still report the shutdown that already happened, and the stuck reason must say which of the two failed — a test that only asserts `Stuck` would pass on a producer that conflated them. `ErrMergeInProgress` gets its own case, so the mapping is not silently dirtiness-only. A `Shutdown` returning a non-empty abandoned-session string on an otherwise-`Done` row is its own case.
`LoomRun` needs the full `loom-run-poll-bound` verdict table over a fake `ReadStatus` and a fake clock, covering **all five** `shedengine.State` values rather than three: `done` → `Done`; `blocked`, `paused`, and `failed` → `Stuck` with loom's `error` and `current_producer` in the reason; `running` continues polling; status absent after a successful `Spawn` → `Stuck`; decode failure → hard error, asserted as an error rather than a verdict; `Spawn` failure → `Stuck`.
The `failed` case needs asserting explicitly that it does **not** consume poll attempts — treating it as a polling state is the specific regression that would stall a lifecycle run for the full attempt budget.
The attempt cap is a TDD candidate and needs a test that never sleeps: a fake clock plus a `ReadStatus` that always answers `running` must exhaust `poll_attempts` and return `Stuck` in unmeasurable wall-clock, which is also what keeps it inside Test Tier Purity's no-`time.Sleep`-≥-1s rule.
Assert the cap fires on attempt count, not elapsed time, by holding the clock still — the Live-Substrate invariant's requirement is otherwise untestable and would silently regress to a deadline.

`internal/shedrecipe` — extend the existing entry tests: each new entry validates exactly the `Env` fields it reads and rejects nil closures (including typed-nil, which `requireSeam` handles), rejects an empty `Slug`, and rejects unknown `Config` keys via `configRejectUnknown`. The registry `Names()` test moves from fourteen to seventeen keys.
A new external-test-package guard (`package shedrecipe_test`) owns the cross-consumer closed-coverage assertion described in the `registry-coverage-guard-spans-consumers` decision: it unions `loomrecipe.RecipeEngines()` and `lifecyclerecipe.RecipeEngines()` and asserts no `shedrecipe.Names()` entry falls outside that union plus the relocated `SingleLLM`/`Stub` allowlist.
It needs its own drift direction too — an allowlist entry naming an engine that is no longer registered, or that some consumer now does reach, must fail rather than linger.

`internal/loomrecipe` — `coverage_guard_test.go` keeps its three loom-local directions and loses only the fourth half plus the now-dead `coverageGuardAllowedUnreachableEngines` map. Prove the remaining three still fail on their own triggers (a row absent from `loomRowEngines`, a table key naming no row, a table engine that does not resolve) rather than assuming a deletion left them intact.

`internal/lifecyclerecipe` — parse and build the embedded recipe against a minimal fake `Env`, asserting the three row names, the `Worktree-Create → Loom-Run → Worktree-Teardown` `on_done` chain, `Loom-Run`'s empty `on_stuck`, `Worktree-Teardown`'s empty `on_done`, and that no row declares a `Segment`. A row-names-match-the-Go-constants test mirrors loom's own durable-identity guard, since these names are on-disk identities that resume depends on.
Its own three-direction coverage guard mirrors loom's surviving three. `RecipeEngines()` gets a test in each recipe package asserting it reports exactly its own recipe's engine set — it is the input the cross-consumer guard trusts, so a silently empty return would disable that guard rather than fail it.

`internal/lifecyclecli` — Tier 1 CLI tests in the shape the existing `*cli` packages use: `Short` non-empty on every command, arg arity, `clihelp.ShouldAbort` honoured, JSON error envelopes, and the help-tree test. Path-derivation tests assert the `<prime anchor>/.lyx/lifecycle/<slug>/` layout and that the three paths are distinct (`LockPath != StatusLockPath` is enforced by `Shed.validate()` and must not first fail at runtime); they double as the Lifecycle Bookend Invariant's mechanical proxy, so they must assert the paths are anchored to prime and not to the managed worktree.
The `lifecycle-run-resumes-and-refuses-concurrency` dispositions are CLI contract and each needs a case, enumerated per `shedengine.State` rather than by the word "terminal": `running`, `blocked`, `failed`, and `paused` each resume from `current_producer` — `blocked` especially, since it is the state every escalation produces; `done` refuses on the envelope naming the per-slug directory; an absent status file starts fresh; a held per-slug `run.lock` refuses on the envelope naming the lock path, without waiting.
The prime-wide lock from `cross-slug-concurrency-takes-a-prime-wide-lock` is tested at the **producer** layer, not the CLI layer, since that is where it is acquired: over a fake `PrimeLock.Acquire`, assert that each bookend producer takes it, that it is released before `Call` returns on **every** path including the `Stuck` ones, and that an unavailable lock yields `Stuck` with the **lock path** in the reason — not a holder identity, which the lock cannot report.
The release-on-every-path case is the one that would otherwise rot silently: a leaked lock is invisible until some other slug blocks hours later.
The non-prime-worktree refusal from `lifecycle-driver-runs-from-prime` needs its own case on **both** verbs, driven through `RunCLIIn` with an injected cwd — it is the runtime check standing in for the Bookend invariant's missing enforcing test, so it is a TDD candidate rather than an afterthought.
It is **not** a Tier-1 case: `fabricengine.PrimeName` reaches `List` → `gitexec.Run` (`internal/fabricengine/worktreelist.go:29,147-148`), and getting there at all needs `lyxcwd.Resolve`, which runs `git rev-parse --show-toplevel` — both barred from untagged files by Test Tier Purity. It lives in an `integration`-tagged file over a `hubforge` hub, where a real prime and a real task worktree both exist.
The refusal's *decision* is still unit-testable and should be split out as such: a tiny pure helper taking the resolved worktree name and the prime name and returning the refusal (or nil) gets Tier-1 table coverage, including the `PrimeName`-error case below, leaving only the git-backed resolution to the integration tier.
The lazy seams need a test proving none of them is evaluated at `wire()` time — building the wiring against a slug whose worktree does not exist must succeed. Cover `ResolveStatus` and both `TeardownDeps` fields, not just the first: eager evaluation is exactly the failure laziness exists to avoid, and a test covering only the named example would let the other two regress silently.
`abandonedSession` needs two cases, one per half of its split carrier: the producer logging it at `Warn` when `Shutdown` returns a non-empty value, and the CLI closure's recorded variable reaching the result envelope under that key on a `Done` teardown. Nothing else would catch either half being silently dropped.

`internal/loomcli` — `--no-attach` needs a test proving the verb performs every step through the handshake and then returns without attaching, and that the flag's absence leaves today's behaviour byte-identical. The existing `parity_test.go`/`cli_test.go` are the right homes.

Integration, `integration`-tagged — one end-to-end over a real `hubforge` hub, stubbing at the `LoomRunDeps` field level (a no-op `Spawn` and a `ReadStatus` answering `done`) rather than replacing the producer, so the real `LoomRun` logic is exercised: assert the pair exists after `Worktree-Create` and is gone after `Worktree-Teardown`.
A second case answers `blocked` from `ReadStatus` and asserts the run halts `blocked` with the worktree still present — the safety property the whole design turns on.
A third leaves prime dirty and asserts `Worktree-Create` halts `blocked` before creating anything, since that refusal fires on every lifecycle run and is invisible to the unit tests' fakes.
`TestMain` calls `gitkit.HermeticGitEnv()`.

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
- **Q:** Round 1 review, BLOCKING: adding three registry engines fails `internal/loomrecipe/coverage_guard_test.go`'s fourth half, and the discussion described that as a key-count bump rather than deciding how a shared registry's coverage guard composes across two consumers. **A:** [auto-resolved] Move the closed-coverage claim into a cross-consumer guard in `shedrecipe`'s external test package, unioning each consumer's recipe-derived `RecipeEngines()`; loom's guard keeps its three local directions and drops the fourth half plus its now-dead allowlist, which relocates verbatim. **Why:** the assertion was correct but stated where it structurally cannot be answered once the registry has two consumers; allowlisting the three new keys would record a shipped recipe's engines as tolerated orphans and invert the allowlist's purpose.
- **Q:** Round 2, BLOCKING: the `Env.RunLoom` closure signature implied the CLI layer owns spawn+poll, while `loom-run-drives-via-no-attach` put both inside the producer and Testing demanded per-seam fakes. **A:** [auto-resolved] The seam shape is now chosen per producer: single closures for Create/Teardown, a whole-struct `LoomRunDeps` passthrough (`Spawn`, `ResolveStatus`, `ReadStatus`, `Now`) for `LoomRun`, following `Env.Landing`'s precedent. `Env.TaskWorktreeRoot` is deleted — it was validated by entries and read by nobody. **Why:** forcing one uniform seam shape onto three differently-shaped producers is what put the spawn-and-poll logic in two layers at once.
- **Q:** Round 2, BLOCKING: nobody was assigned loom's status path in the task worktree, which cannot be told up front (the worktree does not exist at `wire()` time) and cannot be joined by `lifecycleshed` (Lyxdirs Single-Declarer) or resolved by it (Told-Geometry). **A:** [auto-resolved] `lifecyclecli` derives it lazily inside `ResolveStatus`, evaluated at `Call` time, via `fabricengine.WorktreePath` → `lyxcwd.ResolveWorktree` → `loomengine.LoomStatusFile`/`LoomStatusLock`. **Why:** laziness is required for the same reason `landingshed.Deps.OpenFabric` records, and routing through loomengine's accessors clears both invariants at once.
- **Q:** Round 2, BLOCKING: "polls until terminal" named no interval, no cap, and no never-terminal outcome, while Live-Substrate Spawn Observability requires an attempt-COUNT cap. **A:** [auto-resolved] `poll_interval_s` (default 5) and `poll_attempts` (default 8640 ≈ 12h) as `Loom-Run` recipe config keys, plus an exhaustive verdict table covering cap exhaustion, absent status, decode failure, and spawn failure. **Why:** a real loom run spans hours, so the budget is a per-deployment design parameter, not an implementation detail.
- **Q:** Round 2, BLOCKING: `lyx lifecycle run` had no stated behaviour for a pre-existing status file or a held `run.lock`, yet the resume test assumed mid-list restart. **A:** [auto-resolved] Non-terminal status resumes silently; `done` status refuses on the envelope naming the per-slug directory to delete; a held `run.lock` refuses on the envelope, never waits. **Why:** the verb *is* the driver in the foreground, so loom's handshake machinery has no counterpart; a foreground verb that waits silently is indistinguishable from a hang.
- **Q:** Round 2, BLOCKING: only teardown-on-dirtiness was mapped to an Outcome; `Add` also refuses on a dirty prime and a pre-existing branch, and `Remove` refuses on `ErrMergeInProgress`. **A:** [auto-resolved] One rule — operator-fixable refusal is `Stuck`, hard `error` is reserved for mechanism failure and cancellation — applied per producer, with the dirty-prime and branch-exists refusals called out by name and fabric's error text passed through unreworded. **Why:** `Stuck` plus an empty `OnStuck` persists the reason and preserves state to resume into; a hard error returns an unpopulated `Result` and the reason never reaches the status file.
- **Q:** Round 3, BLOCKING: `Env.TeardownWorktree` as one opaque closure could not express the ordering, the no-removal-on-shutdown-failure rule, the which-half-failed reason, or the abandoned-session value — the same defect round 2 fixed for `LoomRun`, left uncorrected for teardown. **A:** [auto-resolved] `Env.Teardown` becomes a `lifecycleshed.TeardownDeps` passthrough with separate `Shutdown func(ctx) (abandonedSession string, err error)` and `Remove func(ctx) error` fields. **Why:** the governing test is whether a producer has behaviour a caller must observe; teardown has four such behaviours and `WorktreeCreate` has none, which is why the seam shapes differ per producer.
- **Q:** Round 3, BLOCKING: the sandbox scenario claimed it could drive `lyx lifecycle run` without spawning an LLM, but a fresh pair has no loom status and `mustSpawnDriver` is `!runLockHeld`, so `Loom-Run` always spawns a real driver into loom's LLM rows. **A:** [auto-resolved] The scenario never completes a `Loom-Run` row. It covers the lifecycle status fixture round-trip plus four refusal paths, following `SANDBOX-CORE-SUITE.md` S8's hand-written-fixture precedent and carrying the same kind of **Fixture note**; the full driven path is covered by the integration test instead. **Why:** the earlier claim was simply false, and module-level coverage does not require driving the LLM path.
- **Q:** Round 3, BLOCKING: `lifecycle-driver-runs-from-prime` rested on a claim that `Add`/`Remove` refuse to operate on the worktree they are invoked in — `refusePrimeSlug` only compares the *named slug* against `PrimeName`, so nothing stops a driver deleting its own cwd. **A:** [auto-resolved] The claim is corrected in the decision text, and `lifecyclecli` now refuses on the envelope on both verbs when its resolved `Location` is not the hub's prime worktree. **Why:** the arrangement the whole design rests on has to be enforced rather than assumed, and this refusal is the runtime check standing in for the Bookend invariant's untestable cwd claim.
- **Q:** Round 3, NIT (demoted from BLOCKING): does `lifecyclecli` carry `RunCLIIn`, and what happens to the CLI/Cobra Invariant's "eleven of twelve" count? **A:** [auto-resolved] Yes — the Bookend proxy tests and the non-prime refusal both need an injectable cwd — and the count line becomes "twelve of thirteen" in the same commit. **Why:** skipping it would leave the two tests that matter most here unreachable.
- **Q:** Round 5, BLOCKING: `run.lock` is per-slug, so two `lyx lifecycle run` invocations for different slugs could drive `Topology.Add`/`Remove` against the same prime concurrently — and `Add` takes no lock of its own while reading prime's working tree. **A:** [auto-resolved] A second, prime-wide advisory lock at `<prime anchor>/.lyx/lifecycle/run.lock`, taken non-blockingly and held only across `Worktree-Create` and `Worktree-Teardown`, refusing on the envelope and naming the holding slug. **Why:** scoping it to the two bookends protects the real race without serializing hours of independent LLM work, which a run-wide prime lock would.
- **Q:** Round 5, NIT (demoted from BLOCKING): the CLI dispositions split on "non-terminal" vs `done`, but `blocked` — the everyday outcome of every `Stuck` — matched neither branch, and `shedengine.Run` itself resumes from `StateBlocked`/`StateFailed`. **A:** [auto-resolved] Dispositions restated per `shedengine.State` value: `running`/`blocked`/`failed`/`paused` resume, `done` refuses, absent starts fresh; the word "terminal" is dropped here because it means something different in `loom-run-poll-bound`. **Why:** the ambiguity sat on the dominant recovery path, and `refusal-to-outcome-mapping`'s rationale had already promised resume-from-blocked.
- **Q:** Round 6, BLOCKING: the `LoomRun` verdict table was declared exhaustive over loom's `state` but covered four of five values — `failed` is persisted by `shedengine.Run` on a producer hard error (`run.go:226`, `:289`) and would have fallen through to the attempt cap. **A:** [auto-resolved] Added a `failed` → `Stuck` row and corrected the "three clean-exit values" claim, which is true of `RunOutcome` and not of the persisted `State`. **Why:** `failed` is not a polling state, so the omission would have stalled the lifecycle for the full twelve-hour default and then escalated with a misleading timeout reason.
- **Q:** Round 6, BLOCKING: the child's `Dir` was stated as "the task worktree", and the only derivation given yields the worktree root — but `lyxcwd.Resolve` gates cwd to `Join(worktreeRoot, AnchorRel)`. **A:** [auto-resolved] `Spawn`'s `Dir` is the resolved `Location.AnchorPath()`, derived in the same lazy closure as the status paths. **Why:** a bare root fails with `ErrCwdOutsideAnchor` on any subpath-anchored hub; `hubgeom.ReedGeometry` already fills `PaneCwd` this way for the same reason.
- **Q:** Round 6, BLOCKING: the prime-wide lock was assigned to `lyx lifecycle run` yet scoped to two specific rows — a scope the CLI cannot express from outside an opaque `Shed.Run` call, with no story for a resume directly into teardown. **A:** [auto-resolved] The two bookend producers acquire and release it themselves via an injected `Env.PrimeLock` seam, returning `Stuck` when it is unavailable; the round-5 "refuses on the envelope" wording is corrected. **Why:** producer-side ownership makes the bound structural, handles resume-into-teardown with no special case, and is unit-testable — where `Shed.Step` would force the CLI to re-implement `Run`'s routing for no gain.
- **Q:** Round 7, BLOCKING: the prime-lock `Stuck` reason was required to name the holding slug, but `lock.TryAcquireWriteLock` reports contention as a bare `(nil, false, nil)`, the lock file carries no holder record, and the `Acquire` seam returns no identity — unsatisfiable as written. **A:** [auto-resolved] Dropped the holder-naming requirement; the reason names the lock path only, matching the per-slug refusal. **Why:** a sidecar holder file would buy one word of diagnostics and bring its own staleness bug — a crashed holder leaves a name nothing clears, so the reason would confidently blame a slug that is no longer running.
- **Q:** Round 7, BLOCKING: `abandonedSession` had no carrier — `Call` returns only `(Outcome, OutputPointer, error)` and `Result` carries no free-text field, yet the discussion ruled out both the stuck reason and the CLI closure. **A:** [auto-resolved] The carrier is split: the producer logs it at `Warn`, and the CLI-side `Shutdown` closure records it into a variable `lifecyclecli` reads after `Run` and puts on the envelope. The "the producer owns putting it on the result" sentence is corrected. **Why:** nothing reaches an envelope from inside a producer, so the envelope half was never the producer's to own.
- **Q:** Round 7, NIT (demoted): both fixture enumerations listed four new `newTestEnv` seams, but round 6 added a fifth, `Env.PrimeLock`. **A:** [auto-resolved] Both lists now name five, with a fake `Acquire` returning a no-op release. **Why:** `TestBuild_EveryRegisteredEngineBuilds` drives off `shedrecipe.Names()` and fails until every required seam is filled.
- **Q:** Round 7, NIT (demoted): the Constraints bullet stated a conditional about `mutations`/`partial` that no section resolved, while the chosen seams silently discard `AddResult`/`RemoveResult`. **A:** [auto-resolved] New `mutation-records-are-logged-not-enveloped` decision: the lifecycle envelope carries neither key, and the CLI closures log the record at `Info` instead. **Why:** a Shed run has no coherent scope for a single `mutations` array and `partial` is meaningless at run level, but the record's diagnostic value in the rollback case is real, so logging beats discarding.
- **Q:** Testing split? **A:** [auto-pick] Tier-1 unit tests per producer over injected seams, plus recipe parse/build and registry coverage, plus one `integration`-tagged end-to-end over a real `hubforge` hub with `LoomRunDeps`' own fields stubbed — including the `Stuck`-preserves-the-worktree case and a mid-list resume case. **Why:** Test Tier Purity bars real git and process spawns from untagged files, so the seam split is forced regardless.


### From _mill/plan/00-overview.md


```yaml
task: "Worktree spawn/teardown as Shed producers"
slug: "worktree-lifecycle-shed-producers"
approved: true
started: "20260919-051253"
parent: "main"
root: ""
verify: null
discussion_sha: "e06b75bb9266e68f5f2071bbec6842d5e49909ee"
```

### From _mill/plan/01-lifecycleshed-producers.md


```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "lifecycleshed producers"
number: 1
cards: 7
verify: go test ./internal/lifecycleshed/...
depends-on: []
```



- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/doc.go`
  - `internal/lifecycleshed/ctx.go`
  - `internal/lifecycleshed/ctx_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/stuck.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/deps.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/create.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/loomrun.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/teardown.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/create_test.go`
  - `internal/lifecycleshed/loomrun_test.go`
  - `internal/lifecycleshed/teardown_test.go`
  - `internal/lifecycleshed/seam_enforcement_test.go`
- **Deletes:** none

### From _mill/plan/02-shedrecipe-entries.md


```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "shedrecipe lifecycle entries"
number: 2
cards: 5
verify: go test ./internal/shedrecipe/... ./internal/shedbuild/...
depends-on: [1]
```



- **Edits:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/env_test.go`
  - `internal/shedrecipe/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_lifecycle.go`
- **Deletes:** none
- **Edits:**
  - `internal/shedrecipe/registry.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/shedrecipe/registry_test.go`
  - `internal/shedrecipe/fixture_test.go`
- **Creates:**
  - `internal/shedrecipe/entries_lifecycle_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedbuild/build_engines_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/03-lifecycle-recipe-and-coverage.md


```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "lifecycle recipe and coverage guard"
number: 3
cards: 6
verify: go test ./internal/lifecyclerecipe/... ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/... ./contracts/...
depends-on: [2]
```



- **Edits:**
  - `contracts/recipes/recipes.go`
- **Creates:**
  - `contracts/recipes/lifecycle-recipe.yaml`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecyclerecipe/doc.go`
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
  - `internal/lifecyclerecipe/names.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecyclerecipe/fixture_test.go`
  - `internal/lifecyclerecipe/recipe_test.go`
  - `internal/lifecyclerecipe/coverage_guard_test.go`
  - `internal/lifecyclerecipe/seam_enforcement_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/loomrecipe/recipe_test.go`
- **Creates:**
  - `internal/loomrecipe/names.go`
- **Deletes:** none
- **Edits:**
  - `internal/loomrecipe/coverage_guard_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/coverage_guard_test.go`
- **Deletes:** none

### From _mill/plan/04-loom-run-no-attach.md


```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "loom run --no-attach"
number: 4
cards: 2
verify: go test ./internal/loomcli/...
depends-on: []
```



- **Edits:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/bootstrap_test.go`
  - `internal/loomcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/loomcli/bootstrap_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/05-lifecyclecli-module.md


```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "lifecyclecli module"
number: 5
cards: 8
verify: go test ./internal/lifecyclecli/... && go test -tags integration ./internal/lifecyclecli/...
depends-on: [3, 4]
```



- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/paths.go`
  - `internal/lifecyclecli/paths_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/refusal.go`
  - `internal/lifecyclecli/refusal_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/wire.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/cli.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/run.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/status.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/cli_test.go`
  - `internal/lifecyclecli/run_test.go`
  - `internal/lifecyclecli/wire_test.go`
  - `internal/lifecyclecli/testmain_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/lifecycle_integration_test.go`
  - `internal/lifecyclecli/testmain_integration_test.go`
- **Deletes:** none

### From _mill/plan/06-registration-and-docs.md


```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "registration and docs"
number: 6
cards: 6
verify: go test ./cmd/lyx/...
depends-on: [5]
```



- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/notransients_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/worktree-lifecycle-shed-producers.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `CONSTRAINTS.md`
- `manifest/roadmap.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides.
   When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure.
   Do NOT pick one side wholesale just because the region overlaps syntactically;
   picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values).
   Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Before keeping content from either side inside a conflict hunk, search the rest of the file (outside the hunk) for that same content.
   This judgment call is scoped narrowly — it applies only when a hunk's content might be a moved duplicate of content living elsewhere in the file;
   it does NOT apply to every ordinary step-3 disjoint-region combine (e.g. the column-A/column-B worked example above), which remains today's silent, high-confidence success path.
   Two branches:
   - **Confident case:** if the content clearly already exists elsewhere and the surrounding context makes it unambiguous that this is the same item having been moved (not two independent, separately-intended copies) — do not re-add it in the hunk;
     keep only the other side's unrelated edit.
     Worked example: one side moves a roadmap item from `## Planned` to `## Done`, while the other side makes an unrelated edit elsewhere in the file.
     The resolution keeps the item only under `## Done`;
     it is not re-added under `## Planned`.
   - **Ambiguous case:** if you cannot confidently tell whether this is the same moved content or a legitimate independent duplication — fall back to step 3's default (keep both) rather than guessing, and report the ambiguity via the `discarded` field (see Report section) with the description `"kept both sides of a conflict, ambiguous move-vs-duplicate"`.
     Worked example: a similarly-worded item appears in two different sections and you cannot tell whether it is the same item moved or a legitimate second, independently-added item.
     The resolution keeps both occurrences and reports the ambiguity via `discarded`.
5. Run `git -C /home/knatte/Code/loomyard/wts/worktree-lifecycle-shed-producers add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/worktree-lifecycle-shed-producers rm <file>` instead of editing;
   that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification.
   Instead: a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent. b. Run `git show <deletion-commit>` to inspect context. c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"),
   or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/worktree-lifecycle-shed-producers rm <file>`. d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt.
   Do NOT silently keep the modification.
8. Before reporting `{"status":"success"}` (with or without `discarded`), re-read each file listed in Conflicting files in full and explicitly verify no contradictory losing-side claims survive the resolution — e.g. a stale value from one side of the conflict left alongside the correct value from the other side, or a claim that only made sense before the other side's edit was applied.
   If you find a contradiction you missed, fix it before reporting.
   If you find a contradiction you cannot confidently resolve, report `{"status":"stuck","stuck_type":"logic","reason":"self-verification found an unresolved contradiction in <file>: <description>"}` instead of `{"status":"success"}`.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost.
If anything was discarded, you MUST list it;
an empty list when content was actually dropped is a protocol violation. `discarded` also carries the step 4 ambiguous-case entry `"kept both sides of a conflict, ambiguous move-vs-duplicate"` — even though nothing was technically dropped in that case, the field's purpose is to surface anything the operator should double-check before `git merge --continue`, which covers both a genuine drop and a kept-both ambiguity.
The `mill-merge-in` frontend reads this field and surfaces any losses (or ambiguities) to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation;
the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost.
Do not wrap the JSON in a code fence;
do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob.
Use `git -C /home/knatte/Code/loomyard/wts/worktree-lifecycle-shed-producers` for any git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/worktree-lifecycle-shed-producers`.
