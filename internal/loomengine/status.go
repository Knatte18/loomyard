// status.go defines Status, the loom-owned product payload that shedengine.Status carries verbatim.
// It holds the handoff pointers that a spawn-time lym command writes;
// shedengine round-trips them without inspecting them.

// Package loomengine holds loom's own engine pieces.
// They are the status product payload and the guard that refuses a status file inherited from another task.
// It holds the loom and tapestry config modules.
// It also holds the discussion, raddle, plan and review tables a loom run reads.
package loomengine

// Status is loom's product payload, carried verbatim inside shedengine.Status.Product per
// contracts/specs/loom-status-spec.md.
// The rest of the shell that carries it belongs to shedengine.Status and is documented in the shedengine package doc;
// this type is only loom's half.
// StartSha is optional, so it is a pointer that is nil for JSON null or absent; Slug and Parent are required values.
// The zero Status value is invalid.
type Status struct {
	Slug     string  `json:"slug"`
	Parent   string  `json:"parent"`
	StartSha *string `json:"start_sha"`
}
