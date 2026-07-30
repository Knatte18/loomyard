// reexecguard.go implements the CLI-reexec refusal guard HermeticGitEnv runs
// before anything else: a Go test binary that receives a positional (non-flag)
// leading argument is being mistaken for a CLI by whatever spawned it, and
// must refuse to run rather than silently execute its entire test suite.
//
// The concrete failure this kills: reedengine's header pane launches
// "<os.Executable()> reed header --blocking". Under go test that executable
// IS the test binary, and Go's flag parsing stops at the first non-flag
// argument — so the positional args are ignored, no error is raised, and the
// full suite runs with no -run filter. With -tags smoke that re-runs every
// live-substrate test from inside a pane the suite itself booted, recursively
// (the 2026-07-30 RAM-exhaustion incidents: ~30 tmux server + claude pairs
// from one permitted single-test invocation). reedengine now suppresses the
// re-exec itself (headerLaunchLine's underTest branch); this guard is the
// defense-in-depth backstop for any FUTURE spawn point that hands
// os.Executable() to a child while running under go test.
//
// Only the FIRST argument is inspected: a leading non-flag argument can never
// be a flag's value, so it is unambiguously positional. Later positional-
// looking arguments stay allowed because they can be legitimate two-token
// flag values (`./pkg.test -test.run Foo`).

package lyxtest

import (
	"fmt"
	"os"
	"strings"
)

// cliReexecArg returns the leading positional argument in args and true when
// one exists; args is the os.Args[1:] slice of a test binary. Pure and
// host-testable; the exiting wrapper below stays thin.
func cliReexecArg(args []string) (string, bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", false
	}
	return args[0], true
}

// refuseCLIReexec aborts the test binary with a loud diagnostic when it was
// invoked with a leading positional argument. Called from HermeticGitEnv so
// every TestMain already wired to the hermetic git environment (machine-
// enforced for every git-spawning package — see CONSTRAINTS.md's Hermetic
// Git Test Environment Invariant) gets the guard for free.
func refuseCLIReexec() {
	if arg, ok := cliReexecArg(os.Args[1:]); ok {
		fmt.Fprintf(os.Stderr,
			"lyxtest: this test binary was invoked with positional argument %q — it is a Go test suite, not a CLI; "+
				"refusing to run the full suite (a spawned child re-executing os.Executable() under go test reaches "+
				"this path — see CONSTRAINTS.md, Live-Substrate Spawn Observability, and the 2026-07-30 header "+
				"re-exec incident)\n", arg)
		os.Exit(2)
	}
}
