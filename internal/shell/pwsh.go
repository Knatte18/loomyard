// pwsh.go implements the Shell interface for PowerShell (pwsh), the pane shell tmux
// launches on Windows.

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

// ReadFile returns the pwsh `(Get-Content -Raw <quoted path>)` idiom,
// expanding path's contents into a single argument.
func (p pwshShell) ReadFile(path string) string {
	return "(Get-Content -Raw " + p.Quote(path) + ")"
}

// WithEnv prefixes cmd with `$env:key = <quoted value>; ` (session-wide, acceptable for per-run panes).
func (p pwshShell) WithEnv(key, value, cmd string) string {
	return "$env:" + key + " = " + p.Quote(value) + "; " + cmd
}
