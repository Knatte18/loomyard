// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so stencilcli's git-spawning fixtures (the
// integration-tagged hub fixture, and this package's own direct board-history commits) never
// inherit the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment
// Invariant).

package stencilcli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this package spawns git, then delegates
// to the normal test run.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(m.Run())
}
