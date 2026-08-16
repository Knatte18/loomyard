// status.go defines the canonical Go type for _lyx/loom/status.json: loom's single source of truth for
// orchestration state, from the t=0 seed a spawn-time lyx command writes through to the fields loom
// rewrites on every phase-machine step.

// Package loomengine implements loom's Preflight precondition validator: the four checks (worktree
// geometry, worktree cleanliness, fabric readiness/sync, and _lyx/loom/status.json coherence) that must
// all pass before a task is fit to run.
//
// Callers MUST NOT invoke Preflight except when the task is at the fresh/preflight stage.
// Invoking it on an already-advanced task (non-empty history, set start_sha, …) is a caller error
// that will be reported as a half-finished precondition failure, not diagnosed as misuse, because
// Preflight is a stateless validator.
package loomengine

// Status is the canonical Go type for _lyx/loom/status.json: loom's single source of truth for
// orchestration state pinned by contracts/specs/loom-status-spec.md.
// The t=0 "seed" has only handoff fields populated;
// StartSha and NextAction are *string (nil for JSON null/absent) because they are optional;
// others are value types.
// The zero Status value is invalid.
type Status struct {
	Slug           string         `json:"slug"`
	Parent         string         `json:"parent"`
	Phase          string         `json:"phase"`
	Stage          string         `json:"stage"`
	Narration      string         `json:"narration"`
	History        []HistoryEntry `json:"history"`
	StartSha       *string        `json:"start_sha"`
	PauseRequested bool           `json:"pause_requested"`
	NextAction     *string        `json:"next_action"`
}

// HistoryEntry is one entry in Status.History: one record per phase attempt, including
// stuck-handler bounce-backs.
// BouncedTo is present only on "stuck" entries that route back to an earlier phase.
type HistoryEntry struct {
	Phase     string  `json:"phase"`
	Outcome   string  `json:"outcome"`
	BouncedTo *string `json:"bounced_to,omitempty"`
	Ts        string  `json:"ts"`
}
