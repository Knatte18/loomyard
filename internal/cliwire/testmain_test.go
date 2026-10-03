// testmain_test.go runs the test binary under tmux isolation through tmuxkit.Main.
// No test here spawns git, so it needs no hermetic git call.
// The file carries no build tag, so its one TestMain compiles under every tag set.

package cliwire

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
