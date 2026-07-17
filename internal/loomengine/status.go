// status.go defines the canonical Go type for _lyx/status.json: loom's single
// source of truth for orchestration state, from the t=0 seed a spawn-time lyx
// command writes through to the fields loom rewrites on every phase-machine step.

// Package loomengine implements loom's Preflight precondition validator: the
// four checks (worktree geometry, host cleanliness, weft pairing/sync, and
// _lyx/status.json coherence) that must all pass before a task is fit to run.
//
// Callers MUST NOT invoke Preflight except when the task is at the
// fresh/preflight stage. Invoking it on an already-advanced task (non-empty
// history, set start_sha, …) is a caller error that will be reported as a
// half-finished precondition failure, not diagnosed as misuse, because
// Preflight is a stateless validator.
package loomengine

// Status is the canonical Go type for _lyx/status.json, pinned by
// docs/reference/status-schema.md. It is loom's single source of truth for
// orchestration state: current phase, current review sub-state, the
// phase-level outcome trail, and the human-readable narration
// `lyx loom status --watch` prints. The t=0 "seed" — the handoff instant a
// task is spawned and given to loom, before any `lyx loom run` has executed —
// is the same Status shape with only the handoff fields populated and
// everything else at its zero/null value; see status-schema.md's
// "The seed / handover" section.
//
// Per status-schema.md's field-presence-and-nullability rule, StartSha and
// NextAction are *string (nil ⇔ JSON null/absent) because they are genuinely
// optional; every other mandatory string field and PauseRequested/History are
// value types because the coherence validator treats their zero value as
// "missing" for mandatory strings, or as a legitimately valid default for
// PauseRequested/History. The zero Status value is not a valid instance —
// checkCoherence rejects it as half-finished (fresh-start invariants) as well
// as incoherent (empty mandatory strings).
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

// HistoryEntry is one entry in Status.History: a per-phase outcome trail
// record — one entry per phase attempt, including stuck-handler bounce-backs.
// Per-round verdicts are not duplicated here; those live in perch's own block
// files. BouncedTo is present only on a "stuck" entry that routes back to an
// earlier phase, per status-schema.md.
type HistoryEntry struct {
	Phase     string  `json:"phase"`
	Outcome   string  `json:"outcome"`
	BouncedTo *string `json:"bounced_to,omitempty"`
	Ts        string  `json:"ts"`
}
