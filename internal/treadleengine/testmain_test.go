// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so treadleengine's git-spawning fixtures (the
// moved smoke test spawns git via a gitkit fixture helper) never inherit the operator's global
// gitconfig (see PATTERN-test-isolation).

package treadleengine

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs hermetic git environment setup before tests, then runs them under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
