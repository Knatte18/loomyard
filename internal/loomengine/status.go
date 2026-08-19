// status.go defines the canonical Go type for the "product" payload loom carries inside
// shedengine.Status.Product: the thin, loom-owned handoff pointers a spawn-time lyx command
// writes at t=0 and Shed round-trips verbatim thereafter, never inspecting or interpreting them.

// Package loomengine implements loom's Preflight precondition validator: the four checks (worktree
// geometry, worktree cleanliness, fabric readiness/sync, and _lyx/loom/status.json coherence) that must
// all pass before a task is fit to run.
//
// Callers MUST NOT invoke Preflight except when the task is at the fresh/preflight stage.
// Invoking it on an already-advanced task (non-empty history, set start_sha, …) is a caller error
// that will be reported as a half-finished precondition failure, not diagnosed as misuse, because
// Preflight is a stateless validator.
package loomengine

// Status is loom's product payload, carried verbatim inside shedengine.Status.Product per
// contracts/specs/loom-status-spec.md.
// Everything about the shell that carries it -- current_producer, state, error, activity, history,
// pause_requested -- is shedengine.Status's own concern, documented in internal/shedengine's own
// package documentation; this type is only the loom-specific half.
// StartSha is *string (nil for JSON null/absent) because it is optional; Slug and Parent are
// mandatory value types.
// The zero Status value is invalid.
type Status struct {
	Slug     string  `json:"slug"`
	Parent   string  `json:"parent"`
	StartSha *string `json:"start_sha"`
}
