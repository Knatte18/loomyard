// testmain_test.go runs the package's tests under `tmuxkit.Main`.
// Their temp files live in the process run directory, so they go with it.

package ideconfig_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
