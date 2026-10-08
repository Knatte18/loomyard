//go:build integration

// testmain_integration_test.go wires this package's integration test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, since ensure_integration_test.go spawns git through hubforge fixtures.

package hubreconcile_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs HermeticGitEnv before any test spawns git.
// It sets FABRIC_SKIP_PUSH so a commit's detached push child never re-executes the test binary, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Setenv("FABRIC_SKIP_PUSH", "1")
	os.Exit(tmuxkit.Main(m))
}
