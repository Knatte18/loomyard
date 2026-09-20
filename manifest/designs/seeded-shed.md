# Seeded Shed — one engine, N recipes, the seed decides

**Status:** landed (2026-09-19), as the `seeded Shed core: run addressing, seed contract, batten` roadmap item — the concept, run addressing, the seed contract, and the batten rename below all describe what shipped, not a proposal.
Two pieces from the original discussion are explicitly not part of this landing: `driver: llm` (see the Next Up `seeded driver choice: ly-drive strand as the child's driver` roadmap item) and relay-stepping (see [Rejected: relay-stepping](#rejected-relay-stepping) below).

## The concept

Shed is the only execution machinery.
A run is three things: a **host worktree** (where the FSM's state lives and its producers anchor their paths), a **run directory** holding that run's seed and status, and the **generic verbs** (`start`/`run`/`step`) that drive it.
The verb never knows what it starts — it reads the seed and looks the recipe up in the compiled-in registry.
"Loom", "batten", and eventually "Hardener" are recipe names, not modules with their own execution machinery or their own verbs.

What a seed selects is always a *registered* recipe: producer wiring is Go (perch segments, registry entries, capability seams), so a seed can pick among compiled-in FSMs but never define one.
This is the Shed Recipe Registry Invariant seen from the other side.

## Run addressing

Every run lives in a run directory named by a **run-id**: the durable `_lyx/shed/<run-id>/` holds `seed.json` and `status.json`; the run's own locks live at the mirrored ephemeral subpath, `.lyx/shed/<run-id>/`, per the Durable-vs-Ephemeral State Invariant — a lock is machine-local advisory state, never something a fresh clone on another machine needs to see.
The generic verbs take the run-id as an optional argument: `lyx shed run <run-id>`, `lyx shed step <run-id>`.

- **Default run-id is the literal `self`.**
  `lyx shed run` with no argument means `_lyx/shed/self/` — the worktree's own primary run, which is what Seed-Child (below) always writes for a task worktree.
  If `self` is absent the verb refuses with a list of the run-ids that do exist; it never guesses by scanning, because guessing is how the wrong run gets resumed.
- **`seed.json` and `status.json`, always durable: `_lyx/shed/<run-id>/`.**
  Every run's directory is weft-tracked, fabric-synced state, like loom's status file today — a worktree recreated on another machine (the mill-resume pattern) must still know what it is running.
  This deliberately includes management runs in the hub's prime worktree, which an earlier draft placed ephemerally (`.lyx/shed/`, like `.lyx/lifecycle/<slug>/` today) on the argument that the Board is the durable truth they derive from.
  That argument only covers *what* should run; it says nothing about *how far* a run has come.
  On a machine switch mid-run, the Board says a task is active, but only a durable batten status in prime says the worktree is already created, the child already seeded, `Run-Shed` already in flight — without it, resuming means reconstructing that from branch inspection.
  The cost is that batten's rows must self-heal machine-local resources a durable status promises but a cold machine lacks (recreate the child worktree from its branch before watching it) — the same pattern `loom start` already applies to a cold task worktree.

The run-id replaces `lyx lifecycle run <slug>`'s positional argument (in prime, run-id = task slug), and the seed's params absorb `lyx loom start --parent` — both were flagged by review as loose ends of the verb extraction, and this is their landing place.

## The seed

`seed.json` carries the run's startup choices:

- `recipe` — which registered recipe this run executes (`loom`, `batten`, later `hardener`).
- `driver` — who steps the FSM: `go` (a detached runner process, today's `lyx loom run`) or `llm` (a Claude session running the ly-drive skill, as an interactive tmux strand via reed).
- params — per-recipe startup values, e.g. the parent branch that is a CLI flag today, or the child slug for a batten run.

`internal/shedrun` is the sole declarer of the closed `recipe` vocabulary, alongside the `driver` vocabulary it already owns; `internal/shedcli`'s own name-to-arming-function table is pinned against it by a sync meta-test rather than the other way around, because the natural seeding site (`internal/battencli`, which writes a child's seed) already imports `internal/shedcli`, so a reverse import from `shedcli` into `battencli`'s validation would be a cycle.

The seed is the run's *recorded* startup choice, not the durable truth about the task.
The durable truth lives on the Board: a task entry carries a **type** field naming the inner recipe (default `loom`), and whoever seeds a run copies from it.

Drivers are chosen per level and live in their own host worktree's reed session.
Reed sessions are per-worktree, so an LLM driving a batten run in prime and an LLM driving a loom run in the child can never clutter each other.
The expected defaults: `llm` for task-work runs (the whole point of ly-drive is an intelligence that can fix what a mechanical gate cannot), `go` for batten runs (five mechanical rows with deterministic outcomes — a failure there becomes `Stuck`, which is a better escalation point than a watching LLM).

## One FSM per worktree

A run's producers anchor every path at the run's own host worktree (the Cwd Resolution Invariant), so **a single recipe can never span two worktrees**.
Composition across a worktree boundary is by reference: the parent writes the child's seed and reads the child's status file, and nothing else crosses.

This is why the tempting flattening — one long recipe of `Worktree-Create` + all seventeen loom rows + `Worktree-Teardown` — was rejected:

- loom's rows resolve `_lyx` paths, spawn tmux strands, and commit fabric pairs *in the child*; running them from prime would mean threading a target-worktree parameter through every producer and seam.
- loom's status is durable and fabric-synced with the *child's* pair; flattened, a task's progress would stop being a property of the task worktree itself.
- a half-run task worktree can resume standalone today (`loom start` self-heals); flattened, progress would exist only at the outer level.

The flatness worth having is the driver's view, and that survives: an LLM stepping the outer run sees create → seed → the child's steps → teardown as one linear stream of envelopes.

## Batten — the outer recipe

The recipe today named lifecycle is renamed **batten**: in a loom, the batten is the swinging frame that carries the reed — and this recipe is literally what raises the child's reed session into place and lowers it again.
Its rows:

1. `Worktree-Create` — fabric only, no seeding (unchanged).
2. `Seed-Child` — new: writes the child's `_lyx/shed/self/seed.json`, copying the recipe choice from the Board task's type field and the driver choice from batten's own seed params.
3. `Run-Shed` — the row named `Loom-Run` today, made product-neutral: spawn the child's run per the child's seed, then watch the child's status file to a terminal state.
   For `driver: llm` the spawn boots a reed strand running ly-drive instead of the detached Go runner — one changed command in the existing Spawn seam, nothing else, because status-watching is driver-agnostic.
   The row is self-routed on `Stuck` (`on_stuck: Run-Shed`) with a row-level `max_bounces: 1440` and a `config: {poll_interval_s: 30}` pair, encoding a 12-hour watch window as one poll per bounce rather than one blocking `Run` call held for the child's whole duration — the property a step-driven outer run needs. Every non-running, non-done child state is a hard error, not a bounce: only "the child is still running" re-enters, and shedengine turns the hard-error return into `StateFailed` at this row with the run halted, never routing on to `Worktree-Teardown`.
4. `Worktree-Teardown` — session shutdown before worktree removal (unchanged).

Optional comfort rows can come later: launching VS Code into the child after seeding, and closing it before teardown — the launcher-variant half of this is described in the VS Code opt-in discussion (see `internal/fabricengine/launchers.go`).

### Durable row identity

`Loom-Run` is a durable on-disk identity: `status.json`'s `current_producer` names it, and the recipe header pins the constant and the YAML against each other.
Today's lifecycle run state is per-machine and ephemeral (`.lyx/lifecycle/`), so the rename to `Run-Shed` (and the recipe rename) lands whenever no such runs are in flight — a coordination point, not a migration.
Once batten's state moves to the durable tree, its row names are pinned the same way loom's are, so the rename must land no later than that move.
Engine names, deps structs, and log strings carry no on-disk identity and rename freely.

## Rejected: relay-stepping

A variant considered and set aside: `Run-Shed` in step mode executes `lyx shed step` in the child as a subprocess (with `cmd.Dir` set to the child's anchor, the same mechanism the Spawn seam already uses), relaying one outer step into one inner step.
Mechanically sound, and it would hide the two levels from the driver entirely.
Rejected as the primary mode because it puts the driving intelligence *outside* the worktree: the LLM should live inside the child's tmux session with the task's full context, exactly as if an operator had started ly-drive there by hand — started from outside, but not run from outside.
The envelope contract (`continue`, closed refusal kinds) makes either mode cheap to drive, so this stays a possible future mode, not a foreclosed one.

## What collapses, what stays

- `loomcli` and `lifecyclecli` shrink toward bindings over the generic verbs; "loom" survives as a recipe name and a directory of producers, not as a verb owner.
- `shedengine`/`shedbuild`/`shedrecipe`, the status contract, the envelope contract, and the perch segments are untouched — this design adds addressing and a seed contract *around* them, not new machinery *in* them.
- `.lyx/lifecycle/` goes away entirely: the per-run directory shape generalizes into the single durable `_lyx/shed/` tree, and with it both the product-named segment and the durable-vs-ephemeral split disappear.

## Depends on

The generic verb set from **generalize `ly-drive` and loom's `start`/`run`/`step` CLI verbs into a Shed-generic watchdog** (see [shed-generic-watchdog.md](shed-generic-watchdog.md)) is the substrate everything above binds to.
