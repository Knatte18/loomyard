// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so any git the oracle's tests spawn never inherits the operator's global gitconfig (see PATTERN-test-isolation).
// The tests then run through tmuxkit.Main, which removes the run directory HermeticGitEnv created once the run ends.
// The file carries no build tag, so its one TestMain compiles under every tag set;
// HermeticGitEnv() only sets environment and is exempt from Test Tier Purity.

package gitoracle

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this package spawns git, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
