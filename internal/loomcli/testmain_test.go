// testmain_test.go wires the package's test binary into the hermetic git test environment.
// gitkit.HermeticGitEnv() runs once before any test.
// It keeps this package's tests from inheriting the operator's global gitconfig.
// The tier1 tests, which spawn git, are covered by the same declaration.

package loomcli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the hermetic git setup once before the whole suite, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	// The test binary is no lym, so a launch the tests prepare names a stand-in lym path in the python guard hook.
	claudeengine.LymBinary = func() (string, error) { return "/stub/lym", nil }
	os.Exit(tmuxkit.Main(m))
}
