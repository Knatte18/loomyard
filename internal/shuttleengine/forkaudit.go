// forkaudit.go defines the provider-invariant value types a fork-authorized run's
// audit surfaces: ForkAudit summarizes the parent session's own fork-spawning
// behavior, and ForkReport summarizes one fork subagent's transcript. Both are plain
// value types with no methods — the engine populates them from provider-specific
// transcript formats (claudeengine's own knowledge, never this package's), and a
// caller (burlerengine's cluster round) interprets the counts against its own policy.

package shuttleengine

// ForkAudit summarizes fork-spawning behavior in a fork-authorized run's parent session.
// It counts Agent tool invocations (SpawnCalls) and named spawns (NamedSpawns, a defect signal since named forks lose inherited context).
// Forks holds one ForkReport per fork subagent the parent spawned.
type ForkAudit struct {
	Forks              []ForkReport // One ForkReport per fork subagent, in discovery order.
	SpawnCalls         int          // Agent tool invocations in parent's transcript (attempts, not confirmations).
	NamedSpawns        int          // SpawnCalls with a name parameter (defect signal; named forks lose inherited context).
	ParentWriteCalls   int          // Write/Edit/NotebookEdit tool_use blocks in parent's transcript (policy interpretation is caller's).
	ParentWrites       []string     // File paths of every parent Write/Edit/NotebookEdit block, in transcript order (caller's policy for interpretation).
	ParentBashCommands []string     // Verbatim Bash commands in parent's transcript (caller classifies git-mutating or disallowed).
}

// ForkReport summarizes one fork subagent's transcript: what it attempted and whether it produced a final report.
// Policy over these fields (e.g. what counts as git-mutating, or whether nested Agent calls are errors) is the caller's job; ForkReport only carries facts.
type ForkReport struct {
	TranscriptPath string         // Path to the fork's transcript file (engine locates per provider layout).
	AgentCalls     int            // Agent tool_use attempts inside fork's transcript (counted even when denied; defect signal).
	WriteCalls     int            // Write/Edit/NotebookEdit tool_use attempts (defect signal; forks expected to review, not mutate).
	WritePaths     []string       // File paths of every fork Write/Edit/NotebookEdit block, in transcript order (analog of ForkAudit.ParentWrites).
	BashCommands   []string       // Every Bash tool_use command string in fork's transcript, verbatim and in order (caller classifies mutating or disallowed).
	ToolCalls      map[string]int // Tool_use count keyed by tool name (caller inspects unusual volume).
	ReportReturned bool           // Whether fork produced a final assistant message ("report returned" signal).
}
