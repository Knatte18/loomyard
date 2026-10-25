# PATTERN-shed-driver

`internal/sheddriver` is the one Go driver of shed runs: it steps a run to its stops, decides each stop by a closed rule table, spawns the tackler for a stop the table does not settle, and hands back what it cannot.
No `<module>cli` and no producer runs a step-until-stop loop of its own.
That second clause is review discipline, not a scan, as the verb set's first clause below is.

## Told geometry

- `sheddriver` is a told-geometry engine: it takes every path from its caller and reaches recipe behaviour only through `Deps` and `TacklerDeps`.
- It imports no `lyxcwd`, `loomengine`, `loomshed` or `internal/*cli` package, and its other imports are a closed allowlist.
- `shedrun` is on that list for the driver-kind vocabulary and its validator only, since every `shedrun` path constructor takes a `*lyxcwd.Location` the package can never hold.
- Enforced by `internal/sheddriver/seam_enforcement_test.go`.

## Seams a recipe fills

The arming module fills `Deps` and `TacklerDeps`; the optional groups are the recipe-specific part.

- `Arbiter`, `Verify`, `HandOver` and `Tamper` are optional groups on `Deps`, and `Watched`, `CommitRecords` and `WriteFrictionNote` optional closures.
- `TacklerDeps.Snapshot` and `GotoRoutedFrom` shape the tackler's tamper comparison and the rows its resume may name.
- A nil group or closure only removes rows or agents from the rule table.
  A stop a removed row would have caught meets the later rows unchanged, so it ends in the tackler under its spawn budget, a re-step under the repair cap, or a hand-back.
  No unwired seam adds a resume path.

## Outside it

Batten's InnerRun checks a child run it does not step once per pass, yields when the check ends nothing and keeps its memory between passes in a per-run record, so it is not a step loop and stays in `internal/battenshed`.
A yield is logged at Debug.

## The verb set

`internal/shedverbs` owns the generic `run`, `step`, `status`, `pause` and `goto` verb bodies; no `<module>cli` reimplements one.
That first clause is review discipline, not a scan: "reimplements" has no static shape a scan can see.
A recipe's `run` verb may be `internal/sheddriver`'s body, which steps through `shedverbs`' `--until-stop` and goto bodies and restates neither.

### No derived paths

- `shedverbs` derives no path and imports no resolver — no `lyxcwd`, no `os.Getwd`, no `git rev-parse` — and imports no `<module>cli`.
  That keeps it a leaf and keeps `internal/shedcli`'s own imports acyclic.
- `shedverbs` does import `internal/logger`, for step-boundary logging, and `logger.TraceFile` and `logger.TraceDir` are the only path sources admitted into it.
  They are not derived paths here: each returns the logger's own sink location, which `shedverbs` reports verbatim on the envelope and never joins, reads, writes or uses to locate run state.
- Every run-state path (`StatusPath`, `ScratchDir`, `FrictionDir`, …) stays told through `Spec`, and the `lyxcwd` import stays denied.
- The detached loop and its waiter behind `step --until-stop` live in `internal/shedverbs` (`loop.go`, `loopwait.go`, `looppid.go`, `loopenvelope.go`).
  Its loop paths reach it told through `Spec.Loop`, and a child's trace files through the told `TraceFiles` and `StopFiles` closures; the loop reads and copies only the paths they tell it, while `logger.TraceFile` and `logger.TraceDir` stay reported verbatim.
- Its non-standard-library imports are an allowlist: `buildvcs`, `clihelp`, `fswatch`, `lock`, `logger`, `output`, `proc` (the child record type, the tied start, the tree kill and the named job), `shedengine`, `state` and `cobra`.
- Enforced by `internal/shedverbs/seam_enforcement_test.go`.

### Recipe table

The `lyx shed` recipe table lives in `internal/shedcli` alone, as one map literal reached through accessors, with every name armed by exactly one arming function and no `init()` self-registration.
Enforced by `internal/shedcli/table_test.go`.

### Step envelope

- The `step` refusal-kind vocabulary stays closed at its six values; `KindInterrupted` is the one the loop emits, for a child step that ended without an envelope.
- The full step envelope's key set is closed by doc comment and test, and is the one the step record holds.
  Stdout prints a short envelope by default, whose two closed key sets (success and error) are stated in `internal/shedverbs`; `--full`, or a record that could not be written, prints the full envelope instead.
  The full envelope carries `trace_file`, `friction_dir`, `scratch_dir`, `trace_id` and `run_id` on the success and every error envelope.
  The success envelope and every error envelope also carry `friction` (the `AfterStep` hook's status, or empty).
  Every error envelope also carries `transient` (the transient class name, or empty; it is a key, not a sixth kind).
  `run`'s error envelope carries a module's `PostRun` extras, and the status envelope carries `trace_dir` on every envelope.
- Enforced by `internal/shedverbs/step_test.go` for the full envelope and `internal/shedverbs/steprecord_test.go` for the short ones.

### goto

`goto` is a history-only outcome: `internal/shedengine` declares it (`OutcomeGoto`), no producer returns it, and every reader that validates history outcomes accepts it.

### Layering

- `internal/shedverbs` is not itself a CLI module and is not counted in the cli-cobra entry's tally.
  It exposes no `Command()` or `RunCLI` seam, only the `Verbs(texts, spec)` constructor the three subtrees build from.
- Neither `shedverbs` nor `shedcli` is added to the told-geometry bound-packages list in `PATTERN-cwd-resolution`, which binds engines; both sit above that layer.
- `shedverbs` keeps this section's no-resolver clause verbatim as its no-derived-paths obligation.
- `internal/shedcli` is carved out of that clause by name, as the one site in this pair that resolves.
  `resolvePersistentPreRun` must read a seed before it knows which recipe to arm, a seed read is a path read, and a path read needs an anchor, so `shedcli` calls `lyxcwd.Resolve` and builds seed paths from the result.
  Its narrower obligation is that every path it touches comes from an `internal/shedrun` constructor and none is derived locally.

## The run directory

`internal/shedrun` is the sole declarer of the `shed` path segment and of the run-id vocabulary including the literal `self`, and the sole parser and writer of `seed.json`.

- No other production file names the `shed` segment or the `seed.json` filename in path-construction context.
- No other package decodes or encodes the `Seed` struct.
- A run-id is validated as a single path segment (`ValidateRunID`) before being joined onto any anchor.

### The `self` alias

- `self` is an alias only `shedrun.ResolveRunID` interprets.
  It maps to the told location's worktree slug (`l.WorktreeName`), and a run's directory is named by that slug.
- One exception: a read-only legacy fallback in `shedrun`'s directory-segment resolution.
  When `_lyx/shed/<slug>/` is absent and `_lyx/shed/self/` exists, both spellings join `self`, so a run started before the rename keeps working with no on-disk migration.
- No other package resolves `self`, and none joins `SelfRunID` onto a path itself.

### Forked branches

A freshly forked weft branch never carries its parent's shed run records.
`fabricengine.Add` drops the root `shedrun.RunsRootRel` names in the pair's first weft commit, and the adopt path drops nothing.
