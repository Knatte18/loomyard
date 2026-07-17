// posix.go implements the Shell interface for a POSIX shell (sh/bash), the pane shell
// tmux launches on Linux. It is deliberately plain (untagged) Go — not a `_linux.go`
// file — so it is host-testable on Windows even though it is only ever *selected* at
// runtime on Linux (see ForGOOS in shell.go).

package shell

import "strings"

// posixShell implements Shell for a POSIX shell. It carries no state — every method is
// a pure function of its arguments, so a single zero-value posixShell is safe to share
// across concurrent callers.
type posixShell struct{}

// Quote wraps s in POSIX single quotes, closing the quoted string around any embedded
// single quote and re-opening it after a backslash-escaped literal quote ('\”) — the
// standard POSIX idiom, since a POSIX single-quoted string has no in-quote escape
// character of its own.
func (posixShell) Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Invoke returns bin quoted as a bare command: unlike pwsh, a POSIX shell runs a quoted
// string directly, with no call operator required.
func (p posixShell) Invoke(bin string) string {
	return p.Quote(bin)
}

// ReadFile returns a double-quoted command substitution that expands path's contents
// into a single command-line argument, reproducing pwsh's `(Get-Content -Raw path)`
// single-argument-prompt semantics: the outer double quotes are load-bearing, since an
// unquoted `$(cat path)` would let the shell word-split and glob-expand the substituted
// text into multiple arguments. This is a documented, benign divergence rather than a
// byte-exact one — POSIX command substitution strips trailing newlines from its output,
// whereas Get-Content -Raw is byte-exact — which is harmless for prompt text, since no
// caller depends on trailing-newline preservation there.
func (p posixShell) ReadFile(path string) string {
	return `"$(cat ` + p.Quote(path) + `)"`
}

// WithEnv prefixes cmd with a POSIX command-scoped assignment (`key=value cmd`), which
// sets the variable only for the duration of cmd's own execution — no leakage into the
// rest of the pane session. value is always quoted, never interpolated raw.
func (p posixShell) WithEnv(key, value, cmd string) string {
	return key + "=" + p.Quote(value) + " " + cmd
}
