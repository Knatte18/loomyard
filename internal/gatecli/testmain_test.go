// testmain_test.go runs the test binary under the hermetic git environment and tmux isolation, and lets the integration test re-execute the binary as a helper process.
// It carries no build tag, so it compiles into every tag set; the helper roles themselves register from the integration-tagged file, since they spawn.

package gatecli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// helperEnv, when set to a key of helperRoles, turns the test binary into that helper role.
const helperEnv = "GATECLI_TEST_HELPER"

// helperRoles maps each helper role to the function that runs it and returns the process's exit code.
// It is empty outside the integration tier.
var helperRoles = map[string]func() int{}

// TestMain runs the tests under tmuxkit.Main, or runs the helper role helperEnv names.
func TestMain(m *testing.M) {
	if role, ok := helperRoles[os.Getenv(helperEnv)]; ok {
		os.Exit(role())
	}
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
