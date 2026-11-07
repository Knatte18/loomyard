// testmain_test.go wires the package's test binary into the hermetic git environment (see PATTERN-test-isolation).
// gitkit.HermeticGitEnv() runs before any test, so spawned git never inherits the operator's global gitconfig.
// tmuxkit.Main then runs the tests and removes the run directory that HermeticGitEnv writes its neutral config into.

package gitoracle

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test spawns git, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
