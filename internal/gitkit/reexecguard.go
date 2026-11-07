// reexecguard.go holds the guard that stops a Go test binary from running its whole suite.
// It backstops any spawn point that hands os.Executable() to a child while the binary runs under go test.
// A child that re-executes the test binary with a leading positional argument would otherwise run it unfiltered:
// Go's flag parsing stops at the first non-flag argument and ignores the rest.
// Only the first argument is inspected, because a leading non-flag argument cannot be a flag's value.
// Later positional-looking arguments stay allowed, because they can be two-token flag values.
// For example, `./pkg.test -test.run Foo` passes Foo as the value of -test.run.

package gitkit

import (
	"fmt"
	"os"
	"strings"
)

// cliReexecArg returns the leading positional argument in args (or empty string and false).
func cliReexecArg(args []string) (string, bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", false
	}
	return args[0], true
}

// refuseCLIReexec exits the test binary with status 2 when its first argument is not a flag.
func refuseCLIReexec() {
	if arg, ok := cliReexecArg(os.Args[1:]); ok {
		fmt.Fprintf(os.Stderr,
			"gitkit: this test binary was invoked with positional argument %q — it is a Go test suite, not a CLI; "+
				"refusing to run the full suite (a spawned child re-executing os.Executable() under go test reaches "+
				"this path — see PATTERN-child-processes and the 2026-07-30 header "+
				"re-exec incident)\n", arg)
		os.Exit(2)
	}
}
