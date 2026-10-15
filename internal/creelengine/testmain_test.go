// testmain_test.go runs the package's tests under tmuxkit.Main, so every temp file and socket lives in the lyxtemp run directory.

package creelengine

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
