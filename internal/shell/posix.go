// posix.go implements the Shell interface for a POSIX shell (sh/bash), the pane shell tmux launches
// on Linux.
// It is deliberately plain (untagged) Go — not a `_linux.go` file — so it is host-testable on
// Windows even though it is only ever *selected* at runtime on Linux (see ForGOOS in shell.go).

package shell

import "strings"

// posixShell implements Shell for a POSIX shell. It carries no state and is safe to share.
type posixShell struct{}

// Quote wraps s in POSIX single quotes, escaping embedded quotes via '\'' idiom.
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
