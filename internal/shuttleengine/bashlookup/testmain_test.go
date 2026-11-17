// testmain_test.go gives the package's test binary an isolated tmux socket directory through tmuxkit.Main.

package bashlookup_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
