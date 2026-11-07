// testmain_test.go runs the package's tests under the hermetic git environment and tmuxkit.Main.
// The hermetic environment keeps the hub-backed integration tests off the operator's gitconfig.
// tmuxkit.Main keeps every temp file in the process run directory.

package hibernatecli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
