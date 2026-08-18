// engine.go defines the provider seam: the Engine interface every LLM adapter implements,
// and the plain value types that cross it (Launch, PaneInput, Event, StartupState, Outcome).
// shuttleengine owns this seam and never imports a concrete engine — the provider-seam import rule,
// enforced by seam_enforcement_test.go — so a second provider only ever needs to satisfy Engine,
// never touch the run loop or CLI machinery.

package shuttleengine

// Outcome classifies how a shuttle run ended: a terminal classification, not an error.
type Outcome string

// Outcomes a shuttle run can be classified into.
const (
	// OutcomeDone: agent wrote every OutputFiles entry and file contract is satisfied.
	OutcomeDone Outcome = "done"
	// OutcomeAsking: agent ended a turn (or opened a live AskUserQuestion call) without every
	// OutputFiles entry existing.
	// The classification tests the FILE CONTRACT only — Wait never inspects the message — so this is
	// "the agent stopped without finishing", of which a question is the common case rather than the
	// definition: a blocked agent and a mid-task status report classify identically.
	// Result.LastAssistantMessage therefore carries whatever the agent last said, not a guaranteed
	// question.
	OutcomeAsking Outcome = "asking"
	// OutcomeDied: pane died (or provider never became ready inside startup window) before output
	// files written.
	// Pane death is the only observable process failure;
	// a provider crash mid-run behind a live pane shell classifies OutcomeTimeout instead.
	OutcomeDied Outcome = "died"
	// OutcomeTimeout: wall-clock Timeout elapsed before output files written.
	// This also covers provider crashes mid-run behind a still-live pane shell (pane tells the
	// operator which happened).
	OutcomeTimeout Outcome = "timeout"
)

// Launch carries the opaque, provider-specific command strings an Engine's Prepare produces.
// Cmd is typed into a fresh pane to start, ResumeCmd to reattach an existing session (both name the
// session via SessionID).
// shuttle sends Cmd/ResumeCmd verbatim;
// it never parses or modifies them.
type Launch struct {
	Cmd       string
	ResumeCmd string
	SessionID string
}

// PaneInput is one step of provider-specific key choreography sent to a pane via reed's send-keys.
// Exactly one of Key or Text is set;
// when Submit is true and Text is set, an Enter key follows Text.
type PaneInput struct {
	Key      string // Tmux named key (e.g. "Escape"); empty if this step types Text instead.
	Text     string // Literal text typed into the pane; empty if this step sends a Key.
	Submit   bool   // When true and Text is set, appends an Enter key press after Text.
	SettleMS int    // Milliseconds to pause after this step lands before the next step is sent (prevents escape-sequence coalescing).
}

// EventKind discriminates the two signals ParseEvents can surface from events.jsonl: a turn-end and
// a live question.
// It is a parse-time discriminator only, selecting which payload field an Event's Message comes
// from.
type EventKind int

// Kinds a parsed Event can carry.
const (
	// EventStop: provider's turn-end signal, agent ended its turn without writing output files.
	EventStop EventKind = iota
	// EventAsk: live, in-progress tool-call signal when the agent is asking a question (observed when
	// tool call opens, not at turn end).
	EventAsk
)

// Event is one parsed line from events.jsonl: either a turn-end signal (EventStop) or a live ask
// (EventAsk).
// Message carries the agent's final message (EventStop) or question text (EventAsk);
// Raw is the exact JSON line.
type Event struct {
	Kind    EventKind // Discriminates which signal this Event carries.
	Message string    // Agent's final message (EventStop) or question text (EventAsk); "" if event carried none.
	Raw     []byte    // Exact JSON line this Event was parsed from.
}

// StartupState classifies a pane's captured content during startup, between launch and provider
// ready.
type StartupState int

// States Startup can classify a pane capture into.
const (
	// StartupPending: provider not yet at input-ready or trust-prompt state, still booting.
	StartupPending StartupState = iota
	// StartupReady: provider's input prompt is visible; run loop may proceed with ComposeSend.
	StartupReady
	// StartupTrustPrompt: provider showing one-time trust-this-folder gate;
	// must be dismissed before becoming ready.
	StartupTrustPrompt
)

// Engine is the provider seam: the interface every LLM adapter implements so the run loop can drive
// any provider identically.
// shuttleengine defines Engine and never imports a concrete implementation (the provider-seam
// import rule);
// concrete engines (e.g.
// claudeengine) import shuttleengine and satisfy this interface.
type Engine interface {
	// Prepare writes provider-specific artifacts (prompt file, settings/hooks) and returns opaque Launch command strings.
	Prepare(runDir string, spec Spec, cfg Config) (Launch, error)
	// ParseEvents parses events.jsonl into Events (turn-end signal and live ask).
	// It is lenient: malformed or unrecognized lines are skipped.
	ParseEvents(data []byte) ([]Event, error)
	// Startup classifies pane capture during startup: still-booting, trust prompt, or ready.
	// It also serves as the pre-key probe for Interrupt/Send (StartupReady answers "is the provider's TUI on screen?").
	Startup(capture string) StartupState
	// InterruptSequence returns the key choreography that interrupts an in-progress turn (e.g. Escape).
	InterruptSequence() []PaneInput
	// TrustDismissSequence returns the key choreography that dismisses the trust gate (e.g. Enter).
	// It lives on the seam because which keys dismiss a provider's gate is pane key choreography.
	TrustDismissSequence() []PaneInput
	// ComposeSend returns the key choreography that submits text as a new turn (e.g. clearing auto-suggest before typing).
	ComposeSend(text string) []PaneInput
	// ModelSwitchSequence returns the key choreography that switches a live session's model (e.g. `/model <name>` typed and submitted).
	// It lives on the seam because which command string and key sequence switch a provider's model is provider grammar.
	ModelSwitchSequence(model string) []PaneInput
	// AuditForks reads the provider's on-disk record of fork subagents for a fork-authorized session (identified by sessionID).
	// workdir is the pane's actual process cwd. Returns mechanical facts observed with no policy interpretation.
	// Returns error (not zero ForkAudit) if the provider has no fork concept, so callers can distinguish "no forks" from "cannot audit".
	AuditForks(sessionID, workdir string) (ForkAudit, error)
	// AuditForksIncremental is AuditForks for callers that have already processed some fork transcripts and want only new ones.
	// Parent facts (SpawnCalls, NamedSpawns, ParentWriteCalls, ParentWrites, ParentBashCommands) are always full/cumulative.
	// Forks holds one ForkReport per transcript not in seenTranscripts; nil seenTranscripts reports every fork transcript (equivalent to AuditForks).
	AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (ForkAudit, error)
}
