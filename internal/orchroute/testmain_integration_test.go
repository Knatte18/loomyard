//go:build tier1

// testmain_integration_test.go wires this package's tier1 test binary into the hermetic git test environment, since its hub fixtures spawn git.

package orchroute

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs HermeticGitEnv before any test spawns git, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
