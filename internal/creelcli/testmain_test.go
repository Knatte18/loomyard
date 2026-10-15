// testmain_test.go runs the package's tests under the hermetic git environment and tmuxkit.Main, so the hub-backed tests never reach the operator's gitconfig and every temp file lives in the process run directory.

package creelcli

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
