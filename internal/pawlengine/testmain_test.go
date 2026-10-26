// testmain_test.go runs the package's tests under tmuxkit.Main, so their temp files live in the process run directory and go with it.

package pawlengine_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
