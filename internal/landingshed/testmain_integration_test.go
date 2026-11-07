// testmain_integration_test.go wires this package's integration test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test.
// Sibling files finalize_integration_test.go and publish_integration_test.go spawn git through hubforge fixtures.
// They follow the Test Tier Purity Invariant and the Hermetic Git Test Environment Invariant.

package landingshed_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs HermeticGitEnv before any test spawns git, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
