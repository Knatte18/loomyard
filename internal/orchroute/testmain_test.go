//go:build !tier1

// testmain_test.go wires the package's test binary into the hermetic git test environment, so its tests never inherit the operator's global gitconfig (see PATTERN-test-isolation).
// The `!tier1` constraint keeps it from colliding with testmain_integration_test.go, which declares the TestMain of the tier1 build.

package orchroute

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the hermetic git setup once before the whole suite, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
