// lyxtest.go implements the shared test-fixture builders and copy helpers
// used across worktree, weft, and paths tests.

package lyxtest

import (
	"os/exec"
	"testing"
)

// MustRun runs a command with the given arguments in the specified directory.
// It calls tb.Fatalf if the command returns a non-zero exit code.
// Call tb.Helper() is delegated to the caller.
func MustRun(tb testing.TB, dir string, args ...string) {
	tb.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir

	if output, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("command failed: %v; output: %s", err, output)
	}
}
