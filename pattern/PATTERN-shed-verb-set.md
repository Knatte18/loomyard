# PATTERN-shed-verb-set

`internal/shedverbs` owns the generic `run`, `step`, `status`, `pause` and `goto` verb bodies; no `<module>cli` reimplements one.
That first clause is review discipline, not a scan: "reimplements" has no static shape a scan can see.

## No derived paths

- `shedverbs` derives no path and imports no resolver — no `lyxcwd`, no `os.Getwd`, no `git rev-parse` — and imports no `<module>cli`.
  That keeps it a leaf and keeps `internal/shedcli`'s own imports acyclic.
- `shedverbs` does import `internal/logger`, for step-boundary logging, and `logger.TraceFile` and `logger.TraceDir` are the only path sources admitted into it.
  They are not derived paths here: each returns the logger's own sink location, which `shedverbs` reports verbatim on the envelope and never joins, reads, writes or uses to locate run state.
- Every run-state path (`StatusPath`, `ScratchDir`, `FrictionDir`, …) stays told through `Spec`, and the `lyxcwd` import stays denied.
- Enforced by `internal/shedverbs/seam_enforcement_test.go`.

## Recipe table

The `lyx shed` recipe table lives in `internal/shedcli` alone, as one map literal reached through accessors, with every name armed by exactly one arming function and no `init()` self-registration.
Enforced by `internal/shedcli/table_test.go`.

## Step envelope

- The `step` refusal-kind vocabulary stays closed at its six values; `KindInterrupted` is the one the loop emits, for a child step that ended without an envelope.
- The full step envelope's key set is closed by doc comment and test, and is the one the step record holds.
  Stdout prints a short envelope by default, whose two closed key sets (success and error) are stated in `internal/shedverbs`; `--full`, or a record that could not be written, prints the full envelope instead.
  The full envelope carries `trace_file`, `friction_dir`, `scratch_dir`, `trace_id` and `run_id` on the success and every error envelope.
  The success envelope and every error envelope also carry `friction` (the `AfterStep` hook's status, or empty).
  Every error envelope also carries `transient` (the transient class name, or empty; it is a key, not a sixth kind).
  `run`'s error envelope carries a module's `PostRun` extras, and the status envelope carries `trace_dir` on every envelope.
- Enforced by `internal/shedverbs/step_test.go` for the full envelope and `internal/shedverbs/steprecord_test.go` for the short ones.

## goto

`goto` is a history-only outcome: `internal/shedengine` declares it (`OutcomeGoto`), no producer returns it, and every reader that validates history outcomes accepts it.

## Layering

- `internal/shedverbs` is not itself a CLI module and is not counted in the cli-cobra entry's tally.
  It exposes no `Command()` or `RunCLI` seam, only the `Verbs(texts, spec)` constructor the three subtrees build from.
- Neither `shedverbs` nor `shedcli` is added to the told-geometry entry's bound-packages list, which binds engines; both sit above that layer.
- `shedverbs` keeps this entry's no-resolver clause verbatim as its no-derived-paths obligation.
- `internal/shedcli` is carved out of that clause by name, as the one site in this pair that resolves.
  `resolvePersistentPreRun` must read a seed before it knows which recipe to arm, a seed read is a path read, and a path read needs an anchor, so `shedcli` calls `lyxcwd.Resolve` and builds seed paths from the result.
  Its narrower obligation is that every path it touches comes from an `internal/shedrun` constructor and none is derived locally.
