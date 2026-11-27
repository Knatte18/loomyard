// shell.go defines the Shell interface — pane-shell mechanics (argument quoting, the call operator, a line appended to a file, session-scoped env export, live-PATH prepend, single-line statement chaining, script sourcing, the sourced-script file extension, and the key that discards an unsubmitted input line, output discard, zero exit, the command-line flag, and a directory change to an already-quoted word) that every provider engine composes its launch/resume command strings from — and the ForGOOS/Pwsh/Posix constructors that select or directly expose an implementation.

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
	// WithEnv returns cmd prefixed with key=value set for execution.
	// The implementations diverge in scope: POSIX command-scoped vs. pwsh session-wide (acceptable for per-run panes).
	WithEnv(key, value, cmd string) string
	// Touch returns the shell syntax that creates path as an empty file, truncating it if it
	// already exists.
	Touch(path string) string
	// AppendLine returns a standalone statement that appends line and a line ending to the file at path, creating the file when it is absent.
	AppendLine(path, line string) string
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
	// ChangeDir returns a standalone statement that changes the shell's current directory to dir.
	ChangeDir(dir string) string
	// Chain joins statements into one single-line string safe to hand to a send-keys literal
	// payload.
	// The separator is ";" rather than "&&", so a rejected earlier statement cannot suppress a
	// later one, and empty parts are dropped so a trailing separator is never emitted.
	Chain(parts ...string) string
	// OrElse returns a line that runs second only when first fails.
	// Each operand must be one pipeline with no statement separator, because ";" binds looser than "||" in both dialects.
	OrElse(first, second string) string
	// Exit returns the statement that ends the pane's shell.
	// Typed into a launch script the pane's shell sources in its own scope, it ends the shell.
	Exit() string
	// ExitSuccess returns the statement that ends the shell with status zero, whatever the previous statement returned.
	// It closes a line typed into a pane whose exit status must not read as a failure.
	ExitSuccess() string
	// DiscardOutput returns cmd with its standard output and standard error discarded, so a pane running it shows nothing.
	DiscardOutput(cmd string) string
	// CommandFlag returns the flag that makes the shell binary run its next argument as a command line.
	CommandFlag() string
	// ChangeDirToWord returns a standalone statement that changes the shell's current directory to the directory word names.
	// word is already one quoted shell word and is not quoted again,
	// so a caller may pass an expansion another tool substitutes into the line.
	ChangeDirToWord(word string) string
	// Source returns a standalone statement that runs the statements in the file at path in the current shell's own scope,
	// so exports and PATH changes land in the calling session.
	Source(path string) string
	// EnvRef returns the syntax that expands the environment variable key, read when the pane shell runs the line, into exactly one argument.
	// An unset or spaced value never splits into several words.
	EnvRef(key string) string
	// ScriptExt returns the file extension, leading dot included, that a script this dialect sources carries.
	ScriptExt() string
	// DiscardLineKey returns the tmux key name that discards the shell's unsubmitted input line, sent ahead of a typed line so stray input never prefixes it.
	// It is empty for a dialect with no key that is safe to send right before typed text.
	DiscardLineKey() string
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
