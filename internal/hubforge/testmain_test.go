// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so hubforge's git-spawning fixtures never
// inherit the operator's global gitconfig (see
// PATTERN-test-isolation).
// The binary also runs under tmux isolation through tmuxkit.Main.
// This is what satisfies that invariant for a package whose hub_test.go spawns git; the guard named
// in PATTERN-test-isolation and enforced by cmd/lyx/hermeticenv_test.go looks for exactly this call.
//
// It carries no build tag deliberately: it must be compiled into the test binary on both a plain
// `go test` and a `-tags integration` run, or the integration-tagged suite this package's hub_test.go
// carries would run without the hermetic environment ever having been installed.

package hubforge

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
