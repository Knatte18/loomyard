// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so gitexec's git-spawning fixtures never
// inherit the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment
// Invariant).
// The binary also runs under tmux isolation through tmuxkit.Main.
// This file lives in the external package gitexec_test, matching the directory's existing
// external-only test files.

package gitexec_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this package spawns git, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
