//go:build integration

// testmain_integration_test.go wires the integration-tagged tests into the hermetic git test
// environment, so naming_integration_test.go's `git init` never inherits the operator's global
// gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment Invariant).

package standalonegeom

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMain arms the hermetic git test environment before any test runs.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(m.Run())
}
