// shell.go defines the Shell interface — pane-shell mechanics (argument quoting, the call operator,
// and the prompt-file read idiom) that every provider engine composes its launch/resume command
// strings from — and the ForGOOS/Pwsh/Posix constructors that select or directly expose an
// implementation.

package shell

import "runtime"

// Shell is the provider-invariant seam for pane-shell mechanics: quoting, invoking, and file
// reading.
// Implementations carry no provider-specific knowledge (Shell Mechanics Seam invariant).
type Shell interface {
	// Quote wraps s so it round-trips as one shell argument.
	Quote(s string) string
	// Invoke returns the shell syntax that runs bin as a command.
	Invoke(bin string) string
	// ReadFile returns the shell syntax that expands path's contents into a single argument.
	ReadFile(path string) string
	// WithEnv returns cmd prefixed with key=value set for execution.
	// The implementations diverge in scope: POSIX command-scoped vs. pwsh session-wide (acceptable for per-run panes).
	WithEnv(key, value, cmd string) string
	// Touch returns the shell syntax that creates path as an empty file, truncating it if it
	// already exists.
	Touch(path string) string
}

// ForGOOS returns the Shell implementation for the current host (pwsh on Windows, posix elsewhere).
func ForGOOS() Shell {
	if runtime.GOOS == "windows" {
		return Pwsh()
	}
	return Posix()
}

// Pwsh returns the pwsh pane-shell implementation, directly constructible for host-agnostic
// testing.
func Pwsh() Shell {
	return pwshShell{}
}

// Posix returns the posix pane-shell implementation, directly constructible for host-agnostic
// testing.
func Posix() Shell {
	return posixShell{}
}
