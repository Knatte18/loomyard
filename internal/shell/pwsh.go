// pwsh.go implements the Shell interface for PowerShell (pwsh), the pane shell tmux launches on
// Windows.

package shell

import "strings"

// pwshShell implements Shell for pwsh. It carries no state and is safe to share.
type pwshShell struct{}

// Quote wraps s in pwsh single quotes, doubling embedded single quotes (pwsh's escape).
func (pwshShell) Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// Invoke returns the pwsh call-operator form ("& <quoted bin>").
func (p pwshShell) Invoke(bin string) string {
	return "& " + p.Quote(bin)
}

// readFile returns the pwsh `(Get-Content -Raw <quoted path>)` idiom, expanding path's contents
// into a single argument; Source builds on it.
func (p pwshShell) readFile(path string) string {
	return "(Get-Content -Raw " + p.Quote(path) + ")"
}

// WithEnv prefixes cmd with `$env:key = <quoted value>; ` (session-wide, acceptable for per-run
// panes).
func (p pwshShell) WithEnv(key, value, cmd string) string {
	return "$env:" + key + " = " + p.Quote(value) + "; " + cmd
}

// Touch returns the `New-Item -ItemType File -Force -Path <quoted path> | Out-Null` idiom.
// `-Force` is what makes an existing file be truncated rather than an error, and `| Out-Null`
// suppresses the `FileInfo` object pwsh would otherwise emit.
// Only the POSIX dialect is executed in practice today (see reedengine's resizeHookCommand,
// which runs through tmux's `run-shell` on GOOS-selected pane shells).
func (p pwshShell) Touch(path string) string {
	return "New-Item -ItemType File -Force -Path " + p.Quote(path) + " | Out-Null"
}

// ExportEnv returns a pwsh `$env:key = <quoted value>` statement, session-scoped.
func (p pwshShell) ExportEnv(key, value string) string {
	return "$env:" + key + " = " + p.Quote(value)
}

// PrependPathEntry returns a pwsh `$env:PATH = <quoted dir> + $(if ($env:PATH) { ... })`
// statement.
// The `if` has no `else`, so the subexpression yields nothing when $env:PATH is empty or unset
// and the assignment is the directory alone.
func (p pwshShell) PrependPathEntry(dir string) string {
	return "$env:PATH = " + p.Quote(dir) + ` + $(if ($env:PATH) { [IO.Path]::PathSeparator + $env:PATH })`
}

// Source returns a pwsh dot-source of a script block built from the file's text, `. ([scriptblock]::Create((Get-Content -Raw <quoted path>)))`.
// It is never a dot-source of the file path itself: evaluating text runs no script file,
// so no ExecutionPolicy (Restricted, AllSigned, Windows PowerShell 5.1's client default) can refuse it,
// and a refused launch line would fail the strand launch.
func (p pwshShell) Source(path string) string {
	return ". ([scriptblock]::Create(" + p.readFile(path) + "))"
}

// EnvRef returns the double-quoted `"$env:KEY"` expansion, which stays one argument whatever the value holds.
func (pwshShell) EnvRef(key string) string {
	return `"$env:` + key + `"`
}

// ScriptExt returns ".ps1".
func (pwshShell) ScriptExt() string {
	return ".ps1"
}

// Chain joins parts with "; ", dropping empty parts. See chainStatements.
func (p pwshShell) Chain(parts ...string) string {
	return chainStatements(parts...)
}
