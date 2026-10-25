// testmain_test.go runs the package's tests under the hermetic git environment and tmuxkit.Main, so tests that spawn git never reach the operator's gitconfig and every temp file lives in the process run directory.

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
