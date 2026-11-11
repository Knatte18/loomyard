# PATTERN-shed-driver

This cluster guards how a shed run is driven and stopped.
Verbs live in `shedverbs`, the loop in `sheddriver`, and paths in `shedrun`;
shared machinery lives in shed, with one stop chain and transient marks only via `shedengine`.

## Driver

Rule: Driving a shed run: generic verb bodies only in `internal/shedverbs`, the step-until-stop loop only in `internal/sheddriver`, run paths, ids and `Seed` only in `internal/shedrun`. (test)

`internal/sheddriver` is the one Go driver of shed runs.
It steps a run to its stops and decides each stop by a closed rule table.
It spawns the tackler for a stop the table does not settle, and it hands back what it cannot.
No `<module>cli` and no producer runs a step-until-stop loop of its own.
That second clause is review discipline, not a scan, as the verb set's first clause below is.

### Told geometry

- `sheddriver` is a told-geometry engine:
  it takes every path from its caller and reaches recipe behaviour only through `Deps` and `TacklerDeps`.
- It imports no `lymcwd`, `loomengine`, `loomshed` or `internal/*cli` package.
  Its other imports are a closed allowlist.
- `shedrun` is on that list for the driver-kind vocabulary and its validator only.
  Every `shedrun` path constructor takes a `*lymcwd.Location`, which the package can never hold.
- Enforced by `internal/sheddriver/seam_enforcement_test.go`.

### Seams a recipe fills

The arming module fills `Deps` and `TacklerDeps`; the optional groups are the recipe-specific part.

- `Arbiter`, `Verify`, `HandOver` and `Tamper` are optional groups on `Deps`.
  `Watched`, `CommitRecords` and `WriteFrictionNote` are optional closures.
- `TacklerDeps.Snapshot` and `GotoRoutedFrom` shape the tackler's tamper comparison and the rows its resume may name.
- A nil group or closure only removes rows or agents from the rule table.
  A stop a removed row would have caught meets the later rows unchanged.
  It ends in the tackler under its spawn budget, a re-step under the repair cap, or a hand-back.
  No unwired seam adds a resume path.

### Outside it

Batten's InnerRun checks a child run it does not step, once per pass, and yields when the check ends nothing.
It keeps its memory between passes in a per-run record, so it is not a step loop and stays in `internal/battenshed`.
A yield is logged at Debug.

### The verb set

`internal/shedverbs` owns the generic `run`, `step`, `status`, `pause` and `goto` verb bodies;
no `<module>cli` reimplements one.
That first clause is review discipline, not a scan: "reimplements" has no static shape a scan can see.
A recipe's `run` verb may be `internal/sheddriver`'s body.
That body steps through `shedverbs`' `--until-stop` and goto bodies and restates neither.

#### No derived paths

- `shedverbs` derives no path and imports no resolver: no `lymcwd`, no `os.Getwd`, no `git rev-parse`.
  It also imports no `<module>cli`, which keeps it a leaf and keeps `internal/shedcli`'s own imports acyclic.
- `shedverbs` does import `internal/logger`, for step-boundary logging.
  `logger.TraceFile` and `logger.TraceDir` are the only path sources admitted into it.
  They are not derived paths here: each returns the logger's own sink location.
  `shedverbs` reports it verbatim on the envelope and never joins, reads, writes or uses it to locate run state.
- Every run-state path (`StatusPath`, `ScratchDir`, `FrictionDir`, …) stays told through `Spec`,
  and the `lymcwd` import stays denied.
- The detached loop and its waiter behind `step --until-stop` live in `internal/shedverbs`:
  `loop.go`, `loopwait.go`, `loopkeeper.go`, `looppid.go` and `loopenvelope.go`.
  Its loop paths reach it told through `Spec.Loop`.
  A child's trace files reach it through the told `TraceFiles` and `StopFiles` closures.
  The loop reads and copies only the paths they tell it.
  `logger.TraceFile` and `logger.TraceDir` stay reported verbatim.
- Its non-standard-library imports are an allowlist:
  `buildvcs`, `clihelp`, `fswatch`, `lock`, `logger`, `output`, `proc`, `shedengine`, `state` and `cobra`.
  From `proc` it takes the child record type, the tied start, the tree kill and the named job.
- Enforced by `internal/shedverbs/seam_enforcement_test.go`.

#### Recipe table

The `lym shed` recipe table lives in `internal/shedcli` alone, as one map literal reached through accessors.
Every name is armed by exactly one arming function, and there is no `init()` self-registration.
Enforced by `internal/shedcli/table_test.go`.

#### Step envelope

- The `step` refusal-kind vocabulary stays closed;
  `KindInterrupted` is the one the loop emits, for a child step that ended without an envelope.
- The full step envelope's key set is closed by doc comment and test, and is the one the step record holds.
  Stdout prints a short envelope by default.
  Its success and error key sets are closed and stated in `internal/shedverbs`.
  `--full`, or a record that could not be written, prints the full envelope instead.
  The full envelope carries these keys on the success and every error envelope:
  `trace_file`, `friction_dir`, `scratch_dir`, `trace_id` and `run_id`.
  The success envelope and every error envelope also carry `friction` (the `AfterStep` hook's status, or empty).
  Every error envelope also carries `transient` (the transient class name, or empty; it is a key, not a kind).
  `run`'s error envelope carries a module's `PostRun` extras,
  and the status envelope carries `trace_dir` on every envelope.
- `internal/shedverbs/step_test.go` enforces the full envelope.
  `internal/shedverbs/steprecord_test.go` enforces the short ones.

#### goto

`goto` is a history-only outcome.
`internal/shedengine` declares it (`OutcomeGoto`) and no producer returns it.
Every reader that validates history outcomes accepts it.

#### Layering

- `internal/shedverbs` is not itself a CLI module and is not counted in the cli-cobra entry's tally.
  It exposes no `Command()` or `RunCLI` seam, only the `Verbs(texts, spec)` constructor the subtrees build from.
- Neither `shedverbs` nor `shedcli` is added to the told-geometry bound-packages list in `PATTERN-cwd-resolution`.
  That list binds engines, and both packages sit above that layer.
- `shedverbs` keeps this section's no-resolver clause verbatim as its no-derived-paths obligation.
- `internal/shedcli` is carved out of that clause by name, as the one site in this pair that resolves.
  `resolvePersistentPreRun` must read a seed before it knows which recipe to arm.
  A seed read is a path read, and a path read needs an anchor.
  So `shedcli` calls `lymcwd.Resolve` and builds seed paths from the result.
  Its narrower obligation is that every path it touches comes from an `internal/shedrun` constructor.
  None is derived locally.

### The run directory

`internal/shedrun` is the sole declarer of the `shed` path segment and the run-id vocabulary.
That vocabulary includes the literal `self`.
It is also the sole parser and writer of `seed.json`.

- No other production file names the `shed` segment or the `seed.json` filename in path-construction context.
- No other package decodes or encodes the `Seed` struct.
- A run-id is validated as a single path segment (`ValidateRunID`) before being joined onto any anchor.

#### The `self` alias

- `self` is an alias only `shedrun.ResolveRunID` interprets.
  It maps to the told location's worktree slug (`l.WorktreeName`), and a run's directory is named by that slug.
- One exception: a read-only legacy fallback in `shedrun`'s directory-segment resolution.
  When `_lym/shed/<slug>/` is absent and `_lym/shed/self/` exists, both spellings join `self`.
  A run directory under the `self` spelling keeps working with no on-disk migration.
- No other package resolves `self`, and none joins `SelfRunID` onto a path itself.

#### Forked branches

A freshly forked weft branch never carries its parent's shed run records.
`fabricengine.Add` drops the root `shedrun.RunsRootRel` names in the pair's first weft commit,
and the adopt path drops nothing.

## Stop chain

Rule: A shed run's stop: rule table, tackler, arbiter, then parent, for every recipe; a stop the driver's arbiter seam marks as judged first, by row and reason, skips the tackler; arbiter keys in shed.yaml, batten's overridable in batten.yaml, a loom recipe's own via its profile block.

Held by review alone; no test enforces it.

## Transient stop

Rule: Marking a failure transient: only `internal/shedengine`'s mark, translated by `internal/shedtransient` at a producer or step-bootstrap boundary; never on a gate verdict.

Held by review alone; no test enforces it.

## Shared machinery

Rule: Adding what every phase machine needs: once, in shed's packages, never in one recipe's or module's own package.

Held by review alone; no test enforces it.
