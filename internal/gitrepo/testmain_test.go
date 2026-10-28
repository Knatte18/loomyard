// testmain_test.go wires the package's test binary into the hermetic git test
// environment: gitkit.HermeticGitEnv() runs once before any test, so
// gitrepo's git-spawning fixtures never inherit the operator's global
// gitconfig (see PATTERN-test-isolation).
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

// childBodies maps an environment variable to the child-process body a test re-execs this binary to run, given the variable's value.
// A body returns the child's exit code.
// Tier-tagged files register their bodies from init, so a body never shows up as a test the runner skips.
var childBodies = map[string]func(value string) int{}

// TestMain runs gitkit.HermeticGitEnv() before any test in this package spawns git.
// A binary re-exec'd with a registered child variable set runs that child body and exits;
// otherwise the tests run under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	for name, body := range childBodies {
		if value := os.Getenv(name); value != "" {
			os.Exit(body(value))
		}
	}
	os.Exit(tmuxkit.Main(m))
}
