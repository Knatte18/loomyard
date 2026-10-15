// Package shellguard is the one neutral table of every guard lyx installs against shell commands an agent runs.
// An engine renders each row of Rows in its own tool-guard mechanism, and the no-denied-recovery scan reads the rows' Forms.
// A row carries what the guard matches, the deny reason shown to the agent, the sentence announcing it in the session's system prompt, and the config switch it hangs on.
//
// The payload contract is part of the table's interface:
// every pattern is a grep -E pattern matched against a JSON payload whose shell command is a JSON string value under a "command" key, as Claude's PreToolUse payload is.
// An engine renders the rows against a payload of that shape, or the task that adds the engine changes the table and this contract.
//
// Its imports are the standard library and the two packages that own the environment-variable names the raw go row's pattern names.
package shellguard
