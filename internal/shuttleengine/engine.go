// engine.go defines the provider seam: the Engine interface every LLM
// adapter implements, and the plain value types that cross it (Launch,
// PaneInput, StopEvent, StartupState, Outcome). shuttleengine owns this
// seam and never imports a concrete engine — the provider-seam import rule,
// enforced by seam_enforcement_test.go — so a second provider only ever
// needs to satisfy Engine, never touch the run loop or CLI machinery.

package shuttleengine

// Outcome classifies how a shuttle run ended. It is the run loop's terminal
// classification, not an error: a run that finished normally, one that is
// asking a question, one whose pane died, and one that timed out are all
// valid, reportable outcomes a caller branches on.
type Outcome string

// The set of outcomes a shuttle run can be classified into.
const (
	// OutcomeDone means the agent wrote every OutputFiles entry and the run
	// loop observed the file contract satisfied.
	OutcomeDone Outcome = "done"
	// OutcomeAsking means the agent ended its turn without writing the
	// output files and its last message reads as a question — it is
	// blocked on operator input.
	OutcomeAsking Outcome = "asking"
	// OutcomeDied means the run's PANE died (or its provider never became
	// ready inside the startup window) before the output files were written
	// and without an asking signal. Pane death is the only process failure
	// shuttle can observe: a provider process that crashes MID-RUN while its
	// pane's shell survives is invisible to the liveness check (the pane
	// stays live, no Stop ever arrives) and classifies OutcomeTimeout at the
	// deadline instead — proven live, and not probe-able either, since the
	// dead provider's final TUI frame stays rendered in the pane.
	OutcomeDied Outcome = "died"
	// OutcomeTimeout means the run's wall-clock Timeout elapsed before the
	// output files were written. Besides a genuinely slow or hung agent,
	// this is also the honest classification for a provider that crashed
	// mid-run behind a still-live pane shell (see OutcomeDied) — the strand
	// and run directory are kept either way, so the pane tells the operator
	// which of the two happened.
	OutcomeTimeout Outcome = "timeout"
)

// Launch carries the opaque, provider-specific command strings an Engine's
// Prepare produces: Cmd is typed into a fresh pane to start the run, and
// ResumeCmd is typed into a pane to reattach an existing session (never
// `--continue`, which is ambiguous under concurrent runs — Cmd/ResumeCmd
// both name the session explicitly via SessionID). SessionID is the
// provider session identity Prepare minted or was handed, persisted into
// RunState so a later resume can reconstruct ResumeCmd without re-deriving
// it. shuttle sends Cmd/ResumeCmd into a pane verbatim; it never parses or
// modifies them.
type Launch struct {
	Cmd       string
	ResumeCmd string
	SessionID string
}

// PaneInput is one step of provider-specific key choreography a run loop
// sends into a pane via mux's send-keys primitives. Exactly one of Key or
// Text is set: Key names a psmux key (e.g. "Escape", "Enter") sent as a key
// press, Text is literal text typed into the pane. When Submit is true and
// Text is set, an Enter key follows Text — the two-step "type then submit"
// psmux requires for literal text.
type PaneInput struct {
	// Key is a psmux named key (e.g. "Escape"). Empty when this step types
	// literal Text instead.
	Key string
	// Text is literal text typed into the pane. Empty when this step sends
	// a named Key instead.
	Text string
	// Submit, when true and Text is set, appends an Enter key press after
	// Text so the pane's input line is submitted.
	Submit bool
	// SettleMS pauses this many milliseconds after the step lands before
	// the next step is sent. An engine sets it when the provider's input
	// parser needs the step delivered in its own read: an Escape byte
	// immediately followed by text can coalesce into an Alt-/escape-
	// sequence read and be discarded wholesale (observed live — a Send's
	// entire text silently swallowed).
	SettleMS int
}

// StopEvent is one parsed Stop-hook line from a run's events.jsonl: the
// provider's turn-end signal, carrying the last message the agent produced
// before ending its turn (used to classify OutcomeAsking) and the raw JSON
// line it was parsed from (for callers that need fields ParseEvents does
// not surface).
type StopEvent struct {
	// LastAssistantMessage is the agent's final message for this turn, or
	// "" if the event carried none.
	LastAssistantMessage string
	// Raw is the exact JSON line this StopEvent was parsed from.
	Raw []byte
}

// StartupState classifies a pane's captured content during the startup
// window between launch and the provider becoming ready for input.
type StartupState int

// The set of states Startup can classify a pane capture into.
const (
	// StartupPending means the provider has not yet reached an input-ready
	// or trust-prompt state — still booting.
	StartupPending StartupState = iota
	// StartupReady means the provider's input prompt is visible; the run
	// loop may proceed with ComposeSend.
	StartupReady
	// StartupTrustPrompt means the provider is showing a one-time
	// trust-this-folder gate that must be dismissed before it becomes
	// ready.
	StartupTrustPrompt
)

// Engine is the provider seam: the interface every LLM adapter implements
// so the run loop and CLI can drive any provider identically. shuttleengine
// defines Engine and its value types and never imports a concrete
// implementation (the provider-seam import rule); concrete engines (e.g.
// claudeengine) import shuttleengine and satisfy this interface.
type Engine interface {
	// Prepare writes the run directory's provider-specific artifacts (the
	// prompt file, any settings/hook configuration) for one run described
	// by spec and cfg, and returns the opaque Launch command strings the
	// run loop types into a pane to start or resume the run. runDir already
	// exists; Prepare only ever writes files inside it.
	Prepare(runDir string, spec Spec, cfg Config) (Launch, error)
	// ParseEvents parses one run's events.jsonl contents into StopEvents.
	// It is lenient: malformed or unrecognized lines are skipped, never
	// fatal, since partial appends and cross-version unknown fields are
	// expected while a run is still in progress.
	ParseEvents(data []byte) ([]StopEvent, error)
	// Startup classifies a pane capture taken during the startup window,
	// distinguishing a still-booting pane from one showing a trust prompt
	// from one that has reached its input-ready state. The same
	// classification doubles as the Interrupt/Send pre-key probe
	// (requireReadyAgentPane): StartupReady is the seam's answer to "is the
	// provider's TUI on screen right now", at any point in a run's life.
	Startup(capture string) StartupState
	// InterruptSequence returns the provider-specific key choreography that
	// interrupts an in-progress turn (e.g. an Escape key press).
	InterruptSequence() []PaneInput
	// TrustDismissSequence returns the provider-specific key choreography
	// that dismisses the trust gate Startup classifies as
	// StartupTrustPrompt (e.g. a single Enter key press). It lives on the
	// seam — not hardcoded in the run loop — because which keys dismiss a
	// provider's gate is pane key choreography, exactly like
	// InterruptSequence/ComposeSend (the Shuttle Provider-Seam Invariant's
	// semantic half).
	TrustDismissSequence() []PaneInput
	// ComposeSend returns the provider-specific key choreography that
	// submits text as a new turn (e.g. clearing a leaked auto-suggest
	// before typing text and submitting it).
	ComposeSend(text string) []PaneInput
}
