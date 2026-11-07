// testmain_integration_test.go wires the integration-tagged tests into the hermetic git test environment.
// It keeps naming_integration_test.go's `git init` from inheriting the operator's global gitconfig.

package standalonegeom

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain arms the hermetic git test environment before any test runs, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
