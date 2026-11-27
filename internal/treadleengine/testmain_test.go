// testmain_test.go wires the package's test binary into the hermetic git test environment.
// gitkit.HermeticGitEnv() runs once before any test,
// so the git-spawning fixtures never inherit the operator's global gitconfig.

package treadleengine

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
