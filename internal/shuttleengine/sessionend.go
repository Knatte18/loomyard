// sessionend.go declares SessionEnd, the report of one ended session, and the Runner hook that delivers it.

package shuttleengine

import "time"

// SessionEnd reports one ended session: who ran it, on what, when, how it ended and the tokens it used.
type SessionEnd struct {
	// SessionID is the provider's id of the session.
	SessionID string
	// Engine is the name of the engine the session ran on.
	Engine string
	// Role is the spec's role segment, empty when the spec named none.
	Role string
	// Model is the spec's model, empty for the engine's default.
	Model string
	// Worktree is the pane cwd the provider's transcripts derive from.
	Worktree string
	// StartedAt is when the session's run was created, zero when its record's time does not parse.
	StartedAt time.Time
	// EndedAt is when the session ended.
	EndedAt time.Time
	// Outcome is how the session ended: the run's classification, or "stopped" for a session the stop verb ended.
	Outcome Outcome
	// Usage is the session's token reading, the unknown reading when the engine cannot read it.
	Usage SessionUsage
}

// SetSessionEnd sets the function called once for each session that ends.
// The hook runs at every terminal outcome of a run, and when the stop verb ends an unfinished run's record.
// A session that never started, with no session id, is not reported.
// A nil fn, the default, reports nothing.
// fn runs synchronously and its panics are not recovered, so it must never block and must report its own failures.
func (r *Runner) SetSessionEnd(fn func(SessionEnd)) {
	r.sessionEnd = fn
}

// reportSessionEnd hands end to the session-end hook when one is set and the session started.
func (r *Runner) reportSessionEnd(end SessionEnd) {
	if r.sessionEnd == nil || end.SessionID == "" {
		return
	}
	r.sessionEnd(end)
}
