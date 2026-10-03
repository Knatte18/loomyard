// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so boardengine's new git-worktree fixture
// (sync_integration_test.go) never inherits the operator's global gitconfig (see CONSTRAINTS.md's
// Hermetic Git Test Environment Invariant).
// The binary also runs under tmux isolation through tmuxkit.Main.
// Mirrors internal/fabricengine/testmain_test.go.

package boardengine

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
