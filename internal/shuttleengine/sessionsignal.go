// sessionsignal.go declares the provider-neutral session signals: what a provider's hook lines say a session did, with the hook-side time of each.
// It is a capability beside Engine, separate from the events ParseEvents yields, so the wait loop's classification does not depend on it.

package shuttleengine

import "time"

// SessionSignalKind names what a hook line says the session did.
type SessionSignalKind string

// Kinds a SessionSignal can carry.
const (
	// SessionSignalTurnStart: the operator or a parent submitted a prompt, so the agent began a turn.
	SessionSignalTurnStart SessionSignalKind = "turn_start"
	// SessionSignalTurnEnd: the agent's turn ended, possibly with background work still outstanding.
	SessionSignalTurnEnd SessionSignalKind = "turn_end"
	// SessionSignalAPIErrorTurnEnd: the agent's turn ended because the provider's API failed.
	SessionSignalAPIErrorTurnEnd SessionSignalKind = "api_error_turn_end"
	// SessionSignalAsk: the session is waiting on an answer, through a question tool or a permission or elicitation dialog.
	SessionSignalAsk SessionSignalKind = "ask"
	// SessionSignalIdleNotice: the provider noticed the session sitting idle at its input.
	SessionSignalIdleNotice SessionSignalKind = "idle_notice"
	// SessionSignalSessionEnd: the provider reported the session ended.
	SessionSignalSessionEnd SessionSignalKind = "session_end"
	// SessionSignalSessionStart: the provider started or resumed the session, or restarted its context after a compaction.
	// It says nothing about what the session is doing, so a session fold does not move on it.
	SessionSignalSessionStart SessionSignalKind = "session_start"
)

// SessionSignal is one provider hook line read as a session fact.
type SessionSignal struct {
	// Kind discriminates which fact the signal carries.
	Kind SessionSignalKind
	// At is the hook-side time of the line; zero when the line has none.
	At time.Time
	// SessionID is the session id the hook line names; empty when it names none.
	SessionID string
	// Outstanding is a turn end's background tasks, each with the signal that reported it; empty on every other kind.
	Outstanding []BackgroundTask
	// ErrorText is an API-error turn end's text; empty on every other kind.
	ErrorText string
	// Reason is a session end's reason; empty on every other kind.
	Reason string
	// EndsProcess is true when a session end's reason ends the provider's process; the provider decides which reasons do.
	EndsProcess bool
	// Source is a session start's source, such as startup or compact; empty on every other kind.
	Source string
	// Event is true when ParseEvents also yields an event from the same line.
	Event bool
	// Raw is the exact hook payload line the signal was read from.
	Raw []byte
}

// SessionSignalParser is an optional capability beside Engine: the provider's reading of a run's hook lines as session signals.
// An Engine without it yields no session state.
type SessionSignalParser interface {
	// ParseSessionSignals parses data, the bytes of an events file from some offset, into signals in file order.
	// consumed is the byte count a following incremental read resumes from:
	// it ends at the last complete line, except that it stops before the earliest line of the trailing run of time-stamp lines with no payload line after them,
	// so the next read sees each such stamp together with its payload.
	// It is lenient: malformed or unrecognized lines are skipped.
	ParseSessionSignals(data []byte) (signals []SessionSignal, consumed int)
}

// SessionProber is an optional capability beside Engine: the provider-specific facts a session state needs that no hook line carries.
// An Engine without it leaves liveness unproven and reads no interrupt.
type SessionProber interface {
	// ProcessLiveness reports whether the process driving sessionID is alive, dead or unproven.
	// Dead needs positive evidence that the session's process is gone; every doubt reads unproven.
	ProcessLiveness(sessionID string) Liveness
	// TurnStartInterrupt reports whether the user interrupted the turn that turnStart began, with the interrupt's own time.
	// It reads the provider's transcript, and any failure to read it reads as not interrupted.
	TurnStartInterrupt(turnStart SessionSignal) (at time.Time, interrupted bool)
}
