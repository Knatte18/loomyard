// headerpane.go implements headerLaunchCmd, the pure helper that composes
// the shell command line the always-present header pane runs. It is kept
// separate from the boot site (lifecycle.go) that actually creates the pane
// so the command-string assembly stays host-testable with a fake exe: the
// real os.Executable() lookup happens only at the boot site, never here.

package muxengine

import "github.com/Knatte18/loomyard/internal/shell"

// headerLaunchCmd returns the shell command line the header pane runs at
// boot: exe invoked via sh's call operator, followed by the "mux header
// --blocking" arguments, each quoted through sh so the whole line is
// composed entirely via the Shell Mechanics Seam rather than any raw shell
// syntax of muxengine's own. The header pane always runs exactly this
// command — it never receives a strand's cmd/resumeCmd and is never added
// to MuxState.Strands. sh and exe are both parameters (never resolved
// in-function) so this stays a pure, host-testable function; the boot site
// (lifecycle.go) supplies shell.ForGOOS() and os.Executable()'s result.
func headerLaunchCmd(sh shell.Shell, exe string) string {
	return sh.Invoke(exe) + " " + sh.Quote("mux") + " " + sh.Quote("header") + " " + sh.Quote("--blocking")
}
