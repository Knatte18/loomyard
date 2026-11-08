// reexecguard.go holds Main's refusal of a test binary started with a positional first argument.

package tmuxkit

import (
	"fmt"
	"io"
	"strings"
)

// refusedReexecExit is the exit status Main returns for a test binary started with a positional first argument.
const refusedReexecExit = 2

// positionalFirstArg returns the first of args when it is not a flag.
// Only the first word is read, since a later word may be a flag's value, as in `-test.run Foo`.
func positionalFirstArg(args []string) (string, bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", false
	}
	return args[0], true
}

// refusesReexec reports whether args, a test binary's arguments after its path, open with a positional word.
// It writes the refusal to stderr when they do.
// A pane, tmux job, binding or child that names a test binary in place of lym hands it a lym verb such as `slack bar`.
// Go's flag parsing stops at that word, so the binary would run its whole suite,
// and each session that suite starts could run the same command again, without end.
func refusesReexec(stderr io.Writer, args []string) bool {
	arg, ok := positionalFirstArg(args)
	if !ok {
		return false
	}
	fmt.Fprintf(stderr, "tmuxkit: refusing the non-test arguments %q: this is a go test binary, not lym, "+
		"so it runs no suite for a command that names it in place of lym (PATTERN-child-processes)\n", append([]string{arg}, args[1:]...))
	return true
}
