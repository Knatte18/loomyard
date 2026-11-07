// testmain_test.go runs the package's tests under tmuxkit.Main, so their temp files live in the process run directory.
// The files are removed with that directory.

package stencil

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
