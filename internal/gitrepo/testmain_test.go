// testmain_test.go wires the package's test binary into the hermetic git test
// environment: gitkit.HermeticGitEnv() runs once before any test, so
// gitrepo's git-spawning fixtures never inherit the operator's global
// gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment Invariant).
// The binary also runs under tmux isolation through tmuxkit.Main.
// The file carries no build tag, so its one TestMain compiles under every tag set;
// HermeticGitEnv() only sets environment and is exempt from Test Tier Purity.

package gitrepo_test

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
