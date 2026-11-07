// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so burlercli's git-spawning fixtures never
// inherit the operator's global gitconfig (see PATTERN-test-isolation).
// The binary also runs under tmux isolation through tmuxkit.Main.
//
// This file is required because RunCLIIn reaches preflight.ResolveMode.
// Through it, lymcwd.Resolve runs a real `git rev-parse` spawn, and no other test in this package spawns git.
// The Hermetic Git Test Environment guard is token-keyed.
// It does not catch a spawn reached indirectly through a CLI seam, so nothing else would flag the gap.
// Git-config isolation here and the per-test state-root redirect (t.Setenv) are two separate halves,
// and this package needs both.

package burlercli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test spawns git, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
