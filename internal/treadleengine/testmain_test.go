// testmain_test.go wires the package's test binary into the hermetic git test environment.
// gitkit.HermeticGitEnv() runs once before any test,
// so the git-spawning fixtures never inherit the operator's global gitconfig.

package treadleengine

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
