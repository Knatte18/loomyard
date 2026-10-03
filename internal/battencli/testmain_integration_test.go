//go:build integration

// testmain_integration_test.go wires this package's integration test binary into the hermetic git
// test environment: gitkit.HermeticGitEnv() runs once before any test, since
// lifecycle_integration_test.go spawns git via hubforge fixtures (Test Tier Purity Invariant /
// Hermetic Git Test Environment Invariant), in the shape internal/landingshed's own equivalent
// already uses.
// The binary also runs under tmux isolation through tmuxkit.Main.

package battencli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/ideengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs HermeticGitEnv before any test spawns git, and replaces ideengine.CodeLauncher with a no-op for the whole binary:
// the tests stub Spawn but keep the real OpenIDE, so any that reaches a successful spawn runs the real driven path, and none may start a real `code` process.
// It then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	ideengine.CodeLauncher = func(string) error { return nil }
	os.Exit(tmuxkit.Main(m))
}
