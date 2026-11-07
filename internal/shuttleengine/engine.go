// engine.go defines the provider seam: the Engine interface LLM adapters implement, and the value types that cross it.
// shuttleengine never imports a concrete engine; seam_enforcement_test.go enforces that.
// A further provider implements Engine and registers in internal/shuttleengines, without touching the run loop or CLI.

package shuttleengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// Outcome classifies how a shuttle run ended: a terminal classification, not an error.
type Outcome string

// Outcomes a shuttle run can be classified into.
const (
	// OutcomeDone: the agent wrote every output file the spec lists, and the file contract is satisfied.
	OutcomeDone Outcome = "done"
	// OutcomeDied: before output files were written, a strand that reed still tracks has a pane that is not alive.
	// It also covers a provider that never became ready inside the startup window.
	// A provider process gone behind a live pane shell also ends here;
	// the registry or a process-ending session end is what proves it gone.
	// Reaching it that way needs an engine that parses session signals and probes liveness.
	// A strand that reed no longer tracks at all is not this outcome;
	// reed's bookkeeping going away says nothing about the agent, so the wait reports it as a mechanism failure.
	// The exception is a handle whose own Stop removed its strand;
	// that handle also ends here, and the run is recorded as stopped.
	OutcomeDied Outcome = "died"
	// OutcomeTimeout: the wall-clock timeout elapsed before output files were written.
	// A provider crash behind a still-live pane shell that nothing proves also ends here;
	// an engine without liveness probing is one such case.
	// So does a start whose skill-load turn is still open when the run's deadline passes.
	OutcomeTimeout Outcome = "timeout"
	// OutcomeAPIError: the provider's API ended the agent's turn on an error once the spec's re-send limit was spent.
	// It also ends the turn at once when the provider rejects the model, a skill-load turn's included.
	// Both apply only before output files are written.
	OutcomeAPIError Outcome = "api-error"
)

// Launch carries the opaque, provider-specific command strings that a provider's preparation step produces.
// Cmd is typed into a fresh pane to start the session, and ResumeCmd reattaches an existing one;
// both name the session via SessionID.
// shuttle sends Cmd/ResumeCmd verbatim;
// it never parses or modifies them.
type Launch struct {
	Cmd       string
	ResumeCmd string
	SessionID string
	// PromptLine, when set, means the engine left the prompt pointer off Cmd.
	// The engine carried the skill-load message there as the first turn.
	// shuttle awaits that turn, types the color, and then delivers this text as a turn once the spec's skills are loaded.
	PromptLine string
	// RecordsSessionStarts declares that the launch writes a session-start line into the run's events file.
	// The line is written when the provider's session begins, so startup waits for it beside the ready pane.
	RecordsSessionStarts bool
	// CreelHooks declares that the launch's settings carry creel's hook delivery:
	// the tool-boundary claim, the turn-end Stop delivery and the idle waiter.
	// That allows creel to ring the strand's bell.
	CreelHooks bool
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

// EventKind discriminates the signals parsed from events.jsonl:
// a turn-end, a live question, and a turn-end with background work still outstanding.
// It only selects, at parse time, which payload field the event's message comes from.
// The wait loop reads it in one place, to treat a turn end with background work as still running.
type EventKind int

// Kinds a parsed Event can carry.
const (
	// EventStop: provider's turn-end signal, agent ended its turn without writing output files.
	EventStop EventKind = iota
	// EventAsk: live, in-progress tool-call signal when the agent is asking a question.
	// It is observed when the tool call opens, not at the turn end.
	EventAsk
	// EventWaiting: the provider's turn ended, but background work the session launched is still outstanding;
	// the agent is not asking anything and will resume on its own.
	// The wait loop treats it as still running; a later turn end or question classifies as usual.
	EventWaiting
)

// Event is one parsed line from events.jsonl:
// a turn-end signal, a live ask, or a turn end with background work outstanding.
// Message carries the agent's final message for a turn end, or the question text for a live ask;
// Raw is the exact JSON line.
type Event struct {
	Kind        EventKind        // Discriminates which signal this Event carries.
	Message     string           // Agent's final message (EventStop, EventWaiting) or question text (EventAsk); "" if event carried none.
	Raw         []byte           // Exact JSON line this Event was parsed from.
	Outstanding []BackgroundTask // Background tasks still running at the turn end; set only on an EventWaiting.
	At          time.Time        // Hook-side time of the line this Event was parsed from; zero when the engine gives none. Only Runner.ReadEvents sets it.
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

// BackgroundTask is one piece of background work a waiting turn end left outstanding, in a provider-neutral shape.
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

// StartupState classifies a pane's captured content during startup, between launch and provider ready.
type StartupState int

// The states a pane capture taken during startup can be classified as.
const (
	// StartupPending: provider not yet at input-ready or trust-prompt state, still booting.
	StartupPending StartupState = iota
	// StartupReady: the provider's input prompt is visible; the run loop may proceed to send a new turn.
	StartupReady
	// StartupTrustPrompt: the provider shows its one-time trust-this-folder gate;
	// the gate must be dismissed before the provider becomes ready.
	StartupTrustPrompt
)

// Engine is the interface every LLM adapter implements, so the run loop can drive any provider identically.
// Only internal/shuttleengines constructs or imports implementations of it.
type Engine interface {
	// Prepare writes provider-specific artifacts (prompt file, settings/hooks) and returns opaque Launch command strings.
	Prepare(runDir string, spec Spec, cfg Config) (Launch, error)
	// PromptCacheLifetime returns how long the provider keeps a session's prompt cache warm while idle.
	// The lifetime is for a session started from spec under cfg.
	// Zero means the provider knows no warm lifetime, so a caller keeps nothing warm.
	// An error means cfg cannot be resolved for spec.
	PromptCacheLifetime(spec Spec, cfg Config) (time.Duration, error)
	// KeepWarmCaps returns two caps for a role:
	// capWait, the longest keep-warm wait a session of that role blocks;
	// longCap, the long cap a long gate wait uses.
	// role is a base role, with any numbered suffix already dropped.
	// A zero cap means the engine keeps nothing warm, so a caller blocks to its end.
	// An error means the engine's config cannot be resolved.
	KeepWarmCaps(role string) (capWait, longCap time.Duration, err error)
	// ParseEvents parses events.jsonl into Events (turn-end signal, waiting turn end, and live ask).
	// It is lenient: malformed or unrecognized lines are skipped.
	ParseEvents(data []byte) ([]Event, error)
	// Startup classifies pane capture during startup: still-booting, trust prompt, or ready.
	// It also probes a pane before keys are sent to it: StartupReady means the provider's TUI is on screen.
	Startup(capture string) StartupState
	// InterruptSequence returns the key choreography that interrupts an in-progress turn (e.g. Escape).
	InterruptSequence() []PaneInput
	// TrustDismissSequence returns the key choreography that accepts the trust gate rendered in capture.
	// It takes the capture because a provider's gate is a selection list:
	// its caret does not necessarily start on the accepting option.
	// Confirming whatever is selected turns a dismissal into a refusal that quits the provider outright.
	// An implementation that cannot locate the accepting option in capture returns no inputs at all.
	// It never sends a blind confirmation; the startup window then expires as the died outcome.
	// That is the end state a wrong keypress reaches, without lym itself having pressed the button.
	// The keys that move and confirm a provider's gate are pane key choreography, so the method lives on the seam.
	TrustDismissSequence(capture string) []PaneInput
	// ComposeSend returns the key choreography that submits text as a new turn (e.g. clearing auto-suggest before typing).
	ComposeSend(text string) []PaneInput
	// ColorSequence returns the key choreography that sets a live session's display color.
	// For example, the sequence types `/color <name>` and submits it.
	// It returns no inputs for a provider without that choreography, or for a color it has no name for.
	// It lives on this interface because the command string and key sequence that color a session are provider grammar.
	ColorSequence(color segmentcolor.Color) []PaneInput
}

// Requirement is one thing a run may require of its engine; the set is closed to the constants below.
type Requirement string

// Requirements a run may state of its engine.
const (
	// RequirementForks: the engine's sessions can spawn fork subagents.
	RequirementForks Requirement = "forks"
	// RequirementBackgroundWake: the engine's sessions wake on the completion of their own background work.
	RequirementBackgroundWake Requirement = "background-wake"
)

// RequirementSupporter is an optional capability of an engine:
// the provider's answer to whether it meets a Requirement.
// An engine without it meets no requirement.
type RequirementSupporter interface {
	// Supports reports whether the engine meets req.
	Supports(req Requirement) bool
}

// ForkAuditor is an optional capability of an engine:
// the provider's read of its on-disk record of fork subagents.
// An engine without it has no fork audit.
type ForkAuditor interface {
	// AuditForks reads the provider's on-disk record of fork subagents for the fork-authorized session sessionID.
	// workdir is the pane's actual process cwd.
	// It reports mechanical facts observed, with no policy interpretation.
	// It returns an error rather than a zero ForkAudit when the provider has no fork concept.
	// That lets callers distinguish "no forks" from "cannot audit".
	AuditForks(sessionID, workdir string) (ForkAudit, error)
	// AuditForksIncremental reads the provider's on-disk record of fork subagents, skipping transcripts already seen.
	// Parent facts (SpawnCalls, NamedSpawns, ParentWriteCalls, ParentWrites, ParentBashCommands) are always full/cumulative.
	// Forks holds one ForkReport per transcript not in seenTranscripts; nil seenTranscripts reports every fork transcript.
	AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (ForkAudit, error)
}

// OutstandingForkReader is an optional capability of an engine:
// the provider's read of which fork subagents a session launched are still running.
// An engine without it cannot have a session wait on its own forks.
type OutstandingForkReader interface {
	// OutstandingForks lists the forks the session identified by sessionID launched whose completion the provider has not yet recorded.
	// workdir is the pane's process cwd.
	// An unreadable record is an error, never an empty list, since an empty list ends a wait.
	OutstandingForks(sessionID, workdir string) ([]BackgroundTask, error)
}

// ResumeContextRenderer is an optional capability of an engine:
// the provider's rendering of resume text as its session-start hook output.
// An engine without it has no session-start context to render.
type ResumeContextRenderer interface {
	// ResumeContext renders text as the provider's session-start hook output.
	ResumeContext(text string) ([]byte, error)
}

// HookEvent names a provider hook event creel's delivery acts on.
type HookEvent string

const (
	// HookEventToolBoundary is the hook the provider runs after each tool call.
	HookEventToolBoundary HookEvent = "tool-boundary"
	// HookEventStop is the hook the provider runs when a turn ends.
	HookEventStop HookEvent = "stop"
)

// HookInput is what a creel hook needs from the provider's hook input.
type HookInput struct {
	// Event is the hook event the input belongs to, empty for an event creel does not act on.
	Event HookEvent
	// Subagent is true when the call comes from a subagent or fork rather than the session's main thread.
	Subagent bool
	// StopHookActive is true when the turn-end input says an earlier Stop hook's output already continues the session.
	StopHookActive bool
}

// ToolBoundaryContextRenderer is an optional capability of an engine:
// the provider's side of creel's hook delivery.
// It parses a hook's input.
// It renders the hook output that hands mail to the session and shows the operator the echo.
// It also renders what an idle waiter hook writes to wake the session.
// An engine without it delivers creel mail by no hook.
type ToolBoundaryContextRenderer interface {
	// HookInput parses the provider's hook input payload, refusing a payload that is not a hook input.
	HookInput(payload []byte) (HookInput, error)
	// HookOutput renders the provider's hook output for event.
	// The mail, the claimed lines joined by newlines, goes to the session once.
	// The echo, the operator-only text of a mail an idle wake already handed over, goes to the operator.
	// Either may be empty.
	HookOutput(event HookEvent, mail, echo string) ([]byte, error)
	// RewakeOutput returns the standard error and exit code an idle waiter hook uses to wake the session with text.
	RewakeOutput(text string) (stderr []byte, exitCode int)
}

// IdleWaker is an optional capability of an engine, for a provider whose idle session no hook's exit wakes.
// Its idle waiter hook starts the woken turn itself, in place of writing the wake text.
type IdleWaker interface {
	// WakeIdle starts a turn carrying text in the idle session the waiter hook's input payload names.
	// An error means no turn was started.
	WakeIdle(payload []byte, text string) error
}

// BatchingHookVerb is the name of the hidden `lym shuttle` verb an engine's after-tool-call hook runs.
// It nudges the session back to batching its calls.
const BatchingHookVerb = "hook-batching"

// BatchingNudger is an optional capability of an engine:
// the after-tool-call hook that nudges the session back to batching.
// It adds one line to the calling chain's context when its recent calls break lym's batching rules.
// An engine without it installs no such hook.
type BatchingNudger interface {
	// BatchingNudge judges the tool call the hook input payload describes against the run's nudge state file at statePath.
	// It returns the provider's hook output carrying the nudge, or nil when no nudge is due.
	BatchingNudge(statePath string, payload []byte) ([]byte, error)
}

// InstructionFileLister is an optional capability of an engine:
// the names of the files the provider loads as project instructions.
// An engine without it has no project instruction files.
type InstructionFileLister interface {
	// InstructionFiles returns the file names the provider loads as project instructions.
	InstructionFiles() []string
}

// StartupChecker is an optional capability of an engine:
// a check the provider runs once a launch is past its startup gates.
// It is for a provider whose own start may rewrite the artifacts the preparation wrote.
// It reports through the log and never fails the launch.
// An engine without it is checked by nothing after startup.
type StartupChecker interface {
	// CheckAfterStartup checks the artifacts the preparation wrote, which the provider's start may have changed.
	CheckAfterStartup()
}

// SessionResumer is an optional capability of an engine:
// the provider's check that an existing session may be resumed by a new run.
// A run that resumes a session requires it of the engine, so no run resumes a session unchecked.
// Most implementers and test fakes never resume a session, so it is not required of every engine.
type SessionResumer interface {
	// CheckResume answers whether a new run may resume the provider session sessionID.
	// The session is recorded under the pane cwd workdir.
	// A non-nil error refuses the resume.
	// A non-empty warning means the check could not confirm something, and the resume proceeds.
	CheckResume(sessionID, workdir string) (warning string, err error)
}

// OpenSpec describes one named session that runs with no prompt:
// the caller's strand role, the resolved model choice, and the segment its window is colored by.
// It also carries the permission mode and the skills its first turn loads.
type OpenSpec struct {
	// Role is the strand role, from the caller's flag.
	Role string
	// ModelChoice is the resolved engine and model choice, each field empty for the default.
	// Opening the session refuses an engine name the engine set does not hold.
	ModelChoice
	// Segment is the loom segment the session's window is colored by.
	Segment segmentcolor.Segment
	// PermissionMode is the caller-owned engine vocabulary for the session's permission mode.
	// Empty selects the engine's bypass mode.
	// Callers pass it uninspected; the engine is the sole validator and realizer.
	PermissionMode string
	// Skills names the provider-neutral skills the session loads, all in its first turn and in list order.
	// Empty leaves the session with no turn; opening refuses a non-empty list for an engine that cannot load skills.
	Skills []string
}

// SessionOpener is an optional capability of an engine:
// the provider's way of opening a named interactive session with no prompt.
// OpenLaunch returns the launch and resume lines and the session id.
// The session runs with no prompt, no run directory and no run record.
// dir is a directory of the session's own that does not yet exist.
// The engine creates it only to write files its launch lines read, such as settings that carry creel's hook delivery.
// It then marks the launch CreelHooks.
// The session runs under the caller's own worktree settings and any the engine adds.
// It takes no turn but the load of spec.Skills, and the runner opens it.
// For a non-empty spec.Skills, the engine puts their load message on the fresh launch line.
// That line is the session's first turn; a resume line carries none.
// An implementing engine honours spec.PermissionMode or refuses it from OpenLaunch before it writes anything,
// so the refusal lands before the marker directory is created or a strand is added.
// Most implementers and test fakes never open a session, so it is not required of every engine.
type SessionOpener interface {
	OpenLaunch(dir string, spec OpenSpec, cfg Config) (Launch, error)
}

// SkillLoader is an optional capability an engine may implement:
// the provider's way of loading a list of named skills into a live session in one submitted turn.
// It also checks that turn afterwards.
// Starting a session whose spec names skills requires the engine to implement it.
// The engine then carries the `SkillLoadMessage` text on its launch line as the session's first turn.
// Most implementers and test fakes never load a skill, so it is kept out of the engine interface.
type SkillLoader interface {
	// SkillLoadMessage returns the one line that asks the model to load every skill of skills in a single turn.
	SkillLoadMessage(skills []string) string
	// ClassifySkillLoad checks the load turn of skills that turnEnd ended and reports the result as a SkillLoadReport.
	ClassifySkillLoad(turnEnd Event, skills []string) SkillLoadReport
	// DefaultSkillLoadTimeout is the engine-owned bound on one load turn.
	DefaultSkillLoadTimeout() time.Duration
}

// SkillLoadReport is the turn-end classification of one load turn that asked the model to load a list of skills.
// Loaded names the skills the model loaded.
// Unknown names the skills the provider does not know.
// Missing names the skills the provider knows but the model did not load.
// Each list keeps the request order.
// Verified false means the evidence was unreadable:
// the three lists are then empty, and the caller treats every requested skill as confirmed but unverified.
type SkillLoadReport struct {
	Verified bool
	Loaded   []string
	Unknown  []string
	Missing  []string
}

// InputBoxReader is an optional capability an engine may implement:
// the provider's way of reading the text its input box holds in a pane capture.
// A provider without it keeps the appearance-only send check,
// and the rule that matches the read text against the sent text is shuttleengine's, not the provider's.
type InputBoxReader interface {
	// InputBoxText returns the text the provider's input box holds in capture.
	// The caret, side bars and line breaks are stripped.
	// ok is false when capture shows no input box the provider can read.
	InputBoxText(capture string) (text string, ok bool)
	// SubmitSettle is how long after an Enter the box needs to be redrawn before it is read,
	// so a send whose Enter landed is not read as still pending.
	SubmitSettle() time.Duration
	// TypeSequence returns the key choreography that types text as the next turn without submitting it.
	// The caller submits with its own Enter once the box has settled,
	// so an engine that reads its box separates typing from submitting.
	TypeSequence(text string) []PaneInput
}

// InputBoxClearer is an optional capability an engine may implement:
// the provider's way of emptying its input box.
// The caller sends the clear only into a box its latest read shows:
// a box holding exactly its own text, or exactly one stray-character candidate its guard reading found,
// or a box read empty right before a command that starts no turn is pasted, which may still hold whitespace.
// The choreography never uses a key that interrupts a running turn.
type InputBoxClearer interface {
	// ClearInputSequence returns the key choreography that empties the input box without interrupting a running turn.
	ClearInputSequence() []PaneInput
}

// RestoredPromptClearer is an optional capability an engine may implement:
// the provider's way of emptying an input box.
// The box holds a prompt the provider restored after lym's own interrupt stopped a turn.
// Such a prompt can span several box lines, so the clear is sized from the capture of the box.
type RestoredPromptClearer interface {
	// RestoredPromptClearSequence returns the keys that empty the input box capture shows.
	// The sequence holds no Escape, C-c or Enter key and only deletes box text.
	// It holds at most two keys per box line the capture shows.
	// A capture showing no box text, or no box, answers no keys.
	RestoredPromptClearSequence(capture string) []PaneInput
}

// StrayInputReader is an optional capability an engine may implement:
// the provider's judgment of whether its input box holds a lone stray character.
// The judgment is the provider's, since what a stray character looks like on screen is provider knowledge.
type StrayInputReader interface {
	// StrayInput reports whether capture shows the provider idle, apart from its input box.
	// That box holds exactly one stray-character candidate, and StrayInput returns that character.
	StrayInput(capture string) (text string, ok bool)
}

// InterruptGuard is an optional capability an engine may implement:
// the provider's reading of whether a pane shows a turn in progress, and how long an interrupt key needs to settle.
type InterruptGuard interface {
	// RunningTurn reports whether capture shows a turn in progress.
	RunningTurn(capture string) bool
	// InterruptSettle is how long after an interrupt key the pane needs before it is read again.
	// It is longer than the provider's double-interrupt window.
	InterruptSettle() time.Duration
}

// RewindPickerReader is an optional capability an engine may implement:
// the provider's reading of whether a pane shows its rewind picker in place of its input box.
type RewindPickerReader interface {
	// RewindPicker reports whether capture shows the provider's rewind picker.
	RewindPicker(capture string) bool
}

// DismissiblePickerReader is an optional capability an engine may implement:
// the provider's reading of a known mid-session picker that covers its input box.
// The provider's own keys close such a picker while keeping the current setting.
type DismissiblePickerReader interface {
	// DismissiblePicker reports whether capture shows a dismissible picker in place of the input box.
	// It returns the picker's name and the keys that close it.
	DismissiblePicker(capture string) (name string, keys []PaneInput, ok bool)
}

// StyledBoxReader is an optional capability an engine may implement:
// the provider's way of turning a styled pane capture into the plain capture its box readings take.
type StyledBoxReader interface {
	// PlainBoxCapture takes a styled capture, as `CapturePaneStyled` returns it, and answers the plain capture.
	// It removes every SGR and other escape sequence.
	// It trims each line's trailing ASCII spaces (U+0020) as `capture-pane -p` trims them.
	// It blanks to spaces the cells of an input box that hold only the provider's placeholder.
	// The answer therefore matches what `capture-pane -p` returns for a pane with no placeholder.
	// A caller takes the box, the running turn and every other classification of one decision from one plain capture.
	PlainBoxCapture(styled string) string
}

// TextPaster is an optional capability an engine may implement: a provider that takes text as one bracketed paste.
// An engine that also implements `InputBoxReader` and `SessionCycler` is written to through a paste-submit.
// Typed keys are not used for that text.
type TextPaster interface {
	// PasteSubmits reports whether an Enter right after the paste end submits the pasted text,
	// so the paste goes out with its trailing Enter (true) or without it (false).
	PasteSubmits() bool
	// OwnPastePlaceholder reports whether boxText is exactly the collapsed-paste placeholder a paste of text produces.
	// boxText is an input-box reading.
	OwnPastePlaceholder(boxText, text string) bool
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

// CompactionBoundary is a provider-neutral reading of the newest compaction boundary in a session's transcript.
// It also covers the turn ends that follow it.
type CompactionBoundary struct {
	// At is the boundary's timestamp.
	At time.Time
	// TurnEndsAfter is the number of main-chain turn ends after the boundary in the transcript.
	TurnEndsAfter int
	// ReadTurnEndAfter is true when the turn end the caller passed is the newest of the turn ends after the boundary.
	// It is false when the transcript has none.
	// It is also false when the caller's turn end carries no message to match.
	// It is also false when its message differs from the newest turn end's.
	ReadTurnEndAfter bool
}

// IdleProbe is a provider-neutral answer to whether a live session's pane shows the provider idle.
type IdleProbe struct {
	// Idle is true when the session can take typed input.
	// The hook-derived readiness reading decides it when that reading is known.
	// When the reading is unknown, the pane decides: it shows the provider's input box empty and no turn in progress.
	Idle bool
	// TooShort is true when the pane is too short to draw an input box, so Idle false says nothing about the session.
	// It is always false when Idle is true.
	TooShort bool
	// Reason says why the session is held when Idle is false and the hook-derived readiness reading decided.
	// It is empty when Idle is true or when the pane probe decided.
	Reason string
	// Draft is true when Idle is false and the input box holds text, on either reading.
	// It is always false when Idle is true.
	Draft bool
	// NoInputBox is true when Idle is false and the pane shows no input box to an engine that reads one.
	// This happens when the pane runs no agent or when a dialog covers the box.
	// It is always false when Idle is true.
	// It is also false for an engine that reads no box, and when the pane is too short to draw one.
	NoInputBox bool
	// NoTurn is true when no turn is in progress, whatever the input box holds.
	// A draft or a missing box leaves it true while Idle stays false.
	// On the pane probe it is the pane's idle answer.
	// For an engine that reads running turns from a capture, it is true when:
	// the input box was found, the pane is not too short, and no running turn shows.
	NoTurn bool
	// TurnEndedAt is when the hook-derived reading behind the no-turn answer began;
	// zero when the pane probe decided or a turn is in progress.
	TurnEndedAt time.Time
}

// SessionCycler is an optional engine capability: the provider operations that cycle a live session's context.
// They read the context's usage, probe whether the session is idle, clear the context and compact it.
// An engine that implements it lets the runner's session methods work;
// one that does not makes them return an error naming the missing capability.
// SessionCycler is a separate interface, so engines whose callers never cycle a session need not implement it.
type SessionCycler interface {
	// ContextTokens returns the provider's context reading as of the turn end turnEnd records.
	ContextTokens(turnEnd Event) ContextReading
	// CompactedSince returns the newest main-chain compaction boundary after since in the transcript turnEnd names.
	// The boundary reports how many main-chain turn ends follow it.
	// found is false when there is none or the transcript cannot be read.
	CompactedSince(turnEnd Event, since time.Time) (boundary CompactionBoundary, found bool)
	// IdleSession reports whether capture shows the provider idle: its input box present and empty, and no turn in progress.
	IdleSession(capture string) bool
	// PaneTooShort reports whether capture shows a pane too short to draw the provider's input box;
	// such a pane makes a not-idle reading unreliable.
	PaneTooShort(capture string) bool
	// ClearSessionSequence returns the key choreography that clears the live session's context.
	ClearSessionSequence() []PaneInput
	// CompactSessionSequence returns the key choreography that compacts the live session's context, keeping what focus names.
	// An empty focus compacts with no instruction.
	// The caller guarantees focus is a single line.
	CompactSessionSequence(focus string) []PaneInput
	// ReloadPluginsSequence returns the key choreography that makes a live session re-read its installed plugins and skills.
	// The sequence starts no turn.
	ReloadPluginsSequence() []PaneInput
}
