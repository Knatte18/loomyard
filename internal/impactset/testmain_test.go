// testmain_test.go runs the test binary under the hermetic git environment and tmux isolation.
// It carries no build tag, so it compiles into every tag set.

package impactset

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain installs the hermetic git environment and runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
