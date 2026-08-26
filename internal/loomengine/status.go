// status.go defines the canonical Go type for the "product" payload loom carries inside
// shedengine.Status.Product: the thin, loom-owned handoff pointers a spawn-time lyx command
// writes at t=0 and Shed round-trips verbatim thereafter, never inspecting or interpreting them.

// Package loomengine implements loom's own seed-coherence check, CheckSeed: one of the four
// preconditions a task must meet before it is fit to run, with the other three now
// internal/preflight's orchestrator-agnostic tier-1/tier-2 checks (worktree geometry, worktree
// cleanliness, fabric readiness/sync). CheckSeed validates .lyx/loom/status.json coherence over told
// absolute paths, reporting a determined verdict rather than erroring on anything short of an infra
// failure.
//
// Invoking the seed check on an already-advanced task (non-empty history, set start_sha, …) is a
// caller error that will be reported as a half-finished precondition failure, not diagnosed as
// misuse, because the check is a stateless validator.
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
