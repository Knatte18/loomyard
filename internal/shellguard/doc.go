// Package shellguard is the one neutral table of every guard lym installs against shell commands an agent runs.
// An engine renders each row of Rows in its own tool-guard mechanism, and the no-denied-recovery scan reads the rows' Forms.
// A row carries what the guard matches, the deny reason shown to the agent, the sentence announcing it in the session's system prompt, and the config switch it hangs on.
//
// The payload contract is part of the table's interface:
// every pattern is a grep -E pattern matched against a JSON payload whose shell command is a JSON string value under a "command" key, as Claude's PreToolUse payload is.
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
// A row hangs on a Switch.
// A SwitchNone row is installed for every run, and SwitchDenyPython follows the shuttle config's python key.
// SwitchDenyPublish, which the publish row hangs on, is turned on only by a launch spec, so only the session that sets it cannot run git push or gh.
//
// Its imports are the standard library and the two packages that own the environment-variable names the raw go row's pattern names.
package shellguard
