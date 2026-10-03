//go:build !integration

// testmain_test.go runs the untagged test binary under the hermetic git test environment and tmux isolation:
// HermeticGitEnv() runs once before any test, then tmuxkit.Main runs the tests.
//
// The `!integration` constraint is load-bearing: gitkit_test.go declares its own TestMain under the `integration` tag,
// and an untagged file always compiles, so without this negation both would collide under `go test -tags integration`.

package gitkit

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain wires up the hermetic git environment, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
