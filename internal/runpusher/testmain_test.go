// testmain_test.go runs the package's tests under tmuxkit.Main, in the hermetic git environment its hub fixtures need.
// Their temp files then live in the process run directory and go with it.

package runpusher

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
