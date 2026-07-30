// headerpane.go implements headerLaunchCmd/headerLaunchLine, the pure
// helpers that compose the shell command line the always-present header pane
// runs. They are kept separate from the boot site (lifecycle.go) that
// actually creates the pane so the command-string assembly stays
// host-testable with a fake exe: the real os.Executable() lookup happens
// only at the boot site, never here.

package reedengine

import "github.com/Knatte18/loomyard/internal/shell"

// headerLaunchCmd returns the shell command line the header pane runs at
// boot: exe invoked via sh's call operator, followed by the "reed header
// --blocking" arguments, each quoted through sh so the whole line is
// composed entirely via the Shell Mechanics Seam rather than any raw shell
// syntax of reedengine's own. The header pane always runs exactly this
// command — it never receives a strand's cmd/resumeCmd and is never added
// to ReedState.Strands. sh and exe are both parameters (never resolved
// in-function) so this stays a pure, host-testable function; the boot site
// (lifecycle.go) supplies shell.ForGOOS() and os.Executable()'s result.
func headerLaunchCmd(sh shell.Shell, exe string) string {
	return sh.Invoke(exe) + " " + sh.Quote("reed") + " " + sh.Quote("header") + " " + sh.Quote("--blocking")
}

// headerLaunchLine returns the command line to type into the freshly split
// header pane, or "" when the pane must be left as a bare blocking shell
// instead: underTest guards the go-test context, where exe is the TEST
// BINARY rather than the lyx CLI. A Go test binary invoked with positional
// arguments ("reed header --blocking") does not error — flag parsing stops
// at the first non-flag argument — it silently runs its ENTIRE suite with
// no -run filter, so under -tags smoke the header pane would re-run every
// live-substrate test from inside the pane, each booting a fresh server
// whose own header pane repeats the re-exec: unbounded recursion (the
// 2026-07-30 RAM-exhaustion incidents). The header is cosmetic; a bare
// shell pane keeps the layout geometry and up/resume idempotence identical.
// The boot site passes testing.Testing() for underTest; it is a parameter
// so both branches stay unit-testable.
func headerLaunchLine(sh shell.Shell, exe string, underTest bool) string {
	if underTest {
		return ""
	}
	return headerLaunchCmd(sh, exe)
}
