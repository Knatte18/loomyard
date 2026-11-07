// testmain_test.go sets up the package's test binary.
// gitkit.HermeticGitEnv runs once before any test.
// It keeps git-spawning fixtures from inheriting the operator's global gitconfig; see PATTERN-test-isolation.
// The binary also runs under tmux isolation through tmuxkit.Main.

package boardtest

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
