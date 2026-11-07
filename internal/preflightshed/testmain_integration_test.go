// testmain_integration_test.go wires this package's integration test binary into the hermetic git test environment.
// gitkit.HermeticGitEnv() runs once before any test.
// The sibling preflight_integration_test.go spawns git through hubforge fixtures.
// The Test Tier Purity Invariant and the Hermetic Git Test Environment Invariant require this.

package preflightshed

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
