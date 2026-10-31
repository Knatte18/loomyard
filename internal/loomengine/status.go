// status.go defines the canonical Go type for the "product" payload loom carries inside
// shedengine.Status.Product: the thin, loom-owned handoff pointers a spawn-time lym command
// writes at t=0 and Shed round-trips verbatim thereafter, never inspecting or interpreting them.

// Package loomengine holds loom's own engine pieces: its status product payload, the guard that refuses a status file inherited from another task, the loom and tapestry config modules, and the discussion, plan and review tables a loom run reads.
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
