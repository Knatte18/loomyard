// testmain_test.go runs the package's tests under tmuxkit.Main,
// so their temp files live in the process run directory and go with it.
// gitkit.HermeticGitEnv runs once before any test, since the wiring tests spawn git through hubforge fixtures.

package weavercli

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
