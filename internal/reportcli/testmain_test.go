// testmain_test.go runs the package's tests under the hermetic git environment and tmuxkit.Main, so the hub-backed integration test never reaches the operator's gitconfig and every temp file lives in the process run directory.

package reportcli

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
