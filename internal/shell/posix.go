// posix.go implements the Shell interface for a POSIX shell (sh/bash), the pane shell tmux launches
// on Linux.
// It is deliberately plain (untagged) Go — not a `_linux.go` file — so it is host-testable on
// Windows even though it is only ever *selected* at runtime on Linux (see ForGOOS in shell.go).

package shell

import "strings"

// posixShell implements Shell for a POSIX shell. It carries no state and is safe to share.
type posixShell struct{}

// Quote wraps s in POSIX single quotes, escaping each embedded quote by closing the quoting, adding a backslash-escaped quote and reopening it.
func (posixShell) Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Invoke returns bin quoted as a bare command (POSIX shells run quoted strings directly).
func (p posixShell) Invoke(bin string) string {
	return p.Quote(bin)
}

// WithEnv prefixes cmd with a POSIX command-scoped assignment (key=value cmd).
func (p posixShell) WithEnv(key, value, cmd string) string {
	return key + "=" + p.Quote(value) + " " + cmd
}

// Touch returns the POSIX `: > <quoted path>` idiom: the `:` no-op builtin plus an output
// redirection, so the fragment creates or truncates path without spawning any process.
func (p posixShell) Touch(path string) string {
	return ": > " + p.Quote(path)
}

// ExportEnv returns a POSIX `export key=<quoted value>` statement, session-scoped.
func (p posixShell) ExportEnv(key, value string) string {
	return "export " + key + "=" + p.Quote(value)
}

// PrependPathEntry returns a POSIX `export PATH=<quoted dir>${PATH:+:$PATH}` statement.
// The `${PATH:+…}` parameter expansion yields nothing when PATH is unset or empty, which is what
// keeps a trailing `:` — read by POSIX shells as the current directory — from ever being
// produced.
func (p posixShell) PrependPathEntry(dir string) string {
	return "export PATH=" + p.Quote(dir) + `${PATH:+:$PATH}`
}

// ChangeDir returns the POSIX `cd <quoted dir>` statement.
func (p posixShell) ChangeDir(dir string) string {
	return "cd " + p.Quote(dir)
}

// Source returns the POSIX `. <quoted path>` statement, which runs the file's statements in the current shell's own scope.
func (p posixShell) Source(path string) string {
	return ". " + p.Quote(path)
}

// EnvRef returns the double-quoted `"${KEY}"` expansion, so an unset or spaced value stays one word.
func (posixShell) EnvRef(key string) string {
	return `"${` + key + `}"`
}

// ScriptExt returns ".sh".
func (posixShell) ScriptExt() string {
	return ".sh"
}

// DiscardLineKey returns "C-u", which erases the pending line both in the tty's canonical mode, before the shell's line editor starts, and in readline's unix-line-discard.
func (posixShell) DiscardLineKey() string {
	return "C-u"
}

// Chain joins parts with "; ", dropping empty parts. See chainStatements.
func (p posixShell) Chain(parts ...string) string {
	return chainStatements(parts...)
}

// OrElse returns `first || second`.
func (posixShell) OrElse(first, second string) string {
	return first + " || " + second
}

// Exit returns the POSIX `exit` statement.
func (posixShell) Exit() string {
	return "exit"
}

// DiscardOutput returns cmd with its standard output and standard error redirected to /dev/null.
func (posixShell) DiscardOutput(cmd string) string {
	return cmd + " >/dev/null 2>&1"
}

// ExitSuccess returns the POSIX `exit 0` statement.
func (posixShell) ExitSuccess() string {
	return "exit 0"
}

// CommandFlag returns the `-c` flag that makes a POSIX shell run its next argument as a command line.
func (posixShell) CommandFlag() string {
	return "-c"
}

// ChangeDirToWord returns the POSIX `cd <word>` statement, word being one already-quoted shell word.
func (posixShell) ChangeDirToWord(word string) string {
	return "cd " + word
}
