// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so fabriccli's git-spawning fixtures never
// inherit the operator's global gitconfig (see
// PATTERN-test-isolation).
// The binary also runs under tmux isolation through tmuxkit.Main.

package fabriccli_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test spawns git, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
