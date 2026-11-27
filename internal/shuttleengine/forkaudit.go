// forkaudit.go defines the provider-invariant value types a fork-authorized run's audit surfaces:
// ForkAudit summarizes the parent session's own fork-spawning behavior,
// and ForkReport summarizes one subagent's transcript, a fork's or a fresh subagent's.
// Both are plain value types with no methods.
// The engine fills them from each provider's transcript format.
// This package holds no provider-specific knowledge,
// and the caller interprets the counts against its own policy.

package shuttleengine

import "time"

// WriteEvent is one Write, Edit or NotebookEdit call in a transcript, resolved against its tool result.
// At is the result line's timestamp, or the call's own line timestamp when no result exists.
// Succeeded is true only when a result exists and it is not an error.
type WriteEvent struct {
	Path      string    // File path the call targeted.
	At        time.Time // When the call's result arrived, or when the call was made if it has no result.
	Succeeded bool      // Whether the call's tool result exists and is not an error.
}

// ForkAudit summarizes fork-spawning behavior in a fork-authorized run's parent session.
// It counts Agent tool invocations (SpawnCalls) and named spawns (NamedSpawns).
// A named spawn is a defect signal, since named forks lose inherited context;
// NamedForkSpawns counts the named spawns that are forks, leaving out a named fresh subagent.
// Forks holds one report per subagent of the session, fork or fresh, whether the session or a subagent spawned it.
type ForkAudit struct {
	Forks              []ForkReport // One ForkReport per subagent, in discovery order.
	SpawnCalls         int          // Agent tool invocations in parent's transcript (attempts, not confirmations).
	NamedSpawns        int          // SpawnCalls with a name parameter (defect signal; named forks lose inherited context).
	NamedForkSpawns    int          // NamedSpawns whose subagent type is the fork type (a named fresh subagent, or a named call that records no subagent type, is not counted).
	ParentWriteCalls   int          // Write/Edit/NotebookEdit tool_use blocks in parent's transcript (policy interpretation is caller's).
	ParentWrites       []string     // File paths of every parent Write/Edit/NotebookEdit block, in transcript order (caller's policy for interpretation).
	ParentWriteEvents  []WriteEvent // One event per parent Write/Edit/NotebookEdit block with a path, in transcript order, with time and result.
	ParentBashCommands []string     // Verbatim Bash commands in parent's transcript (caller classifies git-mutating or disallowed).
}

// ForkReport summarizes one subagent's transcript: what it attempted and whether it produced a final report.
// FreshSubagent separates a subagent started with a fresh context from a fork.
// A fresh subagent is briefed by the parent like a delegate; a fork inherits the parent's context.
// A transcript whose kind the provider does not record reads as a fork.
// Policy over these fields is the caller's job.
// For example, the caller decides what counts as git-mutating or whether nested Agent calls are errors.
// ForkReport only carries facts.
type ForkReport struct {
	TranscriptPath string         // Path to the subagent's transcript file (engine locates per provider layout).
	AgentCalls     int            // Agent tool_use attempts inside the subagent's transcript (counted even when denied; defect signal).
	WriteCalls     int            // Write/Edit/NotebookEdit tool_use attempts (defect signal; forks expected to review, not mutate).
	WritePaths     []string       // File paths of every fork Write/Edit/NotebookEdit block, in transcript order (analog of ForkAudit.ParentWrites).
	WriteEvents    []WriteEvent   // One event per fork Write/Edit/NotebookEdit block with a path, in transcript order, with time and result.
	BashCommands   []string       // Every Bash tool_use command string in the subagent's transcript, verbatim and in order (caller classifies mutating or disallowed).
	ToolCalls      map[string]int // Tool_use count keyed by tool name (caller inspects unusual volume).
	ReportReturned bool           // Whether fork produced a final assistant message ("report returned" signal).
	// FreshSubagent is true for a subagent started with a fresh context and false for a fork.
	FreshSubagent bool
	// SpawnerTranscriptPath is the spawning subagent's TranscriptPath, empty when the parent session spawned this one.
	SpawnerTranscriptPath string
}
