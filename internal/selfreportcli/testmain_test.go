// testmain_test.go runs the package's tests under tmuxkit.Main.
// tmuxkit.Main keeps the tests' temp files in the process run directory, so they go with it.

package selfreportcli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
