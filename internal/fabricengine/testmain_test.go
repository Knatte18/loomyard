// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so fabricengine's git-worktree/clone fixtures
// (added by later batches) never inherit the operator's global gitconfig (see CONSTRAINTS.md's
// Hermetic Git Test Environment Invariant).
// This batch's own tests spawn no git, but testmain_test.go is created here so later batches'
// integration tests land in a package that already satisfies the invariant.
// The binary also runs under tmux isolation through tmuxkit.Main.

package fabricengine

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
