//go:build integration

// testmain_integration_test.go wires the integration-tagged tests into the hermetic git test environment,
// so naming_integration_test.go's `git init` never inherits the operator's global gitconfig (see PATTERN-hermetic-git-tests).

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
