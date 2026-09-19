# Discussion: Seeded driver choice: ly-drive strand as the child's driver

```yaml
task: 'Seeded driver choice: ly-drive strand as the child''s driver'
slug: seeded-driver-choice
status: discussing
parent: main
```

## Problem

A Shed run's seed records **who steps the FSM** in its `driver` field, but only one of the two values does anything.
The predecessor task (`seeded Shed core`, slug `seeded-shed-core`, already `discussed` and not yet merged) establishes `seed.json` at `_lyx/shed/<run-id>/`, declares both `go` and `llm` as `shedrun` constants, wires `--driver`/`--child-driver` flags onto the seeding paths — and then refuses `llm` at arming time with a message naming this roadmap item.
So today the only driver a run can have is the detached Go runner (`lyx loom run`), which steps the machine mechanically and escalates to `blocked` whenever a gate concludes a human is needed.

The whole point of `ly-drive` is the other option: an intelligence inside the task worktree that reads each step's envelope, notices what a mechanical gate cannot, and keeps the run moving.
Today that only happens when an operator opens a worktree by hand and types `/ly-drive` into a Claude session.
There is no way to say, at seeding time, "this run drives itself with an LLM" — which is what the design (`manifest/designs/seeded-shed.md`, "The seed" and "Batten — the outer recipe") describes as the expected default for task-work runs.

Why now: `seeded-shed-core` deliberately ships the field without its second value, precisely so this task can be a *spawn-command change* rather than a contract change.
The seed format is the durable on-disk contract; every day `llm` stays unimplemented is a day the refusal text in `shedrun`/the arming paths points at a roadmap item instead of behaviour.

**Gate:** `seeded-shed-core` is `discussed` but not merged — `internal/shedrun`, `internal/battencli`, `internal/battenshed`, `Run-Shed`, and the `driver` field do not yet exist on `main`.
Every file reference below to a `shedrun`/`batten*` symbol is to that task's *decided contract*, not to code on disk.
Do not finalize this task's plan until `seeded-shed-core` has merged to `main`.

## Scope

**In:**

- Lifting the `llm` refusal for the **loom** recipe's own run: `lyx loom start` reads its run's seed, and on `driver: llm` boots a Claude strand running `ly-drive` instead of spawning the detached `lyx loom run` process.
- A new driver-launch seam in `internal/loomcli` that composes the driver run through `internal/shuttleengine` + `internal/shuttleengine/claudeengine`, mirroring how loom's own producer rounds spawn agents.
- Driver-strand identity and re-entrancy: a fixed display name (`loom-driver`), a liveness pre-check, and dead-corpse removal before a relaunch, with `mustSpawnDriver` widened to take the run-lock and strand signals together on **both** driver paths.
- A driver-specific handshake: the run-lock handshake stays the `go` path's alone; the `llm` path's readiness signal is the strand being registered and its pane alive.
- `internal/loomengine`'s `Config` gains one `driver` model-spec key, validated at load time exactly like the four existing role keys.
- `plugins/ly/skills/ly-drive/SKILL.md` gains an **autonomous driver** section: no operator prompts, the report written to the driver run's output file rather than spoken to a human, and the stop conditions restated for a session with nobody to hand back to.
- Accepting `--child-driver llm` on `lyx batten run|step <slug>` and `--driver llm` on `lyx shed seed <run-id>` when the seeded recipe is `loom`, replacing the refusals `seeded-shed-core` installs.
- Doc updates in the same commit: `manifest/designs/seeded-shed.md` (the driver section moves from "expected" to as-built), `docs/overview.md` if the module table or execution stack changes, `CONSTRAINTS.md` for the new invariant below, and `manifest/roadmap.md` (the Planned item moves to shipped).

**Out:**

- **`driver: llm` for the batten recipe itself.** Batten has no bootstrap verb — no `lyx batten start` — so there is no spawn seam to branch, and `lyx batten run <slug>`'s driver *is* the process the operator typed.
  `--driver llm` on the batten path therefore stays refused, with the message rewritten to name the missing bootstrap verb rather than this roadmap item.
  Only `--child-driver llm` is accepted here.
- Flipping any default. `driver` still defaults to `go` everywhere, including `--child-driver`.
  The design's "expected default: `llm` for task-work runs" is a config decision for a later pass, once an llm-driven run has actually been watched end to end.
- Relay-stepping, the comfort rows (VS Code launch/close around the child), and Hardener — all still out, exactly as `seeded-shed-core` left them.
- Any change to `Run-Shed`'s disposition table, to `shedengine`'s two-value `Outcome` vocabulary, to the envelope contract, or to the five closed `step` refusal kinds.
  Status-watching is driver-agnostic by construction and stays that way.
- Any non-Claude engine. The launch goes through `shuttleengine`'s provider seam, so a second engine is a `claudeengine` sibling later, not a fork of this work.
- Teaching `lyx shed run|step` to gate on the seed's `driver`. They are drivers being invoked by hand; see the decision below.
- Killing, supervising, or restarting a driver strand that dies mid-run. See `dead-driver-is-an-operator-case`.

## Decisions

### driver-branch-lives-in-the-bootstrap-verb

- Decision: the `driver` read and the two-way branch live in **`lyx loom start`** (`internal/loomcli/start.go`, step 5), not in batten's `Run-Shed` spawn seam.
  Batten's seam keeps running `lyx loom start --no-attach` in the child worktree, byte-identical to today; the child's own bootstrap reads the child's own `_lyx/shed/self/seed.json` and decides.
- Rationale: three things fall out of this placement that do not fall out of the alternative.
  First, an operator typing `lyx loom start` by hand in a seeded worktree gets the driver the seed asked for — under the alternative, driver choice would only ever apply to runs batten started, and a hand-started run would silently take `go`.
  Second, the Cwd Resolution Invariant is satisfied for free: the seed being read is the *current worktree's own*, resolved by that worktree's own `lyxcwd.Location`, never a path threaded in from prime.
  Third, batten's seam needs no change at all, which is the cheapest possible interaction with a row (`Run-Shed`) whose disposition table `seeded-shed-core` just pinned.
- The design text says "one changed command in the existing Spawn seam".
  That reads as batten's seam because that is where the spawn is *initiated*, but the command batten's seam runs is itself a bootstrap, and the branch belongs at the innermost point that knows whose run it is.
  The design's substance — one command changes, nothing else — holds either way.
- Generalization, stated so a future recipe does not have to rediscover it: **a recipe's own bootstrap verb is the site that reads its run's `driver`.** `loom start` is the only such verb today.
  A future recipe with a bootstrap verb reads its own seed the same way; a recipe without one cannot support `llm` until it grows one, which is exactly why batten is out of scope above.
- Rejected: branching inside `Run-Shed`'s `deps.Spawn` (leaves hand-started runs unable to honour the seed, and makes the child's driver choice a property of the parent's code path rather than the child's own seed); a `--driver` flag on `lyx loom start` overriding the seed (a second source of truth whose only behaviour is to disagree — the same argument `seeded-shed-core` used to delete the `--recipe` persistent flag).

### llm-driver-launches-through-shuttle

- Decision: the `llm` branch launches its Claude session through `internal/shuttleengine`'s `Runner.Start` with a `shuttleengine.Spec`, exactly as loom's producer rounds do — never by hand-composing a `claude` command line onto a bare `reedengine.AddStrand`.
- Rationale: the launch line is not a string, it is a contract.
  `claudeengine.Prepare` writes `prompt.md`, builds and writes `settings.json` (the autonomous permission posture, the `AskUserQuestion` PreToolUse deny, the `Agent` deny, the events hook), mints the session id, resolves `(model, version)` into a model id, validates effort, and composes both the launch line and the `ResumeCmd` through `internal/shell` — which the Shell Mechanics Seam invariant requires and which a hand-built command would violate on its first line.
  Duplicating any of that in `loomcli` would also breach the Shuttle Provider-Seam Invariant, since `loomcli` would then be naming Claude specifics outside `claudeengine`.
- The driver run is an ordinary shuttle run in every respect but one: **`loom start` never calls `Wait`.** `Runner.Start` is already non-blocking — it returns once the strand is added and `run.json` is persisted — so the bootstrap starts the driver and returns, and whoever cares about progress reads the shed status file, which is what `Run-Shed` and `lyx loom status` already do.
- Rejected: `reedengine.AddStrand` with a locally composed `claude …` command (duplicates `claudeengine`, breaks two invariants, and loses the resume line); adding a "long-lived run" mode to `shuttleengine` (a new run class in the package whose whole contract is "the output file is the return value", for one caller).

### driver-report-is-the-run-s-output-file

- Decision: the driver run's `Spec.OutputFiles` is a single **drive report** under the run's own ephemeral tree, made unique per bootstrap attempt: `.lyx/shed/<run-id>/drive-report-<compact-timestamp>-<4-hex>.md`.
  `ly-drive`'s autonomous mode writes it at every stop condition — a `continue: false` envelope, an error envelope it hands back on, the step cap, or an interrupted-invocation hand-back.
- Rationale: `shuttleengine.Spec.validate` requires at least one output file and rejects one that already exists, and the Completion Signal Invariant makes that file the *only* thing that answers "did this run finish".
  A driver session that stops without writing one would be classified as died/timed-out by any later `Attach`, which is wrong — an autonomous driver that hands back has finished its job.
  Giving the driver a real return value is therefore not ceremony to satisfy a validator; it is what makes the driver legible to the machinery that already exists.
- The per-attempt suffix is what keeps a relaunch legal: a second bootstrap after a driver stopped must not trip the must-not-exist check on a report the first one wrote.
  A compact timestamp alone is second-granular and **does** collide on a fast relaunch — corpse removal plus a fresh `Start` inside one second is not hypothetical, it is exactly what the smoke test's instantly-exiting stub pane produces — and the collision's symptom is `Spec.validate` refusing the relaunch, the one case the suffix exists to permit.
  The random component removes that case rather than documenting it; the timestamp stays because it is what makes a directory listing of past attempts readable in order.
  Ephemeral placement is right by the Durable-vs-Ephemeral State Invariant — the report is a per-machine record of one session's own narration; the durable truth about the run is `status.json` beside it under `_lyx`.
- **The directory already exists by the time the driver writes there**, and nothing in this task needs to create it: `.lyx/shed/<run-id>/` is where `seeded-shed-core` puts the run lock and the status lock, and `start.go` already `MkdirAll`s that directory at step 4 for the bootstrap lock — which is why its own comment notes that creating it there also covers the run lock and driver log.
  Worth stating because `Spec.validate` only rejects a pre-existing output file; it creates no directory, and a driver writing its report into a missing one would fail at the very end of a long session.
- Rejected: a fixed report path (a second bootstrap refuses on a stale file, and the stale file is evidence, not debris to delete); a bare second-granular timestamp with the collision accepted as a self-describing residual (it fires precisely under the automated relaunch the smoke test drives, so it would be a residual that shows up as a red test rather than as a rare operator surprise); reusing `status.json` as the output file (it already exists when the driver starts, so `validate` refuses it, and the driver would then be declaring ownership of a file the engine writes); relaxing shuttle's output-file requirement for this caller (see the previous decision's rejected list).

### handshake-is-driver-specific

- Decision: the bootstrap's run-lock handshake (`awaitRunLock`, `bootstrapHandshakeAttempts`, `dispositionForHandshake`) stays on the **`go` path alone**, unchanged.
  The `llm` path's readiness signal is `Runner.Start` returning successfully: the strand is registered, its pane is live, and the launch line is running in it.
- Rationale: the run lock is held for the *whole* of a `lyx loom run`, which is exactly what makes it a valid "a driver is alive" signal there.
  An ly-drive session takes that lock only inside each `lyx shed step`, and releases it between steps — so a handshake waiting on it would either race (the first step has not started yet, because a Claude session takes tens of seconds to boot and read its skill) or observe a free lock between two perfectly healthy steps.
  Reusing it would convert a working driver into a refused bootstrap.
- `--no-attach` keeps its documented meaning on both paths: perform every bootstrap step, confirm the driver is up by that path's own readiness signal, and return without the terminal handover.
  Its `Long` text must be amended to say so per driver rather than naming the run lock unconditionally.
- Rejected: waiting for the driver's first status transition (a fresh run's first row can legitimately take minutes, and an already-halted resumed run makes no transition at all); waiting for the run lock with a longer deadline (the race above is structural, not a tuning problem); no readiness check at all on either path (throws away a working guard on the `go` path to make the two symmetric).

### re-entrancy-by-named-driver-strand

- Decision: the spawn predicate takes **both** signals on **both** paths.
  `mustSpawnDriver` gains a second input, and a driver is spawned only when the run lock is free **and** no live strand named `loom-driver` exists in this worktree's reed session.
  The `llm` branch then resolves the strand half three ways: live strand → do nothing, no strand → start a driver run, strand present but its pane dead → `RemoveStrand` the corpse, then start a fresh driver run.
- **Both signals on both paths, because a run's driver kind is not fixed by its seed.** `generic-verbs-never-gate-on-driver` deliberately lets an operator run `lyx loom run` by hand against an `llm`-seeded run — so a later `lyx loom start` that consulted the strand table alone would find no strand and launch a Claude driver alongside the live Go one, which is the two-drivers collision this decision exists to prevent, surfacing as spurious `busy` refusals rather than as anything legible.
  The mirror case is real too: a live `llm` driver between two steps holds no run lock, so a `go`-path bootstrap consulting the lock alone would spawn a second driver into a worktree that already has one.
  Each signal is blind to exactly the driver kind the other sees, so the predicate is the conjunction and neither path may drop its half.
  Both reads are cheap and already performed on the `go` path today (`lock.TryAcquireWriteLock` non-blocking, released immediately; `reed.Status()`, which step 4 has already made a live session for).
- Rationale: `lyx loom start` is explicitly re-entrant, and a second invocation must never stack a second Claude session onto the same run — two drivers stepping one FSM is exactly the collision the run lock protects against, and it would surface as spurious `busy` refusals rather than as anything legible.
  The name is pinned as a constant beside `statusStrandDisplayName` and `operatorStrandDisplayName` in `internal/loomcli/bootstrap.go`, for the same reason those are: reed's add has no upsert semantics, so every add and every lookup must agree on one byte-stable literal.
- The pre-check is what keeps the no-op case clean.
  `AddSpec.IfAbsent` would no-op correctly at the reed layer, but shuttle's `Runner.Start` creates the run directory and writes `prompt.md`/`settings.json` *before* it calls `AddStrand` — so an `IfAbsent` no-op would leave an orphan run directory behind on every re-entrant bootstrap.
  Checking first means `Start` is only ever called when a strand is genuinely going to be added.
  The `NameOverride` still travels on the spec (a new pass-through field on `shuttleengine.Spec`, forwarded verbatim into the `AddSpec` `Runner.Start` already builds), because the name is what the next bootstrap looks up.
- Explicit corpse removal rather than `IfAbsent`'s relaunch-dead arm: a dead driver's pane must not be relaunched with the *old* launch line, which points at the previous attempt's `prompt.md` and its already-written report path.
  A fresh shuttle run with a fresh report path is the only correct relaunch.
- Note that this does **not** reintroduce the run lock as the `llm` path's *readiness* signal — `handshake-is-driver-specific` still stands.
  The lock answers "is something already driving this run", which is a question about the *past*; the handshake asked "has the thing I just spawned started driving", which is a question about the *future* and is what an ly-drive session cannot answer through a lock it holds only inside each step.
- Rejected: the strand table as the `llm` path's only signal (misses a hand-started `go` driver, per the paragraph above); the run lock as the `llm` path's only signal (misses a live ly-drive session between two steps); `IfAbsent: true` with no pre-check (orphan run directories, and a relaunch against a stale launch line); a marker file recording the driver's pid (reed already holds the authoritative strand table, and a second record of the same fact drifts).

### generic-verbs-never-gate-on-driver

- Decision: `lyx shed run|step|status|pause` and `lyx loom run|step` never read the seed's `driver` and never refuse on it.
  The field is consumed at bootstrap, by the bootstrap verb, and nowhere else.
- Rationale: those verbs *are* drivers. An operator typing `lyx shed step` against a run seeded `llm` is driving it by hand, which is legitimate and is the one thing that still works when an llm driver has wedged.
  Gating there would mean the seed's recorded startup choice could lock an operator out of their own run.
  It would also put a new refusal shape inside the verb body whose refusal-kind vocabulary the Shed Verb-Set Invariant pins closed at five.
- What stops two drivers colliding is the bootstrap's conjunction predicate (run lock free **and** no live driver strand — see `re-entrancy-by-named-driver-strand`), backed by the run lock itself inside each step.
  A hand-driven run is exactly the case that predicate's second half was widened for.
- Rejected: refusing `lyx loom run` when the seed says `llm` (locks the operator out of the manual-recovery path, and a `go` driver started by hand against an `llm` seed is a legitimate override); warning on the envelope (a field nothing branches on is noise, and the envelope's shape is pinned by tests and by `ly-drive`'s contract).

### ly-drive-gains-an-autonomous-mode

- Decision: `plugins/ly/skills/ly-drive/SKILL.md` gains an **Autonomous driver** section, entered when the launch prompt says so.
  Four things change from the operator-driven path, and nothing else does:
  1. **No operator choices.** The `$TMUX_PANE` self-check's tracked-absent branch and every other numbered-list prompt in the skill become a line in the report, never a question.
     There is no operator in the session to answer one, and `settings.json`'s `AskUserQuestion` deny means the tool is not even available.
  2. **The report goes to the file.** Every place the skill says "report to the operator" or "hand back with a report", the autonomous path writes that report to the output-file path named in its launch prompt and then stops.
  3. **The step cap is a budget, not a check-in, and the number is 120.** The operator-driven cap of 40 exists so a human can look; with no human, 40 would stop a healthy run three times before it finished.
     120 is loom's own worst case as the skill already computes it — seventeen rows, six review rows at five bounces each, three validator rows at the inherited default of ten, which lands near a hundred steps — plus a margin that keeps an unlucky-but-legitimate run inside the budget.
     The literal `120` is pinned in **both** places and must agree: the SKILL.md autonomous section states it as the cap, and the Go-composed launch prompt repeats it as the number this session runs under, so a reader of either sees the same value.
     Drift between the two is silent, so it gets a cheap mechanical check rather than a convention: one Go constant is the single source, the prompt composer interpolates it, and a test reads `plugins/ly/skills/ly-drive/SKILL.md` from the repo and asserts the constant's value appears in its cap sentence — the same shape `internal/loomcli`'s `discussiontemplate_test.go` already uses to pin stencil text against Go.
     On exhausting it the driver writes the report and stops, leaving the run exactly as it is.
  4. **Stop conditions are otherwise unchanged and remain absolute.** `continue: false` stops. An error envelope stops, with the single `producer`-kind retry the skill already allows. The skill still never clears `state`, never edits the status file, never re-seeds, never pushes, never kills a pane, and never touches git.
- Rationale: the skill's judgment is the product; autonomy changes only *who reads the output* and *what to do when there is nobody to ask*.
  Rewriting its stop conditions for the autonomous case would be a second, divergent driving policy — the thing this repo's one-skill-one-policy shape exists to avoid.
- The skill keeps `disable-model-invocation: true`. The driver session invokes it explicitly from its launch prompt, which is exactly the "explicit invocation only" contract, not an exception to it.
- Rejected: a separate `ly-drive-auto` skill (two copies of one policy, guaranteed to drift); leaving the skill untouched and putting the autonomous rules in the launch prompt alone (the prompt is composed in Go, so the policy would live in a Go string literal and be invisible to anyone reading the skill).

### driver-is-a-loom-config-key

- Decision: `internal/loomengine`'s `Config` gains exactly one new key, `Driver string \`yaml:"driver"\``, added to `ConfigTemplate` and validated with `modelspec.Parse` at load time when non-empty, exactly as `friction` is.
  **No `driver_timeout_min` knob.** `shuttleengine.Spec.Timeout` feeds `Run.deadline`, which only `Wait` reads, and `RunState` never persists it — so on a path that deliberately never calls `Wait` and never `Attach`es, a timeout key would configure nothing and a test asserting it would pass against a dead field.
  The driver spec leaves `Timeout` at its zero value, which `Spec.validate` fills from shuttle's own `run_timeout_min`, inert and unread.
- **What actually bounds a driver session, since no clock in this task does:** the skill's own 120-step budget, and above it `Run-Shed`'s 12-hour watch budget, which lands the outer run in `StateBlocked` whether the driver is looping, wedged, or dead.
  Neither is a wall clock on the Claude session itself, and this task adds none.
  If one is wanted later, it belongs where it can be enforced — something that `Wait`s or probes — not as an unread field on a spec.
- Rationale: every agent-spawning role in the stack says which model runs it through a model spec, and the driver is an agent-spawning role.
  Validating at load time is the existing discipline and the stated reason for it applies here with more force than anywhere: a driver spawn failing on a bad model id fails *after* the bootstrap has already seeded and committed, leaving a run that looks started and is not.
- Empty `driver` means the engine default, the same "defer to the provider" meaning `Spec.Model`'s empty value already carries.
  The driver's reasoning effort travels in the bracket form the spec grammar already provides, so no separate effort key is added.
- Rejected: a `--driver-model` flag on `lyx loom start` (per-invocation model choice for a role every other role configures in a file); reusing the `plan` or `review` key (a driver is a different role with a different cost profile, and sharing a key means a change to one silently moves the other); a `driver_timeout_min` key shipped now as a forward-compatible placeholder (an unread config key is a promise the code does not keep, and the first person to set it would get silence).

### autonomous-posture-is-shuttle-s-default

- Decision: the driver `Spec` takes `Interactive: false` (shuttle's Go zero value, meaning autonomous) and `ForkSubagents: false`.
- Rationale: `Interactive: false` is what adds `--dangerously-skip-permissions` and the `AskUserQuestion` deny, which is precisely the posture an unattended driver needs — the skill's own operator prompts are what the deny is there to make structurally impossible rather than merely discouraged.
  `ForkSubagents: false` because the driving loop reads envelopes and invokes a CLI; it has no research fan-out to delegate, and authorizing subagents for a session that loops for hours widens the blast radius for nothing.
- The `Agent` tool deny that shuttle applies in both modes is what keeps the driver from dispatching agents of its own — correct here for the same reason.
- Rejected: `Interactive: true` (every permission prompt in a pane nobody is watching becomes a wedged run); `ForkSubagents: true` (capability with no caller).

### dead-driver-is-an-operator-case

- Decision: a driver strand that dies while its run is still `running` is **not** detected, reported, or recovered by anything in this task.
  `Run-Shed` keeps polling the child's status for its 12-hour budget and then lands the outer run in `StateBlocked`; an operator re-runs `lyx loom start`, whose corpse-removal branch relaunches a fresh driver.
- Rationale: detecting it means something must watch the strand table, and the only candidates are `Run-Shed` (whose disposition table `seeded-shed-core` just pinned, and which is deliberately driver-agnostic) or a new supervisor (a module this task has no mandate to invent).
  The failure mode is bounded and visible — a run that stops progressing, which is the same shape as a wedged `go` driver today — and the remedy is one command an operator already knows.
- Stated as an accepted residual rather than left to be discovered, because it is the one behaviour where `llm` is genuinely weaker than `go`: a detached Go driver that dies frees the run lock, and the next bootstrap's handshake notices; a dead Claude strand frees nothing and the bootstrap notices only because this task's corpse check looks.
- Rejected: teaching `Run-Shed` to probe the child's strand table (a cross-worktree read from prime into the child's reed state, which the one-FSM-per-worktree rule exists to prevent — composition across a worktree boundary is by status file and nothing else); a watchdog extension (out of scope, and the per-hub watchdog reconciles panes, it does not adjudicate run health).

### new-invariant-driver-choice-single-site

- Decision: record a new invariant in `CONSTRAINTS.md` in the same commit — **Driver Choice Single-Site Invariant**: a *recorded* `Seed.Driver` value is read in exactly one place per recipe, that recipe's own bootstrap verb, and the branch on it selects a spawn and nothing else.
  No producer, no generic verb, and no engine reads the recorded value; no code path gates a refusal on it.
- **The invariant governs the read, not the vocabulary.** The driver *constants* (`shedrun`'s `go`/`llm`) are legitimately named at the seeding sites, which validate a flag before a seed exists — `lyx shed seed`'s `--driver`, and `lyx batten run|step`'s `--driver`/`--child-driver`, including the batten-path refusal this task rewrites rather than lifts.
  Those sites validate an *argument*; they never read a written seed's driver and never decide who drives.
  Conflating the two is what makes a naive "one consumer of the constants" rule false against this task's own scope.
- Rationale: the recorded value is a startup choice, and the failure mode a second *reader* introduces is silent divergence between what a run was seeded as and what it is actually doing — which is unobservable from either the status file or the envelope.
  One read site keeps "who drives this" answerable by reading one function.
  A flag validator, by contrast, fails loudly at the command line, where a mistake is visible immediately.
- The invariant's mechanical proxy: a scan asserting that the only production reader of `shedrun.Seed`'s `Driver` **field** outside `shedrun` itself is `internal/loomcli`, with the seeding sites named in the invariant as the permitted consumers of the constants alone (`internal/shedcli`'s `seed` command and `internal/battencli`'s flag validation).
  The proxy is a tripwire, not a completeness proof: adding a reader fails it and forces a human to confirm.

## Technical context

Everything under this heading that names a `shedrun`, `batten*`, or `Run-Shed` symbol describes `seeded-shed-core`'s **decided contract**; none of it exists on `main` yet.
The rest describes code on disk today.

**The bootstrap, which is where this task's change lands.**
`internal/loomcli/start.go`'s `startCmd` runs seven numbered steps.
Steps 1–4 (parent resolution, status seed, fabric commit, bootstrap lock, reed substrate + status strand, watchdog spawn) are driver-agnostic and untouched.
**Step 5** is the branch point: it probes the run lock non-blockingly, and `mustSpawnDriver(runLockHeld)` decides whether to `exec.Command(exe, "loom", "run")` with `proc.Detach`, a driver log, and a background `Wait` reaper.
**Step 6** is the handshake — `awaitRunLock(lockHeld, alive, halted, wait, bootstrapHandshakeAttempts)` with its three dispositions and its two breadcrumb log lines.
**Step 7** is the tmux handover behind `mustAttach(noAttachFlag)`, which also adds the operator strand.
The `llm` branch replaces steps 5 and 6 with the strand-liveness check and the shuttle `Start`; steps 1–4 and 7 run identically on both paths.
Note that the bootstrap lock is held across steps 5 and 6 and released explicitly at step 7, never deferred — the `llm` branch must release it on every one of its own error paths, exactly as the `go` branch does.

**`internal/loomcli/bootstrap.go`** holds the bootstrap's pure decisions, each already under test with no real lock, process, or clock: `statusStrandDisplayName`, `operatorStrandDisplayName`, `operatorStrandAddSpec()`, `mustSpawnDriver`, `mustAttach`, `awaitRunLock`, `dispositionForHandshake`.
The new driver-strand name constant and the new "spawn or not" predicate belong here beside them, for the same reason: the verb body in `start.go` should stay assembly over judgment that is already tested.
`operatorStrandAddSpec`'s doc comment is the model for the new spec's — it pins each non-default field with its reason, which is the standard this package holds.

**`internal/shuttleengine`.**
`Runner.Start(spec)` validates the spec, sweeps orphans opportunistically, creates a run directory under the config's run-dir root, calls `engine.Prepare(runDir, spec, cfg)` for a `Launch{Cmd, ResumeCmd, SessionID}`, then `reed.AddStrand(reedengine.AddSpec{Role, Round, Parent, Cmd, ResumeCmd, SessionID, Display})` and persists `run.json`.
`AddSpec` already carries `NameOverride` and `IfAbsent`; `Spec` does not forward either, so `Spec` gains a `NameOverride` pass-through (forwarded verbatim, never interpreted — the same shape `SessionID` already has).
`Spec.validate` requires a non-empty `Prompt` and at least one `OutputFiles` entry, resolves relative entries against the worktree root, rejects an entry that already exists, and defaults `Timeout` from `cfg.RunTimeoutMin`.
`claudeengine.Prepare` caps the prompt at 30000 bytes (the whole prompt expands into one command-line argument), validates effort against the five lowercase values, resolves `(model, version)`, writes `prompt.md`/`settings.json`, and composes both command lines through `internal/shell`.
Keep the launch prompt short and make it a pointer: the skill invocation, the run-id, the report path, and the autonomous-mode statement — never a copy of the skill.

**`internal/reedengine`.**
`AddSpec.IfAbsent` requires a non-empty `NameOverride` and gives no-op / relaunch-dead / fall-through-to-add semantics; this task uses `NameOverride` without `IfAbsent`, per the re-entrancy decision.
`Strand` carries `Name`, `GUID`, `SessionID`, and display state; `reed.Status()` is the read this task's liveness check goes through, and `RemoveStrand` cascades to descendants (a driver strand is parentless and childless, so the cascade is inert here — worth a word in the call's comment so a reader does not wonder).
`Display.Anchor` must be `render.AnchorBelowParent` or `render.AnchorHidden`; `AnchorOwnWindow` is refused in v1.

**`internal/loomengine/config.go`** holds `Config` and `LoadConfig`.
The four `modelspec.Parse` call sites and the negative-timeout guards are the pattern the two new keys follow line for line; `friction`'s "validated only when non-empty" arm is the pattern `driver` follows specifically.
`ConfigTemplate` is registered in `internal/configreg`'s module list as `loom`, so the template gains the two keys with their comments and existing worktrees pick them up through `configengine.Load`'s template merge.

**The seed read.**
`lyx loom start` already resolves its own `*lyxcwd.Location` in the pre-run; the seed read is `shedrun.ReadSeed` against `shedrun.SeedFile(location, runID)` with `runID` being `shedrun.SelfRunID` on the loom path.
`seeded-shed-core` puts a seed-presence check in `loomcli.Arm` that refuses with the run-id listing when absent — so by the time `startCmd`'s body runs on a started run there is a seed; but `start` is also the one verb that *writes* the seed, so the driver read happens after `seedAndCommitBootstrap`, against the seed that call just wrote or found.
Per `shedrun`'s contract an absent `driver` means `go`, so the branch is a two-value switch with `go` as both the default and the zero-config answer.

**Enumerate by literal.**
The `llm` refusal `seeded-shed-core` installs appears in more than one place — the arming-time refusal, `lyx shed seed`'s `--driver` flag, and `lyx batten run|step`'s `--driver`/`--child-driver` flags — and each refusal text names this roadmap item.
Sweep for the roadmap item's own name and for the `llm` literal across `internal/`, `contracts/`, `plugins/`, `docs/`, `manifest/`, and repo-root `CONSTRAINTS.md` before assuming the list is three entries long.
Three of those refusals are lifted and one (`--driver llm` on the batten path) is rewritten rather than removed.

**Prime's reed session vs. the child's.**
Reed sessions are per-worktree, so a driver strand in the child's session and anything prime is running can never collide.
The child's session and its status strand already exist by step 4 of the bootstrap, so the driver strand joins a live session rather than booting one.

## Constraints

From `CONSTRAINTS.md`, the ones this task is most likely to trip:

- **Shuttle Provider-Seam Invariant** — provider specifics live only under `internal/shuttleengine/claudeengine`, and `shuttleengine` never imports it.
  `internal/loomcli` composes a `Spec` and never a `claude` command line; nothing in this task may name a Claude flag outside `claudeengine`.
- **Shell Mechanics Seam** — pane-shell command strings are built only via `internal/shell`.
  This task builds none of its own; it is listed because a hand-composed launch line is the obvious wrong turn here.
- **Completion Signal Invariant** — every negative answer to "did this run finish" consults `allOutputFilesExist` first.
  The driver's output file is what makes this answerable for a driver run; the invariant's tripwire test pins the audited call sites, so any touch inside `wait.go`/`attach.go` fails loudly and deliberately.
- **Live-Substrate Spawn Observability** — the driver-strand launch starts a real OS process and must log its spawn via `internal/logger` at `Info`, the way the detached-driver spawn already does.
  Never re-exec `os.Executable()` under `go test`.
- **Cwd Resolution Invariant** — the seed path comes from a `shedrun` constructor over the bootstrap's own already-resolved `*lyxcwd.Location`; `loomcli` derives no path of its own and calls no resolver a second time.
- **Told-Geometry Invariant** — nothing new is handed to an engine unresolved; the report path and the run-id reach the spec as absolute/told values.
- **Durable-vs-Ephemeral State Invariant** — the drive report is ephemeral, under `.lyx/shed/<run-id>/`, at the mirrored subpath of the durable run directory.
- **Lyxdirs Single-Declarer Invariant** — the report path is composed from `shedrun`'s ephemeral accessor, never from a second `.lyx` literal.
- **Shed Verb-Set Invariant** — the `step` refusal-kind vocabulary stays closed at five; nothing in this task adds a sixth, and no driver-related refusal is raised inside a verb body.
- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `state`, `lock`.
  Nothing here adds an import there.
- **CLI / Cobra Invariant** — no new command is added, but `lyx loom start`'s `Long` text changes (the `--no-attach` paragraph and the step-5/6 description are driver-specific now), every `RunE` still checks `clihelp.ShouldAbort` first, and errors still go out through `internal/output`.
- **Batten Bookend Invariant** (the renamed Lifecycle one) — untouched, and must stay so: this task adds nothing that runs from inside a worktree batten creates or destroys.
- **Test Tier Purity Invariant** — no `gitexec.Run`, `exec.Command`, `gitkit.Copy*`, or `hubforge.NewHub` outside `integration`/`smoke`-tagged files, and no `time.Sleep` ≥ 1s untagged.
  The driver-launch seam must be injectable so the branch is testable at Tier 1.
- **Hermetic Git Test Environment Invariant** — any new test package spawning git calls `gitkit.HermeticGitEnv()` in `TestMain`.
- **Sandbox Suite Coverage** — the llm driver path takes the **explicit exclusion**, recorded in the suite document in the same commit, with this reason: exercising it end to end spawns a live, billed Claude session that runs for as long as the task takes, which a sandbox scenario cannot bound or assert against;
  the mechanics that are testable without one — the branch, the conjunction predicate, the spec composition, the strand count across relaunches — are covered by the Tier 1 and smoke tests below, against a stubbed binary.
  The `go` path's existing suite coverage is unchanged and still exercises the bootstrap around the branch.
- **Markdown Link Integrity** — `manifest/designs/seeded-shed.md`, `docs/overview.md`, and `manifest/roadmap.md` all change; the allowlist is keyed by `(file, target)`.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle`.

**New invariant to record in `CONSTRAINTS.md` in the same commit** — *Driver Choice Single-Site Invariant*, per the decision above.

**Sequencing constraint, not a code constraint:** this task's plan must not be finalized until `seeded-shed-core` has merged to `main`, and the branch must be rebased onto that merge before implementation starts.
Every symbol this task edits in `loomcli`, `shedrun`, and the batten packages is either created or moved by that task.

## Testing

Tier 1 (untagged, offline, fast) unless stated otherwise.

- **`internal/loomcli` (bootstrap decisions)** — the strongest TDD candidate, and the one to write first, because the whole task reduces to a branch and a predicate that are both pure.
  Cover: the driver branch selecting the detached spawn for `go`, for an absent/empty driver value, and the strand launch for `llm`; the three-way strand-liveness predicate (live → no spawn, absent → spawn, dead → remove-then-spawn); the driver-strand name constant being the same literal the lookup uses (a table test over add and lookup, the way the status and operator strand names are already pinned).
  Assert explicitly that **`awaitRunLock` is not reached on the `llm` path** — the handshake's absence there is a decision, and a refactor that "unifies" the two paths would silently reintroduce the race.
  The widened `mustSpawnDriver`, by contrast, *is* reached on both paths and gets its own truth table: spawn only when the lock is free and no driver strand is live, with the two mixed cases called out by name — a hand-started `lyx loom run` against an `llm` seed (lock held, no strand → no spawn) and a live ly-drive session between two steps (lock free, strand live → no spawn).
  Those two rows are the finding this predicate was widened for; a test covering only the two pure cases passes against the narrow version.
- **`internal/loomcli` (spec composition)** — the `shuttleengine.Spec` the driver launch builds: `Interactive` false, `ForkSubagents` false, `NameOverride` set to the driver-strand constant, exactly one `OutputFiles` entry under the run's ephemeral directory, and the model taken from the loom config's `driver` key.
  Do **not** assert anything about `Spec.Timeout`: nothing on this path reads it, so an assertion there would pin a dead field.
  The load-bearing one: **two launches in the same worktree produce two different report paths under a frozen clock** — a test that advances the clock between them proves nothing, since it is the same-second relaunch that the random component exists for, and a fixed path would pass every other test here.
  Pin that the prompt names the run-id, the report path, and the autonomous mode, and that it stays well under the 30000-byte cap (a prompt that grew into a copy of the skill would fail only at launch).
- **`internal/loomcli` (error paths)** — the bootstrap lock is released on every `llm`-branch failure: a config load failure, a `reed.Status()` error, a `RemoveStrand` failure, a `Runner.Start` failure.
  A leaked bootstrap lock wedges every subsequent `lyx loom start` in that worktree, and it is invisible until the second invocation.
- **`internal/loomengine`** — the one new config key: `driver` validated by `modelspec.Parse` only when non-empty, a bad spec failing at load time with the key named, an absent key loading cleanly, and the key present in `ConfigTemplate` so an existing worktree picks it up.
- **`internal/shuttleengine`** — `Spec.NameOverride` forwarding verbatim into the `AddSpec` `Start` builds, and being inert (no validation, no interpretation) exactly as `SessionID` is.
  Nothing else in the package changes; if a test in `wait.go`/`attach.go`'s completion-signal tripwire needs touching, that is a signal the change drifted out of scope.
- **`internal/shedrun` / the seeding paths** — `llm` now accepted where `seeded-shed-core` refused it: `lyx shed seed <run-id> --driver llm` for a `loom` recipe, and `lyx batten run|step <slug> --child-driver llm`.
  The one that must stay refused: **`--driver llm` on the batten path**, with a message naming the missing bootstrap verb rather than the (now shipped) roadmap item.
  A plan writer's likeliest regression is lifting all four refusals for symmetry.
- **Smoke (`smoke`-tagged)** — `lyx loom start --no-attach` in a worktree seeded `driver: llm` leaves exactly one strand under the driver name, against a stubbed `claude` binary; a second `--no-attach` invocation leaves exactly **one** (the re-entrancy property, which no Tier 1 test can prove against real reed); a third, after the stub's pane has exited, leaves one again and not a corpse plus a live pane.
  `internal/loomcli/smoke_operatorstrand_test.go` is the existing model for counting strands by display name.
- **Integration (`integration`-tagged)** — one end-to-end `driver: llm` bootstrap over a `hubforge`-built fixture with a stubbed driver that writes a report and exits: assert the run reaches a terminal state, the report file exists at the path the spec named, and `Runner.Start`'s `run.json` is persisted.
  The point is that the bootstrap returns without waiting for the driver, which is what makes an `llm` child watchable by `Run-Shed` at all.
- **`plugins/ly/skills/ly-drive`** — one Go test, plus a review.
  The test reads `SKILL.md` from the repo and asserts the step-cap constant's value appears in its cap sentence, so the SKILL.md literal and the Go-composed prompt cannot drift apart silently.
  The review is the part no test covers: the autonomous section against the four changes the decision enumerates, and the launch prompt naming the same skill invocation the section documents.
  A drift between the prompt and the skill is silent: the session simply does something else.

## Q&A log

- **Q:** Where does the `driver` branch live — batten's `Run-Shed` spawn seam, or the child's own `lyx loom start`? **A:** [auto-pick] `lyx loom start`, the recipe's own bootstrap verb. **Why:** a hand-started run must honour its seed too, the cwd resolution stays local to the worktree that owns the run, and batten's just-pinned row needs no change.
- **Q:** How is the Claude session launched — through `shuttleengine`, or a hand-composed command on `reedengine.AddStrand`? **A:** [auto-pick] through `shuttleengine` + `claudeengine`. **Why:** the launch line carries settings, permission posture, session id, model resolution and a resume line; hand-composing it breaks the Shuttle Provider-Seam and Shell Mechanics invariants on its first line.
- **Q:** Shuttle requires an output file per run. What is a driver run's? **A:** [auto-pick] a timestamped drive report under `.lyx/shed/<run-id>/`. **Why:** it satisfies the Completion Signal Invariant honestly rather than by exemption, and the timestamp is what lets a relaunch past the must-not-exist check.
- **Q:** Does the run-lock handshake apply to the llm driver? **A:** [auto-pick] no — it stays the `go` path's alone. **Why:** an ly-drive session holds the run lock only inside each step, so a handshake on it either races the Claude boot or observes a free lock between two healthy steps.
- **Q:** What replaces `mustSpawnDriver` for the llm path? **A:** [auto-pick] nothing replaces it — it is widened, taking the run lock and a live `loom-driver` strand together on both paths, with explicit corpse removal before a relaunch. **Why:** two drivers stepping one FSM must be impossible, and each signal is blind to exactly the driver kind the other sees; `IfAbsent` alone would additionally leak a shuttle run directory per re-entrant bootstrap and relaunch a stale launch line.
- **Q:** Does the driver role get a timeout knob like the other loom roles? **A:** [auto-pick] no — `driver` alone, no `driver_timeout_min`. **Why:** `Spec.Timeout` is read only by `Wait`, which this path never calls, so the key would configure nothing; the real bounds are the skill's 120-step budget and `Run-Shed`'s 12-hour watch.
- **Q:** Sandbox suite — exercise the llm bootstrap or exclude it? **A:** [auto-pick] exclude, with the reason recorded in the suite document. **Why:** an end-to-end run spawns a live billed Claude session of unbounded duration, which a sandbox scenario cannot bound or assert against; the stub-backed smoke tests cover the mechanics.
- **Q:** Should `lyx shed run|step` refuse when the seed says `llm`? **A:** [auto-pick] no, never. **Why:** those verbs are the manual-recovery path, and gating them would lock an operator out of their own run while adding a refusal shape to a vocabulary pinned closed at five.
- **Q:** Does `driver: llm` work for batten's own run too? **A:** [auto-pick] no — out of scope, refusal rewritten rather than lifted. **Why:** batten has no bootstrap verb to branch; its `go` driver is the process the operator typed, so there is nothing to replace.
- **Q:** Does this task flip any default to `llm`? **A:** [auto-pick] no. **Why:** the design's "expected default" is a config decision worth making after an llm-driven run has been watched end to end; the flags already exist, so flipping it later changes a default rather than a surface.
- **Q:** How does `ly-drive` behave with no operator to ask or report to? **A:** [auto-pick] a new Autonomous driver section in the same skill: no prompts, report to the output file, an own-budget step cap, stop conditions unchanged. **Why:** one skill, one driving policy — a second skill would drift, and putting the rules in a Go prompt literal would hide them from the skill's readers.
- **Q:** What happens when a driver strand dies mid-run? **A:** [auto-pick] nothing automatic — `Run-Shed` blocks on its budget and an operator re-runs `lyx loom start`. **Why:** detecting it means either widening `Run-Shed` across a worktree boundary or inventing a supervisor; the failure is bounded, visible, and has a one-command remedy, so it is recorded as an accepted residual instead.
