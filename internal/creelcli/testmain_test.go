// testmain_test.go runs the package's tests under the hermetic git environment and tmuxkit.Main.
// The hub-backed tests never reach the operator's gitconfig, and every temp file lives in the process run directory.

package creelcli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	// The test binary is no lym, so a launch the tests prepare names a stand-in lym path in the python guard hook.
	claudeengine.LymBinary = func() (string, error) { return "/stub/lym", nil }
	os.Exit(tmuxkit.Main(m))
}
