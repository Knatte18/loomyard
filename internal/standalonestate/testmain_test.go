// testmain_test.go runs the test binary under tmux isolation through tmuxkit.Main.
// It serves every tier's tests, since the package's one test binary holds them all.

package standalonestate

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
