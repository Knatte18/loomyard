// testmain_test.go wires the package's test binary into the hermetic git test
// environment: lyxtest.HermeticGitEnv() runs once before any test, so
// boardengine's new git-worktree fixture (sync_integration_test.go) never
// inherits the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git
// Test Environment Invariant). Mirrors internal/fabricengine/testmain_test.go.

package boardengine

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestMain runs lyxtest.HermeticGitEnv() before any test in this package spawns
// git, then delegates to the normal test run.
func TestMain(m *testing.M) {
	lyxtest.HermeticGitEnv()
	os.Exit(m.Run())
}
