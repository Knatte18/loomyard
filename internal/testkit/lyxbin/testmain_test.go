// testmain_test.go runs the test binary under tmux isolation through tmuxkit.Main.
// It carries no build tag, so it compiles into every tag set.

package lyxbin

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
