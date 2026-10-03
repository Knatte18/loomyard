//go:build integration

// testmain_integration_test.go wires this package's integration test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, since teardown_integration_test.go spawns git via hubforge fixtures (Test Tier Purity Invariant / Hermetic Git Test Environment Invariant).
// The binary also runs under tmux isolation through tmuxkit.Main.

package pairteardown

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
