// Package shellguard is the one neutral table of every guard lym installs against shell commands an agent runs.
// An engine renders each row of Rows in its own tool-guard mechanism, and the no-denied-recovery scan reads the rows' Forms.
// A row carries what the guard matches, the deny reason shown to the agent, the sentence announcing it in the session's system prompt, and the config switch it hangs on.
//
// The payload contract is part of the table's interface:
// every pattern is a grep -E pattern matched against a JSON payload whose shell command is a JSON string value under a "command" key, as the PreToolUse payloads of Claude and Codex are.
// An engine renders the rows against a payload of that shape, or the task that adds the engine changes the table and this contract.
// A pattern may hold a single quote, so an engine quotes each pattern for its shell rather than wrapping it in bare single quotes.
//
// A command-shaped row reads the command from the start of the "command" value as the shell would, one unit at a time:
// unquoted text, a single-quoted string, a double-quoted string, and at most one heredoc operator with the rest of its line.
// It matches its command word only in command position reached that way, so text inside quotes, a heredoc body or another payload field does not trip it, bar the quotes the scan misreads.
// Nothing after a heredoc operator's line is read, so the write-then-run shape, a heredoc that writes a script followed by a line that runs it, leaves the later lines unguarded.
// A command substitution inside a double-quoted string and a < before a quote or a backslash, as in <"f" or <<<"f", are not read as command position either, so a command behind them passes.
// Every row is a guardrail over static text and not a barrier; each pattern's comment states what it lets through and what it falsely denies.
//
// The gate rule, how to run lym gate test, is stated once in the system prompt, in the raw go row's notice;
// the refuse-raw row's notice names only what it refuses, while the steers of the raw go and refuse-raw rows each carry the full rule, so a denied call has the way forward at hand.
// The raw go row's steer and notice also name a package-scoped go build or go vet as the compile check, each only while no refuse-raw prefix refuses it, so a refused ./... run has a way forward short of a gate run.
// The edit rule, reading a file with Read and changing it with Edit or Write, is stated likewise: once in the system prompt, in the shell-edit row's notice, and in full in the steers of the shell-edit and python rows.
// The python row's notice names a version check as refused, since agents otherwise probe for Python before choosing how to edit or parse.
// Its steer and notice both give the reason, that Python differs from machine to machine and is often slow, and the way forward for work that needs Python itself: the agent says so to the operator or its parent, who can turn the switch off for the hub.
// The headless claude row's steer and notice name a subagent through the Agent tool as the way to hand work off, since a subagent's work shows in the session's own pane and a headless session's shows in none.
//
// The plain steer and notice name Claude's tools.
// A row whose text names a tool another engine lacks carries that engine's own text in EngineTexts, keyed by the engine's registry name, and SteerFor and NoticeFor return it, or the plain text for an engine without one.
// The Codex texts name Codex's apply_patch tool for the edit rule and Codex's own subagents for the headless claude row.
//
// A row hangs on a Switch.
// A SwitchNone row is installed for every run, and SwitchDenyPython follows the shuttle config's python key.
// SwitchDenyPublish, which the publish row hangs on, is turned on only by a launch spec, so only the session that sets it cannot run git push or a gh call outside the reads, comments and title and body edits the row's table lets through.
// The row's pattern refuses every gh command and subcommand the table does not list, through a generated pattern that matches any word but the listed ones, so a new gh subcommand is refused until the table lists it.
//
// Its imports are the standard library and the two packages that own the environment-variable names the raw go row's pattern names.
package shellguard
