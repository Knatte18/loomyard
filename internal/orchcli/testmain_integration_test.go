// testmain_integration_test.go wires this package's integration test binary into the hermetic git test environment: gitkit.HermeticGitEnv() runs once before any test, since prime_integration_test.go spawns git via hubforge fixtures (Test Tier Purity Invariant / Hermetic Git Test Environment Invariant), in the shape internal/landingshed's own equivalent already uses.

package orchcli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/codexengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs HermeticGitEnv before any test spawns git, then runs the tests under tmuxkit.Main.
// It points both engines' lym binary at a stand-in path, since the test binary is no lym.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	standIn := func() (string, error) { return "/stub/lym", nil }
	claudeengine.LymBinary = standIn
	codexengine.LymBinary = standIn
	os.Exit(tmuxkit.Main(m))
}
