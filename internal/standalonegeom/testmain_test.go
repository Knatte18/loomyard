//go:build !integration

// testmain_test.go runs the test binary compiled without the `integration` tag under tmux isolation through tmuxkit.Main.
// The `tmux`-tagged naming_integration_test.go runs `git init` here, so gitkit.HermeticGitEnv() arms the hermetic git test environment too (see PATTERN-test-isolation).
//
// The `!integration` constraint is load-bearing: testmain_integration_test.go declares its own TestMain under the `integration` tag,
// and an untagged file always compiles, so without this negation both would collide under `go test -tags integration`.

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
