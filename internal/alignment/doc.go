// Package alignment is the alignment agent's records, prompt and launch spec: the cold session that settles loom's parent review and PR-Gate in the run's place, or escalates to the run's parent.
//
// The agent settles a run's parent review and its PR-Gate.
// What it measures a run against at `pr-gate`, and what counts there as a scope deviation, is a told fragment named by `PromptInput.MeasureStencil`, so the stencil names no recipe row.
//
// The agent reads the board, the other in-flight runs' records through `lyx loom peers` and the parent's landed commits.
// It approves, or rejects on a scope deviation, unless it finds a design conflict with another run or a question it cannot settle, which it escalates.
// An overlap that is no design conflict is not escalated: the agent records its landing order, the run furthest along by its recipe row landing first.
// Go applies the verdict through the stores the existing review and approval verbs write, and every failure falls back to the path that existed before the agent.
//
// # Points
//
// The agent runs at one of two points of a run: `parent-review`, where the writer's discussion awaits the parent's verdict, and `pr-gate`, where the run's pull request awaits the parent's approval.
// A point is a single path segment, and a point's attempts live under the run's alignment directory, `shedrun.AlignmentDir`.
//
// # Attempts
//
// One spawn is an attempt, a numbered `attempt-<N>` directory.
// `attempt.json` is written before the spawn, so a restarted driver finds the attempt and reuses it under the same idempotence key instead of spawning a second agent.
// The directory also holds `prompt.md` and the agent's two output files, whose names carry the attempt's random token: `verdict-<token>.yaml` and `report-<token>.md`.
// The token appears only in the agent's own prompt, so no other session can forge the output.
//
// # Verdicts
//
// The verdict file is the agent's result, written last, after its report.
// `ReadVerdict` accepts three verdict words (approve, reject and escalate) and refuses a file that does not parse, an unknown key, an empty reason, an escalation with no question, a `superseded` list on anything but an approve, and a missing or empty report.
//
// # Session
//
// A `Session` runs one point's attempts over a `Shuttle`, the slice of shuttle it drives, and never blocks its caller, since the parent-review gate polls it on each re-evaluation.
// The `Settings` a spawn takes carry a model choice, engine included, beside the run's timeout.
// `Begin` spawns an agent for an idempotence key (`round-<N>` at parent review, `pr-<number>-<head>` at PR-Gate) and returns the attempt that already holds the key without starting anything, closed or not, so a driver restart never spawns a second agent for one round or one head.
// It writes `attempt.json` before the spawn and clears any file at the new attempt's output paths, so only a verdict written after the spawn can be read.
// A failed start closes the attempt with the error as its reason.
//
// `Poll` re-reads the record and reports pending, or done with the agent's valid verdict, or done with a cause: `CauseInvalid`, `CauseTimeout`, `CauseDied`, `CauseMissing` or `CauseClosed`.
// A closed attempt's verdict file is never read, so a verdict written after a fallback, a parent verdict, a pause or a goto is dropped.
// A background wait on the attempt's run stores its result for the next `Poll`;
// after a driver restart, which holds no such wait, `Poll` probes once for a live run and waits on it, or reads `CauseDied` when none is live.
// `Close` stops a strand that may still be live and marks the attempt closed, and `MarkApplied` records the verdict Go applied.
//
// # Decision snapshot
//
// `Snapshot` hashes the decision record and the recorded notes, so a decision recorded after an attempt spawned is detectable when its verdict arrives.
//
// # Told geometry
//
// The package takes every absolute path it works on from its caller and never imports `internal/lyxcwd`; the allowlist test pins its imports.
// The agent's stencil is read at call time from the told stencils directory.
package alignment
