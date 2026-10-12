// testmain_test.go runs the package's tests under the hermetic git environment and tmuxkit.Main, so the hub-backed integration tests never reach the operator's gitconfig and every temp file lives in the process run directory.

package pausecli

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
