// testmain_test.go runs the package's tests under tmuxkit.Main, in the hermetic git test environment.
// Their temp files live in the process run directory and go with it.

package kwinplace

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
