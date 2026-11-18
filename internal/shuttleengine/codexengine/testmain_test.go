// testmain_test.go gives the package's test binary the hermetic git environment, because a test spawns `sh`.
// It also runs the tests under tmuxkit.Main, so their temp files live in the process run directory and go with it.

package codexengine

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// testLymPath is the stand-in path of the lym binary that Prepare names in the python guard hook under test.
const testLymPath = "/stub/lym"

// TestMain runs gitkit.HermeticGitEnv() before any test spawns a process, then runs the tests under tmuxkit.Main.
// It points LymBinary at testLymPath, since the test binary is no lym.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	LymBinary = func() (string, error) { return testLymPath, nil }
	os.Exit(tmuxkit.Main(m))
}
