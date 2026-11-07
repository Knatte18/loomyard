// testmain_test.go runs the package's tests under the hermetic git environment and tmuxkit.Main.
// Together they keep tests that spawn git away from the operator's gitconfig,
// and they put every temp file in the process run directory.

package driftengine

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
