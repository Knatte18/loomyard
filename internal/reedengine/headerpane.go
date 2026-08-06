// headerpane.go implements headerLaunchCmd/headerLaunchLine, the pure
// helpers that compose the shell command line the always-present header pane
// runs. They are kept separate from the boot site (lifecycle.go) that
// actually creates the pane so the command-string assembly stays
// host-testable with a fake exe: the real os.Executable() lookup happens
// only at the boot site, never here.

package reedengine

import "github.com/Knatte18/loomyard/internal/shell"

// headerLaunchCmd returns the shell command line the header pane runs at boot.
func headerLaunchCmd(sh shell.Shell, exe string) string {
	return sh.Invoke(exe) + " " + sh.Quote("reed") + " " + sh.Quote("header") + " " + sh.Quote("--blocking")
}

// headerLaunchLine returns the command line to type into the header pane,
// or "" when the pane must be left as a bare shell (underTest=true).
func headerLaunchLine(sh shell.Shell, exe string, underTest bool) string {
	if underTest {
		return ""
	}
	return headerLaunchCmd(sh, exe)
}
