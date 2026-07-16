// forkaudit.go defines the provider-invariant value types a fork-authorized run's
// audit surfaces: ForkAudit summarizes the parent session's own fork-spawning
// behavior, and ForkReport summarizes one fork subagent's transcript. Both are plain
// value types with no methods — the engine populates them from provider-specific
// transcript formats (claudeengine's own knowledge, never this package's), and a
// caller (burlerengine's cluster round) interprets the counts against its own policy.

package shuttleengine

// ForkAudit summarizes the fork-spawning behavior observed in a fork-authorized run's
// parent session: how many Agent tool invocations it made (SpawnCalls) and, of those,
// how many carried a name parameter (NamedSpawns). A named spawn is a defect signal —
// named forks silently lose inherited context in Claude Code ≤2.1.206 — so a caller
// enforcing the fail-loud posture treats NamedSpawns > 0 as a hard error, not a
// warning. Forks holds one ForkReport per fork subagent the parent session spawned.
type ForkAudit struct {
	// Forks holds one ForkReport per fork subagent observed in the parent session,
	// in the order the engine discovered them.
	Forks []ForkReport
	// SpawnCalls counts Agent tool invocations observed in the parent session's own
	// transcript — attempts, not confirmations that a fork actually ran.
	SpawnCalls int
	// NamedSpawns counts SpawnCalls that carried a name parameter. Named forks
	// silently lose inherited context in Claude Code ≤2.1.206, so a non-zero
	// NamedSpawns is a defect signal a caller should hard-error on, not merely warn
	// about.
	NamedSpawns int
}

// ForkReport summarizes one fork subagent's transcript: what it attempted (AgentCalls,
// WriteCalls, BashCommands) and whether it produced a final report (ReportReturned).
// Policy over these fields — e.g. what counts as a git-mutating Bash command, or
// whether a nested Agent call is itself a hard error — is the caller's job, not this
// package's; ForkReport only carries the observed facts.
type ForkReport struct {
	// TranscriptPath is the path to the fork's own transcript file, as located by the
	// engine (claudeengine's own knowledge of the provider's transcript layout — see
	// the Shuttle Provider-Seam Invariant and Shell Mechanics Seam Shared Decision).
	TranscriptPath string
	// AgentCalls counts Agent tool_use attempts inside the fork's own transcript.
	// Attempts count even when the tool call was denied — the fail-loud posture
	// treats an attempted nested spawn as a defect regardless of whether it was
	// allowed to proceed.
	AgentCalls int
	// WriteCalls counts Write/Edit/NotebookEdit tool_use attempts inside the fork's
	// own transcript — a fork subagent is expected to review, not mutate files, so a
	// non-zero WriteCalls is a defect signal a caller may hard-error on.
	WriteCalls int
	// BashCommands carries every Bash tool_use command string observed in the fork's
	// own transcript, verbatim and in order. Classifying which of these are
	// git-mutating (or otherwise disallowed) is the caller's policy, not this
	// package's — ForkReport only carries the raw strings.
	BashCommands []string
	// ToolCalls counts every tool_use observed in the fork's own transcript, keyed by
	// tool name, so a caller can inspect unusual tool-call volume without this
	// package hardcoding which tool names matter.
	ToolCalls map[string]int
	// ReportReturned reports whether the fork produced a final assistant message —
	// its own "report returned" signal, independent of whether it also mutated
	// anything it should not have.
	ReportReturned bool
}
