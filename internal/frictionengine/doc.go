// Package frictionengine implements the Tier 2 aggregation-and-reflection step: it scans a task's
// friction directory for notes left behind by the agents that ran before it, and, when there is
// something to reflect on, spawns exactly one autonomous reflection agent over the aggregated notes
// through a narrow Shuttle seam.
//
// Zero notes means no agent is spawned at all. A clean run produces no friction notes and is the
// common case, so spawning a full LLM session to conclude "nothing to report" on every clean run
// would be pure cost for a step whose entire purpose is optional bookkeeping.
//
// A failed reflection leaves the friction notes in place, never archived. A failure means nothing
// was reflected on and nothing was filed, so archiving would discard the run's un-acted-on signal
// while creating no duplicate-filing risk to avoid, and the next trigger in the same task must
// reflect on those same notes again.
//
// The agent's own report file (internal/friction.ReportFileName) is excluded from the note scan, so
// a stale report left behind by a timed-out run is never mistaken for a fresh note on the next scan,
// and is deleted before a new spec is composed so shuttleengine.Spec's own OutputFiles-must-not-
// already-exist rule can never reject the very run meant to replace it.
//
// Every runtime failure below Deps validation returns a nil error: this step can never change the
// run's own outcome, because failing a successful, already-merged run over an optional bookkeeping
// agent timing out would be strictly worse than filing nothing. The one exception is a malformed
// Deps, which is a wiring bug at the one call site and is surfaced loudly as a non-nil error.
package frictionengine
