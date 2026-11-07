// livestate_helpers_test.go holds mustGit, the one helper the live-state harness's test files share.
// It belongs to none of those files, so it stays in this one.
// mustGit is copied from internal/hubforge/hub.go.

package fabricengine_test

import (
	"os/exec"
	"strings"
)

// mustGit runs a git subcommand in dir, panicking on non-zero exit.
// It panics rather than taking a testing.TB.
// State and verb builders often run where no testing.TB is convenient to thread through.
func mustGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		panic("git " + strings.Join(args, " ") + ": " + err.Error() + "; " + string(output))
	}
}
