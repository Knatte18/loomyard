// testmain_test.go runs the package's tests under tmuxkit.Main.
// That puts their temp files in the process run directory, so they go with it.

package frictionengine

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
