// engine.go defines the provider seam: the Engine interface every LLM adapter implements,
// and the plain value types that cross it (Launch, PaneInput, Event, StartupState, Outcome).
// shuttleengine owns this seam and never imports a concrete engine — the provider-seam import rule,
// enforced by seam_enforcement_test.go — so a second provider only ever needs to satisfy Engine,
// never touch the run loop or CLI machinery.

package shuttleengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// Outcome classifies how a shuttle run ended: a terminal classification, not an error.
type Outcome string

// Outcomes a shuttle run can be classified into.
const (
	// OutcomeDone: agent wrote every OutputFiles entry and file contract is satisfied.
	OutcomeDone Outcome = "done"
	// OutcomeDied: a strand reed STILL TRACKS has a pane that is not alive (or the provider never
	// became ready inside the startup window) before output files were written.
	// Pane death is the only observable process failure;
	// a provider crash mid-run behind a live pane shell classifies OutcomeTimeout instead.
	// A strand reed no longer tracks AT ALL is deliberately NOT this outcome — reed's bookkeeping
	// going away says nothing about the agent, so Wait reports that as a mechanism failure.
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
	// PromptLine, when set, means the engine left the prompt pointer off Cmd:
	// shuttle delivers this text as a turn once the spec's skills are loaded.
	PromptLine string
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

// EventKind discriminates the signals ParseEvents can surface from events.jsonl: a turn-end, a live
// question, and a turn-end with background work still outstanding.
// It is a parse-time discriminator only, selecting which payload field an Event's Message comes
// from; the wait loop reads it in one place, to treat EventWaiting as still running.
type EventKind int

// Kinds a parsed Event can carry.
const (
	// EventStop: provider's turn-end signal, agent ended its turn without writing output files.
	EventStop EventKind = iota
	// EventAsk: live, in-progress tool-call signal when the agent is asking a question (observed when
	// tool call opens, not at turn end).
	EventAsk
	// EventWaiting: provider's turn ended, but background work the session launched is still
	// outstanding, so the agent is not asking anything and will resume on its own.
	// The wait loop treats it as still running; a later EventStop or EventAsk classifies as usual.
	EventWaiting
)

// Event is one parsed line from events.jsonl: a turn-end signal (EventStop), a live ask (EventAsk),
// or a turn-end with background work outstanding (EventWaiting).
// Message carries the agent's final message (EventStop, EventWaiting) or question text (EventAsk);
// Raw is the exact JSON line.
type Event struct {
	Kind        EventKind        // Discriminates which signal this Event carries.
	Message     string           // Agent's final message (EventStop, EventWaiting) or question text (EventAsk); "" if event carried none.
	Raw         []byte           // Exact JSON line this Event was parsed from.
	Outstanding []BackgroundTask // Background tasks still running at the turn end; set only on an EventWaiting.
}

// BackgroundKind tells a forked subagent from a background shell.
type BackgroundKind string

// Kinds of background work a waiting turn end can leave outstanding.
const (
	// BackgroundFork is an Agent or Task subagent.
	BackgroundFork BackgroundKind = "fork"
	// BackgroundShell is a backgrounded Bash or a Monitor.
	BackgroundShell BackgroundKind = "shell"
)

// BackgroundTask is one piece of background work a waiting turn end left outstanding, in a
// provider-neutral shape.
type BackgroundTask struct {
	Kind   BackgroundKind // Fork or shell.
	ID     string         // Provider's id for the task.
	Label  string         // The shell's command or the Monitor's description, or the fork's description; empty when the provider reports none.
	Signal string         // Which signal reported the task: SignalPayload or SignalTranscript.
}

// Signals that can report an outstanding background task.
const (
	// SignalPayload is the Stop payload's own task list.
	SignalPayload = "payload"
	// SignalTranscript is the transcript fallback, read when the payload carries no list.
	SignalTranscript = "transcript"
)

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
	// TrustDismissSequence returns the key choreography that ACCEPTS the trust gate rendered in
	// capture — the same capture Startup classified StartupTrustPrompt from.
	// It takes the capture rather than returning a fixed sequence because a provider's gate is a
	// selection list whose caret does not necessarily start on the accepting option: confirming
	// whatever happens to be selected is how a "dismissal" turns into a refusal that quits the
	// provider outright.
	// An implementation that cannot locate the accepting option in capture returns no inputs at
	// all, never a blind confirmation — the startup window then expires into OutcomeDied, which is
	// the same end state a wrong keypress reaches, without lyx itself having pressed the button.
	// It lives on the seam because which keys move and confirm a provider's gate is pane key
	// choreography.
	TrustDismissSequence(capture string) []PaneInput
	// ComposeSend returns the key choreography that submits text as a new turn (e.g. clearing auto-suggest before typing).
	ComposeSend(text string) []PaneInput
	// ModelSwitchSequence returns the key choreography that switches a live session's model (e.g. `/model <name>` typed and submitted).
	// It lives on the seam because which command string and key sequence switch a provider's model is provider grammar.
	ModelSwitchSequence(model string) []PaneInput
	// ColorSequence returns the key choreography that sets a live session's display color (e.g. `/color <name>` typed and submitted), or no inputs for a provider without one or a color it has no name for.
	// It lives on the seam because which command string and key sequence color a provider's session is provider grammar.
	ColorSequence(color segmentcolor.Color) []PaneInput
	// AuditForks reads the provider's on-disk record of fork subagents for a fork-authorized session (identified by sessionID).
	// workdir is the pane's actual process cwd. Returns mechanical facts observed with no policy interpretation.
	// Returns error (not zero ForkAudit) if the provider has no fork concept, so callers can distinguish "no forks" from "cannot audit".
	AuditForks(sessionID, workdir string) (ForkAudit, error)
	// AuditForksIncremental is AuditForks for callers that have already processed some fork transcripts and want only new ones.
	// Parent facts (SpawnCalls, NamedSpawns, ParentWriteCalls, ParentWrites, ParentBashCommands) are always full/cumulative.
	// Forks holds one ForkReport per transcript not in seenTranscripts; nil seenTranscripts reports every fork transcript (equivalent to AuditForks).
	AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (ForkAudit, error)
}

// SessionResumer is an optional capability beside Engine: the provider's check that an existing session may be resumed by a new run.
// Runner.start requires it of an Engine whenever a spec carries a ResumeSessionID, so no run resumes a session unchecked.
// It is separate from Engine for the same reason SessionCycler is: most implementers and test fakes never resume a session.
type SessionResumer interface {
	// CheckResume answers whether the provider session sessionID, recorded under the pane cwd workdir, may be resumed by a new run.
	// A non-nil error refuses the resume.
	// A non-empty warning means the check could not confirm something, and the resume proceeds.
	CheckResume(sessionID, workdir string) (warning string, err error)
}

// SkillLoader is an optional capability beside Engine: the provider's way of loading a list of named skills into a live session in one submitted turn, and of checking that turn afterwards.
// Runner.start requires it of an Engine whenever a spec names skills,
// and the orch watcher's per-tick reload uses it through Runner.LoadSkills and Runner.ClassifySkillLoad.
// It is separate from Engine for the same reason SessionCycler is: most implementers and test fakes never load a skill.
type SkillLoader interface {
	// SkillLoadMessage returns the one line that asks the model to load every skill of skills in a single turn.
	SkillLoadMessage(skills []string) string
	// ClassifySkillLoad classifies the load turn of skills that turnEnd ended.
	ClassifySkillLoad(turnEnd Event, skills []string) SkillLoadReport
	// DefaultSkillLoadTimeout is the engine-owned bound on one load turn.
	DefaultSkillLoadTimeout() time.Duration
}

// SkillLoadReport is the turn-end classification of one load turn that asked the model to load a list of skills.
// Loaded names the skills the model loaded, Unknown the skills the provider does not know, and Missing the skills it knew but the model did not load, each in request order.
// Verified false means the evidence was unreadable: the three lists are then empty and the caller treats every requested skill as confirmed but unverified.
type SkillLoadReport struct {
	Verified bool
	Loaded   []string
	Unknown  []string
	Missing  []string
}

// InputBoxReader is an optional capability beside Engine: the provider's way of reading the text its input box holds in a pane capture.
// A provider without it keeps the appearance-only send check,
// and the rule that matches the read text against the sent text lives in shuttleengine, not here.
type InputBoxReader interface {
	// InputBoxText returns the text the provider's input box holds in capture, with the caret, side bars and line breaks stripped.
	// ok is false when capture shows no input box the provider can read.
	InputBoxText(capture string) (text string, ok bool)
	// SubmitSettle is how long after an Enter the box needs to be redrawn before it is read,
	// so a send whose Enter landed is not read as still pending.
	SubmitSettle() time.Duration
}

// ContextReading is a provider-neutral reading of how much context a live session holds.
// A reading with Known false could not be read, and a caller must never treat it as over any threshold.
type ContextReading struct {
	// Tokens is the context size; meaningful only when Known is true.
	Tokens int
	// Known is false when the usage could not be read.
	Known bool
	// Compacted is true when the reading came from a compaction boundary rather than a turn's usage.
	Compacted bool
	// BoundaryAt is the compaction boundary's timestamp; zero unless Compacted is true.
	BoundaryAt time.Time
}

// CompactionBoundary is a provider-neutral reading of the newest compaction boundary in a session's transcript and the turn ends that follow it.
type CompactionBoundary struct {
	// At is the boundary's timestamp.
	At time.Time
	// TurnEndsAfter is the number of main-chain turn ends after the boundary in the transcript.
	TurnEndsAfter int
	// ReadTurnEndAfter is true when the turn end the caller passed is the newest of those turn ends.
	// It is false when the transcript has none, when the caller's turn end carries no message to match, or when its message differs from the newest turn end's.
	ReadTurnEndAfter bool
}

// IdleProbe is a provider-neutral answer to whether a live session's pane shows the provider idle.
type IdleProbe struct {
	// Idle is true when the pane shows the provider's input box empty and no turn in progress.
	Idle bool
	// TooShort is true when the pane is too short to draw an input box, so Idle false says nothing about the session; always false when Idle is true.
	TooShort bool
}

// SessionCycler is an optional capability beside Engine: the provider operations a caller needs to cycle a live session's context (read its usage, probe whether it is idle, clear it, compact it).
// An Engine that also implements it lets Runner's session methods work;
// one that does not makes them return an error naming the missing capability.
// It is separate from Engine so the many Engine implementers and test fakes whose callers never cycle a session need no method set with no behaviour behind it.
type SessionCycler interface {
	// ContextTokens returns the provider's context reading as of the turn end turnEnd records.
	ContextTokens(turnEnd Event) ContextReading
	// CompactedSince returns the newest main-chain compaction boundary after since in the transcript turnEnd names, with the turn ends that follow it.
	// found is false when there is none or the transcript cannot be read.
	CompactedSince(turnEnd Event, since time.Time) (boundary CompactionBoundary, found bool)
	// IdleSession reports whether capture shows the provider idle: its input box present and empty, and no turn in progress.
	IdleSession(capture string) bool
	// PaneTooShort reports whether capture shows a pane too short to draw the provider's input box, which makes a not-idle answer from IdleSession unreliable.
	PaneTooShort(capture string) bool
	// ClearSessionSequence returns the key choreography that clears the live session's context.
	ClearSessionSequence() []PaneInput
	// CompactSessionSequence returns the key choreography that compacts the live session's context, keeping what focus names.
	// An empty focus compacts with no instruction.
	// The caller guarantees focus is a single line.
	CompactSessionSequence(focus string) []PaneInput
	// ReloadPluginsSequence returns the key choreography that makes the live session re-read its installed plugins and skills without starting a turn.
	ReloadPluginsSequence() []PaneInput
}
