// testmain_test.go wires the package's test binary into the hermetic git test environment.
// It runs gitkit.HermeticGitEnv() once before any test.
// This keeps hubforge's git-spawning fixtures from inheriting the operator's global gitconfig.
// The binary also runs under tmux isolation through tmuxkit.Main.
// This call satisfies the invariant in PATTERN-test-isolation for a package whose hub_test.go spawns git.
// cmd/lym/hermeticenv_test.go enforces that invariant by looking for exactly this call.
//
// The call applies to every tier's tests.
// The one test binary holds the integration suite this package's hub_test.go carries.
// Without this call that suite would run without the hermetic environment ever having been installed.

package hubforge

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this package spawns git.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
