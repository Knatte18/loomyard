// Package alignment is the alignment agent's records, prompt and launch spec: the cold session that settles loom's parent review and PR-Gate in the run's place, or escalates to the run's parent.
//
// The agent reads the board, the other in-flight runs' records through `lyx loom peers` and the parent's landed commits.
// It approves, or rejects on a scope deviation, unless it finds a substantive overlap with another run or a question it cannot settle, which it escalates.
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
// # Decision snapshot
//
// `Snapshot` hashes the decision record and the recorded notes, so a decision recorded after an attempt spawned is detectable when its verdict arrives.
//
// # Told geometry
//
// The package takes every absolute path it works on from its caller and never imports `internal/lyxcwd`; the allowlist test pins its imports.
// The agent's stencil is read at call time from the told stencils directory.
package alignment
