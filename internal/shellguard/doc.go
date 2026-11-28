// Package shellguard is the one neutral table of every guard lym installs against shell commands an agent runs.
// An engine renders each row of Rows in its own tool-guard mechanism,
// and the no-denied-recovery scan reads the rows' Forms.
// A row carries what the guard matches and the deny reason shown to the agent.
// It also carries the sentence announcing it in the session's system prompt and the config switch it hangs on.
//
// The payload contract is part of the table's interface:
// every pattern is a grep -E pattern matched against a JSON payload, as the PreToolUse payloads of Claude and Codex are.
// The payload's shell command is a JSON string value under a "command" key.
// An engine renders the rows against a payload of that shape, or the table and this contract change with it.
// A pattern may hold a single quote.
// An engine therefore quotes each pattern for its shell rather than wrapping it in bare single quotes.
//
// A command-shaped row reads the command from the start of the "command" value as the shell would, one unit at a time:
// unquoted text, a single-quoted string, a double-quoted string, and at most one heredoc operator plus its line.
// It matches its command word only in command position reached that way.
// Text inside quotes, a heredoc body or another payload field does not trip it, bar the quotes the scan misreads.
// Nothing after a heredoc operator's line is read.
// The write-then-run shape leaves the later lines unguarded:
// a heredoc that writes a script, followed by a line that runs it.
// A command substitution inside a double-quoted string is not read as command position, so a command behind it passes.
// The same holds for a < before a quote or a backslash, as in <"f" or <<<"f".
// Every row is a guardrail over static text and not a barrier;
// each pattern's comment states what it lets through and what it falsely denies.
//
// The gate rule, how to run lym gate test, is stated once in the system prompt, in the raw go row's notice;
// the refuse-raw row's notice names only what it refuses.
// The steers of the raw go and refuse-raw rows each carry the full rule, so a denied call has the way forward at hand.
// The raw go row's steer and notice also name a package-scoped go build or go vet as the compile check.
// Each is named only while no refuse-raw prefix refuses it.
// A refused ./... run thus has a way forward short of a gate run.
// The edit rule, reading a file with Read and changing it with Edit or Write, is stated likewise:
// once in the system prompt, in the shell-edit row's notice;
// and in full in the steers of the shell-edit and python rows.
// The python row's notice names a version check as refused.
// Agents otherwise probe for Python before choosing how to edit.
// Its steer and notice both give the reason: Python differs from machine to machine and is often slow.
// They also give the way forward for work that needs Python itself:
// the agent says so to the operator or its parent, who can turn the switch off for the hub.
// The headless claude row's steer and notice name a subagent through the Agent tool as the way to hand work off.
// A subagent's work shows in the session's own pane, and a headless session's shows in none.
//
// The plain steer and notice name Claude's tools.
// A row whose text names a tool another engine lacks carries that engine's own text in EngineTexts.
// EngineTexts is keyed by the engine's registry name.
// SteerFor and NoticeFor return that text, or the plain text for an engine without one.
// The Codex texts name Codex's apply_patch tool for the edit rule and Codex's own subagents for the headless claude row.
//
// A row hangs on a Switch.
// A SwitchNone row is installed for every run, and SwitchDenyPython follows the shuttle config's python key.
// SwitchDenyPublish, which the publish row hangs on, is turned on only by a launch spec.
// Only the session that sets it cannot run git push or a gh call outside what the row's table lets through:
// reads, comments, and title and body edits.
// The row's pattern refuses every gh command and subcommand the table does not list.
// A generated pattern matches any word but the listed ones, so a new gh subcommand is refused until listed.
//
// It imports the standard library and the packages that own the environment variables the raw go row's pattern names.
package shellguard
