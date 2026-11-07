// testmain_test.go wires the package's test binary into the hermetic git test environment.
// gitkit.HermeticGitEnv() runs once before any test.
// It keeps configcli's git-spawning fixtures from inheriting the operator's global gitconfig.
// The binary also runs under tmux isolation through tmuxkit.Main.

package configcli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain sets the hermetic git environment, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
