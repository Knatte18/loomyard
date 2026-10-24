//go:build !tier1

// testmain_test.go runs the tier 0 test binary under tmux isolation through tmuxkit.Main.
// No tier 0 test here spawns git, so it needs no hermetic git call.
//
// The `!tier1` constraint is load-bearing: testmain_integration_test.go declares its own TestMain under the `tier1` tag,
// and a file without the negation always compiles, so both would collide under the tier 1 command.

package preflightshed

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
