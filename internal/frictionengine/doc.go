// Package frictionengine implements the Tier 2 aggregation-and-reflection step.
// It scans a task's friction directory for notes left behind by the agents that ran before it.
// When there is something to reflect on, it spawns exactly one autonomous reflection agent over the aggregated notes.
// The spawn goes through a narrow Shuttle seam.
//
// Zero notes means no agent is spawned at all.
//
// Reflect is re-entrant across a killed driving process.
// Before it spawns, it writes a covered-notes record naming the notes that reflection covers.
// A later call settles a prior reflection first, when a record exists:
// it waits on a live agent;
// with no live agent and the report present it archives the covered files without spawning;
// with no live agent and no report it discards the record, and the notes are reflected again.
// The agent writes the report only after filing succeeded, so a report is what licenses an archive without a spawn.
// A probe error, an attached run that does not finish and a spawn that does not finish are failures:
// the notes, the record and the report stay in place, and the next trigger settles or reflects again.
// One call spends at most one positive Deps.Timeout, attach wait and spawn together, measured through Deps.Clock;
// a zero Timeout defers each spec to shuttle.run_timeout_min.
//
// A successful reflection archives only covered files into a timestamped sibling directory.
// The friction directory itself is never renamed or removed,
// so a note written after the record stays behind for the next reflection.
//
// The agent's own report file (internal/friction.ReportFileName) is excluded from the note scan.
// With no record, a leftover report is stale and is deleted before a new spec is composed,
// so the rule of shuttleengine.Spec that OutputFiles must not already exist cannot reject the run meant to replace it.
//
// The reflection agent's stencil carries the parent directive.
// internal/parentdirective renders it from the told Deps.ParentName.
//
// Every runtime failure below Deps validation returns a nil error:
// this step never changes the run's own outcome.
// The one exception is a malformed Deps, which is a wiring bug at the one call site and is surfaced as a non-nil error.
package frictionengine
