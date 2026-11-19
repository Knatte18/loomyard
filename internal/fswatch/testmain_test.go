// testmain_test.go runs the package's tests under tmuxkit.Main.
// Their temp files live in the process run directory and go with it.
// It is an external test package because tmuxkit imports fswatch, which an in-package test file could not import back.

package fswatch_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
