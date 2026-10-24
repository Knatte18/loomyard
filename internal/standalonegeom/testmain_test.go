//go:build !tier1

// testmain_test.go runs the test binary compiled without the `tier1` tag under tmux isolation through tmuxkit.Main.
// The `tier2`-tagged naming_integration_test.go runs `git init` here, so gitkit.HermeticGitEnv() arms the hermetic git test environment too (see PATTERN-test-isolation).
//
// The `!tier1` constraint is load-bearing: testmain_integration_test.go declares its own TestMain under the `tier1` tag,
// and a file without the negation always compiles, so both would collide under the tier 1 command.

package standalonegeom

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain arms the hermetic git test environment before any test runs, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
