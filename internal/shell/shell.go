// shell.go defines the Shell interface — pane-shell mechanics (argument quoting, the call operator,
// the prompt-file read idiom, session-scoped env export, live-PATH prepend, and single-line statement
// chaining) that every provider engine composes its launch/resume command strings from — and the
// ForGOOS/Pwsh/Posix constructors that select or directly expose an implementation.

package shell

import (
	"runtime"
	"strings"
)

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
	// ExportEnv returns a standalone statement that exports key with value into the shell session.
	// Session-scoped in both dialects.
	// This differs deliberately from WithEnv, which is command-scoped on POSIX and therefore
	// emits nothing usable for a pane whose command is empty.
	ExportEnv(key, value string) string
	// PrependPathEntry returns a standalone statement that prepends dir to the shell's own live
	// PATH.
	// The statement references the live PATH variable rather than baking in a value computed by
	// the calling Go process, and it must not leave a trailing empty PATH entry when PATH is
	// unset or empty.
	PrependPathEntry(dir string) string
	// Chain joins statements into one single-line string safe to hand to a send-keys literal
	// payload.
	// The separator is ";" rather than "&&", so a rejected earlier statement cannot suppress a
	// later one, and empty parts are dropped so a trailing separator is never emitted.
	Chain(parts ...string) string
}

// chainStatements drops empty strings from parts and joins the remainder with "; ".
// Both dialects' Chain methods delegate to this so the joining rule is declared once rather than
// duplicated per dialect.
func chainStatements(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "; ")
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
