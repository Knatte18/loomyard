// posix.go implements the Shell interface for a POSIX shell (sh/bash), the pane shell tmux launches
// on Linux.
// It is deliberately plain (untagged) Go — not a `_linux.go` file — so it is host-testable on
// Windows even though it is only ever *selected* at runtime on Linux (see ForGOOS in shell.go).

package shell

import "strings"

// posixShell implements Shell for a POSIX shell. It carries no state and is safe to share.
type posixShell struct{}

// Quote wraps s in POSIX single quotes, escaping embedded quotes via the '\'' idiom.
func (posixShell) Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Invoke returns bin quoted as a bare command (POSIX shells run quoted strings directly).
func (p posixShell) Invoke(bin string) string {
	return p.Quote(bin)
}

// ReadFile returns a double-quoted command substitution that expands path's contents into one
// argument, matching pwsh's Get-Content semantics.
func (p posixShell) ReadFile(path string) string {
	return `"$(cat ` + p.Quote(path) + `)"`
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

// Chain joins parts with "; ", dropping empty parts. See chainStatements.
func (p posixShell) Chain(parts ...string) string {
	return chainStatements(parts...)
}
