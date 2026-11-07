// testmain_test.go wires the package's test binary into the hermetic git test environment.
// gitkit.HermeticGitEnv() runs once before any test.
// It keeps webstercli's git-spawning fixtures from inheriting the operator's global gitconfig.

package webstercli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this package spawns git.
// It then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
