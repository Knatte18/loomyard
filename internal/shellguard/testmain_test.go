// testmain_test.go gives the package's test binary an isolated tmux socket directory through tmuxkit.Main, since the package carries an `integration` test file (see PATTERN-test-isolation), and the hermetic git test environment, since that file spawns processes.

package shellguard

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test spawns a process, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
