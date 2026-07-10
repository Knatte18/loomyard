// pwsh.go implements the Shell interface for PowerShell (pwsh), the pane shell psmux
// launches on Windows.

package shell

import "strings"

// pwshShell implements Shell for pwsh. It carries no state — every method is a pure
// function of its arguments, so a single zero-value pwshShell is safe to share across
// concurrent callers.
type pwshShell struct{}

// Quote wraps s in pwsh single quotes, doubling any embedded single quote (pwsh's own
// escape for a literal ' inside a single-quoted string) so a path containing a quote or
// a space still round-trips as one argument.
func (pwshShell) Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// Invoke returns the pwsh call-operator form ("& <quoted bin>"): pwsh does not execute a
// bare quoted string as a command, so the leading & is required to run bin rather than
// merely evaluate the string.
func (p pwshShell) Invoke(bin string) string {
	return "& " + p.Quote(bin)
}

// ReadFile returns the pwsh `(Get-Content -Raw <quoted path>)` idiom, which expands
// path's entire byte-exact contents into a single command-line argument — the mechanism
// every provider engine relies on to keep a large or quote-laden prompt off of psmux
// send-keys and pwsh string escaping.
func (p pwshShell) ReadFile(path string) string {
	return "(Get-Content -Raw " + p.Quote(path) + ")"
}
