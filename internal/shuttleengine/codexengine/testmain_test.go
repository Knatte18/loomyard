// testmain_test.go gives the package's test binary the hermetic git environment, because a test spawns `sh`.
// It also runs the tests under tmuxkit.Main, so their temp files live in the process run directory and go with it.

package codexengine

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
