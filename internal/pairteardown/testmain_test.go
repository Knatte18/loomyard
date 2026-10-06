//go:build !integration

// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so this package's tests never inherit the operator's global gitconfig (see PATTERN-test-isolation).
// The binary also runs under tmux isolation through tmuxkit.Main.
//
// The `!integration` constraint is load-bearing: testmain_integration_test.go declares its own TestMain for the same package, and without this negation both files would compile together under `go test -tags integration` and collide as two definitions of TestMain.

package pairteardown

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the hermetic git setup once before the whole suite, then runs it under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
