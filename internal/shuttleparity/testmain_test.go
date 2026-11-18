// testmain_test.go wires the package's test binary into the hermetic git test environment and tmux isolation.

package shuttleparity

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/codexengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this package spawns git.
// It points both engines' lym binary at a stand-in path, since the test binary is no lym.
// It then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	standIn := func() (string, error) { return "/stub/lym", nil }
	claudeengine.LymBinary = standIn
	codexengine.LymBinary = standIn
	os.Exit(tmuxkit.Main(m))
}
