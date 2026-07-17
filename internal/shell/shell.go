// shell.go defines the Shell interface — pane-shell mechanics (argument quoting, the
// call operator, and the prompt-file read idiom) that every provider engine composes
// its launch/resume command strings from — and the ForGOOS/Pwsh/Posix constructors
// that select or directly expose an implementation.

package shell

import "runtime"

// Shell is the provider-invariant seam for pane-shell mechanics: quoting an argument so
// it survives being typed into a pane as one token, invoking a binary via the shell's
// call syntax, and reading a file's contents into a single command-line argument.
// Implementations carry no state and no provider-specific knowledge — a Claude flag,
// marker string, or hook shape must never appear on this interface or behind it (the
// Shell Mechanics Seam invariant).
type Shell interface {
	// Quote wraps s so it round-trips as exactly one shell argument, escaping any
	// character the shell would otherwise treat as a delimiter or metacharacter.
	Quote(s string) string
	// Invoke returns the shell syntax that runs bin as a command — the call operator
	// under pwsh, a plain quoted path under a posix shell.
	Invoke(bin string) string
	// ReadFile returns the shell syntax that expands path's contents into a single
	// command-line argument, reproducing the "prompt read from a file, not typed
	// inline" idiom every provider engine relies on to keep a large or quote-laden
	// prompt off of tmux send-keys and shell string escaping.
	ReadFile(path string) string
	// WithEnv returns cmd prefixed so that the environment variable key=value is set
	// for the command when the returned line is typed into a pane shell. key must be a
	// plain identifier ([A-Za-z_][A-Za-z0-9_]*); WithEnv performs no key validation of
	// its own (mirroring how Invoke trusts bin) — callers pass compile-time constants.
	// value is always routed through Quote, never interpolated raw, for the same
	// injection-hardening reason documented on buildLaunchCmd in
	// internal/shuttleengine/claudeengine/command.go. The two implementations diverge
	// in scope: a POSIX shell has a command-scoped assignment form, so posixShell's
	// prefix affects only cmd; pwsh has no equivalent, so pwshShell's assignment
	// persists for the rest of the pane session — acceptable because shuttle panes are
	// per-run, so nothing else in the pane's lifetime observes the leaked assignment.
	WithEnv(key, value, cmd string) string
}

// ForGOOS returns the Shell implementation for the pane shell tmux launches on the
// current host: the pwsh impl on Windows, the posix impl everywhere else. Callers that
// need to exercise a specific implementation regardless of host OS should call Pwsh or
// Posix directly instead.
func ForGOOS() Shell {
	if runtime.GOOS == "windows" {
		return Pwsh()
	}
	return Posix()
}

// Pwsh returns the pwsh (PowerShell) pane-shell implementation, directly constructible
// so it is host-testable on any OS regardless of what ForGOOS would select.
func Pwsh() Shell {
	return pwshShell{}
}

// Posix returns the posix pane-shell implementation, directly constructible so it is
// host-testable on any OS regardless of what ForGOOS would select.
func Posix() Shell {
	return posixShell{}
}
