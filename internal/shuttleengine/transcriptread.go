// transcriptread.go declares the provider-neutral reading of finished session records: transcript entries, per-message usage, pricing and the RecordReads set an engine hands out.

package shuttleengine

import "time"

// MessageUsage is the token use of one provider message.
// An engine whose provider reports no cache writes or no lifetime split leaves those fields zero.
type MessageUsage struct {
	// Input is the uncached input tokens.
	Input int
	// CacheWrite is the tokens written to the prompt cache, every lifetime included.
	CacheWrite int
	// CacheWriteOneHour is the one-hour-lifetime part of CacheWrite.
	CacheWriteOneHour int
	// CacheRead is the tokens read from the prompt cache.
	CacheRead int
	// Output is the generated tokens.
	Output int
}

// EntryRole says whether a transcript entry is a user or an assistant record.
type EntryRole string

const (
	// EntryUser marks a record the user side wrote, tool results included.
	EntryUser EntryRole = "user"
	// EntryAssistant marks a record the model wrote.
	EntryAssistant EntryRole = "assistant"
)

// ToolKind classifies a tool call by what it does, whatever the provider names the tool.
type ToolKind string

const (
	// ToolShell is a call that runs a shell command.
	ToolShell ToolKind = "shell"
	// ToolFileRead is a call that reads a file.
	ToolFileRead ToolKind = "file-read"
	// ToolOther is any other call.
	ToolOther ToolKind = "other"
)

// ToolCall is one tool call an assistant entry made.
type ToolCall struct {
	// ID identifies the call, so a ToolResult can answer it.
	ID string
	// Kind classifies the call.
	Kind ToolKind
	// Command is a shell call's command, empty for any other kind.
	Command string
	// Path is a file read's path, empty for any other kind.
	Path string
}

// ToolResult is the answer to one tool call.
type ToolResult struct {
	// CallID is the ID of the ToolCall this result answers.
	CallID string
	// Text is the result's text.
	Text string
}

// TranscriptEntry is one user or assistant record of a transcript, in recorded order.
type TranscriptEntry struct {
	// At is the record's time, zero when the record carries no parseable time.
	At time.Time
	// Role says which side wrote the record.
	Role EntryRole
	// StartsTurn is true when the record opens a turn.
	StartsTurn bool
	// EndsTurn is true when the record closes a turn.
	EndsTurn bool
	// MessageKey is the provider's message id, or a key the engine makes unique per message.
	MessageKey string
	// Model is the model that produced the message, empty on a user record.
	Model string
	// Usage is set only on a message's final record.
	// It is nil on every other record and on a record the engine skipped as a fork's replay of its parent.
	Usage *MessageUsage
	// UserTexts is the text blocks a user record carries.
	UserTexts []string
	// ToolCalls is the tool calls an assistant record made.
	ToolCalls []ToolCall
	// ToolResults is the tool results a user record carries.
	ToolResults []ToolResult
	// PromptPath is set when the entry is the engine's own launch line, and holds the prompt file it points at.
	PromptPath string
}

// TranscriptRead is one transcript read through its engine.
type TranscriptRead struct {
	// ID is the transcript's stable name.
	ID string
	// Name is the latest strand name lyx set on the session, empty when none.
	Name string
	// Started is the time of the record's first timestamped line of any kind, zero when none.
	Started time.Time
	// Repeats counts the records collapsed away:
	// streamed records folded into their message's final record, plus, in a fork, every record of a replayed parent message.
	Repeats int
	// Entries is the user and assistant records in recorded order.
	Entries []TranscriptEntry
}

// TranscriptRef names one transcript an engine located.
type TranscriptRef struct {
	// ID is the transcript's stable name; for Claude its file name.
	ID string
	// Ref is an opaque handle only the locating engine reads.
	Ref string
}

// LocatedSession is one session an engine recorded under a worktree, with its fork transcripts.
type LocatedSession struct {
	TranscriptRef
	// Modified is the last-modified time of the session's own record, its forks' left out.
	Modified time.Time
	// Forks is the session's fork transcripts.
	Forks []TranscriptRef
}

// SessionRead is a located session read through its engine.
type SessionRead struct {
	// Main is the session's own transcript.
	Main TranscriptRead
	// Forks is the fork transcripts, in LocatedSession.Forks order.
	Forks []TranscriptRead
}

// TranscriptReader is an optional capability beside Engine: locating and reading the transcripts the provider recorded, with no run and no shuttle config.
// An engine without it has no readable transcripts.
type TranscriptReader interface {
	// LocateSessions returns the sessions the engine recorded under the worktree path.
	// An absent provider root or no session is an empty list and no error.
	LocateSessions(worktree string) ([]LocatedSession, error)
	// ReadSession reads a session this engine located, its forks included.
	// It errors when a transcript cannot be read, a vanished one included.
	ReadSession(s LocatedSession) (SessionRead, error)
}

// MessagePricer is an optional capability beside Engine: the provider's price for one message.
type MessagePricer interface {
	// PriceMessage returns the cost in USD of a message of the given model and usage.
	// priced is false when the model has no known price.
	PriceMessage(model string, usage MessageUsage) (usd float64, priced bool)
}

// RecordReads is one engine's offline read capabilities, each nil when the engine lacks it.
type RecordReads struct {
	// Transcripts locates and reads the engine's session transcripts.
	Transcripts TranscriptReader
	// Pricer prices a message of the engine's provider.
	Pricer MessagePricer
	// Instructions lists the file names the provider loads as project instructions.
	Instructions InstructionFileLister
}
